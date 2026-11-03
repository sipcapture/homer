// Copyright (C) 2026 Homer Server Contributors
// SPDX-License-Identifier: AGPL-3.0-or-later
package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPostgresVolumeSecretsDirectory(t *testing.T) {
	cfg := &NodeConfig{DuckLake: DuckLakeConfig{CatalogPath: "homer_catalog.sqlite", Volumes: []VolumeConfig{{CatalogType: "postgres", CatalogPath: "POSTGRES://user:private@localhost/lake"}}}}
	path := NodeCatalogPath(cfg)
	if NodeCatalogType(cfg) != "postgres" {
		t.Fatal("selected volume type does not match selected path")
	}
	if NodeAuthTokenFilePath(path, "", NodeCatalogType(cfg)) != "/var/lib/homer/.homer_node_auth_token" {
		t.Fatal("volume PostgreSQL catalog was not selected")
	}
	dir := t.TempDir()
	token, _, file, err := ResolveRequiredAuthToken("", path, dir, NodeCatalogType(cfg))
	if err != nil || token == "" || filepath.Dir(file) != dir {
		t.Fatalf("volume secret placement: %q %v", file, err)
	}
	blocked := filepath.Join(t.TempDir(), "blocked")
	if err := os.WriteFile(blocked, []byte("file"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := ResolveRequiredAuthToken("", path, blocked, NodeCatalogType(cfg)); err == nil {
		t.Fatal("unwritable secret directory accepted")
	}
}
func TestLocalSecretDefaultsAndOverride(t *testing.T) {
	if NodeAuthTokenFilePath("", "", "sqlite") != "/data/.homer_node_auth_token" {
		t.Fatal("empty catalog fallback changed")
	}
	if NodeAuthTokenFilePath("/data/catalog.sqlite", "", "sqlite") != "/data/.homer_node_auth_token" {
		t.Fatal("file-backed default changed")
	}
	dir := t.TempDir()
	if filepath.Dir(NodeAuthTokenFilePath("/data/catalog.sqlite", dir, "sqlite")) != dir {
		t.Fatal("explicit local secret directory ignored")
	}
	if filepath.Dir(SettingsDBLocalPath("/data/settings.duckdb", dir, "sqlite")) != dir {
		t.Fatal("JWT local directory ignored")
	}
}

func TestPostgresSecretPlacementUsesType(t *testing.T) {
	const local = "/data/homer/settings.duckdb"
	if got := SettingsDBLocalPath(local, "", "postgres"); got != defaultSettingsDBPath {
		t.Fatalf("PostgreSQL local anchor = %q", got)
	}
	if got := NodeAuthTokenFilePath(local, "", "postgres"); got != "/var/lib/homer/.homer_node_auth_token" {
		t.Fatalf("PostgreSQL token path = %q", got)
	}
	if got := SettingsDBLocalPath(local, "", "sqlite"); got != local {
		t.Fatalf("local settings anchor = %q", got)
	}
}

func TestPostgresInvalidTypeRejectedBeforeSecretCreation(t *testing.T) {
	dir := t.TempDir()
	secrets := filepath.Join(dir, "secrets")
	file := filepath.Join(dir, "homer.json")
	data := `{"coordinator":{"settings_db_type":"sqlite","settings_db_path":"postgres://user:private@localhost/settings"},"node":{"enable":true,"local_secrets_dir":"` + secrets + `","flight_server":{"host":"0.0.0.0"}}}`
	if err := os.WriteFile(file, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(file); err == nil {
		t.Fatal("mismatched backend accepted")
	}
	if _, err := os.Stat(secrets); !os.IsNotExist(err) {
		t.Fatalf("invalid configuration created secret directory: %v", err)
	}
}
