// Copyright (C) 2026 Homer Server Contributors
// SPDX-License-Identifier: AGPL-3.0-or-later

package config

import (
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

// ResolveCatalogType validates catalog_path against the configured backend.
// An empty catalog type defaults to SQLite.
func ResolveCatalogType(ct, path string) (string, error) {
	typ := strings.ToLower(strings.TrimSpace(ct))
	if typ != "" && typ != "sqlite" && typ != "postgres" && typ != "postgresql" {
		return "", fmt.Errorf("unsupported DuckLake catalog_type %q", ct)
	}
	if IsPostgresSettingsPath(path) {
		if typ != "postgres" && typ != "postgresql" {
			return "", fmt.Errorf("PostgreSQL catalog_path requires catalog_type=postgres")
		}
		if _, err := pgx.ParseConfig(NormalizePostgresDSN(path)); err != nil {
			return "", fmt.Errorf("invalid PostgreSQL DuckLake catalog_path")
		}
		return "postgres", nil
	}
	if typ == "postgres" || typ == "postgresql" {
		return "", fmt.Errorf("PostgreSQL DuckLake catalog requires a connection string in catalog_path")
	}
	return "sqlite", nil
}
