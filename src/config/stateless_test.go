package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func statelessConfig() *Config {
	cfg := &Config{Stateless: true}
	cfg.Storage.Enable = true
	cfg.Storage.DuckLake.CatalogPath = "postgres://homer:secret@localhost/lake"
	cfg.Storage.DuckLake.DataPath = "s3://bucket/parquet"
	cfg.Coordinator.Enable = true
	cfg.Coordinator.SettingsDBPath = "postgres://homer:secret@localhost/settings"
	cfg.Coordinator.JWT.Secret = "shared-jwt-secret"
	cfg.Node.Enable = true
	cfg.Node.FlightServer.Host = "0.0.0.0"
	cfg.Node.FlightServer.AuthToken = "shared-node-token"
	cfg.Node.DuckLake = cfg.Storage.DuckLake
	return cfg
}

func TestStatelessValidation(t *testing.T) {
	for _, tc := range []struct {
		name, field string
		change      func(*Config)
	}{
		{"settings", "settings_db_path", func(c *Config) { c.Coordinator.SettingsDBPath = "settings.duckdb" }},
		{"jwt", "jwt.secret", func(c *Config) { c.Coordinator.JWT.Secret = "" }},
		{"placeholder", "jwt.secret", func(c *Config) { c.Coordinator.JWT.Secret = shippedJWTSecretPlaceholder }},
		{"node-token", "flight_server.auth_token", func(c *Config) { c.Node.FlightServer.AuthToken = "" }},
		{"catalog", "catalog_path", func(c *Config) { c.Storage.DuckLake.CatalogPath = "local.sqlite" }},
		{"parquet", "data path", func(c *Config) { c.Storage.DuckLake.DataPath = "/data/parquet" }},
		{"node-volume", "volumes[0]", func(c *Config) {
			c.Node.DuckLake.Volumes = []VolumeConfig{{CatalogPath: c.Node.DuckLake.CatalogPath, Path: "/data/parquet"}}
		}},
		{"tier", "volumes[0]", func(c *Config) {
			c.Storage.DuckLake.StoragePolicy.Enable = true
			c.Storage.DuckLake.StoragePolicy.Volumes = []VolumeConfig{{Path: "/data/hot"}}
		}},
		{"logging", "log.output", func(c *Config) { c.Log.Output = []string{"file"} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := statelessConfig()
			tc.change(cfg)
			if err := validateStateless(cfg); err == nil || !strings.Contains(err.Error(), tc.field) {
				t.Fatalf("expected %s validation, got %v", tc.field, err)
			}
		})
	}
	if err := validateStateless(statelessConfig()); err != nil {
		t.Fatal(err)
	}
}

func TestStatelessLoadDoesNotGenerateLocalSecrets(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "homer.json")
	// Missing node token must fail before a resolver writes beside this local catalog.
	body := `{"stateless":true,"storage":{"enable":false},"node":{"enable":true,"ducklake":{"catalog_path":"` + filepath.Join(dir, "catalog.sqlite") + `"}},"coordinator":{"enable":false}}`
	if err := os.WriteFile(file, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(file); err == nil || !strings.Contains(err.Error(), "auth_token") {
		t.Fatalf("expected missing-token error, got %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("stateless load wrote local state: %v", entries)
	}
}

func TestStatelessEnvironmentAndTokenInheritance(t *testing.T) {
	t.Setenv("HOMER_STATELESS", "true")
	t.Setenv("HOMER_STORAGE_ENABLE", "false")
	t.Setenv("HOMER_NODE_ENABLE", "false")
	t.Setenv("HOMER_COORDINATOR_ENABLE", "true")
	t.Setenv("HOMER_COORDINATOR_SETTINGS_DB_PATH", "postgres://localhost/settings")
	t.Setenv("HOMER_COORDINATOR_JWT_SECRET", "shared-jwt")
	file := filepath.Join(t.TempDir(), "homer.json")
	if err := os.WriteFile(file, []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(file)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Stateless || !cfg.Coordinator.Stateless {
		t.Fatal("stateless setting not propagated")
	}
	cfg = statelessConfig()
	cfg.Node.FlightSQLServer.Enable = true
	cfg.Coordinator.FlightSQLServer.Enable = true
	applyDefaults(cfg)
	if err := validateStateless(cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Coordinator.FlightSQLServer.AuthToken != "shared-node-token" {
		t.Fatal("configured token was not inherited")
	}
}
