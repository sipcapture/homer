// Copyright (C) 2026 Homer Server Contributors
// SPDX-License-Identifier: AGPL-3.0-or-later

package services

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// Exercise the settings bootstrap at its driver boundary. This connection
// accepts only individually executed, schema-qualified PostgreSQL DDL, and
// can fail metadata refresh independently of successful schema creation.
type schemaProtocolConnector struct {
	cacheError     error
	statements     int
	allowLegacyDDL bool
}

func (c *schemaProtocolConnector) Connect(context.Context) (driver.Conn, error) {
	return &schemaProtocolConn{c}, nil
}
func (c *schemaProtocolConnector) Driver() driver.Driver { return &settingsPostgresDriver{} }

type schemaProtocolConn struct{ connector *schemaProtocolConnector }

func (c *schemaProtocolConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("unexpected prepare")
}
func (c *schemaProtocolConn) Close() error { return nil }
func (c *schemaProtocolConn) Begin() (driver.Tx, error) {
	return nil, errors.New("unexpected transaction")
}
func (c *schemaProtocolConn) ExecContext(_ context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	if query == "CALL pg_clear_cache()" {
		return driver.RowsAffected(0), c.connector.cacheError
	}
	if len(args) != 1 {
		return nil, fmt.Errorf("expected one DDL argument")
	}
	if c.connector.allowLegacyDDL {
		c.connector.statements++
		return driver.RowsAffected(0), nil
	}
	ddl, ok := args[0].Value.(string)
	if !ok || strings.Contains(ddl, ";") {
		return nil, errors.New("multiple commands in PostgreSQL statement")
	}
	if !strings.HasPrefix(ddl, "CREATE TABLE IF NOT EXISTS public.") && !strings.HasPrefix(ddl, "CREATE SEQUENCE IF NOT EXISTS public.") && !strings.HasPrefix(ddl, "ALTER TABLE public.") {
		return nil, errors.New("DDL depends on session search_path")
	}
	if strings.Contains(ddl, "nextval('") && !strings.Contains(ddl, "nextval('public.") {
		return nil, errors.New("sequence default depends on search_path")
	}
	c.connector.statements++
	return driver.RowsAffected(0), nil
}
func TestPostgresSettingsSingleStatementDDL(t *testing.T) {
	connector := &schemaProtocolConnector{}
	db := sql.OpenDB(connector)
	defer db.Close()
	if err := EnsureSettingsSchema(db); err != nil {
		t.Fatal(err)
	}
	if connector.statements == 0 {
		t.Fatal("no schema created")
	}
}
func TestPostgresSettingsCacheErrorIsReturned(t *testing.T) {
	missing := errors.New("Catalog Error: pg_clear_cache does not exist")
	connector := &schemaProtocolConnector{cacheError: missing, allowLegacyDDL: true}
	db := sql.OpenDB(connector)
	defer db.Close()
	if err := EnsureSettingsSchema(db); !errors.Is(err, missing) {
		t.Fatalf("lost metadata refresh error: %v", err)
	}
}
