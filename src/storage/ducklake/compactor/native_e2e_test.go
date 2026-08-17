package compactor

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	_ "github.com/duckdb/duckdb-go/v2"
)

// lakeFixture is a real DuckLake catalog with a partitioned table, used to prove
// the compactor against the actual extension rather than a hand-built schema.
type lakeFixture struct {
	db       *sql.DB
	dataPath string
	catalog  string
	lake     string
	table    string
}

// newLakeFixture attaches a fresh DuckLake and creates a partitioned table. It
// skips the test when the ducklake/sqlite extensions are unavailable (offline
// CI), matching repair_e2e_test.go, so the suite stays green without pretending
// to have verified anything.
func newLakeFixture(t *testing.T) *lakeFixture {
	t.Helper()
	return newLakeFixtureConns(t, 1)
}

// newLakeFixtureConns builds a fixture with a given pool size. More than one
// connection is needed to let ingest and compaction actually overlap, the way
// they do on the writer's shared pool.
func newLakeFixtureConns(t *testing.T, conns int) *lakeFixture {
	t.Helper()
	dir := t.TempDir()
	if comp := HiveLikePathComponent(dir); comp != "" {
		t.Skipf("temp dir %q contains a hive-like component %q", dir, comp)
	}
	data := filepath.Join(dir, "data")
	if err := os.MkdirAll(data, 0o755); err != nil {
		t.Fatal(err)
	}
	catalog := filepath.Join(dir, "catalog.sqlite")

	db, err := sql.Open("duckdb", "")
	if err != nil {
		t.Skipf("duckdb unavailable: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	db.SetMaxOpenConns(conns)
	for _, stmt := range []string{"LOAD ducklake;", "LOAD sqlite;"} {
		if _, err := db.Exec(stmt); err != nil {
			t.Skipf("ducklake/sqlite extension unavailable (%q): %v", stmt, err)
		}
	}
	if _, err := db.Exec(fmt.Sprintf(
		"ATTACH 'ducklake:sqlite:%s' AS lake (DATA_PATH '%s');", catalog, data)); err != nil {
		t.Skipf("ATTACH ducklake failed (extension/version mismatch): %v", err)
	}

	f := &lakeFixture{db: db, dataPath: data, catalog: catalog, lake: "lake", table: "calls"}
	// Inlining would keep rows in the catalog where the parquet merge cannot see
	// them; the writer flushes it before compacting, so mirror that here.
	f.mustExec(t, `CALL lake.set_option('data_inlining_row_limit', 0)`)
	f.mustExec(t, `CREATE TABLE lake.calls (d DATE, id BIGINT, method VARCHAR, payload VARCHAR)`)
	f.mustExec(t, `ALTER TABLE lake.calls SET PARTITIONED BY (d)`)
	return f
}

func (f *lakeFixture) mustExec(t *testing.T, q string, args ...any) {
	t.Helper()
	if _, err := f.db.Exec(q, args...); err != nil {
		t.Fatalf("exec %s: %v", q, err)
	}
}

// insertBatch writes one small file per call: each INSERT commits its own snapshot.
func (f *lakeFixture) insertBatch(t *testing.T, date string, from, to int64) {
	t.Helper()
	f.mustExec(t, fmt.Sprintf(
		`INSERT INTO lake.calls
		 SELECT DATE '%s', i, 'INVITE', repeat('x', 100) FROM range(%d, %d) tbl(i)`,
		date, from, to))
}

func (f *lakeFixture) count(t *testing.T) int64 {
	t.Helper()
	var n int64
	if err := f.db.QueryRow(`SELECT COUNT(*) FROM lake.calls`).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	return n
}

func (f *lakeFixture) checksum(t *testing.T) string {
	t.Helper()
	var s sql.NullString
	// Order-independent digest of the full contents.
	if err := f.db.QueryRow(
		`SELECT md5(string_agg(d::VARCHAR || '|' || id::VARCHAR || '|' || method || '|' || payload, ',' ORDER BY id, d))
		   FROM lake.calls`).Scan(&s); err != nil {
		t.Fatalf("checksum: %v", err)
	}
	return s.String
}

func (f *lakeFixture) activeFiles(t *testing.T) int64 {
	t.Helper()
	var n int64
	if err := f.db.QueryRow(
		`SELECT COUNT(*) FROM __ducklake_metadata_lake.ducklake_data_file WHERE end_snapshot IS NULL`).Scan(&n); err != nil {
		t.Fatalf("active files: %v", err)
	}
	return n
}

// assertCatalogHealthy checks the snapshot invariants and additionally requires
// that compaction produced no row-level delete files, since it retires whole
// files. Tests that deliberately create delete files (retention) use
// assertSnapshotsHealthy instead.
func (f *lakeFixture) assertCatalogHealthy(t *testing.T) {
	t.Helper()
	f.assertSnapshotsHealthy(t)
	var deleteFiles int64
	if err := f.db.QueryRow(
		`SELECT COUNT(*) FROM __ducklake_metadata_lake.ducklake_delete_file WHERE end_snapshot IS NULL`).
		Scan(&deleteFiles); err != nil {
		t.Fatalf("delete files: %v", err)
	}
	if deleteFiles != 0 {
		t.Errorf("row-level delete files = %d, want 0 (partition retirement must drop whole files)", deleteFiles)
	}
}

// assertSnapshotsHealthy checks the invariants whose violation produced
// "Corrupt DuckLake - multiple snapshots returned from database".
func (f *lakeFixture) assertSnapshotsHealthy(t *testing.T) {
	t.Helper()
	var latest, dupes int64
	if err := f.db.QueryRow(
		`SELECT COUNT(*) FROM __ducklake_metadata_lake.ducklake_snapshot
		  WHERE snapshot_id = (SELECT MAX(snapshot_id) FROM __ducklake_metadata_lake.ducklake_snapshot)`).
		Scan(&latest); err != nil {
		t.Fatalf("latest snapshot rows: %v", err)
	}
	if latest != 1 {
		t.Errorf("latest snapshot matches %d rows, want 1", latest)
	}
	if err := f.db.QueryRow(
		`SELECT COUNT(*) FROM (SELECT snapshot_id FROM __ducklake_metadata_lake.ducklake_snapshot
		  GROUP BY snapshot_id HAVING COUNT(*) > 1)`).Scan(&dupes); err != nil {
		t.Fatalf("duplicate snapshots: %v", err)
	}
	if dupes != 0 {
		t.Errorf("duplicate snapshot_id groups = %d, want 0", dupes)
	}
}

func (f *lakeFixture) options() Options {
	return Options{
		DB:       f.db,
		LakeName: f.lake,
		DataPath: f.dataPath,
		// Small enough that the fixture's tiny files still get grouped.
		TargetFileSizeBytes: 64 << 10,
	}
}

// TestNativeCompactionPreservesDataAndCatalog is the regression test for the
// failure that destroyed catalogs: after a native cycle the data must be
// byte-identical, the catalog must stay queryable, and the writer must be able to
// commit again. Previously the compactor allocated snapshot ids itself, so the
// next write reused one and DuckLake aborted every query.
func TestNativeCompactionPreservesDataAndCatalog(t *testing.T) {
	f := newLakeFixture(t)

	// Several small files in one partition plus a second partition that must be
	// left completely alone.
	for i := int64(0); i < 6; i++ {
		f.insertBatch(t, "2026-05-30", i*10, i*10+10)
	}
	f.insertBatch(t, "2026-05-31", 1000, 1005)

	rowsBefore := f.count(t)
	sumBefore := f.checksum(t)
	filesBefore := f.activeFiles(t)
	if filesBefore < 7 {
		t.Fatalf("fixture produced %d files, expected at least 7", filesBefore)
	}

	res, err := CompactTable(context.Background(), f.options(), f.table)
	if err != nil {
		t.Fatalf("CompactTable: %v", err)
	}
	if res.Skipped {
		t.Fatalf("compaction skipped: %s", res.SkipReason)
	}
	if res.PartitionsCompacted == 0 {
		t.Fatalf("no partition compacted: %+v", res)
	}
	t.Logf("result: %+v", res)

	if got := f.count(t); got != rowsBefore {
		t.Errorf("row count changed: %d -> %d", rowsBefore, got)
	}
	if got := f.checksum(t); got != sumBefore {
		t.Errorf("table contents changed\n before: %s\n after:  %s", sumBefore, got)
	}
	if got := f.activeFiles(t); got >= filesBefore {
		t.Errorf("active files %d -> %d, expected a reduction", filesBefore, got)
	}
	f.assertCatalogHealthy(t)

	// The partition filter must still work, which requires the registered file to
	// have inherited its partition value.
	var partRows int64
	if err := f.db.QueryRow(`SELECT COUNT(*) FROM lake.calls WHERE d = DATE '2026-05-30'`).Scan(&partRows); err != nil {
		t.Fatalf("partition query: %v", err)
	}
	if partRows != 60 {
		t.Errorf("partition 2026-05-30 has %d rows, want 60", partRows)
	}

	// The regression itself: a subsequent write must commit cleanly.
	f.insertBatch(t, "2026-06-01", 2000, 2003)
	if got := f.count(t); got != rowsBefore+3 {
		t.Errorf("row count after post-compaction insert = %d, want %d", got, rowsBefore+3)
	}
	f.assertCatalogHealthy(t)

	// And a second cycle must be a no-op rather than churn the same files.
	res2, err := CompactTable(context.Background(), f.options(), f.table)
	if err != nil {
		t.Fatalf("second CompactTable: %v", err)
	}
	if got := f.checksum(t); got != f.checksum(t) {
		t.Error("checksum unstable")
	}
	t.Logf("second cycle: %+v", res2)
	f.assertCatalogHealthy(t)
}

// TestSwapRefusesWhenPartitionChanged proves the reconciliation that protects
// against a concurrent flush: if the partition does not hold exactly the rows
// that were merged, the swap must abort and leave the catalog untouched.
func TestSwapRefusesWhenPartitionChanged(t *testing.T) {
	f := newLakeFixture(t)
	f.insertBatch(t, "2026-05-30", 0, 10)
	f.insertBatch(t, "2026-05-30", 10, 20)

	rowsBefore := f.count(t)
	filesBefore := f.activeFiles(t)
	sumBefore := f.checksum(t)

	meta, ok, err := readTableMeta(context.Background(), f.db, f.lake, f.table)
	if err != nil || !ok {
		t.Fatalf("readTableMeta: %v (ok=%v)", err, ok)
	}

	// Claim the merged output holds one row fewer than the partition really has.
	err = swapPartition(context.Background(), f.db, f.lake, swapRequest{
		tableName:    f.table,
		partition:    meta.partition,
		partitionVal: "2026-05-30",
		expectedRows: rowsBefore - 1,
		mergedAbs:    []string{filepath.Join(f.dataPath, "main", "calls", "d=2026-05-30", "nonexistent.parquet")},
	})
	if err == nil {
		t.Fatal("swap succeeded despite a row-count mismatch")
	}
	t.Logf("swap correctly refused: %v", err)

	if got := f.count(t); got != rowsBefore {
		t.Errorf("row count changed after refused swap: %d -> %d", rowsBefore, got)
	}
	if got := f.activeFiles(t); got != filesBefore {
		t.Errorf("active files changed after refused swap: %d -> %d", filesBefore, got)
	}
	if got := f.checksum(t); got != sumBefore {
		t.Error("contents changed after refused swap")
	}
	f.assertCatalogHealthy(t)
}

// TestCompactionSkipsInlinedData guards the subtler loss: inlined rows live in
// the catalog, so the parquet merge cannot see them, but the retirement DELETE
// would remove them.
func TestCompactionSkipsInlinedData(t *testing.T) {
	f := newLakeFixture(t)

	// With inlining disabled the fixture must report a clean lake.
	inlined, err := hasInlinedData(context.Background(), f.db, f.lake)
	if err != nil {
		t.Fatalf("hasInlinedData: %v", err)
	}
	if inlined {
		t.Fatal("fixture reports inlined rows despite inlining being disabled")
	}

	f.insertBatch(t, "2026-05-30", 0, 10)
	f.insertBatch(t, "2026-05-30", 10, 20)
	f.mustExec(t, `CALL lake.set_option('data_inlining_row_limit', 1000)`)
	f.insertBatch(t, "2026-05-30", 20, 25)

	inlined, err = hasInlinedData(context.Background(), f.db, f.lake)
	if err != nil {
		t.Fatalf("hasInlinedData: %v", err)
	}
	if !inlined {
		t.Skip("this DuckLake build did not inline the rows")
	}

	rowsBefore := f.count(t)
	res, err := CompactTable(context.Background(), f.options(), f.table)
	if err != nil {
		t.Fatalf("CompactTable: %v", err)
	}
	if !res.Skipped {
		t.Fatalf("expected the table to be skipped while rows are inlined, got %+v", res)
	}
	if got := f.count(t); got != rowsBefore {
		t.Errorf("row count changed: %d -> %d", rowsBefore, got)
	}
	t.Logf("skipped as expected: %s", res.SkipReason)
}

// TestCompactionSkipsTableWithRowLevelDeletes guards the case the old engine got
// wrong: a partition-wide retirement cannot preserve row-level deletes.
func TestCompactionSkipsTableWithRowLevelDeletes(t *testing.T) {
	f := newLakeFixture(t)
	f.insertBatch(t, "2026-05-30", 0, 10)
	f.insertBatch(t, "2026-05-30", 10, 20)
	// Delete a subset of one file so DuckLake records a row-level delete file.
	f.mustExec(t, `DELETE FROM lake.calls WHERE id = 3`)

	var deleteFiles int64
	if err := f.db.QueryRow(
		`SELECT COUNT(*) FROM __ducklake_metadata_lake.ducklake_delete_file WHERE end_snapshot IS NULL`).
		Scan(&deleteFiles); err != nil {
		t.Fatal(err)
	}
	if deleteFiles == 0 {
		t.Skip("this DuckLake build did not create a row-level delete file")
	}

	res, err := CompactTable(context.Background(), f.options(), f.table)
	if err != nil {
		t.Fatalf("CompactTable: %v", err)
	}
	if !res.Skipped {
		t.Fatalf("expected the table to be skipped, got %+v", res)
	}
	t.Logf("skipped as expected: %s", res.SkipReason)
}
