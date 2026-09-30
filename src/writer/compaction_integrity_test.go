// Copyright (C) 2026 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package writer

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "github.com/duckdb/duckdb-go/v2"
	_ "modernc.org/sqlite"
)

// newDamagedCatalog builds the malformed rows from sipcapture/homer#1048 in
// columns without type affinity (DuckLake's typed schema would coerce them).
func newDamagedCatalog(t *testing.T, corruptTableID bool) string {
	t.Helper()
	duck, err := sql.Open("duckdb", "")
	if err != nil {
		t.Skipf("duckdb unavailable: %v", err)
	}
	if _, err := duck.Exec("LOAD sqlite;"); err != nil {
		duck.Close()
		t.Skipf("sqlite extension unavailable: %v", err)
	}
	duck.Close()

	path := filepath.Join(t.TempDir(), "catalog.sqlite")
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	tableID := "7"
	if corruptTableID {
		tableID = "'2026-09-30 08:22:04.805824+02'"
	}
	for _, stmt := range []string{
		`CREATE TABLE ducklake_files_scheduled_for_deletion(data_file_id, path, path_is_relative, schedule_start)`,
		`INSERT INTO ducklake_files_scheduled_for_deletion VALUES
			(10, 'main/t/ducklake-a.parquet', 1, '2026-09-24 13:29:40.700542+02'),
			(176701, 25, 173227, '')`,
		`CREATE TABLE ducklake_data_file(data_file_id, table_id, path)`,
		`INSERT INTO ducklake_data_file VALUES (190143, ` + tableID + `, NULL)`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	return path
}

func TestVerifyCatalogIntegrityStopsCompactionOnCorruptTableID(t *testing.T) {
	svc := &CompactionService{lakeName: "lake", catalogPath: newDamagedCatalog(t, true)}
	if svc.verifyCatalogIntegrity() {
		t.Fatal("corrupt table_id not detected")
	}
	if svc.catalogIntegrityOK() {
		t.Fatal("compaction not stopped after catalog damage")
	}
	// A stopped service must return before touching the (nil) database.
	svc.runCompaction()
}

func TestVerifyCatalogIntegrityHealthy(t *testing.T) {
	svc := &CompactionService{lakeName: "lake", catalogPath: newDamagedCatalog(t, false)}
	if !svc.verifyCatalogIntegrity() {
		t.Fatal("healthy catalog reported as damaged")
	}
}

func TestQuarantineScheduledDeletionsUnblocksCleanup(t *testing.T) {
	path := newDamagedCatalog(t, false)
	svc := &CompactionService{lakeName: "lake", catalogPath: path}
	svc.quarantineScheduledDeletions()

	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var bad, left int
	if err := db.QueryRow(`SELECT count(*), sum(typeof(path) != 'text')
		FROM ducklake_files_scheduled_for_deletion`).Scan(&left, &bad); err != nil {
		t.Fatal(err)
	}
	if left != 1 || bad != 0 {
		t.Errorf("queue left=%d malformed=%d, want only the healthy row", left, bad)
	}
}
