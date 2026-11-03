// Copyright (C) 2025 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package services

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sipcapture/homer-core/src/config"
)

func TestJWTSecretFilePathWithPostgresSettings(t *testing.T) {
	dir := t.TempDir()
	for _, dsn := range []string{"postgres://user:private@localhost/settings", "POSTGRES://user:private@localhost/settings", "postgresql://user:private@localhost/settings", "host=localhost dbname=settings password=private"} {
		if got := JWTSecretFilePath(dsn, dir); got != filepath.Join(dir, homerJWTSecretFileName) {
			t.Fatalf("DSN interpreted as a secret directory: %q", got)
		}
	}
}

func TestPostgresSecretsPersistInSharedDirectory(t *testing.T) {
	for _, dsn := range []string{
		"postgres://user:private@localhost/homer", "POSTGRES://user:private@localhost/homer",
		"postgresql://user:private@localhost/homer", "host=localhost dbname=homer user=user password=private",
	} {
		name := "keyword"
		if scheme, _, ok := strings.Cut(dsn, "://"); ok {
			name = scheme
		}
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			token, _, tokenFile, err := config.ResolveRequiredAuthToken("", dsn, dir)
			if err != nil {
				t.Fatal(err)
			}
			secret, _, secretFile, err := ResolveCoordinatorJWTSecret("", dsn, dir)
			if err != nil {
				t.Fatal(err)
			}
			for _, file := range []string{tokenFile, secretFile} {
				info, err := os.Stat(file)
				if err != nil {
					t.Fatal(err)
				}
				if filepath.Dir(file) != dir || info.Mode().Perm() != 0600 {
					t.Fatalf("secret file %q must be in %q with mode 0600", file, dir)
				}
			}
			loadedToken, generated, _, err := config.ResolveRequiredAuthToken("", dsn, dir)
			if err != nil || generated || loadedToken != token {
				t.Fatal("node token was not preserved on reload")
			}
			loadedSecret, generated, _, err := ResolveCoordinatorJWTSecret("", dsn, dir)
			if err != nil || generated || loadedSecret != secret {
				t.Fatal("JWT secret was not preserved on reload")
			}
		})
	}

}

func TestResolveCoordinatorJWTSecret_ConfiguredWins(t *testing.T) {
	secret, auto, path, err := ResolveCoordinatorJWTSecret("configured-secret", t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	if secret != "configured-secret" || auto || path != "" {
		t.Fatalf("got secret=%q auto=%v path=%q", secret, auto, path)
	}
}

func TestResolveCoordinatorJWTSecret_PersistAndReload(t *testing.T) {
	dir := t.TempDir()
	settings := filepath.Join(dir, "settings.duckdb")

	secret1, auto1, file1, err := ResolveCoordinatorJWTSecret("", settings, "")
	if err != nil {
		t.Fatal(err)
	}
	if !auto1 || secret1 == "" || file1 != filepath.Join(dir, homerJWTSecretFileName) {
		t.Fatalf("first: secret=%q auto=%v file=%q", secret1, auto1, file1)
	}

	secret2, auto2, file2, err := ResolveCoordinatorJWTSecret("", settings, "")
	if err != nil {
		t.Fatal(err)
	}
	if auto2 || secret2 != secret1 || file2 != file1 {
		t.Fatalf("reload: secret=%q auto=%v file=%q", secret2, auto2, file2)
	}
}

func TestResolveCoordinatorJWTSecret_PlaceholderIsGenerated(t *testing.T) {
	dir := t.TempDir()
	settings := filepath.Join(dir, "settings.duckdb")
	const placeholder = "change-this-to-a-secure-random-string-in-production"

	secret1, auto1, file1, err := ResolveCoordinatorJWTSecret(placeholder, settings, "")
	if err != nil {
		t.Fatal(err)
	}
	if !auto1 || secret1 == "" || secret1 == placeholder {
		t.Fatalf("placeholder must be replaced, got secret=%q auto=%v", secret1, auto1)
	}

	secret2, auto2, _, err := ResolveCoordinatorJWTSecret(placeholder, settings, "")
	if err != nil {
		t.Fatal(err)
	}
	if auto2 || secret2 != secret1 || file1 == "" {
		t.Fatalf("reload: secret=%q auto=%v file=%q", secret2, auto2, file1)
	}
}

func TestResolveCoordinatorJWTSecret_LoadExistingFile(t *testing.T) {
	dir := t.TempDir()
	settings := filepath.Join(dir, "settings.duckdb")
	secretFile := filepath.Join(dir, homerJWTSecretFileName)
	if err := os.WriteFile(secretFile, []byte("from-file\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	secret, auto, _, err := ResolveCoordinatorJWTSecret("", settings, "")
	if err != nil {
		t.Fatal(err)
	}
	if auto || secret != "from-file" {
		t.Fatalf("got secret=%q auto=%v", secret, auto)
	}
}
