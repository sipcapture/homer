// Copyright (C) 2026 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package config

import (
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
// PostgreSQL secrets use localSecretsDir when set, otherwise the
// default settings directory. A DSN is never interpreted as a filesystem path.
func SettingsDBLocalPath(value, localSecretsDir string) string {
	if strings.TrimSpace(value) == "" || IsPostgresSettingsPath(value) {
		if dir := strings.TrimSpace(localSecretsDir); dir != "" {
			return filepath.Join(dir, filepath.Base(defaultSettingsDBPath))
		}
		return defaultSettingsDBPath
	}
	return value
}
