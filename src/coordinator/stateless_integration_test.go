// Copyright (C) 2026 Homer Server Contributors
// SPDX-License-Identifier: AGPL-3.0-or-later

package coordinator

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/sipcapture/homer-core/src/config"
)

// This helper runs in a separate OS process, with its own DuckDB client,
// coordinator, HTTP listener and memory. The parent kills it to model a crash.
func TestStatelessCoordinatorProcess(t *testing.T) {
	path := os.Getenv("HOMER_TEST_STATELESS_CHILD_CONFIG")
	if path == "" {
		t.Skip("subprocess helper")
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	c, err := New(&cfg.Coordinator)
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	c.echo.Listener = listener
	if err := c.Start(); err != nil {
		t.Fatal(err)
	}
	defer c.Stop()
	if err := os.WriteFile(os.Getenv("HOMER_TEST_STATELESS_CHILD_READY"), []byte("http://"+listener.Addr().String()), 0600); err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, os.Stdin)
}

type statelessProcess struct {
	cmd         *exec.Cmd
	ready, logs string
	stopOnce    sync.Once
}

func (p *statelessProcess) stop() {
	p.stopOnce.Do(func() { _ = p.cmd.Process.Kill(); _ = p.cmd.Wait() })
}

func startStatelessProcess(t *testing.T, configPath string) *statelessProcess {
	t.Helper()
	dir := t.TempDir()
	p := &statelessProcess{ready: filepath.Join(dir, "ready"), logs: filepath.Join(dir, "server.log")}
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	p.cmd = exec.Command(binary, "-test.run=^TestStatelessCoordinatorProcess$", "-test.v")
	p.cmd.Env = append(os.Environ(), "HOMER_TEST_STATELESS_CHILD_CONFIG="+configPath, "HOMER_TEST_STATELESS_CHILD_READY="+p.ready)
	log, err := os.OpenFile(p.logs, os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	p.cmd.Stdout, p.cmd.Stderr = log, log
	// Keep stdin open so the helper stays alive until explicitly terminated.
	stdin, err := p.cmd.StdinPipe()
	if err != nil {
		_ = log.Close()
		t.Fatal(err)
	}
	if err := p.cmd.Start(); err != nil {
		_ = log.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { p.stop(); _ = stdin.Close(); _ = log.Close() })
	return p
}

func (p *statelessProcess) address(t *testing.T) string {
	t.Helper()
	deadline := time.NewTimer(45 * time.Second)
	defer deadline.Stop()
	poll := time.NewTicker(50 * time.Millisecond)
	defer poll.Stop()
	for {
		if data, err := os.ReadFile(p.ready); err == nil {
			return string(data)
		}
		select {
		case <-poll.C:
		case <-deadline.C:
			logs, _ := os.ReadFile(p.logs)
			t.Fatalf("coordinator did not start:\n%s", logs)
		}
	}
}

func statelessTestDatabase(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("HOMER_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set HOMER_TEST_POSTGRES_DSN for multi-process integration tests")
	}
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	name := "homer_scale_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		_ = admin.Close(ctx)
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = admin.Exec(ctx, "DROP DATABASE "+name+" WITH (FORCE)"); _ = admin.Close(ctx) })
	installer, err := sql.Open("duckdb", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := installer.Exec("LOAD postgres"); err != nil {
		if _, err := installer.Exec("INSTALL postgres FROM 'https://extensions.duckdb.org'"); err != nil {
			_ = installer.Close()
			t.Fatal(err)
		}
	}
	_ = installer.Close()
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	u := url.URL{Scheme: "postgresql", Host: net.JoinHostPort(cfg.Host, strconv.Itoa(int(cfg.Port))), User: url.UserPassword(cfg.User, cfg.Password), Path: "/" + name, RawQuery: "sslmode=disable"}
	return u.String()
}

func TestPostgresStatelessCoordinatorReplicas(t *testing.T) {
	rootT := t
	dsn := statelessTestDatabase(t)
	var challenge string
	var challengeMu sync.Mutex
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/token":
			_ = r.ParseForm()
			hash := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
			challengeMu.Lock()
			expected := challenge
			challengeMu.Unlock()
			if r.Form.Get("code") != "valid-code" || expected == "" || base64.RawURLEncoding.EncodeToString(hash[:]) != expected {
				http.Error(w, `{"error":"invalid_grant"}`, http.StatusBadRequest)
				return
			}
			_, _ = io.WriteString(w, `{"access_token":"test-access","token_type":"Bearer","expires_in":3600}`)
		case "/profile":
			if r.Header.Get("Authorization") != "Bearer test-access" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			_, _ = io.WriteString(w, `{"sub":"scale-user","preferred_username":"scale-user","email":"scale@example.test"}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer provider.Close()
	password := "scale-test-" + uuid.NewString()
	hash, err := config.HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	settings := map[string]any{
		"stateless": true,
		"ingest":    map[string]any{"enable": false, "hep_stream": map[string]any{"enable": false}}, "storage": map[string]any{"enable": false}, "node": map[string]any{"enable": false},
		"coordinator": map[string]any{
			"hep_stream": map[string]any{"enable": false},
			"enable":     true, "settings_db_type": "postgres", "settings_db_path": dsn,
			"jwt":         map[string]any{"secret": uuid.NewString(), "expire_hours": 1, "cookie_enable": false},
			"auth":        map[string]any{"type": "internal", "admin_user": "scale-admin", "admin_password_hash": hash},
			"correlation": map[string]any{"enable": false, "seed_default": false},
			"oauth2_provider": map[string]any{"enable": true, "name": "scale", "client_id": "scale-client", "use_pkce": true,
				"auth_url": provider.URL + "/authorize", "token_url": provider.URL + "/token", "profile_url": provider.URL + "/profile",
				"redirect_url": "http://localhost/api/v4/auth/oauth2/scale/callback", "callback_url": "/oauth-complete"},
		},
		"log": map[string]any{"output": []string{"stdout"}},
	}
	data, err := json.Marshal(settings)
	if err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(t.TempDir(), "homer.json")
	if err := os.WriteFile(configPath, data, 0600); err != nil {
		t.Fatal(err)
	}
	// Start both processes before waiting, exercising concurrent first-run bootstrap.
	a, b := startStatelessProcess(t, configPath), startStatelessProcess(t, configPath)
	addresses := []string{a.address(t), b.address(t)}
	client := &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	request := func(base, method, path, token, body string) (int, http.Header, []byte, error) {
		req, err := http.NewRequest(method, base+path, strings.NewReader(body))
		if err != nil {
			return 0, nil, nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		resp, err := client.Do(req)
		if err != nil {
			return 0, nil, nil, err
		}
		defer resp.Body.Close()
		payload, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
		return resp.StatusCode, resp.Header, payload, err
	}
	expect := func(t *testing.T, base, method, path, token, body string, status int) (http.Header, []byte) {
		t.Helper()
		got, headers, payload, err := request(base, method, path, token, body)
		if err != nil || got != status {
			t.Fatalf("%s %s: status=%d want=%d error=%v body=%s", method, path, got, status, err, payload)
		}
		return headers, payload
	}
	login := func(base string) string {
		t.Helper()
		body, _ := json.Marshal(map[string]string{"username": "scale-admin", "password": password})
		_, payload := expect(t, base, http.MethodPost, "/api/v4/auth/sessions", "", string(body), http.StatusCreated)
		var result struct {
			Data struct {
				Token string `json:"token"`
			} `json:"data"`
		}
		if err := json.Unmarshal(payload, &result); err != nil || result.Data.Token == "" {
			t.Fatalf("missing login token: %v", err)
		}
		return result.Data.Token
	}
	token := login(addresses[0])
	t.Run("alternating authenticated requests", func(t *testing.T) {
		var wg sync.WaitGroup
		for i := 0; i < 40; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				status, _, _, err := request(addresses[i%2], http.MethodGet, "/api/v4/me", token, "")
				if err != nil || status != http.StatusOK {
					t.Errorf("request %d: status=%d error=%v", i, status, err)
				}
			}(i)
		}
		wg.Wait()
	})
	t.Run("settings and exports cross replicas", func(t *testing.T) {
		expect(t, addresses[0], http.MethodPut, "/api/v4/me/settings/scale-test", token, `{"param":"layout","data":{"marker":"shared-setting"}}`, http.StatusOK)
		_, payload := expect(t, addresses[1], http.MethodGet, "/api/v4/me/settings", token, "", http.StatusOK)
		if !strings.Contains(string(payload), "shared-setting") {
			t.Fatal("settings write not visible on other replica")
		}
		_, payload = expect(t, addresses[0], http.MethodPost, "/api/v4/exports", token, `{"type":"text","query":{"callid":"scale-export"}}`, http.StatusCreated)
		var result struct {
			Data struct {
				ID string `json:"export_id"`
			} `json:"data"`
		}
		if err := json.Unmarshal(payload, &result); err != nil || result.Data.ID == "" {
			t.Fatal("missing export ID")
		}
		path := "/api/v4/exports/" + result.Data.ID
		expect(t, addresses[1], http.MethodGet, path, token, "", http.StatusOK)
		_, payload = expect(t, addresses[1], http.MethodGet, path+"/download", token, "", http.StatusOK)
		if !strings.Contains(string(payload), "scale-export") {
			t.Fatal("export not shared")
		}
		// Crash the creator, restart with the same external configuration, and
		// verify neither the JWT nor the export depends on its original process.
		a.stop()
		a = startStatelessProcess(rootT, configPath)
		addresses[0] = a.address(t)
		_, payload = expect(t, addresses[0], http.MethodGet, path+"/download", token, "", http.StatusOK)
		if !strings.Contains(string(payload), "scale-export") {
			t.Fatal("export lost after restart")
		}
	})
	t.Run("logout and restart preserve revocation", func(t *testing.T) {
		expect(t, addresses[1], http.MethodDelete, "/api/v4/auth/sessions/current", token, "", http.StatusNoContent)
		expect(t, addresses[0], http.MethodGet, "/api/v4/me", token, "", http.StatusUnauthorized)
		b.stop()
		b = startStatelessProcess(rootT, configPath)
		addresses[1] = b.address(t)
		expect(t, addresses[1], http.MethodGet, "/api/v4/me", token, "", http.StatusUnauthorized)
	})
	t.Run("OAuth PKCE handoff and replay", func(t *testing.T) {
		headers, _ := expect(t, addresses[0], http.MethodGet, "/api/v4/auth/oauth2/scale/redirect", "", "", http.StatusFound)
		location, err := url.Parse(headers.Get("Location"))
		if err != nil {
			t.Fatal(err)
		}
		challengeMu.Lock()
		challenge = location.Query().Get("code_challenge")
		challengeMu.Unlock()
		state := location.Query().Get("state")
		if state == "" {
			t.Fatal("missing OAuth state")
		}
		callback := "/api/v4/auth/oauth2/scale/callback?code=valid-code&state=" + url.QueryEscape(state)
		headers, _ = expect(t, addresses[1], http.MethodGet, callback, "", "", http.StatusFound)
		location, err = url.Parse(headers.Get("Location"))
		if err != nil {
			t.Fatal(err)
		}
		oneTime := location.Query().Get("token")
		if oneTime == "" {
			t.Fatalf("OAuth callback failed: %s", location.String())
		}
		expect(t, addresses[0], http.MethodGet, callback, "", "", http.StatusBadRequest)
		body := fmt.Sprintf(`{"token":%q}`, oneTime)
		_, payload := expect(t, addresses[0], http.MethodPost, "/api/v4/auth/oauth2/token", "", body, http.StatusCreated)
		var result struct {
			Data struct {
				Token string `json:"token"`
			} `json:"data"`
		}
		if err := json.Unmarshal(payload, &result); err != nil || result.Data.Token == "" {
			t.Fatal("missing OAuth JWT")
		}
		expect(t, addresses[1], http.MethodGet, "/api/v4/me", result.Data.Token, "", http.StatusOK)
		expect(t, addresses[1], http.MethodPost, "/api/v4/auth/oauth2/token", "", body, http.StatusUnauthorized)
	})
}
