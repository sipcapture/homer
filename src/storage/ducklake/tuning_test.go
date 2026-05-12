// Copyright (C) 2026 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package ducklake

import (
	"database/sql"
	"strings"
	"testing"

	_ "github.com/duckdb/duckdb-go/v2"
)

// TestApplyDuckDBTuning_AllSet opens a fresh in-memory DuckDB, applies
// every tuning knob, and asserts each setting takes effect via
// current_setting().
func TestApplyDuckDBTuning_AllSet(t *testing.T) {
	db, err := sql.Open("duckdb", "")
	if err != nil {
		t.Fatalf("open duckdb: %v", err)
	}
	defer db.Close()

	tmp := t.TempDir()
	ApplyDuckDBTuning(db, 2, "256MB", tmp, "test")

	if got := getSetting(t, db, "threads"); got != "2" {
		t.Fatalf("threads = %q, want %q", got, "2")
	}
	if got := getSetting(t, db, "memory_limit"); got == "" {
		t.Fatalf("memory_limit = empty, want non-empty after SET")
	}
	if got := getSetting(t, db, "temp_directory"); !strings.Contains(got, tmp) {
		t.Fatalf("temp_directory = %q, want path containing %q", got, tmp)
	}
}

// TestApplyDuckDBTuning_NoOpsLeaveDefaults verifies that empty values
// do not touch the corresponding setting, so callers can pass whatever
// the operator left zero-valued in JSON without surprises.
func TestApplyDuckDBTuning_NoOpsLeaveDefaults(t *testing.T) {
	db, err := sql.Open("duckdb", "")
	if err != nil {
		t.Fatalf("open duckdb: %v", err)
	}
	defer db.Close()

	before := getSetting(t, db, "threads")
	ApplyDuckDBTuning(db, 0, "", "", "test")
	after := getSetting(t, db, "threads")
	if before != after {
		t.Fatalf("threads changed from %q to %q despite zero input", before, after)
	}
}

// TestApplyDuckDBTuning_BadMemoryLimitDoesNotPanic asserts that a
// bogus memory_limit string only logs a warning and does not abort
// the connection bring-up.
func TestApplyDuckDBTuning_BadMemoryLimitDoesNotPanic(t *testing.T) {
	db, err := sql.Open("duckdb", "")
	if err != nil {
		t.Fatalf("open duckdb: %v", err)
	}
	defer db.Close()

	ApplyDuckDBTuning(db, 0, "8 angstrom", "", "test")
	// Connection must still be usable after a bad SET.
	if _, err := db.Exec("SELECT 1"); err != nil {
		t.Fatalf("connection broken after bad memory_limit: %v", err)
	}
}

// TestApplyDuckDBTuning_NilDB is a defensive smoke: passing a nil *sql.DB
// must be a silent no-op, not a panic.
func TestApplyDuckDBTuning_NilDB(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("ApplyDuckDBTuning(nil) panicked: %v", r)
		}
	}()
	ApplyDuckDBTuning(nil, 4, "1GB", "/tmp", "test")
}

func TestEnsureWriterS3Secret_MinIOStyle(t *testing.T) {
	db, err := sql.Open("duckdb", "")
	if err != nil {
		t.Fatalf("open duckdb: %v", err)
	}
	defer db.Close()

	if err := EnsureWriterS3Secret(db, "us-east-1", "k", "s", "http://127.0.0.1:9000", false); err != nil {
		t.Fatalf("EnsureWriterS3Secret: %v", err)
	}
	if _, err := db.Exec("SELECT 1"); err != nil {
		t.Fatalf("db after secret: %v", err)
	}
}

func TestApplyDuckDBS3ClientSettings_MinIOStyleEndpoint(t *testing.T) {
	db, err := sql.Open("duckdb", "")
	if err != nil {
		t.Fatalf("open duckdb: %v", err)
	}
	defer db.Close()

	if err := ApplyDuckDBS3ClientSettings(db, "us-east-1", "testkey", "testsecret", "http://127.0.0.1:9000", false); err != nil {
		t.Fatalf("ApplyDuckDBS3ClientSettings: %v", err)
	}
	if got := getSetting(t, db, "s3_url_style"); got != "path" {
		t.Fatalf("s3_url_style = %q, want path", got)
	}
}

func getSetting(t *testing.T, db *sql.DB, name string) string {
	t.Helper()
	var v string
	if err := db.QueryRow("SELECT current_setting('" + name + "')").Scan(&v); err != nil {
		t.Fatalf("current_setting(%q): %v", name, err)
	}
	return strings.TrimSpace(v)
}
