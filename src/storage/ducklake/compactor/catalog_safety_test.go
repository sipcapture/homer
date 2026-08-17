package compactor

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	_ "github.com/duckdb/duckdb-go/v2"
)

// TestSwapDefersWhenFlushLandsInSamePartition is the production race that the
// row-count reconciliation exists for: the merge runs without the catalog lock,
// so a writer flush can commit new files into the partition between planning and
// the swap. Retiring the partition then would drop rows that are not in the
// merged output.
//
// The catalog lock is acquired twice per single-partition run: first for planning,
// then for the swap. Hooking the second acquisition commits a flush exactly in the
// window the reconciliation guards — after the file list was read, before the
// partition is retired.
func TestSwapDefersWhenFlushLandsInSamePartition(t *testing.T) {
	f := newLakeFixture(t)
	for i := int64(0); i < 6; i++ {
		f.insertBatch(t, "2026-05-30", i*10, i*10+10)
	}
	rowsBefore := f.count(t)

	opts := f.options()
	opts.Lock = flushOnSwapHook(t, f, "2026-05-30", 900, 905)

	res, err := CompactTable(context.Background(), opts, f.table)
	if err != nil {
		t.Fatalf("CompactTable: %v", err)
	}
	if res.PartitionsDeferred == 0 {
		t.Errorf("expected the partition to be deferred, got %+v", res)
	}
	if res.PartitionsCompacted != 0 {
		t.Errorf("partition was compacted despite a concurrent flush: %+v", res)
	}

	// Nothing may be lost: the pre-existing rows plus the ones the flush added.
	if got, want := f.count(t), rowsBefore+5; got != want {
		t.Errorf("row count = %d, want %d", got, want)
	}
	f.assertCatalogHealthy(t)

	// The next cycle must pick the partition up cleanly now that it is quiet.
	res2, err := CompactTable(context.Background(), f.options(), f.table)
	if err != nil {
		t.Fatalf("second CompactTable: %v", err)
	}
	if res2.PartitionsCompacted != 1 {
		t.Errorf("deferred partition was not compacted on the next cycle: %+v", res2)
	}
	if got, want := f.count(t), rowsBefore+5; got != want {
		t.Errorf("row count after retry = %d, want %d", got, want)
	}
	f.assertCatalogHealthy(t)
}

// TestSwapProceedsWhenFlushLandsInOtherPartition guards the opposite mistake:
// the reconciliation must not be so blunt that any concurrent write anywhere
// blocks compaction forever.
func TestSwapProceedsWhenFlushLandsInOtherPartition(t *testing.T) {
	f := newLakeFixture(t)
	for i := int64(0); i < 6; i++ {
		f.insertBatch(t, "2026-05-30", i*10, i*10+10)
	}
	rowsBefore := f.count(t)

	opts := f.options()
	opts.Lock = flushOnSwapHook(t, f, "2026-06-15", 900, 905)

	res, err := CompactTable(context.Background(), opts, f.table)
	if err != nil {
		t.Fatalf("CompactTable: %v", err)
	}
	if res.PartitionsCompacted != 1 {
		t.Errorf("expected the quiet partition to be compacted, got %+v", res)
	}
	if got, want := f.count(t), rowsBefore+5; got != want {
		t.Errorf("row count = %d, want %d", got, want)
	}
	f.assertCatalogHealthy(t)
}

// TestMaintenanceAfterNativeKeepsRegisteredFiles is the check the new design
// depends on. The compactor no longer deletes parquet itself; it hands that to
// the standard expire/cleanup calls. Those must remove the *retired* files while
// leaving the freshly registered merged file alone — deleting it would leave the
// catalog pointing at a missing file, which is exactly the "ghost entry" failure
// operators hit after a crash.
func TestMaintenanceAfterNativeKeepsRegisteredFiles(t *testing.T) {
	f := newLakeFixture(t)
	for i := int64(0); i < 6; i++ {
		f.insertBatch(t, "2026-05-30", i*10, i*10+10)
	}
	rowsBefore := f.count(t)
	sumBefore := f.checksum(t)

	res, err := CompactTable(context.Background(), f.options(), f.table)
	if err != nil {
		t.Fatalf("CompactTable: %v", err)
	}
	if res.PartitionsCompacted == 0 {
		t.Fatalf("nothing compacted: %+v", res)
	}

	filesOnDiskBefore := countParquetOnDisk(t, f.dataPath)

	// The aggressive end of the range: expire everything, then clean up. This is
	// what a node with a short snapshot_expire_interval_sec does.
	for _, stmt := range []string{
		`CALL ducklake_expire_snapshots('lake', older_than => NOW())`,
		`CALL ducklake_cleanup_old_files('lake', cleanup_all => true)`,
		`CALL ducklake_delete_orphaned_files('lake', cleanup_all => true)`,
	} {
		if _, err := f.db.Exec(stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}

	// Superseded parquet must actually be gone, otherwise compaction reclaims
	// nothing and the disk grows forever.
	filesOnDiskAfter := countParquetOnDisk(t, f.dataPath)
	if filesOnDiskAfter >= filesOnDiskBefore {
		t.Errorf("parquet on disk %d -> %d, expected retired files to be removed",
			filesOnDiskBefore, filesOnDiskAfter)
	}

	// And the data must still be fully readable: every active catalog entry has
	// to point at a file that still exists.
	if got := f.count(t); got != rowsBefore {
		t.Errorf("row count after maintenance = %d, want %d", got, rowsBefore)
	}
	if got := f.checksum(t); got != sumBefore {
		t.Errorf("contents changed after maintenance\n before: %s\n after:  %s", sumBefore, got)
	}
	assertEveryActiveFileExists(t, f)
	f.assertCatalogHealthy(t)

	// Writing must still work afterwards.
	f.insertBatch(t, "2026-06-01", 500, 503)
	if got := f.count(t); got != rowsBefore+3 {
		t.Errorf("row count after post-maintenance insert = %d, want %d", got, rowsBefore+3)
	}
}

// TestCatalogReadableAfterReattach reproduces the restart path. The corruption
// this design removes made the catalog unreadable for the *next* process, so
// verifying on the same connection is not enough: the catalog file itself must be
// sound after a native cycle plus full maintenance.
func TestCatalogReadableAfterReattach(t *testing.T) {
	f := newLakeFixture(t)
	for i := int64(0); i < 6; i++ {
		f.insertBatch(t, "2026-05-30", i*10, i*10+10)
	}
	f.insertBatch(t, "2026-05-31", 500, 505)
	rowsBefore := f.count(t)
	sumBefore := f.checksum(t)

	if _, err := CompactTable(context.Background(), f.options(), f.table); err != nil {
		t.Fatalf("CompactTable: %v", err)
	}
	for _, stmt := range []string{
		`CALL ducklake_expire_snapshots('lake', older_than => NOW())`,
		`CALL ducklake_cleanup_old_files('lake', cleanup_all => true)`,
	} {
		if _, err := f.db.Exec(stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}

	// Drop the writer entirely, as a restart would.
	if err := f.db.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}

	fresh, err := sql.Open("duckdb", "")
	if err != nil {
		t.Fatalf("open fresh duckdb: %v", err)
	}
	defer fresh.Close()
	fresh.SetMaxOpenConns(1)
	for _, stmt := range []string{"LOAD ducklake;", "LOAD sqlite;"} {
		if _, err := fresh.Exec(stmt); err != nil {
			t.Fatalf("load %s: %v", stmt, err)
		}
	}
	// A failure here is the original bug: "Corrupt DuckLake - multiple snapshots
	// returned from database" surfaced on ATTACH or on the first query.
	if _, err := fresh.Exec(fmt.Sprintf(
		"ATTACH 'ducklake:sqlite:%s' AS lake (DATA_PATH '%s');", f.catalog, f.dataPath)); err != nil {
		t.Fatalf("re-ATTACH after native cycle failed: %v", err)
	}

	var rowsAfter int64
	if err := fresh.QueryRow(`SELECT COUNT(*) FROM lake.calls`).Scan(&rowsAfter); err != nil {
		t.Fatalf("query after re-attach: %v", err)
	}
	if rowsAfter != rowsBefore {
		t.Errorf("row count after re-attach = %d, want %d", rowsAfter, rowsBefore)
	}

	var sumAfter sql.NullString
	if err := fresh.QueryRow(
		`SELECT md5(string_agg(d::VARCHAR || '|' || id::VARCHAR || '|' || method || '|' || payload, ',' ORDER BY id, d))
		   FROM lake.calls`).Scan(&sumAfter); err != nil {
		t.Fatalf("checksum after re-attach: %v", err)
	}
	if sumAfter.String != sumBefore {
		t.Errorf("contents differ after re-attach\n before: %s\n after:  %s", sumBefore, sumAfter.String)
	}

	// The restarted process must be able to write, which is where a stale
	// snapshot id would have blown up.
	if _, err := fresh.Exec(
		`INSERT INTO lake.calls VALUES (DATE '2026-06-02', 777, 'BYE', 'z')`); err != nil {
		t.Fatalf("insert after re-attach: %v", err)
	}
	if err := fresh.QueryRow(`SELECT COUNT(*) FROM lake.calls`).Scan(&rowsAfter); err != nil {
		t.Fatal(err)
	}
	if rowsAfter != rowsBefore+1 {
		t.Errorf("row count after re-attach insert = %d, want %d", rowsAfter, rowsBefore+1)
	}
}

// TestNativeCompactionUnderConcurrentIngest runs compaction cycles while ingest
// keeps committing, serialized by a real mutex the way the writer's CatalogLock
// does. It asserts the two things that matter: not a single row is lost or
// duplicated, and the catalog never develops the duplicate-snapshot corruption.
func TestNativeCompactionUnderConcurrentIngest(t *testing.T) {
	f := newLakeFixtureConns(t, 4)

	const (
		batches   = 20
		rowsEach  = 10
		wantTotal = batches * rowsEach
	)

	var catalogMu sync.Mutex
	opts := f.options()
	opts.Lock = catalogMu.Lock
	opts.Unlock = catalogMu.Unlock

	ingestDone := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(ingestDone)
		for i := 0; i < batches; i++ {
			// The writer flush holds the same lock the swap does.
			catalogMu.Lock()
			_, err := f.db.Exec(fmt.Sprintf(
				`INSERT INTO lake.calls
				 SELECT DATE '2026-05-30', i, 'INVITE', repeat('x', 100) FROM range(%d, %d) tbl(i)`,
				i*rowsEach, i*rowsEach+rowsEach))
			catalogMu.Unlock()
			if err != nil {
				t.Errorf("concurrent insert %d: %v", i, err)
				return
			}
			// Spread ingest across the compaction cycles so merge and flush
			// genuinely interleave instead of finishing back to back.
			time.Sleep(5 * time.Millisecond)
		}
	}()

	// Keep compacting for as long as ingest runs, so cycles overlap flushes.
	var cycles, compacted, deferredCount int
	for {
		select {
		case <-ingestDone:
		default:
			res, err := CompactTable(context.Background(), opts, f.table)
			if err != nil {
				t.Fatalf("CompactTable during ingest: %v", err)
			}
			cycles++
			compacted += res.PartitionsCompacted
			deferredCount += res.PartitionsDeferred
			continue
		}
		break
	}
	wg.Wait()

	t.Logf("cycles=%d partitions compacted=%d deferred=%d", cycles, compacted, deferredCount)
	if cycles == 0 {
		t.Error("no compaction cycle overlapped ingest")
	}

	// A final quiet cycle so the result does not depend on the interleaving.
	if _, err := CompactTable(context.Background(), opts, f.table); err != nil {
		t.Fatalf("final CompactTable: %v", err)
	}

	if got := f.count(t); got != wantTotal {
		t.Errorf("row count = %d, want %d (rows lost or duplicated)", got, wantTotal)
	}
	// Every id must appear exactly once.
	var dupes int64
	if err := f.db.QueryRow(
		`SELECT COUNT(*) FROM (SELECT id FROM lake.calls GROUP BY id HAVING COUNT(*) > 1)`).Scan(&dupes); err != nil {
		t.Fatal(err)
	}
	if dupes != 0 {
		t.Errorf("%d duplicated ids", dupes)
	}
	assertEveryActiveFileExists(t, f)
	f.assertCatalogHealthy(t)
}

// TestNativeCompactionPreservesLogicalTypes reproduces a failure found by running
// a real writer: HEP tables declare data_extra as JSON, and registering a merged
// file was rejected with `Expected type "JSON" but found type "BLOB"`.
//
// The Arrow bridge reads parquet BYTE_ARRAY/JSON as plain `binary` and writes it
// back with no logical annotation, so the merged file no longer matched the table.
// The fixture's other tables use only DATE/BIGINT/VARCHAR, which round-trip
// cleanly and hid this entirely — hence a table with Homer's real column types.
func TestNativeCompactionPreservesLogicalTypes(t *testing.T) {
	f := newLakeFixture(t)
	f.mustExec(t, `CREATE TABLE lake.rich (
	    d DATE, id BIGINT, ts TIMESTAMP, port UINTEGER, extra JSON, txt VARCHAR)`)
	f.mustExec(t, `ALTER TABLE lake.rich SET PARTITIONED BY (d)`)

	const batches = 6
	for i := int64(0); i < batches; i++ {
		f.mustExec(t, fmt.Sprintf(
			`INSERT INTO lake.rich
			 SELECT DATE '2026-05-30', i, TIMESTAMP '2026-05-30 10:00:00' + INTERVAL (i) SECOND,
			        5060 + (i %% 10), json_object('call_id', i::VARCHAR, 'tag', 'x'), repeat('y', 80)
			   FROM range(%d, %d) tbl(i)`, i*10, i*10+10))
	}

	var rowsBefore int64
	if err := f.db.QueryRow(`SELECT COUNT(*) FROM lake.rich`).Scan(&rowsBefore); err != nil {
		t.Fatal(err)
	}

	res, err := CompactTable(context.Background(), f.options(), "rich")
	if err != nil {
		t.Fatalf("CompactTable: %v", err)
	}
	if res.Skipped {
		t.Fatalf("table was skipped: %s", res.SkipReason)
	}
	if res.PartitionsCompacted != 1 {
		t.Fatalf("partition was not compacted: %+v", res)
	}

	var rowsAfter int64
	if err := f.db.QueryRow(`SELECT COUNT(*) FROM lake.rich`).Scan(&rowsAfter); err != nil {
		t.Fatal(err)
	}
	if rowsAfter != rowsBefore {
		t.Errorf("row count = %d, want %d", rowsAfter, rowsBefore)
	}

	// The column must still behave as JSON, not as an opaque blob: a lost
	// annotation would either break registration or degrade the column's type.
	var extracted int64
	if err := f.db.QueryRow(
		`SELECT COUNT(*) FROM lake.rich WHERE json_extract_string(extra, '$.tag') = 'x'`).Scan(&extracted); err != nil {
		t.Fatalf("query JSON column after compaction: %v", err)
	}
	if extracted != rowsBefore {
		t.Errorf("JSON extraction matched %d rows, want %d", extracted, rowsBefore)
	}
	f.assertCatalogHealthy(t)

	// Writing must still work against the compacted partition.
	f.mustExec(t, `INSERT INTO lake.rich VALUES
	    (DATE '2026-05-30', 999, TIMESTAMP '2026-05-30 12:00:00', 5060, '{"call_id":"999"}', 'z')`)
	if err := f.db.QueryRow(`SELECT COUNT(*) FROM lake.rich`).Scan(&rowsAfter); err != nil {
		t.Fatal(err)
	}
	if rowsAfter != rowsBefore+1 {
		t.Errorf("row count after insert = %d, want %d", rowsAfter, rowsBefore+1)
	}
}

// flushOnSwapHook returns an Options.Lock hook that commits one batch on the
// second lock acquisition, i.e. between planning and the swap of the first
// partition. It relies on the run touching a single partition, so the second
// acquisition is unambiguously the swap.
func flushOnSwapHook(t *testing.T, f *lakeFixture, day string, from, to int64) func() {
	t.Helper()
	var mu sync.Mutex
	acquisitions := 0
	return func() {
		mu.Lock()
		acquisitions++
		n := acquisitions
		mu.Unlock()
		if n == 2 {
			f.insertBatch(t, day, from, to)
		}
	}
}

// countParquetOnDisk counts every .parquet file under a lake's data path.
func countParquetOnDisk(t *testing.T, dataPath string) int {
	t.Helper()
	n := 0
	if err := filepath.WalkDir(dataPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && filepath.Ext(path) == ".parquet" {
			n++
		}
		return nil
	}); err != nil {
		t.Fatalf("walk %s: %v", dataPath, err)
	}
	return n
}

// assertEveryActiveFileExists proves the catalog has no entry pointing at a
// missing parquet file — the ghost-entry state that makes later merges fail.
func assertEveryActiveFileExists(t *testing.T, f *lakeFixture) {
	t.Helper()
	rows, err := f.db.Query(
		`SELECT df.path, df.path_is_relative, t.path
		   FROM __ducklake_metadata_lake.ducklake_data_file df
		   JOIN __ducklake_metadata_lake.ducklake_table t ON t.table_id = df.table_id
		  WHERE df.end_snapshot IS NULL AND t.end_snapshot IS NULL`)
	if err != nil {
		t.Fatalf("list active files: %v", err)
	}
	defer rows.Close()
	checked := 0
	for rows.Next() {
		var p, tablePath string
		var isRel int64
		if err := rows.Scan(&p, &isRel, &tablePath); err != nil {
			t.Fatal(err)
		}
		abs := tableFileAbs(f.dataPath, tablePath, p, isRel)
		if _, err := os.Stat(abs); err != nil {
			t.Errorf("active catalog entry points at a missing file: %s (%v)", abs, err)
		}
		checked++
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if checked == 0 {
		t.Error("no active files found to verify")
	}
}
