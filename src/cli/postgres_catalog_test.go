// Copyright (C) 2026 Homer Server Contributors
// SPDX-License-Identifier: AGPL-3.0-or-later
package cli

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/sipcapture/homer-core/src/storage/ducklake"
)

func TestPostgresCatalogCLIReadsInlinedRows(t *testing.T) {
	dsn := os.Getenv("HOMER_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set HOMER_TEST_POSTGRES_DSN for PostgreSQL CLI integration")
	}
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	name := "homer_cli_" + strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		_ = admin.Close(ctx)
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = admin.Exec(ctx, "DROP DATABASE "+name+" WITH (FORCE)"); _ = admin.Close(ctx) })
	pg, _ := pgx.ParseConfig(dsn)
	u := url.URL{Scheme: "postgresql", Host: net.JoinHostPort(pg.Host, strconv.Itoa(int(pg.Port))), User: url.UserPassword(pg.User, pg.Password), Path: "/" + name, RawQuery: "sslmode=disable"}
	cfg := ducklake.Config{CatalogPath: u.String(), LakeName: "lake", DataPath: filepath.Join(t.TempDir(), "data")}
	writer, err := sql.Open("duckdb", "")
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	for _, ext := range []string{"ducklake", "postgres"} {
		if _, err := writer.Exec("LOAD " + ext); err == nil {
			continue
		}
		if _, err := writer.Exec("INSTALL " + ext + " FROM 'https://extensions.duckdb.org'; LOAD " + ext); err != nil {
			t.Fatal(err)
		}
	}
	stmt, err := ducklake.CatalogAttachSQL("", cfg.CatalogPath, cfg.LakeName, cfg.DataPath, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Exec(stmt); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Exec("CREATE TABLE lake.test (id BIGINT); INSERT INTO lake.test VALUES (42)"); err != nil {
		t.Fatal(err)
	}
	quote := func(value string) string {
		return "'" + strings.ReplaceAll(strings.ReplaceAll(value, `\`, `\\`), "'", `\'`) + "'"
	}
	for _, catalogPath := range []string{
		u.String(),
		strings.Replace(u.String(), "postgresql://", "postgres://", 1),
		strings.Replace(u.String(), "postgresql://", "POSTGRES://", 1),
		fmt.Sprintf("host=%s port=%d dbname=%s user=%s password=%s sslmode=disable", quote(pg.Host), pg.Port, quote(name), quote(pg.User), quote(pg.Password)),
	} {
		cfg.CatalogPath = catalogPath
		reader, err := openDuckLakeReadOnly(cfg)
		if err != nil {
			t.Fatal(err)
		}
		defer reader.Close()
		var catalog, schema string
		if err := reader.QueryRow("SELECT current_database(), current_schema()").Scan(&catalog, &schema); err != nil || catalog != "lake" || schema != "main" {
			t.Fatalf("search path: catalog=%q schema=%q error=%v", catalog, schema, err)
		}
		var id int
		if err := reader.QueryRow("SELECT id FROM test").Scan(&id); err != nil || id != 42 {
			t.Fatalf("unqualified read: %d %v", id, err)
		}
		tables := discoverSchemaViews(reader, cfg.LakeName, cfg.CatalogPath)
		if len(tables) != 1 || tables[0] != "lake.test" {
			t.Fatalf("discovered tables = %v, want [lake.test]", tables)
		}
		completer := newSQLCLICompleter(cfg.LakeName, cfg.DataPath, func() []string { return tables })
		line := []rune("SELECT * FROM te")
		matches, _ := completer.Do(line, len(line))
		found := false
		for _, match := range matches {
			found = found || string(match) == "st"
		}
		if !found {
			t.Fatal("discovered table missing from completion")
		}
		if _, err := reader.Exec("INSERT INTO lake.test VALUES (43)"); err == nil {
			t.Fatal("CLI opened a writable PostgreSQL DuckLake catalog")
		}
	}
}
