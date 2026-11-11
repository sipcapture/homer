// Copyright (C) 2026 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package node

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	_ "github.com/duckdb/duckdb-go/v2"
	"github.com/sipcapture/homer-core/src/config"
)

// TestNodeDuckDBSandboxKeepsLakeReadable opens the node DuckDB the way New and
// refreshCatalog do, against a real DuckLake, and checks that search still
// reads the lake while host files are refused.
func TestNodeDuckDBSandboxKeepsLakeReadable(t *testing.T) {
	dir := t.TempDir()
	data := filepath.Join(dir, "parquet") + "/"
	if err := os.MkdirAll(data, 0o755); err != nil {
		t.Fatal(err)
	}
	catalog := filepath.Join(dir, "catalog.sqlite")
	secret := filepath.Join(dir, "secret.csv")
	if err := os.WriteFile(secret, []byte("a\nsecret\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Writer stand-in: create the lake with one table and some rows.
	w, err := sql.Open("duckdb", "")
	if err != nil {
		t.Skipf("duckdb unavailable: %v", err)
	}
	w.SetMaxOpenConns(1)
	for _, stmt := range []string{"LOAD ducklake;", "LOAD sqlite;"} {
		if _, err := w.Exec(stmt); err != nil {
			w.Close()
			t.Skipf("extension unavailable (%q): %v", stmt, err)
		}
	}
	for _, q := range []string{
		fmt.Sprintf("ATTACH 'ducklake:sqlite:%s' AS homer_lake (DATA_PATH '%s');", catalog, data),
		"CREATE TABLE homer_lake.main.hep_proto_1_call AS SELECT range AS id FROM range(5);",
		"DETACH homer_lake;",
	} {
		if _, err := w.Exec(q); err != nil {
			w.Close()
			t.Skipf("writer setup %q: %v", q, err)
		}
	}
	w.Close()

	on := true
	cfg := &config.NodeConfig{
		DuckDBSandbox: &on,
		DuckLake: config.DuckLakeConfig{
			LakeName: "homer_lake",
			Tuning:   config.DuckDBTuning{TempDirectory: filepath.Join(dir, "spill")},
			Volumes: []config.VolumeConfig{{
				Name: "default", Type: "local", Path: data,
				CatalogType: "sqlite", CatalogPath: catalog,
			}},
		},
	}
	db, vols, err := openNodeDuckDB(cfg)
	if err != nil {
		t.Skipf("node DuckDB unavailable: %v", err)
	}
	defer db.Close()
	if len(vols) != 1 {
		t.Fatalf("volumes = %+v", vols)
	}

	var ext string
	if err := db.QueryRow("SELECT current_setting('enable_external_access')::VARCHAR").Scan(&ext); err != nil {
		t.Fatal(err)
	}
	if ext != "false" {
		t.Fatalf("sandbox fell back: enable_external_access = %q", ext)
	}

	var n int
	if err := db.QueryRow("SELECT count(*) FROM homer_lake.main.hep_proto_1_call").Scan(&n); err != nil || n != 5 {
		t.Fatalf("lake read: n=%d err=%v", n, err)
	}
	for _, q := range []string{
		"SELECT * FROM read_text('/etc/passwd')",
		"SELECT * FROM read_csv('" + secret + "')",
		"SELECT * FROM '" + secret + "'",
		"ATTACH '" + filepath.Join(dir, "other.duckdb") + "' AS o",
		"COPY (SELECT 1) TO '" + filepath.Join(dir, "x.csv") + "'",
	} {
		if _, err := db.Exec(q); err == nil {
			t.Errorf("expected %q to be refused", q)
		}
	}

	// Opting out leaves external access on.
	off := false
	cfg.DuckDBSandbox = &off
	db2, _, err := openNodeDuckDB(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer db2.Close()
	if err := db2.QueryRow("SELECT current_setting('enable_external_access')::VARCHAR").Scan(&ext); err != nil || ext != "true" {
		t.Fatalf("opt-out: enable_external_access = %q err=%v", ext, err)
	}
}
