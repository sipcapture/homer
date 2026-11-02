// Copyright (C) 2026 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package services

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	duckdb "github.com/duckdb/duckdb-go/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

const settingsPostgresCatalog = "homer_settings"

// Keep backend configuration on the database's connector, rather than in a
// global registry or every service. The connector still executes SQL with the
// DuckDB driver; no SQL is inspected, intercepted, or retried.
type settingsPostgresDriver struct {
	driver.Driver
	config *pgx.ConnConfig
	closed atomic.Bool
}

type settingsPostgresConnector struct {
	*duckdb.Connector
	backend *settingsPostgresDriver
}

func (c *settingsPostgresConnector) Driver() driver.Driver { return c.backend }

func (c *settingsPostgresConnector) Close() error {
	c.backend.closed.Store(true)
	return c.Connector.Close()
}

func openPostgresSettingsDB(dsn string) (*sql.DB, error) {
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		// Parse errors can contain the DSN and its password.
		return nil, fmt.Errorf("invalid PostgreSQL connection string in coordinator.settings_db_path")
	}
	cfg.RuntimeParams["search_path"] = "public"
	cfg.RuntimeParams["standard_conforming_strings"] = "on"
	if cfg.ConnectTimeout == 0 {
		cfg.ConnectTimeout = 10 * time.Second
	}

	// The attachment belongs to this in-memory DuckDB instance. USE belongs to
	// a session, so initialize the default schema on every pooled connection.
	var once sync.Once
	var attachErr error
	connector, err := duckdb.NewConnector("", func(execer driver.ExecerContext) error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		once.Do(func() {
			if _, err := execer.ExecContext(ctx, "LOAD postgres", nil); err != nil {
				attachErr = fmt.Errorf("load postgres extension (run --install-extensions first): %w", err)
				return
			}
			quoted := "'" + strings.ReplaceAll(dsn, "'", "''") + "'"
			if _, err := execer.ExecContext(ctx,
				"ATTACH "+quoted+" AS "+settingsPostgresCatalog+" (TYPE postgres, SCHEMA 'public')", nil); err != nil {
				// ATTACH errors can echo credentials. Native query errors retain
				// normal diagnostics, but never expose this connection statement.
				attachErr = fmt.Errorf("attach PostgreSQL settings failed; check connection settings and reachability")
			}
		})
		if attachErr != nil {
			return attachErr
		}
		_, err := execer.ExecContext(ctx, "USE "+settingsPostgresCatalog+".public", nil)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("open settings DuckDB client: %w", err)
	}
	db := sql.OpenDB(&settingsPostgresConnector{
		Connector: connector,
		backend:   &settingsPostgresDriver{Driver: connector.Driver(), config: cfg},
	})
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func isPostgresSettings(db *sql.DB) bool {
	if db == nil {
		return false
	}
	_, postgres := db.Driver().(*settingsPostgresDriver)
	return postgres
}

// postgresQueryRow selects native PostgreSQL execution for queries unsupported
// by the DuckDB PostgreSQL extension (including RETURNING). Call only for a
// PostgreSQL-backed settings database. Its signature matches QueryRowContext.
func postgresQueryRow(db *sql.DB) func(context.Context, string, ...any) *sql.Row {
	backend := db.Driver().(*settingsPostgresDriver)
	return func(ctx context.Context, query string, args ...any) *sql.Row {
		native := stdlib.OpenDB(*backend.config.Copy())
		if backend.closed.Load() {
			_ = native.Close()
		}
		// Close the one-query pool after starting the query. database/sql keeps
		// the result's connection alive until Row.Scan releases its rows, then
		// closes that connection because its pool is already closed. Failed
		// queries also release the pool; no idle native connections are kept.
		defer native.Close()
		return native.QueryRowContext(ctx, query, args...)
	}
}
