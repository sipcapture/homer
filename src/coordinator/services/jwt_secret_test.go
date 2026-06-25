// Copyright (C) 2025 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package services

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveCoordinatorJWTSecret_ConfiguredWins(t *testing.T) {
	secret, auto, path, err := ResolveCoordinatorJWTSecret("configured-secret", t.TempDir())
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

	secret1, auto1, file1, err := ResolveCoordinatorJWTSecret("", settings)
	if err != nil {
		t.Fatal(err)
	}
	if !auto1 || secret1 == "" || file1 != filepath.Join(dir, homerJWTSecretFileName) {
		t.Fatalf("first: secret=%q auto=%v file=%q", secret1, auto1, file1)
	}

	secret2, auto2, file2, err := ResolveCoordinatorJWTSecret("", settings)
	if err != nil {
		t.Fatal(err)
	}
	if auto2 || secret2 != secret1 || file2 != file1 {
		t.Fatalf("reload: secret=%q auto=%v file=%q", secret2, auto2, file2)
	}
}

func TestResolveCoordinatorJWTSecret_LoadExistingFile(t *testing.T) {
	dir := t.TempDir()
	settings := filepath.Join(dir, "settings.duckdb")
	secretFile := filepath.Join(dir, homerJWTSecretFileName)
	if err := os.WriteFile(secretFile, []byte("from-file\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	secret, auto, _, err := ResolveCoordinatorJWTSecret("", settings)
	if err != nil {
		t.Fatal(err)
	}
	if auto || secret != "from-file" {
		t.Fatalf("got secret=%q auto=%v", secret, auto)
	}
}
