// Copyright (C) 2026 Homer Server Contributors
// SPDX-License-Identifier: AGPL-3.0-or-later
package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadPostgresDuckLakeCatalog(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "homer.json")
	if err := os.WriteFile(file, []byte(`{"storage":{"ducklake":{"catalog_path":"postgres://homer:private@localhost/lake"}},"node":{"ducklake":{"catalog_type":"postgres","catalog_path":"host=localhost dbname=lake"}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(file)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Storage.DuckLake.CatalogType != "postgres" || cfg.Node.DuckLake.CatalogType != "postgres" {
		t.Fatal("PostgreSQL catalog types were not normalized")
	}
	if NodeAuthTokenFilePath(cfg.Node.DuckLake.CatalogPath) != "/data/.homer_node_auth_token" {
		t.Fatal("connection string interpreted as token directory")
	}
}

func TestPostgresShardsRejectSingleCatalogTiering(t *testing.T) {
	cfg := &Config{}
	cfg.Storage.DuckLake.CatalogPath = "postgres://homer:private@localhost/lake"
	cfg.Storage.DuckLake.ShardCount = 2
	cfg.Storage.DuckLake.StoragePolicy.Enable = true
	if err := validateDuckLakeCatalogTypes(cfg); err == nil || !strings.Contains(err.Error(), "storage_policy") {
		t.Fatalf("unsafe sharded tiering accepted: %v", err)
	}
}
