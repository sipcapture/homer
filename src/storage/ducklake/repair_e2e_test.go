// Copyright (C) 2026 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

//go:build !vet
// +build !vet

package ducklake

import (
	"database/sql"
	"database/sql/driver"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	duckdb "github.com/duckdb/duckdb-go/v2"
	_ "modernc.org/sqlite"
)

// openDuckLake opens a real DuckDB connection and ATTACHes the DuckLake catalog
// exactly like the writer does. It skips the test if the ducklake/sqlite DuckDB
// extensions cannot be loaded (no network / offline CI), so the suite stays
// green in restricted environments while still giving real end-to-end proof
// wherever the extensions are available.
func openDuckLake(t *testing.T, catalogPath, dataPath string) *sql.DB {
	t.Helper()
	connector, err := duckdb.NewConnector("", func(execer driver.ExecerContext) error {
		return nil
	})
	if err != nil {
		t.Skipf("duckdb unavailable: %v", err)
	}
	db := sql.OpenDB(connector)
	db.SetMaxOpenConns(1)

	for _, stmt := range []string{"INSTALL ducklake;", "LOAD ducklake;", "INSTALL sqlite;", "LOAD sqlite;"} {
		if _, err := db.Exec(stmt); err != nil {
			db.Close()
			t.Skipf("ducklake/sqlite extensions unavailable (%q): %v", stmt, err)
		}
	}
	attach := fmt.Sprintf(
		"ATTACH 'ducklake:sqlite:%s' AS lake (DATA_PATH '%s', AUTOMATIC_MIGRATION TRUE);",
		catalogPath, dataPath)
	if _, err := db.Exec(attach); err != nil {
		db.Close()
		t.Skipf("ATTACH ducklake failed (extension/version mismatch): %v", err)
	}
	return db
}

// TestRepairFixesRealDuckLakeCorruption is the end-to-end proof that the startup
// auto-repair makes a catalog readable again after the sipcapture/homer#809
// "multiple snapshots returned" corruption:
//
//  1. build a real DuckLake catalog and write data (several snapshots),
//  2. inject the corruption a racing second writer would (a duplicate row with
//     the current MAX snapshot_id),
//  3. confirm DuckDB itself now aborts every query with the fatal error,
//  4. run RepairCatalogSnapshots (what the writer runs on startup),
//  5. confirm DuckDB can query the catalog again.
func TestRepairFixesRealDuckLakeCorruption(t *testing.T) {
	dir := t.TempDir()
	catalog := filepath.Join(dir, "catalog.sqlite")
	data := filepath.Join(dir, "data")
	if err := os.MkdirAll(data, 0o755); err != nil {
		t.Fatal(err)
	}

	// 1. Build a real catalog with a few snapshots.
	db := openDuckLake(t, catalog, data)
	mustExec(t, db, `CREATE TABLE lake.main.t (id INTEGER, name VARCHAR);`)
	mustExec(t, db, `INSERT INTO lake.main.t VALUES (1,'a');`)
	mustExec(t, db, `INSERT INTO lake.main.t VALUES (2,'b');`)
	mustExec(t, db, `INSERT INTO lake.main.t VALUES (3,'c');`)
	var got int
	if err := db.QueryRow(`SELECT COUNT(*) FROM lake.main.t`).Scan(&got); err != nil {
		t.Fatalf("baseline query: %v", err)
	}
	if got != 3 {
		t.Fatalf("baseline rows=%d want 3", got)
	}
	mustExec(t, db, `DETACH lake;`)
	db.Close()

	// 2. Inject the corruption: duplicate the latest snapshot row, exactly what
	//    a second concurrent writer committing the same snapshot_id produces.
	injectDuplicateLatestSnapshot(t, catalog)

	// 3. Prove a real DuckDB+DuckLake now aborts with the #809 error.
	if err := tryDuckLakeQuery(t, catalog, data); err == nil {
		t.Fatalf("expected corrupt catalog to fail the latest-snapshot probe, but query succeeded")
	} else if !strings.Contains(err.Error(), "multiple snapshots") {
		t.Fatalf("expected 'multiple snapshots' error, got: %v", err)
	}

	// 4. Run the startup auto-repair.
	res, err := RepairCatalogSnapshots(catalog)
	if err != nil {
		t.Fatalf("RepairCatalogSnapshots: %v", err)
	}
	if res.DuplicateSnapshots < 1 {
		t.Fatalf("repair removed %d duplicate snapshots, want >=1", res.DuplicateSnapshots)
	}
	if !res.Healthy() {
		t.Fatalf("repair left catalog unhealthy: latest_snapshot_rows=%d", res.LatestSnapshotRows)
	}

	// 5. Prove DuckDB can read the catalog again, with all data intact.
	if err := tryDuckLakeQuery(t, catalog, data); err != nil {
		t.Fatalf("catalog still broken after repair: %v", err)
	}
	db2 := openDuckLake(t, catalog, data)
	defer db2.Close()
	if err := db2.QueryRow(`SELECT COUNT(*) FROM lake.main.t`).Scan(&got); err != nil {
		t.Fatalf("post-repair query: %v", err)
	}
	if got != 3 {
		t.Fatalf("post-repair rows=%d want 3 (data must be preserved)", got)
	}
}

func mustExec(t *testing.T, db *sql.DB, q string) {
	t.Helper()
	if _, err := db.Exec(q); err != nil {
		t.Fatalf("exec %q: %v", q, err)
	}
}

// tryDuckLakeQuery attaches the catalog and runs a query that forces DuckLake to
// resolve the latest snapshot; returns the error (nil on success). It must use a
// fresh connection so DuckLake re-reads the catalog rather than a cached state.
func tryDuckLakeQuery(t *testing.T, catalogPath, dataPath string) error {
	t.Helper()
	connector, err := duckdb.NewConnector("", func(execer driver.ExecerContext) error { return nil })
	if err != nil {
		t.Skipf("duckdb unavailable: %v", err)
	}
	db := sql.OpenDB(connector)
	db.SetMaxOpenConns(1)
	defer db.Close()
	for _, stmt := range []string{"LOAD ducklake;", "LOAD sqlite;"} {
		if _, err := db.Exec(stmt); err != nil {
			t.Skipf("extensions unavailable: %v", err)
		}
	}
	if _, err := db.Exec(fmt.Sprintf(
		"ATTACH 'ducklake:sqlite:%s' AS lake (DATA_PATH '%s', AUTOMATIC_MIGRATION TRUE);",
		catalogPath, dataPath)); err != nil {
		// A corrupt catalog can fail at ATTACH time too; that's still the bug.
		return err
	}
	var n int
	return db.QueryRow(`SELECT COUNT(*) FROM lake.main.t`).Scan(&n)
}

// injectDuplicateLatestSnapshot reproduces the state a racing second writer (or
// catalog corruption) leaves behind: two ducklake_snapshot rows sharing the
// current MAX snapshot_id.
//
// Current DuckLake creates ducklake_snapshot with `snapshot_id BIGINT PRIMARY
// KEY`, so SQLite's UNIQUE index normally makes a literal duplicate impossible.
// We therefore first drop that uniqueness (rebuild the table without the PK) to
// emulate a catalog whose index no longer enforces uniqueness — e.g. a catalog
// created by an older DuckLake, or one whose index pages were corrupted by
// concurrent writers — and then insert the duplicate. This is exactly the
// physical situation DuckLake's "multiple snapshots returned" probe trips on.
func injectDuplicateLatestSnapshot(t *testing.T, catalogPath string) {
	t.Helper()
	sdb, err := sql.Open("sqlite", "file:"+catalogPath+"?_pragma=foreign_keys(0)")
	if err != nil {
		t.Fatalf("open catalog for injection: %v", err)
	}
	sdb.SetMaxOpenConns(1)
	defer sdb.Close()

	// Drop the UNIQUE-enforcing PRIMARY KEY by rebuilding the table without it.
	stmts := []string{
		`ALTER TABLE ducklake_snapshot RENAME TO _ds_old`,
		`CREATE TABLE ducklake_snapshot (snapshot_id BIGINT, snapshot_time VARCHAR, schema_version BIGINT, next_catalog_id BIGINT, next_file_id BIGINT)`,
		`INSERT INTO ducklake_snapshot SELECT snapshot_id, snapshot_time, schema_version, next_catalog_id, next_file_id FROM _ds_old`,
		`DROP TABLE _ds_old`,
		// The duplicate latest-snapshot row.
		`INSERT INTO ducklake_snapshot (snapshot_id, snapshot_time, schema_version, next_catalog_id, next_file_id)
		 SELECT snapshot_id, snapshot_time, schema_version, next_catalog_id, next_file_id
		 FROM ducklake_snapshot WHERE snapshot_id = (SELECT MAX(snapshot_id) FROM ducklake_snapshot)`,
	}
	for _, s := range stmts {
		if _, err := sdb.Exec(s); err != nil {
			t.Fatalf("inject duplicate snapshot %q: %v", s, err)
		}
	}
}
