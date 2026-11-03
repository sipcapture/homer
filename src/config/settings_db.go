// Copyright (C) 2026 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package config

import (
	"fmt"
	"path/filepath"
	"strings"
)

const defaultSettingsDBPath = "/var/lib/homer/homer_settings.duckdb"

// IsPostgresSettingsPath recognizes PostgreSQL URLs and libpq keyword DSNs.
// Detect the syntax before parsing so malformed PostgreSQL strings do not
// silently fall back to being used as local database filenames.
func IsPostgresSettingsPath(value string) bool {
	value = strings.TrimSpace(value)
	lower := strings.ToLower(value)
	if strings.HasPrefix(lower, "postgres://") || strings.HasPrefix(lower, "postgresql://") {
		return true
	}
	key, _, found := strings.Cut(value, "=")
	if !found {
		return false
	}
	switch strings.TrimSpace(key) {
	case "host", "hostaddr", "port", "dbname", "user", "password", "passfile", "service", "servicefile",
		"sslmode", "sslcert", "sslkey", "sslrootcert", "sslpassword", "sslcrl", "sslcrldir", "sslsni",
		"connect_timeout", "options", "application_name", "fallback_application_name", "client_encoding",
		"target_session_attrs", "channel_binding", "gssencmode", "krbsrvname", "gsslib",
		"keepalives", "keepalives_idle", "keepalives_interval", "keepalives_count", "tcp_user_timeout":
		return true
	}
	return false
}

// NormalizePostgresDSN normalizes URL schemes for PostgreSQL clients while
// preserving connection values and libpq keyword DSNs.
func NormalizePostgresDSN(value string) string {
	value = strings.TrimSpace(value)
	if scheme, rest, ok := strings.Cut(value, "://"); ok &&
		(strings.EqualFold(scheme, "postgres") || strings.EqualFold(scheme, "postgresql")) {
		return strings.ToLower(scheme) + "://" + rest
	}
	return value
}

// SettingsDBLocalPath provides a local anchor for coordinator secret files.
// A configured directory overrides backend defaults; PostgreSQL otherwise uses
// the default settings directory. A DSN is never treated as a filesystem path.
func SettingsDBLocalPath(value, localSecretsDir, settingsDBType string) string {
	if dir := strings.TrimSpace(localSecretsDir); dir != "" {
		return filepath.Join(dir, filepath.Base(defaultSettingsDBPath))
	}
	if strings.TrimSpace(value) == "" || isPostgresType(settingsDBType) {
		return defaultSettingsDBPath
	}
	return value
}

// NodeCatalogPath selects an attached PostgreSQL volume even when the legacy
// single-catalog field still contains its SQLite default.
func NodeCatalogPath(cfg *NodeConfig) string {
	_, path := nodeCatalog(cfg)
	return path
}

// NodeCatalogType returns the configured type for the catalog selected by NodeCatalogPath.
func NodeCatalogType(cfg *NodeConfig) string {
	typ, _ := nodeCatalog(cfg)
	typ = strings.ToLower(strings.TrimSpace(typ))
	if typ == "postgresql" {
		return "postgres"
	}
	if typ == "" {
		return "sqlite"
	}
	return typ
}

func nodeCatalog(cfg *NodeConfig) (string, string) {
	typ := cfg.DuckLake.CatalogType
	path := strings.TrimSpace(cfg.DuckLake.CatalogPath)
	for _, volume := range cfg.DuckLake.Volumes {
		typ := strings.ToLower(strings.TrimSpace(volume.CatalogType))
		if typ == "postgres" || typ == "postgresql" {
			return typ, volume.CatalogPath
		}
	}
	if path == "" && len(cfg.DuckLake.Volumes) > 0 {
		path = cfg.DuckLake.Volumes[0].CatalogPath
		typ = cfg.DuckLake.Volumes[0].CatalogType
	}
	return typ, path
}

// ResolveSettingsDBType validates the settings path against the configured backend.
// An empty type preserves the local DuckDB default.
func ResolveSettingsDBType(typ, path string) (string, error) {
	typ = strings.ToLower(strings.TrimSpace(typ))
	if typ == "" {
		typ = "sqlite"
	}
	switch typ {
	case "sqlite":
		if IsPostgresSettingsPath(path) {
			return "", fmt.Errorf("coordinator.settings_db_type: PostgreSQL settings_db_path requires settings_db_type=postgres")
		}
	case "postgres", "postgresql":
		if !IsPostgresSettingsPath(path) {
			return "", fmt.Errorf("coordinator.settings_db_type: postgres requires a PostgreSQL connection string in settings_db_path")
		}
		typ = "postgres"
	default:
		return "", fmt.Errorf("coordinator.settings_db_type: expected sqlite or postgres")
	}
	return typ, nil
}

func isPostgresType(typ string) bool {
	typ = strings.ToLower(strings.TrimSpace(typ))
	return typ == "postgres" || typ == "postgresql"
}
