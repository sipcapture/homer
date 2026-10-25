// Copyright (C) 2026 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package node

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "github.com/duckdb/duckdb-go/v2"
)

func TestMagicBytesPath(t *testing.T) {
	err := stubError("Invalid Input Error: No magic bytes found at end of file '/data/homer/parquet/main/hep_proto_1_call/date=2026-10-08/ducklake-01a11e0a.parquet'")
	path, ok := magicBytesPath(err)
	if !ok {
		t.Fatal("expected path")
	}
	if path != "/data/homer/parquet/main/hep_proto_1_call/date=2026-10-08/ducklake-01a11e0a.parquet" {
		t.Fatalf("path = %q", path)
	}
	if _, ok := magicBytesPath(stubError("other")); ok {
		t.Fatal("expected no path")
	}
}

func TestParquetWorthCacheReset(t *testing.T) {
	if parquetWorthCacheReset("s3://bucket/a.parquet") {
		t.Fatal("remote path is not checked from disk and must not flush the cache")
	}
	dir := t.TempDir()
	bad := filepath.Join(dir, "bad.parquet")
	if err := os.WriteFile(bad, []byte("not a parquet file!!!!"), 0o644); err != nil {
		t.Fatal(err)
	}
	if parquetWorthCacheReset(bad) {
		t.Fatal("corrupt local file must not flush the cache")
	}

	db := openCacheTestDB(t)
	good := filepath.Join(dir, "good.parquet")
	writeParquet(t, db, good)
	if !parquetWorthCacheReset(good) {
		t.Fatal("valid local parquet should be retried")
	}
}

// TestQueryRetriesAfterStaleParquetCache reproduces sipcapture/homer#1061:
// DuckDB 1.5.5 caches a local file by size and mtime-in-seconds. A read of a
// not-yet-finished file stores that view, and a later read of the finished
// file (same inode, size and mtime second) keeps failing until the external
// file cache is dropped.
func TestQueryRetriesAfterStaleParquetCache(t *testing.T) {
	db := openCacheTestDB(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "call.parquet")
	writeParquet(t, db, path)
	good, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(good) < 12 {
		t.Fatalf("parquet too small: %d", len(good))
	}

	if err := os.WriteFile(path, make([]byte, len(good)), 0o644); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	zeroMtime := info.ModTime()

	q := "SELECT count(*) FROM read_parquet('" + strings.ReplaceAll(path, "'", "''") + "')"
	if rows, err := db.Query(q); err == nil || !staleParquetMagic(err) {
		if rows != nil {
			rows.Close()
		}
		t.Fatalf("zeroed file should fail magic check, got %v", err)
	} else if rows != nil {
		rows.Close()
	}

	if err := os.WriteFile(path, good, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, zeroMtime, zeroMtime); err != nil {
		t.Fatal(err)
	}

	rows, rawErr := db.Query(q)
	if rows != nil {
		rows.Close()
	}
	if rawErr == nil {
		t.Fatal("external file cache self-healed; the #1061 reproduction did not stick")
	} else if !staleParquetMagic(rawErr) {
		t.Fatalf("restored file failed differently: %v", rawErr)
	}

	ctx := context.Background()
	rows, err = queryContextRetryingStaleParquet(ctx, db, q)
	if err != nil {
		if rows != nil {
			rows.Close()
		}
		t.Fatalf("retry after cache reset: %v", err)
	}
	defer rows.Close()
	if !rows.Next() {
		t.Fatal("expected a row")
	}
	var n int
	if err := rows.Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("count = %d, want 1", n)
	}
}

// TestResetExternalFileCacheKeepsItOff: tuning turns the cache off on every
// lake instance, and a #1061 recovery must not turn it back on.
func TestResetExternalFileCacheKeepsItOff(t *testing.T) {
	db := openCacheTestDB(t)
	if _, err := db.Exec("SET enable_external_file_cache = false"); err != nil {
		t.Fatal(err)
	}
	if err := resetExternalFileCache(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	var v string
	if err := db.QueryRow("SELECT current_setting('enable_external_file_cache')::VARCHAR").Scan(&v); err != nil {
		t.Fatal(err)
	}
	if v != "false" {
		t.Fatalf("enable_external_file_cache = %q after reset, want false", v)
	}
}

func openCacheTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("duckdb", "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	db.SetMaxOpenConns(1)
	if _, err := db.Exec("SET enable_external_file_cache = true"); err != nil {
		t.Fatal(err)
	}
	return db
}

func writeParquet(t *testing.T, db *sql.DB, path string) {
	t.Helper()
	quoted := strings.ReplaceAll(path, "'", "''")
	_, err := db.Exec("COPY (SELECT 1 AS id) TO '" + quoted + "' (FORMAT PARQUET)")
	if err != nil {
		t.Fatal(err)
	}
}

type stubError string

func (e stubError) Error() string { return string(e) }
