// Copyright (C) 2026 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package ducklake

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "github.com/duckdb/duckdb-go/v2"
)

func TestApplyQuerySandboxBlocksOutsideLake(t *testing.T) {
	root := t.TempDir()
	data := filepath.Join(root, "parquet")
	spill := filepath.Join(root, "spill")
	outside := filepath.Join(root, "parquet-old")
	for _, d := range []string{data, spill, outside} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	catalog := filepath.Join(root, "catalog.duckdb")
	secret := filepath.Join(outside, "secret.csv")
	if err := os.WriteFile(secret, []byte("a\nsecret\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	db, err := sql.Open("duckdb", "")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)

	lakeFile := filepath.Join(data, "t.parquet")
	sandboxExec(t, db, "COPY (SELECT range AS a FROM range(10)) TO '"+lakeFile+"'")
	sandboxExec(t, db, "ATTACH '"+catalog+"' AS cat")
	sandboxExec(t, db, "CREATE TABLE cat.x AS SELECT 1 AS a")
	sandboxExec(t, db, "SET temp_directory = '"+spill+"'")

	err = ApplyQuerySandbox(db, QuerySandbox{
		Directories: []string{data, spill, "", "/"},
		Paths:       SandboxCatalogPaths(CatalogSQLite, catalog),
	})
	if err != nil {
		t.Fatalf("ApplyQuerySandbox: %v", err)
	}

	// The lake, the attached catalog and the spill directory still work.
	var n int
	if err := db.QueryRow("SELECT count(*) FROM read_parquet('" + lakeFile + "')").Scan(&n); err != nil || n != 10 {
		t.Fatalf("lake read: n=%d err=%v", n, err)
	}
	sandboxExec(t, db, "INSERT INTO cat.x VALUES (2)")
	sandboxExec(t, db, "COPY (SELECT 1 AS a) TO '"+filepath.Join(data, "ok.parquet")+"'")

	denied := []string{
		"SELECT * FROM read_text('/etc/passwd')",
		"SELECT * FROM read_csv('" + secret + "')",
		"SELECT * FROM read_csv('" + filepath.Join(data, "..", "parquet-old", "secret.csv") + "')",
		"SELECT * FROM '" + secret + "'",
		"SELECT * FROM read_text('file:///etc/passwd')",
		"SELECT * FROM read_text('https://example.com/x')",
		"COPY (SELECT 1) TO '" + filepath.Join(outside, "w.csv") + "'",
		"ATTACH '" + filepath.Join(outside, "o.duckdb") + "' AS o",
		"LOAD '" + filepath.Join(outside, "evil.duckdb_extension") + "'",
		"INSTALL spatial",
		"EXPORT DATABASE '" + filepath.Join(outside, "exp") + "'",
		// The sandbox cannot be undone from SQL.
		"SET enable_external_access = true",
		"RESET enable_external_access",
		"SET allowed_directories = ['/']",
		"SET allowed_paths = ['/etc/passwd']",
		"SET temp_directory = '/'",
		"SET allow_community_extensions = true",
	}
	for _, q := range denied {
		if _, err := db.Exec(q); err == nil {
			t.Errorf("expected %q to be refused", q)
		}
	}

	// The stale-footer workaround still toggles this at runtime (#1061).
	sandboxExec(t, db, "SET enable_external_file_cache = false")
	sandboxExec(t, db, "SET enable_external_file_cache = true")
}

func TestApplyQuerySandboxRejectsUnknownRemotePrefix(t *testing.T) {
	db, err := sql.Open("duckdb", "")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)

	// Without httpfs no file system claims s3://, so DuckDB would expand the
	// prefix against the cwd and lock the volume out.
	if _, err := db.Exec("LOAD httpfs"); err == nil {
		t.Skip("httpfs is loaded; DuckDB accepts s3:// prefixes")
	}
	err = ApplyQuerySandbox(db, QuerySandbox{Directories: []string{"s3://bucket/lake"}})
	if err == nil {
		t.Fatal("expected an error for an s3:// prefix DuckDB cannot resolve")
	}
	var ext string
	if err := db.QueryRow("SELECT current_setting('enable_external_access')::VARCHAR").Scan(&ext); err != nil || ext != "true" {
		t.Fatalf("external access must stay on after a failed sandbox: %q %v", ext, err)
	}
}

func TestSandboxDirectories(t *testing.T) {
	got := sandboxDirectories([]string{"/var/lib/homer/parquet", "/var/lib/homer/parquet/", "/", " ", "s3://b/p", "/a/../b"})
	want := []string{"/b/", "/var/lib/homer/parquet/", "s3://b/p/"}
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v want %v", got, want)
		}
	}
	if SandboxCatalogPaths(CatalogPostgres, "postgres://x") != nil {
		t.Fatal("postgres catalog has no local files")
	}
}

func sandboxExec(t *testing.T, db *sql.DB, q string) {
	t.Helper()
	if _, err := db.Exec(q); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
}
