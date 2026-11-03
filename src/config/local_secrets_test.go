// Copyright (C) 2026 Homer Server Contributors
// SPDX-License-Identifier: AGPL-3.0-or-later
package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPostgresVolumeSecretsDirectory(t *testing.T) {
	cfg := &NodeConfig{DuckLake: DuckLakeConfig{CatalogPath: "homer_catalog.sqlite", Volumes: []VolumeConfig{{CatalogPath: "POSTGRES://user:private@localhost/lake"}}}}
	path := NodeCatalogPath(cfg)
	if NodeAuthTokenFilePath(path, "") != "/var/lib/homer/.homer_node_auth_token" {
		t.Fatal("volume PostgreSQL catalog was not selected")
	}
	dir := t.TempDir()
	token, _, file, err := ResolveRequiredAuthToken("", path, dir)
	if err != nil || token == "" || filepath.Dir(file) != dir {
		t.Fatalf("volume secret placement: %q %v", file, err)
	}
	blocked := filepath.Join(t.TempDir(), "blocked")
	if err := os.WriteFile(blocked, []byte("file"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := ResolveRequiredAuthToken("", path, blocked); err == nil {
		t.Fatal("unwritable secret directory accepted")
	}
}
func TestLocalSecretDefaultsAndOverride(t *testing.T) {
	if NodeAuthTokenFilePath("", "") != "/data/.homer_node_auth_token" {
		t.Fatal("empty catalog fallback changed")
	}
	if NodeAuthTokenFilePath("/data/catalog.sqlite", "") != "/data/.homer_node_auth_token" {
		t.Fatal("file-backed default changed")
	}
	dir := t.TempDir()
	if filepath.Dir(NodeAuthTokenFilePath("/data/catalog.sqlite", dir)) != dir {
		t.Fatal("explicit local secret directory ignored")
	}
	if filepath.Dir(SettingsDBLocalPath("/data/settings.duckdb", dir)) != dir {
		t.Fatal("JWT local directory ignored")
	}
}
