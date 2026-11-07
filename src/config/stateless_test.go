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
	cfg.Storage.DuckLake.CatalogType = "postgres"
	cfg.Storage.DuckLake.CatalogPath = "postgres://homer:secret@localhost/lake"
	cfg.Storage.DuckLake.DataPath = "s3://bucket/parquet"
	cfg.Coordinator.Enable = true
	cfg.Coordinator.SettingsDBType = "postgres"
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
		{"search-buffer", "search_buffer", func(c *Config) { c.Storage.DuckLake.SearchBuffer = true }},
		{"ingest-stream", "hep_stream", func(c *Config) { c.Ingest.HepStream.Enable = true }},
		{"coordinator-stream", "hep_stream", func(c *Config) { c.Coordinator.HepStream.Enable = true }},
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
		{"tier-catalog", "volumes[0].catalog_path", func(c *Config) {
			c.Storage.DuckLake.StoragePolicy.Enable = true
			c.Storage.DuckLake.StoragePolicy.Volumes = []VolumeConfig{{Path: "s3://bucket/tier", CatalogPath: "local.sqlite"}}
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
	body := `{"ingest":{"hep_stream":{"enable":false}}, "stateless":true,"storage":{"enable":false},"node":{"enable":true,"ducklake":{"catalog_path":"` + filepath.Join(dir, "catalog.sqlite") + `"}},"coordinator":{"enable":false,"hep_stream":{"enable":false}}}`
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
	t.Setenv("HOMER_INGEST_HEP_STREAM_ENABLE", "false")
	t.Setenv("HOMER_COORDINATOR_HEP_STREAM_ENABLE", "false")
	t.Setenv("HOMER_STORAGE_ENABLE", "false")
	t.Setenv("HOMER_NODE_ENABLE", "false")
	t.Setenv("HOMER_COORDINATOR_ENABLE", "true")
	t.Setenv("HOMER_COORDINATOR_SETTINGS_DB_TYPE", "postgres")
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
	if !cfg.Stateless || !cfg.Coordinator.Stateless || !cfg.Storage.DuckLake.Stateless || !cfg.Node.DuckLake.Stateless {
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

// PostgreSQL alone must not opt a deployment into stateless requirements.
func TestPostgresWithoutStatelessKeepsLocalSecrets(t *testing.T) {
	for _, setting := range []string{"", `"stateless":false,`} {
		t.Run(setting, func(t *testing.T) {
			dir := t.TempDir()
			file := filepath.Join(dir, "homer.json")
			body := `{` + setting + `"storage":{"enable":false},"node":{"enable":true,"local_secrets_dir":"` + dir + `","ducklake":{"catalog_type":"postgres","catalog_path":"postgres://localhost/catalog","data_path":"/data/parquet"}},"coordinator":{"enable":false}}`
			if err := os.WriteFile(file, []byte(body), 0600); err != nil {
				t.Fatal(err)
			}
			cfg, err := Load(file)
			if err != nil {
				t.Fatal(err)
			}
			if cfg.Stateless || cfg.Coordinator.Stateless || cfg.Storage.DuckLake.Stateless || cfg.Node.DuckLake.Stateless {
				t.Fatal("PostgreSQL implicitly enabled stateless mode")
			}
			if cfg.Node.FlightServer.AuthToken == "" {
				t.Fatal("stateful mode did not generate a node token")
			}
			if _, err := os.Stat(NodeAuthTokenFilePath(cfg.Node.DuckLake.CatalogPath, dir, cfg.Node.DuckLake.CatalogType)); err != nil {
				t.Fatalf("stateful token was not persisted: %v", err)
			}
		})
	}
}

func TestStatelessMaintenanceInheritance(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		cfg := &Config{Stateless: enabled}
		// Derived values cannot independently enable or disable the feature.
		cfg.Coordinator.Stateless = !enabled
		cfg.Storage.DuckLake.Stateless = !enabled
		cfg.Node.DuckLake.Stateless = !enabled
		applyDefaults(cfg)
		if cfg.Coordinator.Stateless != enabled || cfg.Storage.DuckLake.Stateless != enabled || cfg.Node.DuckLake.Stateless != enabled {
			t.Fatalf("top-level stateless=%v not inherited", enabled)
		}
	}
}
