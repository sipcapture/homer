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
