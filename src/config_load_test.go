package main

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// serverConfigPathEnv makes the test binary run main() as the server, with
// --config-path set to its value (or no flag when empty), instead of the tests.
const serverConfigPathEnv = "TEST_SERVER_CONFIG_PATH"

// A config that fails to load or validate, from a file or from HOMER_*
// variables alone, must stop the server with the reason on stderr and exit
// status 1. Before, the server fell back to legacy mode, which has no HEP
// listener, and kept running. The server runs as a child process, which is
// stopped if it is still running after a timeout.
func TestServerExitsOnConfigLoadError(t *testing.T) {
	if path, ok := os.LookupEnv(serverConfigPathEnv); ok {
		os.Args = []string{"homer"}
		if path != "" {
			os.Args = append(os.Args, "--config-path", path)
		}
		main()
		return
	}

	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	write := func(path, content string) string {
		t.Helper()
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	const badAuth = `{"coordinator": {"auth": {"type": "kerberos"}}}`
	const authErr = `coordinator.auth.type: unsupported "kerberos"`
	foundDir := t.TempDir()
	write(filepath.Join(foundDir, "homer.json"), badAuth)

	// Load applies every HOMER_* variable, and an inherited one can fail
	// validation before the setting a case is about, so none is passed on.
	var env []string
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "HOMER_") {
			env = append(env, kv)
		}
	}

	for _, tc := range []struct {
		name       string
		configPath string // empty: no --config-path flag
		workDir    string
		env        []string // HOMER_* variables for the server
		want       string   // expected on stderr
	}{
		{"invalid auth type", write(filepath.Join(dir, "auth.json"), badAuth), dir, nil, authErr},
		{"JSON syntax error", write(filepath.Join(dir, "syntax.json"), `{"ingest": `), dir, nil, "error reading config file"},
		{"missing file", filepath.Join(dir, "missing.json"), dir, nil, "no such file or directory"},
		{"homer.json found without a path", "", foundDir, nil, authErr},
		{"invalid auth type in the environment without a file", "", t.TempDir(),
			[]string{"HOMER_COORDINATOR_AUTH_TYPE=kerberos"}, authErr},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := exec.Command(exe, "-test.run=^TestServerExitsOnConfigLoadError$")
			cmd.Dir = tc.workDir
			cmd.Env = slices.Concat(env, tc.env,
				[]string{serverConfigPathEnv + "=" + tc.configPath, "HOME=" + t.TempDir()})
			var stderr bytes.Buffer
			cmd.Stderr = &stderr
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			go func() { done <- cmd.Wait() }()
			select {
			case err := <-done:
				var exit *exec.ExitError
				if !errors.As(err, &exit) || exit.ExitCode() != 1 {
					t.Fatalf("server exit = %v, want exit status 1; stderr:\n%s", err, stderr.String())
				}
			case <-time.After(15 * time.Second):
				_ = cmd.Process.Kill()
				<-done
				t.Fatalf("server still running 15s after a config error; stderr:\n%s", stderr.String())
			}
			if !strings.Contains(stderr.String(), tc.want) {
				t.Errorf("stderr does not contain %q:\n%s", tc.want, stderr.String())
			}
		})
	}
}
