// Copyright (C) 2026 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package ducklake

import (
	"bufio"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

// newMalformedCatalog reproduces the rows from sipcapture/homer#1048. The
// columns are declared without type affinity: through DuckLake's typed schema
// SQLite would coerce the values, so no ordinary INSERT can produce them.
func newMalformedCatalog(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "catalog.sqlite")
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, stmt := range []string{
		`CREATE TABLE ducklake_files_scheduled_for_deletion(data_file_id, path, path_is_relative, schedule_start)`,
		`INSERT INTO ducklake_files_scheduled_for_deletion VALUES
			(10, 'main/t/ducklake-a.parquet', 1, '2026-09-24 13:29:40.700542+02'),
			(176701, 25, 173227, ''),
			(11, 'main/t/ducklake-b.parquet', 1, '2026-09-24 13:30:00+02')`,
		`CREATE TABLE ducklake_data_file(data_file_id, table_id, path)`,
		`INSERT INTO ducklake_data_file VALUES
			(1, 7, 'main/t/ok.parquet'),
			(190143, '2026-09-30 08:22:04.805824+02', NULL),
			(190144, 'main/t/ducklake-x.parquet', NULL)`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	return path
}

func skipWithoutSQLiteExtension(t *testing.T) {
	t.Helper()
	db, err := sql.Open("duckdb", "")
	if err != nil {
		t.Skipf("duckdb unavailable: %v", err)
	}
	defer db.Close()
	if _, err := db.Exec("LOAD sqlite;"); err != nil {
		t.Skipf("sqlite extension unavailable: %v", err)
	}
}

func TestQuarantineMalformedScheduledDeletions(t *testing.T) {
	skipWithoutSQLiteExtension(t)
	catalog := newMalformedCatalog(t)

	res, err := QuarantineMalformedScheduledDeletions(catalog, nil)
	if err != nil {
		t.Fatalf("quarantine: %v", err)
	}
	if res.Removed != 1 {
		t.Fatalf("removed %d rows, want 1", res.Removed)
	}

	f, err := os.Open(res.File)
	if err != nil {
		t.Fatalf("quarantine file: %v", err)
	}
	defer f.Close()
	var lines []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	if len(lines) != 1 || !strings.Contains(lines[0], `"data_file_id":"176701"`) || !strings.Contains(lines[0], `"path":"25"`) {
		t.Errorf("quarantine file = %q", lines)
	}

	db, err := sql.Open("sqlite", "file:"+catalog)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var left int
	if err := db.QueryRow(`SELECT count(*) FROM ducklake_files_scheduled_for_deletion`).Scan(&left); err != nil {
		t.Fatal(err)
	}
	if left != 2 {
		t.Errorf("%d rows left in the queue, want the 2 healthy ones", left)
	}

	again, err := QuarantineMalformedScheduledDeletions(catalog, nil)
	if err != nil || again.Removed != 0 {
		t.Errorf("second pass: removed=%d err=%v, want nothing to do", again.Removed, err)
	}
}

func TestCountCorruptDataFileTableIDsLive(t *testing.T) {
	skipWithoutSQLiteExtension(t)
	n, err := CountCorruptDataFileTableIDsLive(newMalformedCatalog(t), nil)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("corrupt table_id rows = %d, want 2 (timestamp and path)", n)
	}
}

// TestCatalogScansAcceptRealDuckLakeRows guards against false positives: rows
// DuckLake itself wrote must never be quarantined or reported.
func TestCatalogScansAcceptRealDuckLakeRows(t *testing.T) {
	cfg, writer := openTestWriterDB(t)
	for _, stmt := range []string{
		`CALL lake.set_option('DATA_INLINING_ROW_LIMIT', 0)`,
		`CREATE TABLE lake.t (d DATE, id BIGINT)`,
		`INSERT INTO lake.t VALUES (DATE '2026-09-30', 1)`,
		`INSERT INTO lake.t VALUES (DATE '2026-09-30', 2)`,
		`INSERT INTO lake.t VALUES (DATE '2026-09-30', 3)`,
		`CALL ducklake_merge_adjacent_files('lake', 't', schema => 'main')`,
		`CALL ducklake_expire_snapshots('lake', older_than => NOW()::TIMESTAMPTZ)`,
	} {
		rows, err := writer.Query(stmt)
		if err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
		rows.Close()
	}
	var queued int
	if err := writer.QueryRow(`SELECT count(*) FROM __ducklake_metadata_lake.ducklake_files_scheduled_for_deletion`).Scan(&queued); err != nil {
		t.Fatal(err)
	}
	if queued == 0 {
		t.Fatal("fixture produced no scheduled deletions")
	}

	res, err := QuarantineMalformedScheduledDeletions(cfg.CatalogPath, writer)
	if err != nil {
		t.Fatalf("quarantine: %v", err)
	}
	if res.Removed != 0 {
		t.Errorf("quarantined %d healthy DuckLake rows", res.Removed)
	}
	if n, err := CountCorruptDataFileTableIDsLive(cfg.CatalogPath, writer); err != nil || n != 0 {
		t.Errorf("corrupt table_id rows = %d, err = %v on a healthy catalog", n, err)
	}
	report, err := QuickCheckCatalog(cfg.CatalogPath)
	if errors.Is(err, ErrSQLiteCLIUnavailable) {
		return
	}
	if err != nil || report != "" {
		t.Errorf("quick_check on a healthy catalog: report=%q err=%v", report, err)
	}

	rows, err := writer.Query(`CALL ducklake_cleanup_old_files('lake', cleanup_all => true)`)
	if err != nil {
		t.Fatalf("cleanup after scan: %v", err)
	}
	rows.Close()
}

func TestQuickCheckCatalogReportsDamage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "broken.sqlite")
	if err := os.WriteFile(path, []byte(strings.Repeat("not a database ", 512)), 0o644); err != nil {
		t.Fatal(err)
	}
	report, err := QuickCheckCatalog(path)
	if errors.Is(err, ErrSQLiteCLIUnavailable) {
		t.Skip("sqlite3 CLI not installed")
	}
	if err != nil {
		t.Fatalf("quick_check: %v", err)
	}
	if report == "" {
		t.Error("damaged file reported healthy")
	}
}
