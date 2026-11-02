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
	if err := os.WriteFile(file, []byte(`{"coordinator":{"local_secrets_dir":"`+dir+`"},"storage":{"ducklake":{"catalog_path":"postgres://homer:private@localhost/lake"}},"node":{"local_secrets_dir":"`+dir+`","ducklake":{"catalog_type":"postgres","catalog_path":"host=localhost dbname=lake"}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(file)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Storage.DuckLake.CatalogType != "postgres" || cfg.Node.DuckLake.CatalogType != "postgres" {
		t.Fatal("PostgreSQL catalog types were not normalized")
	}
	if cfg.Coordinator.LocalSecretsDir != dir || cfg.Node.LocalSecretsDir != dir {
		t.Fatal("local_secrets_dir was not loaded from JSON")
	}
	if NodeAuthTokenFilePath(cfg.Node.DuckLake.CatalogPath, cfg.Node.LocalSecretsDir) != filepath.Join(dir, homerNodeAuthTokenFileName) {
		t.Fatal("connection string interpreted as token directory")
	}
	envDir := filepath.Join(dir, "env-secrets")
	t.Setenv("HOMER_NODE_LOCAL_SECRETS_DIR", envDir)
	t.Setenv("HOMER_COORDINATOR_LOCAL_SECRETS_DIR", envDir)
	cfg, err = Load(file)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Node.LocalSecretsDir != envDir || cfg.Coordinator.LocalSecretsDir != envDir {
		t.Fatal("environment overrides for local_secrets_dir were not loaded")
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
