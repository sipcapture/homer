// Copyright (C) 2026 Homer Server Contributors
// SPDX-License-Identifier: AGPL-3.0-or-later

package ducklake

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/sipcapture/homer-core/src/config"
)

// IsPostgresCatalogPath recognizes PostgreSQL URLs and libpq connection strings.
func IsPostgresCatalogPath(path string) bool { return config.IsPostgresSettingsPath(path) }

// CatalogDisplayPath is safe to include in logs without exposing a DSN.
func CatalogDisplayPath(path string) string {
	if IsPostgresCatalogPath(path) {
		cfg, err := pgx.ParseConfig(config.NormalizePostgresDSN(path))
		if err != nil {
			return "PostgreSQL (invalid connection string)"
		}
		return fmt.Sprintf("PostgreSQL host=%q port=%d database=%q", cfg.Host, cfg.Port, cfg.Database)
	}
	return path
}

// ResolveCatalogType preserves SQLite defaults and recognizes PostgreSQL DSNs
// in catalog_path, including configurations retaining the default catalog_type.
func ResolveCatalogType(ct CatalogType, path string) (CatalogType, error) {
	typ := strings.ToLower(strings.TrimSpace(string(ct)))
	if typ != "" && typ != "sqlite" && typ != "postgres" && typ != "postgresql" {
		return "", fmt.Errorf("unsupported DuckLake catalog_type %q", ct)
	}
	if IsPostgresCatalogPath(path) {
		if _, err := pgx.ParseConfig(config.NormalizePostgresDSN(path)); err != nil {
			return "", fmt.Errorf("invalid PostgreSQL DuckLake catalog_path")
		}
		return CatalogPostgres, nil
	}
	if typ == "postgres" || typ == "postgresql" {
		return "", fmt.Errorf("PostgreSQL DuckLake catalog requires a connection string in catalog_path")
	}
	return CatalogSQLite, nil
}

// CatalogAttachSQL is shared by writers, nodes, volumes and maintenance handles.
func CatalogAttachSQL(ct CatalogType, path, lake, data string, override bool) (string, error) {
	ct, err := ResolveCatalogType(ct, path)
	if err != nil {
		return "", err
	}
	if ct == CatalogPostgres {
		path = config.NormalizePostgresDSN(path)
	}
	quote := func(s string) string { return strings.ReplaceAll(s, "'", "''") }
	extra := ""
	if override {
		extra = ", OVERRIDE_DATA_PATH TRUE"
	}
	return fmt.Sprintf("ATTACH 'ducklake:%s:%s' AS \"%s\" (DATA_PATH '%s', AUTOMATIC_MIGRATION TRUE%s);",
		ct, quote(path), strings.ReplaceAll(lake, `"`, `""`), quote(data), extra), nil
}

// suffixedCatalog keeps PostgreSQL shard/volume catalogs independent by using
// databases provisioned by the operator. It never applies filepath operations
// to DSNs, and preserves credentials, TLS settings and other connection options.
func suffixedPostgresCatalog(path, suffix string) (string, error) {
	path = config.NormalizePostgresDSN(path)
	cfg, err := pgx.ParseConfig(config.NormalizePostgresDSN(path))
	if err != nil {
		return "", fmt.Errorf("invalid PostgreSQL DuckLake catalog_path")
	}
	database := cfg.Database + "_" + suffix
	if len(database) > 63 {
		return "", fmt.Errorf("derived PostgreSQL catalog database name exceeds 63 bytes")
	}
	if strings.HasPrefix(path, "postgres://") || strings.HasPrefix(path, "postgresql://") {
		u, err := url.Parse(path)
		if err != nil {
			return "", fmt.Errorf("invalid PostgreSQL DuckLake catalog URL")
		}
		u.Path, u.RawPath = "/"+database, ""
		q := u.Query()
		q.Del("dbname")
		u.RawQuery = q.Encode()
		return u.String(), nil
	}
	value := strings.ReplaceAll(strings.ReplaceAll(database, `\`, `\\`), "'", `\'`)
	return path + " dbname='" + value + "'", nil
}

// CatalogError retains attach diagnostics without credentials from echoed DSNs.
func CatalogError(path, action string, err error) error {
	if IsPostgresCatalogPath(path) {
		path = strings.TrimSpace(path)
		cfg, parseErr := pgx.ParseConfig(config.NormalizePostgresDSN(path))
		if parseErr != nil {
			return fmt.Errorf("%s PostgreSQL DuckLake catalog failed (invalid connection string)", action)
		}
		message := err.Error()
		// DuckDB may echo the original DSN inside a SQL-quoted ATTACH statement.
		for _, dsn := range []string{strings.ReplaceAll(path, "'", "''"), path, strings.ReplaceAll(config.NormalizePostgresDSN(path), "'", "''"), config.NormalizePostgresDSN(path)} {
			message = strings.ReplaceAll(message, dsn, "[redacted DSN]")
		}
		message = strings.ReplaceAll(message, url.UserPassword(cfg.User, cfg.Password).String()+"@", "[redacted]@")
		message = strings.ReplaceAll(message, url.User(cfg.User).String()+"@", "[redacted]@")
		if cfg.Password != "" {
			libpqPassword := strings.ReplaceAll(strings.ReplaceAll(cfg.Password, `\`, `\\`), "'", `\'`)
			for _, password := range []string{
				strings.ReplaceAll(libpqPassword, "'", "''"), libpqPassword,
				strings.ReplaceAll(cfg.Password, "'", "''"),
				url.QueryEscape(cfg.Password), url.PathEscape(cfg.Password), cfg.Password,
			} {
				message = strings.ReplaceAll(message, password, "[redacted]")
			}
		}
		for _, user := range []string{`"` + cfg.User + `"`, "'" + cfg.User + "'", "user=" + cfg.User} {
			message = strings.ReplaceAll(message, user, "[redacted user]")
		}
		return fmt.Errorf("%s PostgreSQL DuckLake catalog (host=%q port=%d database=%q): %s",
			action, cfg.Host, cfg.Port, cfg.Database, message)
	}
	return fmt.Errorf("%s DuckLake catalog: %w", action, err)
}
