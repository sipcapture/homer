// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package config

import (
	"testing"
)

// TestLoad_EnvScalars_AllSections pins down that every scalar HOMER_*
// env var from a typical production docker-compose set must land in
// Config. Before bindAllEnvs() was wired in, only keys explicitly
// registered in setDefaults() were honoured — the rest were silently
// dropped. This test documents the operator's expectation: "if I set
// an env var, I want that value in the running process".
func TestLoad_EnvScalars_AllSections(t *testing.T) {
	envs := map[string]string{
		// === modules ===
		"HOMER_INGEST_ENABLE":      "true",
		"HOMER_STORAGE_ENABLE":     "true",
		"HOMER_NODE_ENABLE":        "true",
		"HOMER_COORDINATOR_ENABLE": "true",

		// === ingest — load control ===
		"HOMER_INGEST_WORKER_COUNT": "8",
		"HOMER_INGEST_QUEUE_SIZE":   "80000",
		"HOMER_INGEST_WORKER_METRICS_FLUSH_PACKETS": "512",

		// === ingest — receivers ===
		"HOMER_INGEST_UDP_ENABLE":                 "true",
		"HOMER_INGEST_UDP_HOST":                   "0.0.0.0",
		"HOMER_INGEST_UDP_PORT":                   "9060",
		"HOMER_INGEST_UDP_MULTICORE":              "true",
		"HOMER_INGEST_TCP_ENABLE":                 "true",
		"HOMER_INGEST_TCP_HOST":                   "0.0.0.0",
		"HOMER_INGEST_TCP_PORT":                   "9060",
		"HOMER_INGEST_TCP_MULTICORE":              "true",
		"HOMER_INGEST_TLS_ENABLE":                 "false",
		"HOMER_INGEST_HTTP_ENABLE":                "true",
		"HOMER_INGEST_HTTP_HOST":                  "0.0.0.0",
		"HOMER_INGEST_HTTP_PORT":                  "9080",
		"HOMER_INGEST_HTTP_READ_TIMEOUT":          "30",
		"HOMER_INGEST_HTTP_WRITE_TIMEOUT":         "30",
		"HOMER_INGEST_HTTP_MAX_REQUEST_BODY_SIZE": "67108864",
		"HOMER_INGEST_HTTP_WEBSOCKET_ENABLE":      "false",
		"HOMER_INGEST_HTTPS_ENABLE":               "false",
		"HOMER_INGEST_HEP_HEPV2_ENABLE":           "true",
		"HOMER_INGEST_HEP_HEPV3_ENABLE":           "true",
		"HOMER_INGEST_HEP_PROTOBUF_ENABLE":        "true",
		"HOMER_INGEST_HEP_DEDUPLICATE":            "false",

		// === storage / DuckLake ===
		"HOMER_STORAGE_DUCKLAKE_CATALOG_TYPE":       "sqlite",
		"HOMER_STORAGE_DUCKLAKE_CATALOG_PATH":       "/data/homer/homer_catalog.sqlite",
		"HOMER_STORAGE_DUCKLAKE_DATA_PATH":          "/data/homer/parquet",
		"HOMER_STORAGE_DUCKLAKE_LAKE_NAME":          "homer_lake",
		"HOMER_STORAGE_DUCKLAKE_BATCH_SIZE":         "10000",
		"HOMER_STORAGE_DUCKLAKE_FLUSH_INTERVAL_SEC": "30",

		// === storage / compaction ===
		"HOMER_STORAGE_DUCKLAKE_COMPACTION_ENABLE":                       "true",
		"HOMER_STORAGE_DUCKLAKE_COMPACTION_CHECK_INTERVAL_SEC":           "1800",
		"HOMER_STORAGE_DUCKLAKE_COMPACTION_RETENTION_DAYS":               "30",
		"HOMER_STORAGE_DUCKLAKE_COMPACTION_SNAPSHOT_EXPIRE_INTERVAL_SEC": "3600",
		"HOMER_STORAGE_DUCKLAKE_COMPACTION_MIN_AGE_SEC":                  "3600",
		"HOMER_STORAGE_DUCKLAKE_COMPACTION_MIN_FILE_SIZE_BYTES":          "0",
		"HOMER_STORAGE_DUCKLAKE_COMPACTION_MAX_FILE_SIZE_BYTES":          "134217728",
		"HOMER_STORAGE_DUCKLAKE_COMPACTION_MAX_COMPACTED_FILES":          "100",

		// === storage / storage_policy (scalar settings) ===
		"HOMER_STORAGE_DUCKLAKE_STORAGE_POLICY_ENABLE":                "true",
		"HOMER_STORAGE_DUCKLAKE_STORAGE_POLICY_TTL_MOVE_INTERVAL_SEC": "3600",
		"HOMER_STORAGE_DUCKLAKE_STORAGE_POLICY_MOVE_FACTOR":           "0.8",
		"HOMER_STORAGE_DUCKLAKE_STORAGE_POLICY_CONCURRENT_MOVES":      "2",
		"HOMER_STORAGE_DUCKLAKE_STORAGE_POLICY_MOVE_ON_STARTUP":       "false",

		// === node / flight server ===
		"HOMER_NODE_FLIGHT_SERVER_HOST":             "0.0.0.0",
		"HOMER_NODE_FLIGHT_SERVER_PORT":             "50051",
		"HOMER_NODE_FLIGHT_SERVER_AUTH_TOKEN":       "your-secret-token-here",
		"HOMER_NODE_FLIGHT_SERVER_MAX_MESSAGE_SIZE": "16777216",
		"HOMER_NODE_FLIGHTSQL_SERVER_ENABLE":        "false",
		"HOMER_NODE_DUCKLAKE_LAKE_NAME":             "homer_lake",

		// === coordinator ===
		"HOMER_COORDINATOR_HTTP_SERVER_ENABLE":       "true",
		"HOMER_COORDINATOR_HTTP_SERVER_HOST":         "0.0.0.0",
		"HOMER_COORDINATOR_HTTP_SERVER_PORT":         "8080",
		"HOMER_COORDINATOR_HTTP_SERVER_STATIC_PATH":  "/usr/local/homer-core/dist",
		"HOMER_COORDINATOR_SETTINGS_DB_PATH":         "/data/homer/homer_settings.duckdb",
		"HOMER_COORDINATOR_FLIGHTSQL_SERVER_ENABLE":  "false",
		"HOMER_COORDINATOR_JWT_SECRET":               "change-this-to-a-secure-random-string-in-production",
		"HOMER_COORDINATOR_JWT_EXPIRE_HOURS":         "24",
		"HOMER_COORDINATOR_AUTH_ADMIN_USER":          "admin",
		"HOMER_COORDINATOR_AUTH_ADMIN_PASSWORD_HASH": "883ffc1f37fd0fe542b0fb9740035c4383e7d976c411161d24e62edace280f90",

		// === log / prometheus ===
		"HOMER_LOG_LEVEL":         "info",
		"HOMER_LOG_JSON":          "false",
		"HOMER_PROMETHEUS_ENABLE": "true",
		"HOMER_PROMETHEUS_HOST":   "0.0.0.0",
		"HOMER_PROMETHEUS_PORT":   "9090",
		"HOMER_PROMETHEUS_PATH":   "/metrics",
	}
	for k, v := range envs {
		t.Setenv(k, v)
	}

	path := writeTmpConfig(t, `{}`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	type check struct {
		name string
		got  any
		want any
	}
	checks := []check{
		// modules
		{"Ingest.Enable", cfg.Ingest.Enable, true},
		{"Storage.Enable", cfg.Storage.Enable, true},
		{"Node.Enable", cfg.Node.Enable, true},
		{"Coordinator.Enable", cfg.Coordinator.Enable, true},

		// ingest load control
		{"Ingest.WorkerCount", cfg.Ingest.WorkerCount, 8},
		{"Ingest.QueueSize", cfg.Ingest.QueueSize, 80000},
		{"Ingest.WorkerMetricsFlushPackets", cfg.Ingest.WorkerMetricsFlushPackets, 512},

		// receivers
		{"Ingest.UDP.Port", cfg.Ingest.UDP.Port, 9060},
		{"Ingest.UDP.Multicore", cfg.Ingest.UDP.Multicore, true},
		{"Ingest.TCP.Enable", cfg.Ingest.TCP.Enable, true},
		{"Ingest.TCP.Port", cfg.Ingest.TCP.Port, 9060},
		{"Ingest.TCP.Multicore", cfg.Ingest.TCP.Multicore, true},
		{"Ingest.HTTP.Port", cfg.Ingest.HTTP.Port, 9080},
		{"Ingest.HTTP.MaxRequestBodySize", cfg.Ingest.HTTP.MaxRequestBodySize, 67108864},
		{"Ingest.HEP.HepV2Enable", cfg.Ingest.HEP.HepV2Enable, true},
		{"Ingest.HEP.Deduplicate", cfg.Ingest.HEP.Deduplicate, false},

		// storage / ducklake
		{"Storage.DuckLake.CatalogPath", cfg.Storage.DuckLake.CatalogPath, "/data/homer/homer_catalog.sqlite"},
		{"Storage.DuckLake.DataPath", cfg.Storage.DuckLake.DataPath, "/data/homer/parquet"},
		{"Storage.DuckLake.LakeName", cfg.Storage.DuckLake.LakeName, "homer_lake"},
		{"Storage.DuckLake.BatchSize", cfg.Storage.DuckLake.BatchSize, 10000},
		{"Storage.DuckLake.FlushInterval", cfg.Storage.DuckLake.FlushInterval, 30},

		// compaction
		{"Compaction.Enable", cfg.Storage.DuckLake.Compaction.Enable, true},
		{"Compaction.CheckIntervalSec", cfg.Storage.DuckLake.Compaction.CheckIntervalSec, 1800},
		{"Compaction.RetentionDays", cfg.Storage.DuckLake.Compaction.RetentionDays, 30},
		{"Compaction.MaxFileSizeBytes", cfg.Storage.DuckLake.Compaction.MaxFileSizeBytes, int64(134217728)},
		{"Compaction.MaxCompactedFiles", cfg.Storage.DuckLake.Compaction.MaxCompactedFiles, 100},

		// storage_policy (scalars)
		{"StoragePolicy.Enable", cfg.Storage.DuckLake.StoragePolicy.Enable, true},
		{"StoragePolicy.TTLMoveIntervalSec", cfg.Storage.DuckLake.StoragePolicy.TTLMoveIntervalSec, 3600},
		{"StoragePolicy.MoveFactor", cfg.Storage.DuckLake.StoragePolicy.MoveFactor, 0.8},
		{"StoragePolicy.ConcurrentMoves", cfg.Storage.DuckLake.StoragePolicy.ConcurrentMoves, 2},

		// node
		{"Node.FlightServer.Port", cfg.Node.FlightServer.Port, 50051},
		{"Node.FlightServer.AuthToken", cfg.Node.FlightServer.AuthToken, "your-secret-token-here"},
		{"Node.FlightServer.MaxMessageSize", cfg.Node.FlightServer.MaxMessageSize, 16777216},
		{"Node.DuckLake.LakeName", cfg.Node.DuckLake.LakeName, "homer_lake"},

		// coordinator
		{"Coordinator.HTTPServer.Port", cfg.Coordinator.HTTPServer.Port, 8080},
		{"Coordinator.HTTPServer.StaticPath", cfg.Coordinator.HTTPServer.StaticPath, "/usr/local/homer-core/dist"},
		{"Coordinator.SettingsDBPath", cfg.Coordinator.SettingsDBPath, "/data/homer/homer_settings.duckdb"},
		{"Coordinator.JWT.Secret", cfg.Coordinator.JWT.Secret, "change-this-to-a-secure-random-string-in-production"},
		{"Coordinator.JWT.ExpireHours", cfg.Coordinator.JWT.ExpireHours, 24},
		{"Coordinator.Auth.AdminUser", cfg.Coordinator.Auth.AdminUser, "admin"},
		{"Coordinator.Auth.AdminPasswordHash", cfg.Coordinator.Auth.AdminPasswordHash, "883ffc1f37fd0fe542b0fb9740035c4383e7d976c411161d24e62edace280f90"},
		{"Coordinator.Auth.Type", cfg.Coordinator.Auth.Type, "internal"},

		// log + prometheus
		{"Log.Level", cfg.Log.Level, "info"},
		{"Prometheus.Enable", cfg.Prometheus.Enable, true},
		{"Prometheus.Port", cfg.Prometheus.Port, 9090},
		{"Prometheus.Path", cfg.Prometheus.Path, "/metrics"},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s: want %v (%T), got %v (%T)", c.name, c.want, c.want, c.got, c.got)
		}
	}
}

// TestLoad_EnvSlice_StoragePolicyVolumes verifies that indexed volumes
// (HOMER_STORAGE_DUCKLAKE_STORAGE_POLICY_VOLUMES_<idx>_*) land in
// Storage.DuckLake.StoragePolicy.Volumes. This is critical for hot/cold
// tiering — without a working ENV override the operator cannot wire an
// S3 cold-volume from docker-compose.
func TestLoad_EnvSlice_StoragePolicyVolumes(t *testing.T) {
	envs := map[string]string{
		// volume[0] — hot/local
		"HOMER_STORAGE_DUCKLAKE_STORAGE_POLICY_VOLUMES_0_NAME":              "hot",
		"HOMER_STORAGE_DUCKLAKE_STORAGE_POLICY_VOLUMES_0_TYPE":              "local",
		"HOMER_STORAGE_DUCKLAKE_STORAGE_POLICY_VOLUMES_0_PATH":              "/data/homer/parquet",
		"HOMER_STORAGE_DUCKLAKE_STORAGE_POLICY_VOLUMES_0_PRIORITY":          "0",
		"HOMER_STORAGE_DUCKLAKE_STORAGE_POLICY_VOLUMES_0_MAX_DATA_AGE_DAYS": "7",
		"HOMER_STORAGE_DUCKLAKE_STORAGE_POLICY_VOLUMES_0_MAX_SIZE_GB":       "100",

		// volume[1] — cold/S3
		"HOMER_STORAGE_DUCKLAKE_STORAGE_POLICY_VOLUMES_1_NAME":                 "cold",
		"HOMER_STORAGE_DUCKLAKE_STORAGE_POLICY_VOLUMES_1_TYPE":                 "s3",
		"HOMER_STORAGE_DUCKLAKE_STORAGE_POLICY_VOLUMES_1_PATH":                 "s3://homer-cold/data/",
		"HOMER_STORAGE_DUCKLAKE_STORAGE_POLICY_VOLUMES_1_PRIORITY":             "1",
		"HOMER_STORAGE_DUCKLAKE_STORAGE_POLICY_VOLUMES_1_MAX_DATA_AGE_DAYS":    "0",
		"HOMER_STORAGE_DUCKLAKE_STORAGE_POLICY_VOLUMES_1_S3_REGION":            "us-east-1",
		"HOMER_STORAGE_DUCKLAKE_STORAGE_POLICY_VOLUMES_1_S3_ACCESS_KEY_ID":     "rustfsadmin",
		"HOMER_STORAGE_DUCKLAKE_STORAGE_POLICY_VOLUMES_1_S3_SECRET_ACCESS_KEY": "rustfsadmin",
		"HOMER_STORAGE_DUCKLAKE_STORAGE_POLICY_VOLUMES_1_S3_ENDPOINT":          "http://rustfs:9000",
		"HOMER_STORAGE_DUCKLAKE_STORAGE_POLICY_VOLUMES_1_S3_USE_SSL":           "false",
	}
	for k, v := range envs {
		t.Setenv(k, v)
	}

	path := writeTmpConfig(t, `{}`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	vols := cfg.Storage.DuckLake.StoragePolicy.Volumes
	if len(vols) != 2 {
		t.Fatalf("storage_policy.volumes: want 2 entries, got %d (%+v)", len(vols), vols)
	}

	// volume[0] — hot
	if vols[0].Name != "hot" {
		t.Errorf("volumes[0].name: want hot, got %q", vols[0].Name)
	}
	if vols[0].Type != "local" {
		t.Errorf("volumes[0].type: want local, got %q", vols[0].Type)
	}
	if vols[0].Path != "/data/homer/parquet" {
		t.Errorf("volumes[0].path: want /data/homer/parquet, got %q", vols[0].Path)
	}
	if vols[0].Priority != 0 {
		t.Errorf("volumes[0].priority: want 0, got %d", vols[0].Priority)
	}
	if vols[0].MaxDataAgeDays != 7 {
		t.Errorf("volumes[0].max_data_age_days: want 7, got %d", vols[0].MaxDataAgeDays)
	}
	if vols[0].MaxSizeGB != 100 {
		t.Errorf("volumes[0].max_size_gb: want 100, got %d", vols[0].MaxSizeGB)
	}

	// volume[1] — cold/S3
	if vols[1].Name != "cold" {
		t.Errorf("volumes[1].name: want cold, got %q", vols[1].Name)
	}
	if vols[1].Type != "s3" {
		t.Errorf("volumes[1].type: want s3, got %q", vols[1].Type)
	}
	if vols[1].Path != "s3://homer-cold/data/" {
		t.Errorf("volumes[1].path: want s3 URL, got %q", vols[1].Path)
	}
	if vols[1].S3Region != "us-east-1" {
		t.Errorf("volumes[1].s3_region: want us-east-1, got %q", vols[1].S3Region)
	}
	if vols[1].S3AccessKeyID != "rustfsadmin" {
		t.Errorf("volumes[1].s3_access_key_id: want rustfsadmin, got %q", vols[1].S3AccessKeyID)
	}
	if vols[1].S3SecretKey != "rustfsadmin" {
		t.Errorf("volumes[1].s3_secret_access_key: want rustfsadmin, got %q", vols[1].S3SecretKey)
	}
	if vols[1].S3Endpoint != "http://rustfs:9000" {
		t.Errorf("volumes[1].s3_endpoint: want http://rustfs:9000, got %q", vols[1].S3Endpoint)
	}
	if vols[1].S3UseSSL {
		t.Errorf("volumes[1].s3_use_ssl: want false, got true")
	}
}

// TestLoad_EnvSlice_NodeDuckLakeVolumes covers the same volume layout
// but in the node.ducklake.volumes section — consumed by read-only
// nodes (the coordinator-facing facade).
func TestLoad_EnvSlice_NodeDuckLakeVolumes(t *testing.T) {
	envs := map[string]string{
		// volume[0] — hot
		"HOMER_NODE_DUCKLAKE_VOLUMES_0_NAME":         "hot",
		"HOMER_NODE_DUCKLAKE_VOLUMES_0_TYPE":         "local",
		"HOMER_NODE_DUCKLAKE_VOLUMES_0_CATALOG_TYPE": "sqlite",
		"HOMER_NODE_DUCKLAKE_VOLUMES_0_CATALOG_PATH": "/data/homer/homer_catalog.sqlite",
		"HOMER_NODE_DUCKLAKE_VOLUMES_0_PATH":         "/data/homer/parquet",

		// volume[1] — cold/S3
		"HOMER_NODE_DUCKLAKE_VOLUMES_1_NAME":                 "cold",
		"HOMER_NODE_DUCKLAKE_VOLUMES_1_TYPE":                 "s3",
		"HOMER_NODE_DUCKLAKE_VOLUMES_1_CATALOG_TYPE":         "sqlite",
		"HOMER_NODE_DUCKLAKE_VOLUMES_1_CATALOG_PATH":         "/data/homer/homer_catalog_cold.sqlite",
		"HOMER_NODE_DUCKLAKE_VOLUMES_1_PATH":                 "s3://homer-cold/data/",
		"HOMER_NODE_DUCKLAKE_VOLUMES_1_S3_REGION":            "us-east-1",
		"HOMER_NODE_DUCKLAKE_VOLUMES_1_S3_ACCESS_KEY_ID":     "rustfsadmin",
		"HOMER_NODE_DUCKLAKE_VOLUMES_1_S3_SECRET_ACCESS_KEY": "rustfsadmin",
		"HOMER_NODE_DUCKLAKE_VOLUMES_1_S3_ENDPOINT":          "http://rustfs:9000",
		"HOMER_NODE_DUCKLAKE_VOLUMES_1_S3_USE_SSL":           "false",
	}
	for k, v := range envs {
		t.Setenv(k, v)
	}

	path := writeTmpConfig(t, `{}`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	vols := cfg.Node.DuckLake.Volumes
	if len(vols) != 2 {
		t.Fatalf("node.ducklake.volumes: want 2 entries, got %d (%+v)", len(vols), vols)
	}
	if vols[0].Name != "hot" || vols[0].Type != "local" {
		t.Errorf("volumes[0]: want {hot,local}, got {%s,%s}", vols[0].Name, vols[0].Type)
	}
	if vols[0].CatalogPath != "/data/homer/homer_catalog.sqlite" {
		t.Errorf("volumes[0].catalog_path: got %q", vols[0].CatalogPath)
	}
	if vols[1].Name != "cold" || vols[1].Type != "s3" {
		t.Errorf("volumes[1]: want {cold,s3}, got {%s,%s}", vols[1].Name, vols[1].Type)
	}
	if vols[1].CatalogPath != "/data/homer/homer_catalog_cold.sqlite" {
		t.Errorf("volumes[1].catalog_path: got %q", vols[1].CatalogPath)
	}
	if vols[1].S3Endpoint != "http://rustfs:9000" {
		t.Errorf("volumes[1].s3_endpoint: got %q", vols[1].S3Endpoint)
	}
	if vols[1].S3UseSSL {
		t.Errorf("volumes[1].s3_use_ssl: want false, got true")
	}
}

// TestLoad_EnvSlice_CoordinatorNodes verifies that the coordinator
// picks up its node list from HOMER_COORDINATOR_NODES_<idx>_*.
func TestLoad_EnvSlice_CoordinatorNodes(t *testing.T) {
	envs := map[string]string{
		"HOMER_COORDINATOR_NODES_0_NAME":           "local",
		"HOMER_COORDINATOR_NODES_0_HOST":           "127.0.0.1",
		"HOMER_COORDINATOR_NODES_0_PORT":           "50051",
		"HOMER_COORDINATOR_NODES_0_FLIGHTSQL_PORT": "0",
		"HOMER_COORDINATOR_NODES_0_USE_TLS":        "false",
		"HOMER_COORDINATOR_NODES_0_TOKEN":          "your-secret-token-here",
		"HOMER_COORDINATOR_NODES_0_PRIORITY":       "1",
	}
	for k, v := range envs {
		t.Setenv(k, v)
	}

	path := writeTmpConfig(t, `{}`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if len(cfg.Coordinator.Nodes) != 1 {
		t.Fatalf("coordinator.nodes: want 1 entry, got %d", len(cfg.Coordinator.Nodes))
	}
	n := cfg.Coordinator.Nodes[0]
	if n.Name != "local" {
		t.Errorf("nodes[0].name: want local, got %q", n.Name)
	}
	if n.Host != "127.0.0.1" {
		t.Errorf("nodes[0].host: want 127.0.0.1, got %q", n.Host)
	}
	if n.Port != 50051 {
		t.Errorf("nodes[0].port: want 50051, got %d", n.Port)
	}
	if n.Token != "your-secret-token-here" {
		t.Errorf("nodes[0].token: want your-secret-token-here, got %q", n.Token)
	}
	if n.Priority != 1 {
		t.Errorf("nodes[0].priority: want 1, got %d", n.Priority)
	}
}

// TestLoad_Precedence_EnvBeatsFile answers question #1: when a config
// file is present, ENV variables must override values written in the
// file (the standard viper precedence: env > file > default).
//
// Covers both classes of fields fixed in env.go:
//   - scalar leaf inside a nested struct (storage.ducklake.batch_size,
//     coordinator.jwt.secret),
//   - slice of structs (storage.ducklake.storage_policy.volumes) —
//     applySliceEnvOverrides uses v.Set() which is the HIGHEST viper
//     priority, so the file's slice is REPLACED, not merged.
func TestLoad_Precedence_EnvBeatsFile(t *testing.T) {
	// File defines reasonable values for all three fields.
	path := writeTmpConfig(t, `{
  "storage": {
    "ducklake": {
      "batch_size": 5000,
      "storage_policy": {
        "enable": true,
        "volumes": [
          { "name": "hot-from-file", "type": "local", "path": "/file/hot" },
          { "name": "cold-from-file", "type": "local", "path": "/file/cold" }
        ]
      }
    }
  },
  "coordinator": {
    "jwt": { "secret": "from-file" }
  }
}`)

	// ENV overrides each of those three.
	t.Setenv("HOMER_STORAGE_DUCKLAKE_BATCH_SIZE", "99999")
	t.Setenv("HOMER_COORDINATOR_JWT_SECRET", "from-env")
	t.Setenv("HOMER_STORAGE_DUCKLAKE_STORAGE_POLICY_VOLUMES_0_NAME", "hot-from-env")
	t.Setenv("HOMER_STORAGE_DUCKLAKE_STORAGE_POLICY_VOLUMES_0_TYPE", "local")
	t.Setenv("HOMER_STORAGE_DUCKLAKE_STORAGE_POLICY_VOLUMES_0_PATH", "/env/hot")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// Scalars: env wins over file.
	if cfg.Storage.DuckLake.BatchSize != 99999 {
		t.Errorf("batch_size: want 99999 (env), got %d", cfg.Storage.DuckLake.BatchSize)
	}
	if cfg.Coordinator.JWT.Secret != "from-env" {
		t.Errorf("jwt.secret: want from-env, got %q", cfg.Coordinator.JWT.Secret)
	}

	// Slice: env REPLACES the file's array entirely (length 1, not 2).
	// Important caveat — operator should either populate the whole slice
	// via ENV or leave it to the file; partial overlay is not supported.
	vols := cfg.Storage.DuckLake.StoragePolicy.Volumes
	if len(vols) != 1 {
		t.Fatalf("volumes: want 1 entry (env replaces file slice), got %d (%+v)", len(vols), vols)
	}
	if vols[0].Name != "hot-from-env" {
		t.Errorf("volumes[0].name: want hot-from-env, got %q", vols[0].Name)
	}
	if vols[0].Path != "/env/hot" {
		t.Errorf("volumes[0].path: want /env/hot, got %q", vols[0].Path)
	}
}

// TestLoad_Precedence_NoFileDefaultsThenEnv answers question #2: when
// there is no config file, struct-tag / setDefaults values seed the
// Config and ENV variables then override them. Fields not touched by
// ENV keep their default.
func TestLoad_Precedence_NoFileDefaultsThenEnv(t *testing.T) {
	// Empty JSON simulates "no config file present" (the file exists
	// but contains no keys, so Unmarshal sees defaults + env only).
	path := writeTmpConfig(t, `{}`)

	// Override only two fields; everything else must come from defaults.
	t.Setenv("HOMER_INGEST_UDP_PORT", "19060")
	t.Setenv("HOMER_COORDINATOR_JWT_SECRET", "env-only-secret")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// ENV-overridden fields.
	if cfg.Ingest.UDP.Port != 19060 {
		t.Errorf("ingest.udp.port: want 19060 (env), got %d", cfg.Ingest.UDP.Port)
	}
	if cfg.Coordinator.JWT.Secret != "env-only-secret" {
		t.Errorf("jwt.secret: want env-only-secret, got %q", cfg.Coordinator.JWT.Secret)
	}

	// Defaults from struct tags must still hold for untouched fields.
	if cfg.Ingest.UDP.Host != "0.0.0.0" {
		t.Errorf("ingest.udp.host: want 0.0.0.0 (default), got %q", cfg.Ingest.UDP.Host)
	}
	if !cfg.Ingest.UDP.Multicore {
		t.Errorf("ingest.udp.multicore: want true (default), got false")
	}
	if cfg.Ingest.HTTP.Port != 9080 {
		t.Errorf("ingest.http.port: want 9080 (default), got %d", cfg.Ingest.HTTP.Port)
	}
	if cfg.Ingest.HTTP.MaxRequestBodySize != 67108864 {
		t.Errorf("ingest.http.max_request_body_size: want default, got %d",
			cfg.Ingest.HTTP.MaxRequestBodySize)
	}
	if cfg.Storage.DuckLake.LakeName != "homer_lake" {
		t.Errorf("storage.ducklake.lake_name: want homer_lake (default), got %q",
			cfg.Storage.DuckLake.LakeName)
	}
	if cfg.Coordinator.JWT.ExpireHours != 24 {
		t.Errorf("jwt.expire_hours: want 24 (default), got %d", cfg.Coordinator.JWT.ExpireHours)
	}
	if cfg.Coordinator.Auth.AdminUser != "admin" {
		t.Errorf("auth.admin_user: want admin (default), got %q", cfg.Coordinator.Auth.AdminUser)
	}
}

// TestLoad_EnvSlice_LogOutput is the simplest case: a slice of strings
// (log.output). docker-compose convention: HOMER_LOG_OUTPUT_0=...
// Uses non-default values so it actually exercises the indexed primitive
// slice path (log.output defaults to ["stdout"], which would mask a no-op).
func TestLoad_EnvSlice_LogOutput(t *testing.T) {
	t.Setenv("HOMER_LOG_OUTPUT_0", "file")
	t.Setenv("HOMER_LOG_OUTPUT_1", "syslog")

	path := writeTmpConfig(t, `{}`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if len(cfg.Log.Output) != 2 || cfg.Log.Output[0] != "file" || cfg.Log.Output[1] != "syslog" {
		t.Errorf("log.output: want [file syslog], got %v", cfg.Log.Output)
	}
}

// TestLoad_EnvSlice_SIPAlegCustomHeaders covers issue #728: the indexed
// docker-compose form for SIP custom-header extraction must reach
// ingest.sip.aleg_ids / custom_headers (otherwise XCallID/cid correlation
// and data_extra.custom_headers stay empty even with a correct parser).
func TestLoad_EnvSlice_SIPAlegCustomHeaders(t *testing.T) {
	t.Setenv("HOMER_INGEST_SIP_FORCE_ALEG_ID", "true")
	t.Setenv("HOMER_INGEST_SIP_ALEG_IDS_0", "X-Session")
	t.Setenv("HOMER_INGEST_SIP_CUSTOM_HEADERS_0", "X-Session")
	t.Setenv("HOMER_INGEST_SIP_CUSTOM_HEADERS_1", "X-CID")

	path := writeTmpConfig(t, `{}`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if !cfg.Ingest.SIP.ForceALegID {
		t.Errorf("ingest.sip.force_aleg_id: want true, got false")
	}
	if len(cfg.Ingest.SIP.AlegIDs) != 1 || cfg.Ingest.SIP.AlegIDs[0] != "X-Session" {
		t.Errorf("ingest.sip.aleg_ids: want [X-Session], got %#v", cfg.Ingest.SIP.AlegIDs)
	}
	if len(cfg.Ingest.SIP.CustomHeaders) != 2 ||
		cfg.Ingest.SIP.CustomHeaders[0] != "X-Session" ||
		cfg.Ingest.SIP.CustomHeaders[1] != "X-CID" {
		t.Errorf("ingest.sip.custom_headers: want [X-Session X-CID], got %#v", cfg.Ingest.SIP.CustomHeaders)
	}
}

// TestLoad_RealProductionEnvSet is an end-to-end smoke test that feeds
// the exact ENV variable set captured from a running production
// container (docker inspect) and verifies every variable lands in the
// matching Config field. If this test passes, the operator's
// docker-compose values are guaranteed to take effect.
func TestLoad_RealProductionEnvSet(t *testing.T) {
	envs := map[string]string{
		// Modules
		"HOMER_INGEST_ENABLE":      "true",
		"HOMER_STORAGE_ENABLE":     "true",
		"HOMER_NODE_ENABLE":        "true",
		"HOMER_COORDINATOR_ENABLE": "true",

		// Ingest: load + receivers
		"HOMER_INGEST_WORKER_COUNT":               "8",
		"HOMER_INGEST_QUEUE_SIZE":                 "80000",
		"HOMER_INGEST_WORKER_METRICS_FLUSH_PACKETS": "512",
		"HOMER_INGEST_UDP_ENABLE":                 "true",
		"HOMER_INGEST_UDP_HOST":                   "0.0.0.0",
		"HOMER_INGEST_UDP_PORT":                   "9060",
		"HOMER_INGEST_UDP_MULTICORE":              "true",
		"HOMER_INGEST_TCP_ENABLE":                 "true",
		"HOMER_INGEST_TCP_HOST":                   "0.0.0.0",
		"HOMER_INGEST_TCP_PORT":                   "9060",
		"HOMER_INGEST_TCP_MULTICORE":              "true",
		"HOMER_INGEST_TLS_ENABLE":                 "false",
		"HOMER_INGEST_HTTP_ENABLE":                "true",
		"HOMER_INGEST_HTTP_HOST":                  "0.0.0.0",
		"HOMER_INGEST_HTTP_PORT":                  "9080",
		"HOMER_INGEST_HTTP_READ_TIMEOUT":          "30",
		"HOMER_INGEST_HTTP_WRITE_TIMEOUT":         "30",
		"HOMER_INGEST_HTTP_MAX_REQUEST_BODY_SIZE": "67108864",
		"HOMER_INGEST_HTTP_WEBSOCKET_ENABLE":      "false",
		"HOMER_INGEST_HTTPS_ENABLE":               "false",
		"HOMER_INGEST_HEP_HEPV2_ENABLE":           "true",
		"HOMER_INGEST_HEP_HEPV3_ENABLE":           "true",
		"HOMER_INGEST_HEP_PROTOBUF_ENABLE":        "true",
		"HOMER_INGEST_HEP_DEDUPLICATE":            "false",

		// Storage: DuckLake core
		"HOMER_STORAGE_DUCKLAKE_CATALOG_TYPE":       "sqlite",
		"HOMER_STORAGE_DUCKLAKE_CATALOG_PATH":       "/data/homer/homer_catalog.sqlite",
		"HOMER_STORAGE_DUCKLAKE_DATA_PATH":          "/data/homer/parquet",
		"HOMER_STORAGE_DUCKLAKE_LAKE_NAME":          "homer_lake",
		"HOMER_STORAGE_DUCKLAKE_BATCH_SIZE":         "10000",
		"HOMER_STORAGE_DUCKLAKE_FLUSH_INTERVAL_SEC": "30",

		// Storage: compaction
		"HOMER_STORAGE_DUCKLAKE_COMPACTION_ENABLE":                       "true",
		"HOMER_STORAGE_DUCKLAKE_COMPACTION_CHECK_INTERVAL_SEC":           "1800",
		"HOMER_STORAGE_DUCKLAKE_COMPACTION_RETENTION_DAYS":               "30",
		"HOMER_STORAGE_DUCKLAKE_COMPACTION_SNAPSHOT_EXPIRE_INTERVAL_SEC": "3600",
		"HOMER_STORAGE_DUCKLAKE_COMPACTION_MIN_AGE_SEC":                  "3600",
		"HOMER_STORAGE_DUCKLAKE_COMPACTION_MIN_FILE_SIZE_BYTES":          "0",
		"HOMER_STORAGE_DUCKLAKE_COMPACTION_MAX_FILE_SIZE_BYTES":          "134217728",
		"HOMER_STORAGE_DUCKLAKE_COMPACTION_MAX_COMPACTED_FILES":          "100",

		// Storage: storage_policy (scalars). NOTE: enable=false here on
		// purpose — captured from a real container where tiering was off.
		"HOMER_STORAGE_DUCKLAKE_STORAGE_POLICY_ENABLE":                "false",
		"HOMER_STORAGE_DUCKLAKE_STORAGE_POLICY_TTL_MOVE_INTERVAL_SEC": "3600",
		"HOMER_STORAGE_DUCKLAKE_STORAGE_POLICY_MOVE_FACTOR":           "0.8",
		"HOMER_STORAGE_DUCKLAKE_STORAGE_POLICY_CONCURRENT_MOVES":      "2",
		"HOMER_STORAGE_DUCKLAKE_STORAGE_POLICY_MOVE_ON_STARTUP":       "false",

		// Storage: storage_policy.volumes — hot/local + cold/S3
		"HOMER_STORAGE_DUCKLAKE_STORAGE_POLICY_VOLUMES_0_NAME":                 "hot",
		"HOMER_STORAGE_DUCKLAKE_STORAGE_POLICY_VOLUMES_0_TYPE":                 "local",
		"HOMER_STORAGE_DUCKLAKE_STORAGE_POLICY_VOLUMES_0_PATH":                 "/data/homer/parquet",
		"HOMER_STORAGE_DUCKLAKE_STORAGE_POLICY_VOLUMES_0_PRIORITY":             "0",
		"HOMER_STORAGE_DUCKLAKE_STORAGE_POLICY_VOLUMES_0_MAX_DATA_AGE_DAYS":    "7",
		"HOMER_STORAGE_DUCKLAKE_STORAGE_POLICY_VOLUMES_0_MAX_SIZE_GB":          "100",
		"HOMER_STORAGE_DUCKLAKE_STORAGE_POLICY_VOLUMES_1_NAME":                 "cold",
		"HOMER_STORAGE_DUCKLAKE_STORAGE_POLICY_VOLUMES_1_TYPE":                 "s3",
		"HOMER_STORAGE_DUCKLAKE_STORAGE_POLICY_VOLUMES_1_PATH":                 "s3://homer-cold/data/",
		"HOMER_STORAGE_DUCKLAKE_STORAGE_POLICY_VOLUMES_1_PRIORITY":             "1",
		"HOMER_STORAGE_DUCKLAKE_STORAGE_POLICY_VOLUMES_1_MAX_DATA_AGE_DAYS":    "0",
		"HOMER_STORAGE_DUCKLAKE_STORAGE_POLICY_VOLUMES_1_S3_REGION":            "us-east-1",
		"HOMER_STORAGE_DUCKLAKE_STORAGE_POLICY_VOLUMES_1_S3_ACCESS_KEY_ID":     "rustfsadmin",
		"HOMER_STORAGE_DUCKLAKE_STORAGE_POLICY_VOLUMES_1_S3_SECRET_ACCESS_KEY": "rustfsadmin",
		"HOMER_STORAGE_DUCKLAKE_STORAGE_POLICY_VOLUMES_1_S3_ENDPOINT":          "http://rustfs:9000",
		"HOMER_STORAGE_DUCKLAKE_STORAGE_POLICY_VOLUMES_1_S3_USE_SSL":           "false",

		// Node: flight server + ducklake
		"HOMER_NODE_FLIGHT_SERVER_HOST":             "0.0.0.0",
		"HOMER_NODE_FLIGHT_SERVER_PORT":             "50051",
		"HOMER_NODE_FLIGHT_SERVER_AUTH_TOKEN":       "your-secret-token-here",
		"HOMER_NODE_FLIGHT_SERVER_MAX_MESSAGE_SIZE": "16777216",
		"HOMER_NODE_FLIGHTSQL_SERVER_ENABLE":        "false",
		"HOMER_NODE_DUCKLAKE_LAKE_NAME":             "homer_lake",

		// Node: ducklake.volumes — NOTE: volume[0].name is "default"
		// here (not "hot") — matches the captured prod set.
		"HOMER_NODE_DUCKLAKE_VOLUMES_0_NAME":                 "default",
		"HOMER_NODE_DUCKLAKE_VOLUMES_0_TYPE":                 "local",
		"HOMER_NODE_DUCKLAKE_VOLUMES_0_CATALOG_TYPE":         "sqlite",
		"HOMER_NODE_DUCKLAKE_VOLUMES_0_CATALOG_PATH":         "/data/homer/homer_catalog.sqlite",
		"HOMER_NODE_DUCKLAKE_VOLUMES_0_PATH":                 "/data/homer/parquet",
		"HOMER_NODE_DUCKLAKE_VOLUMES_1_NAME":                 "cold",
		"HOMER_NODE_DUCKLAKE_VOLUMES_1_TYPE":                 "s3",
		"HOMER_NODE_DUCKLAKE_VOLUMES_1_CATALOG_TYPE":         "sqlite",
		"HOMER_NODE_DUCKLAKE_VOLUMES_1_CATALOG_PATH":         "/data/homer/homer_catalog_cold.sqlite",
		"HOMER_NODE_DUCKLAKE_VOLUMES_1_PATH":                 "s3://homer-cold/data/",
		"HOMER_NODE_DUCKLAKE_VOLUMES_1_S3_REGION":            "us-east-1",
		"HOMER_NODE_DUCKLAKE_VOLUMES_1_S3_ACCESS_KEY_ID":     "rustfsadmin",
		"HOMER_NODE_DUCKLAKE_VOLUMES_1_S3_SECRET_ACCESS_KEY": "rustfsadmin",
		"HOMER_NODE_DUCKLAKE_VOLUMES_1_S3_ENDPOINT":          "http://rustfs:9000",
		"HOMER_NODE_DUCKLAKE_VOLUMES_1_S3_USE_SSL":           "false",

		// Coordinator
		"HOMER_COORDINATOR_HTTP_SERVER_ENABLE":       "true",
		"HOMER_COORDINATOR_HTTP_SERVER_HOST":         "0.0.0.0",
		"HOMER_COORDINATOR_HTTP_SERVER_PORT":         "8080",
		"HOMER_COORDINATOR_HTTP_SERVER_STATIC_PATH":  "/usr/local/homer-core/dist",
		"HOMER_COORDINATOR_SETTINGS_DB_PATH":         "/data/homer/homer_settings.duckdb",
		"HOMER_COORDINATOR_FLIGHTSQL_SERVER_ENABLE":  "false",
		"HOMER_COORDINATOR_JWT_SECRET":               "change-this-to-a-secure-random-string-in-production",
		"HOMER_COORDINATOR_JWT_EXPIRE_HOURS":         "24",
		"HOMER_COORDINATOR_AUTH_ADMIN_USER":          "admin",
		"HOMER_COORDINATOR_AUTH_ADMIN_PASSWORD_HASH": "883ffc1f37fd0fe542b0fb9740035c4383e7d976c411161d24e62edace280f90",
		"HOMER_COORDINATOR_NODES_0_NAME":             "local",
		"HOMER_COORDINATOR_NODES_0_HOST":             "127.0.0.1",
		"HOMER_COORDINATOR_NODES_0_PORT":             "50051",
		"HOMER_COORDINATOR_NODES_0_FLIGHTSQL_PORT":   "0",
		"HOMER_COORDINATOR_NODES_0_USE_TLS":          "false",
		"HOMER_COORDINATOR_NODES_0_TOKEN":            "your-secret-token-here",
		"HOMER_COORDINATOR_NODES_0_PRIORITY":         "1",

		// Log + Prometheus
		"HOMER_LOG_LEVEL":         "info",
		"HOMER_LOG_JSON":          "false",
		"HOMER_LOG_OUTPUT_0":      "stdout",
		"HOMER_PROMETHEUS_ENABLE": "true",
		"HOMER_PROMETHEUS_HOST":   "0.0.0.0",
		"HOMER_PROMETHEUS_PORT":   "9090",
		"HOMER_PROMETHEUS_PATH":   "/metrics",
	}
	for k, v := range envs {
		t.Setenv(k, v)
	}

	path := writeTmpConfig(t, `{}`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// --- Scalars ---------------------------------------------------
	type check struct {
		name string
		got  any
		want any
	}
	checks := []check{
		// modules
		{"Ingest.Enable", cfg.Ingest.Enable, true},
		{"Storage.Enable", cfg.Storage.Enable, true},
		{"Node.Enable", cfg.Node.Enable, true},
		{"Coordinator.Enable", cfg.Coordinator.Enable, true},

		// ingest load + udp/tcp/http
		{"Ingest.WorkerCount", cfg.Ingest.WorkerCount, 8},
		{"Ingest.QueueSize", cfg.Ingest.QueueSize, 80000},
		{"Ingest.WorkerMetricsFlushPackets", cfg.Ingest.WorkerMetricsFlushPackets, 512},
		{"Ingest.UDP.Enable", cfg.Ingest.UDP.Enable, true},
		{"Ingest.UDP.Host", cfg.Ingest.UDP.Host, "0.0.0.0"},
		{"Ingest.UDP.Port", cfg.Ingest.UDP.Port, 9060},
		{"Ingest.UDP.Multicore", cfg.Ingest.UDP.Multicore, true},
		{"Ingest.TCP.Enable", cfg.Ingest.TCP.Enable, true},
		{"Ingest.TCP.Host", cfg.Ingest.TCP.Host, "0.0.0.0"},
		{"Ingest.TCP.Port", cfg.Ingest.TCP.Port, 9060},
		{"Ingest.TCP.Multicore", cfg.Ingest.TCP.Multicore, true},
		{"Ingest.TLS.Enable", cfg.Ingest.TLS.Enable, false},
		{"Ingest.HTTP.Enable", cfg.Ingest.HTTP.Enable, true},
		{"Ingest.HTTP.Host", cfg.Ingest.HTTP.Host, "0.0.0.0"},
		{"Ingest.HTTP.Port", cfg.Ingest.HTTP.Port, 9080},
		{"Ingest.HTTP.ReadTimeout", cfg.Ingest.HTTP.ReadTimeout, 30},
		{"Ingest.HTTP.WriteTimeout", cfg.Ingest.HTTP.WriteTimeout, 30},
		{"Ingest.HTTP.MaxRequestBodySize", cfg.Ingest.HTTP.MaxRequestBodySize, 67108864},
		{"Ingest.HTTP.WebSocketEnable", cfg.Ingest.HTTP.WebSocketEnable, false},
		{"Ingest.HTTPS.Enable", cfg.Ingest.HTTPS.Enable, false},
		{"Ingest.HEP.HepV2Enable", cfg.Ingest.HEP.HepV2Enable, true},
		{"Ingest.HEP.HepV3Enable", cfg.Ingest.HEP.HepV3Enable, true},
		{"Ingest.HEP.ProtobufEnable", cfg.Ingest.HEP.ProtobufEnable, true},
		{"Ingest.HEP.Deduplicate", cfg.Ingest.HEP.Deduplicate, false},

		// storage ducklake + compaction
		{"Storage.DuckLake.CatalogType", cfg.Storage.DuckLake.CatalogType, "sqlite"},
		{"Storage.DuckLake.CatalogPath", cfg.Storage.DuckLake.CatalogPath, "/data/homer/homer_catalog.sqlite"},
		{"Storage.DuckLake.DataPath", cfg.Storage.DuckLake.DataPath, "/data/homer/parquet"},
		{"Storage.DuckLake.LakeName", cfg.Storage.DuckLake.LakeName, "homer_lake"},
		{"Storage.DuckLake.BatchSize", cfg.Storage.DuckLake.BatchSize, 10000},
		{"Storage.DuckLake.FlushInterval", cfg.Storage.DuckLake.FlushInterval, 30},
		{"Compaction.Enable", cfg.Storage.DuckLake.Compaction.Enable, true},
		{"Compaction.CheckIntervalSec", cfg.Storage.DuckLake.Compaction.CheckIntervalSec, 1800},
		{"Compaction.RetentionDays", cfg.Storage.DuckLake.Compaction.RetentionDays, 30},
		{"Compaction.SnapshotExpireIntervalSec", cfg.Storage.DuckLake.Compaction.SnapshotExpireIntervalSec, 3600},
		{"Compaction.MinAgeSec", cfg.Storage.DuckLake.Compaction.MinAgeSec, 3600},
		{"Compaction.MinFileSizeBytes", cfg.Storage.DuckLake.Compaction.MinFileSizeBytes, int64(0)},
		{"Compaction.MaxFileSizeBytes", cfg.Storage.DuckLake.Compaction.MaxFileSizeBytes, int64(134217728)},
		{"Compaction.MaxCompactedFiles", cfg.Storage.DuckLake.Compaction.MaxCompactedFiles, 100},

		// storage_policy scalars — Enable=false from real container set
		{"StoragePolicy.Enable", cfg.Storage.DuckLake.StoragePolicy.Enable, false},
		{"StoragePolicy.TTLMoveIntervalSec", cfg.Storage.DuckLake.StoragePolicy.TTLMoveIntervalSec, 3600},
		{"StoragePolicy.MoveFactor", cfg.Storage.DuckLake.StoragePolicy.MoveFactor, 0.8},
		{"StoragePolicy.ConcurrentMoves", cfg.Storage.DuckLake.StoragePolicy.ConcurrentMoves, 2},
		{"StoragePolicy.MoveOnStartup", cfg.Storage.DuckLake.StoragePolicy.MoveOnStartup, false},

		// node
		{"Node.FlightServer.Host", cfg.Node.FlightServer.Host, "0.0.0.0"},
		{"Node.FlightServer.Port", cfg.Node.FlightServer.Port, 50051},
		{"Node.FlightServer.AuthToken", cfg.Node.FlightServer.AuthToken, "your-secret-token-here"},
		{"Node.FlightServer.MaxMessageSize", cfg.Node.FlightServer.MaxMessageSize, 16777216},
		{"Node.FlightSQLServer.Enable", cfg.Node.FlightSQLServer.Enable, false},
		{"Node.DuckLake.LakeName", cfg.Node.DuckLake.LakeName, "homer_lake"},

		// coordinator scalars
		{"Coordinator.HTTPServer.Enable", cfg.Coordinator.HTTPServer.Enable, true},
		{"Coordinator.HTTPServer.Host", cfg.Coordinator.HTTPServer.Host, "0.0.0.0"},
		{"Coordinator.HTTPServer.Port", cfg.Coordinator.HTTPServer.Port, 8080},
		{"Coordinator.HTTPServer.StaticPath", cfg.Coordinator.HTTPServer.StaticPath, "/usr/local/homer-core/dist"},
		{"Coordinator.SettingsDBPath", cfg.Coordinator.SettingsDBPath, "/data/homer/homer_settings.duckdb"},
		{"Coordinator.FlightSQLServer.Enable", cfg.Coordinator.FlightSQLServer.Enable, false},
		{"Coordinator.JWT.Secret", cfg.Coordinator.JWT.Secret, "change-this-to-a-secure-random-string-in-production"},
		{"Coordinator.JWT.ExpireHours", cfg.Coordinator.JWT.ExpireHours, 24},
		{"Coordinator.Auth.AdminUser", cfg.Coordinator.Auth.AdminUser, "admin"},
		{"Coordinator.Auth.AdminPasswordHash", cfg.Coordinator.Auth.AdminPasswordHash, "883ffc1f37fd0fe542b0fb9740035c4383e7d976c411161d24e62edace280f90"},
		{"Coordinator.Auth.Type", cfg.Coordinator.Auth.Type, "internal"},

		// log + prometheus
		{"Log.Level", cfg.Log.Level, "info"},
		{"Log.JSON", cfg.Log.JSON, false},
		{"Prometheus.Enable", cfg.Prometheus.Enable, true},
		{"Prometheus.Host", cfg.Prometheus.Host, "0.0.0.0"},
		{"Prometheus.Port", cfg.Prometheus.Port, 9090},
		{"Prometheus.Path", cfg.Prometheus.Path, "/metrics"},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s: want %v (%T), got %v (%T)", c.name, c.want, c.want, c.got, c.got)
		}
	}

	// --- storage.ducklake.storage_policy.volumes -------------------
	spVols := cfg.Storage.DuckLake.StoragePolicy.Volumes
	if len(spVols) != 2 {
		t.Fatalf("storage_policy.volumes: want 2 entries, got %d (%+v)", len(spVols), spVols)
	}
	// volume[0] — hot/local
	if spVols[0].Name != "hot" || spVols[0].Type != "local" ||
		spVols[0].Path != "/data/homer/parquet" || spVols[0].Priority != 0 ||
		spVols[0].MaxDataAgeDays != 7 || spVols[0].MaxSizeGB != 100 {
		t.Errorf("storage_policy.volumes[0] mismatch: %+v", spVols[0])
	}
	// volume[1] — cold/S3
	if spVols[1].Name != "cold" || spVols[1].Type != "s3" ||
		spVols[1].Path != "s3://homer-cold/data/" || spVols[1].Priority != 1 ||
		spVols[1].MaxDataAgeDays != 0 ||
		spVols[1].S3Region != "us-east-1" ||
		spVols[1].S3AccessKeyID != "rustfsadmin" ||
		spVols[1].S3SecretKey != "rustfsadmin" ||
		spVols[1].S3Endpoint != "http://rustfs:9000" ||
		spVols[1].S3UseSSL {
		t.Errorf("storage_policy.volumes[1] mismatch: %+v", spVols[1])
	}

	// --- node.ducklake.volumes -------------------------------------
	ndVols := cfg.Node.DuckLake.Volumes
	if len(ndVols) != 2 {
		t.Fatalf("node.ducklake.volumes: want 2 entries, got %d (%+v)", len(ndVols), ndVols)
	}
	// volume[0] — name is "default" in the captured prod set
	if ndVols[0].Name != "default" || ndVols[0].Type != "local" ||
		ndVols[0].CatalogType != "sqlite" ||
		ndVols[0].CatalogPath != "/data/homer/homer_catalog.sqlite" ||
		ndVols[0].Path != "/data/homer/parquet" {
		t.Errorf("node.ducklake.volumes[0] mismatch: %+v", ndVols[0])
	}
	// volume[1] — cold/S3
	if ndVols[1].Name != "cold" || ndVols[1].Type != "s3" ||
		ndVols[1].CatalogType != "sqlite" ||
		ndVols[1].CatalogPath != "/data/homer/homer_catalog_cold.sqlite" ||
		ndVols[1].Path != "s3://homer-cold/data/" ||
		ndVols[1].S3Region != "us-east-1" ||
		ndVols[1].S3AccessKeyID != "rustfsadmin" ||
		ndVols[1].S3SecretKey != "rustfsadmin" ||
		ndVols[1].S3Endpoint != "http://rustfs:9000" ||
		ndVols[1].S3UseSSL {
		t.Errorf("node.ducklake.volumes[1] mismatch: %+v", ndVols[1])
	}

	// --- coordinator.nodes -----------------------------------------
	if len(cfg.Coordinator.Nodes) != 1 {
		t.Fatalf("coordinator.nodes: want 1 entry, got %d", len(cfg.Coordinator.Nodes))
	}
	n := cfg.Coordinator.Nodes[0]
	if n.Name != "local" || n.Host != "127.0.0.1" || n.Port != 50051 ||
		n.FlightSQLPort != 0 || n.UseTLS ||
		n.Token != "your-secret-token-here" || n.Priority != 1 {
		t.Errorf("coordinator.nodes[0] mismatch: %+v", n)
	}

	// --- log.output ------------------------------------------------
	if len(cfg.Log.Output) != 1 || cfg.Log.Output[0] != "stdout" {
		t.Errorf("log.output: want [stdout], got %v", cfg.Log.Output)
	}
}
