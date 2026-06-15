package ducklake

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func openTestCatalog(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	return db
}

func TestRepairCatalogSnapshots(t *testing.T) {
	path := filepath.Join(t.TempDir(), "catalog.sqlite")
	db := openTestCatalog(t, path)

	stmts := []string{
		`CREATE TABLE ducklake_snapshot (snapshot_id BIGINT, next_file_id BIGINT)`,
		`CREATE TABLE ducklake_table (table_id BIGINT, table_name TEXT)`,
		// snapshot 3 is duplicated (the corruption); 1 and 2 are healthy.
		`INSERT INTO ducklake_snapshot VALUES (1,10),(2,20),(3,30),(3,31)`,
		// table 7 duplicated.
		`INSERT INTO ducklake_table VALUES (5,'a'),(7,'b'),(7,'b')`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("setup %q: %v", s, err)
		}
	}
	db.Close()

	res, err := RepairCatalogSnapshots(path)
	if err != nil {
		t.Fatalf("repair: %v", err)
	}
	if res.DuplicateSnapshots != 1 {
		t.Errorf("DuplicateSnapshots=%d want 1", res.DuplicateSnapshots)
	}
	if res.DuplicateTables != 1 {
		t.Errorf("DuplicateTables=%d want 1", res.DuplicateTables)
	}
	if !res.Changed() {
		t.Errorf("Changed()=false want true")
	}
	if res.LatestSnapshotRows != 1 {
		t.Errorf("LatestSnapshotRows=%d want 1 after repair", res.LatestSnapshotRows)
	}
	if !res.Healthy() {
		t.Errorf("Healthy()=false want true after repair")
	}

	// Verify the kept snapshot row is the last-written one (next_file_id=31).
	db2 := openTestCatalog(t, path)
	var cnt, keptFileID int
	if err := db2.QueryRow(`SELECT COUNT(*) FROM ducklake_snapshot`).Scan(&cnt); err != nil {
		t.Fatal(err)
	}
	if cnt != 3 {
		t.Errorf("snapshot rows=%d want 3", cnt)
	}
	if err := db2.QueryRow(`SELECT next_file_id FROM ducklake_snapshot WHERE snapshot_id=3`).Scan(&keptFileID); err != nil {
		t.Fatal(err)
	}
	if keptFileID != 31 {
		t.Errorf("kept next_file_id=%d want 31 (latest)", keptFileID)
	}
}

// TestRepairCatalogSnapshotsAffinity reproduces the silent-failure case: the
// duplicate latest snapshot_id is stored once as INTEGER and once as TEXT. A
// plain GROUP BY snapshot_id sees two distinct keys and finds no duplicate,
// but DuckLake reads the column as BIGINT and aborts with "multiple snapshots".
// The CAST-normalized dedup must catch and fix it.
func TestRepairCatalogSnapshotsAffinity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "catalog.sqlite")
	db := openTestCatalog(t, path)
	for _, s := range []string{
		`CREATE TABLE ducklake_snapshot (snapshot_id, next_file_id BIGINT)`,
		`INSERT INTO ducklake_snapshot VALUES (1,10),(2,20)`,
		// snapshot 2 duplicated, but stored as TEXT '2' so a raw GROUP BY misses it.
		`INSERT INTO ducklake_snapshot VALUES ('2',21)`,
	} {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("setup %q: %v", s, err)
		}
	}
	db.Close()

	res, err := RepairCatalogSnapshots(path)
	if err != nil {
		t.Fatalf("repair: %v", err)
	}
	if res.DuplicateSnapshots != 1 {
		t.Errorf("DuplicateSnapshots=%d want 1 (affinity duplicate)", res.DuplicateSnapshots)
	}
	if res.LatestSnapshotRows != 1 {
		t.Errorf("LatestSnapshotRows=%d want 1 after repair", res.LatestSnapshotRows)
	}
	if !res.Healthy() {
		t.Errorf("Healthy()=false want true after repair")
	}
}

func TestRepairCatalogSnapshotChanges(t *testing.T) {
	path := filepath.Join(t.TempDir(), "catalog.sqlite")
	db := openTestCatalog(t, path)
	for _, s := range []string{
		`CREATE TABLE ducklake_snapshot (snapshot_id BIGINT, next_file_id BIGINT)`,
		`CREATE TABLE ducklake_snapshot_changes (snapshot_id BIGINT, changes_made TEXT)`,
		`INSERT INTO ducklake_snapshot VALUES (1,10),(2,20)`,
		`INSERT INTO ducklake_snapshot_changes VALUES (1,'a'),(2,'b'),(2,'b')`,
	} {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("setup %q: %v", s, err)
		}
	}
	db.Close()

	res, err := RepairCatalogSnapshots(path)
	if err != nil {
		t.Fatalf("repair: %v", err)
	}
	if res.DuplicateSnapshotChanges != 1 {
		t.Errorf("DuplicateSnapshotChanges=%d want 1", res.DuplicateSnapshotChanges)
	}
	if !res.Changed() {
		t.Errorf("Changed()=false want true")
	}
}

// TestRepairCatalogDuplicateTableNames covers the sipcapture/homer#809 case:
// two writers each registered the same table_name under a different table_id.
// We must DETECT (not auto-delete) it so the operator runs --rebuild-catalog.
func TestRepairCatalogDuplicateTableNames(t *testing.T) {
	path := filepath.Join(t.TempDir(), "catalog.sqlite")
	db := openTestCatalog(t, path)
	for _, s := range []string{
		`CREATE TABLE ducklake_snapshot (snapshot_id BIGINT, next_file_id BIGINT)`,
		`INSERT INTO ducklake_snapshot VALUES (1,10)`,
		`CREATE TABLE ducklake_table (table_id BIGINT, table_name TEXT, end_snapshot BIGINT)`,
		// hep_proto_1_siprec registered twice under ids 313 and 400, both live.
		`INSERT INTO ducklake_table VALUES (313,'hep_proto_1_siprec',NULL),(400,'hep_proto_1_siprec',NULL),(5,'hep_proto_1_call',NULL)`,
	} {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("setup %q: %v", s, err)
		}
	}
	db.Close()

	res, err := RepairCatalogSnapshots(path)
	if err != nil {
		t.Fatalf("repair: %v", err)
	}
	if res.DuplicateTableNames != 1 {
		t.Errorf("DuplicateTableNames=%d want 1", res.DuplicateTableNames)
	}
	if res.DuplicateTables != 0 {
		t.Errorf("DuplicateTables=%d want 0 (ids are distinct, must not auto-delete)", res.DuplicateTables)
	}
	if !res.NeedsRebuild() {
		t.Errorf("NeedsRebuild()=false want true")
	}
	// The duplicate-name rows must still be present (we only detect, never delete).
	db2 := openTestCatalog(t, path)
	var cnt int
	if err := db2.QueryRow(`SELECT COUNT(*) FROM ducklake_table`).Scan(&cnt); err != nil {
		t.Fatal(err)
	}
	if cnt != 3 {
		t.Errorf("ducklake_table rows=%d want 3 (no rows deleted)", cnt)
	}
}

func TestRepairCatalogSnapshotsHealthy(t *testing.T) {
	path := filepath.Join(t.TempDir(), "catalog.sqlite")
	db := openTestCatalog(t, path)
	for _, s := range []string{
		`CREATE TABLE ducklake_snapshot (snapshot_id BIGINT, next_file_id BIGINT)`,
		`INSERT INTO ducklake_snapshot VALUES (1,10),(2,20)`,
	} {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("setup: %v", err)
		}
	}
	db.Close()

	res, err := RepairCatalogSnapshots(path)
	if err != nil {
		t.Fatalf("repair: %v", err)
	}
	if res.Changed() {
		t.Errorf("healthy catalog reported changes: %+v", res)
	}
}

func TestRepairCatalogSnapshotsMissingFile(t *testing.T) {
	res, err := RepairCatalogSnapshots(filepath.Join(t.TempDir(), "does-not-exist.sqlite"))
	if err != nil {
		t.Fatalf("missing file should be no-op, got err: %v", err)
	}
	if res.Changed() {
		t.Errorf("missing file reported changes: %+v", res)
	}
	_ = fmt.Sprint(res) // keep fmt import if test trimmed
}
