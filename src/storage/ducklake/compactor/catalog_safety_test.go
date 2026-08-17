package compactor

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

// TestNativeCompactionCoversEveryColumnType exercises every column type Homer's
// DuckLake tables actually declare, across tables.go, otlp_storage.go and
// vqrtcp_storage.go: VARCHAR, UINTEGER, TIMESTAMP, JSON, DATE, BIGINT and DOUBLE.
//
// The JSON breakage was only found by running a real writer, because the fixtures
// used none of the types the production schema relies on.
func TestNativeCompactionCoversEveryColumnType(t *testing.T) {
	f := newLakeFixture(t)
	f.mustExec(t, `CREATE TABLE lake.every_type (
	    d DATE,
	    id BIGINT,
	    port UINTEGER,
	    dbl DOUBLE,
	    ts TIMESTAMP,
	    extra JSON,
	    txt VARCHAR)`)
	f.mustExec(t, `ALTER TABLE lake.every_type SET PARTITIONED BY (d)`)

	for i := int64(0); i < 6; i++ {
		f.mustExec(t, fmt.Sprintf(
			`INSERT INTO lake.every_type
			 SELECT DATE '2026-05-30',
			        i,
			        (5060 + i %% 10)::UINTEGER,
			        i * 1.5,
			        TIMESTAMP '2026-05-30 10:00:00' + INTERVAL (i) SECOND,
			        json_object('call_id', i::VARCHAR, 'tag', 'x'),
			        repeat('y', 80)
			   FROM range(%d, %d) tbl(i)`, i*10, i*10+10))
	}

	before := scalarRow(t, f, `SELECT COUNT(*), SUM(hash(to_json(t)::VARCHAR)::HUGEINT)::VARCHAR FROM lake.every_type t`)

	res, err := CompactTable(context.Background(), f.options(), "every_type")
	if err != nil {
		t.Fatalf("CompactTable: %v", err)
	}
	if res.Skipped {
		t.Fatalf("table was skipped: %s", res.SkipReason)
	}
	if res.PartitionsCompacted != 1 {
		t.Fatalf("partition was not compacted: %+v", res)
	}

	after := scalarRow(t, f, `SELECT COUNT(*), SUM(hash(to_json(t)::VARCHAR)::HUGEINT)::VARCHAR FROM lake.every_type t`)
	if after[0] != before[0] {
		t.Errorf("row count = %s, want %s", after[0], before[0])
	}
	// An order-insensitive digest over every column: a type that silently lost
	// precision or was reinterpreted would change it even if the count matched.
	if after[1] != before[1] {
		t.Errorf("contents digest changed: %s -> %s", before[1], after[1])
	}

	// Types must still behave as their declared type, not as opaque bytes.
	var typed int64
	if err := f.db.QueryRow(`SELECT COUNT(*) FROM lake.every_type
	     WHERE json_extract_string(extra, '$.tag') = 'x'
	       AND port BETWEEN 5060 AND 5069
	       AND ts >= TIMESTAMP '2026-05-30 10:00:00'
	       AND dbl >= 0`).Scan(&typed); err != nil {
		t.Fatalf("typed query after compaction: %v", err)
	}
	if typed != 60 {
		t.Errorf("typed predicates matched %d rows, want 60", typed)
	}
	f.assertCatalogHealthy(t)
}

// TestCompactionSkipsUnsupportedColumnType covers a type the Arrow round trip
// cannot reproduce. HUGEINT comes back as DOUBLE, which DuckLake rejects at
// registration — so without an up-front check the compactor would rewrite the
// whole partition every cycle only to throw it away. Homer's schema uses no such
// type today; this guards a column added later.
func TestCompactionSkipsUnsupportedColumnType(t *testing.T) {
	f := newLakeFixture(t)
	f.mustExec(t, `CREATE TABLE lake.wide (d DATE, id BIGINT, big HUGEINT)`)
	f.mustExec(t, `ALTER TABLE lake.wide SET PARTITIONED BY (d)`)
	for i := int64(0); i < 6; i++ {
		f.mustExec(t, fmt.Sprintf(
			`INSERT INTO lake.wide
			 SELECT DATE '2026-05-30', i, (i * 170141183460469231731687303715)::HUGEINT
			   FROM range(%d, %d) tbl(i)`, i*10, i*10+10))
	}
	filesBefore := f.activeFiles(t)

	res, err := CompactTable(context.Background(), f.options(), "wide")
	if err != nil {
		t.Fatalf("CompactTable should skip, not fail: %v", err)
	}
	if !res.Skipped {
		t.Fatalf("expected a skip, got %+v", res)
	}
	// Unsupported tells the writer to stop offering this table, so it does not
	// rewrite the partition on every cycle just to discard it.
	if !res.Unsupported {
		t.Errorf("skip was not marked as unsupported: %+v", res)
	}
	if !strings.Contains(res.SkipReason, "big") {
		t.Errorf("skip reason does not name the offending column: %q", res.SkipReason)
	}
	t.Logf("skipped as expected: %s", res.SkipReason)

	// Nothing may be retired or rewritten when the table is refused.
	if got := f.activeFiles(t); got != filesBefore {
		t.Errorf("active files changed on a skipped table: %d -> %d", filesBefore, got)
	}
	var rows int64
	if err := f.db.QueryRow(`SELECT COUNT(*) FROM lake.wide`).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 60 {
		t.Errorf("row count = %d, want 60", rows)
	}
	f.assertCatalogHealthy(t)
}

// scalarRow runs a query returning one row of strings, for comparing digests.
func scalarRow(t *testing.T, f *lakeFixture, q string) []string {
	t.Helper()
	var count int64
	var digest sql.NullString
	if err := f.db.QueryRow(q).Scan(&count, &digest); err != nil {
		t.Fatalf("query %s: %v", q, err)
	}
	return []string{fmt.Sprint(count), digest.String}
}

// TestMinAgeLeavesFreshPartitionAlone covers the setting that keeps the compactor
// off the partition ingest is currently writing. Without it every cycle merges the
// hot partition and then discards the result when the swap sees a changed row
// count, which is pure wasted I/O.
func TestMinAgeLeavesFreshPartitionAlone(t *testing.T) {
	f := newLakeFixture(t)
	for i := int64(0); i < 6; i++ {
		f.insertBatch(t, "2026-05-30", i*10, i*10+10)
	}
	filesBefore := f.activeFiles(t)

	opts := f.options()
	opts.MinAge = time.Hour
	res, err := CompactTable(context.Background(), opts, f.table)
	if err != nil {
		t.Fatalf("CompactTable: %v", err)
	}
	if res.PartitionsSkippedYoung != 1 {
		t.Errorf("fresh partition was not left alone: %+v", res)
	}
	if res.PartitionsCompacted != 0 {
		t.Errorf("fresh partition was compacted: %+v", res)
	}
	if got := f.activeFiles(t); got != filesBefore {
		t.Errorf("active files changed: %d -> %d", filesBefore, got)
	}

	// Once the partition has actually settled it must be compacted. Snapshot age
	// has one-second resolution, so let real time pass rather than asserting on a
	// sub-second threshold that can never be met.
	time.Sleep(2 * time.Second)
	opts.MinAge = time.Second
	res, err = CompactTable(context.Background(), opts, f.table)
	if err != nil {
		t.Fatalf("second CompactTable: %v", err)
	}
	if res.PartitionsSkippedYoung != 0 {
		t.Errorf("settled partition was still treated as fresh: %+v", res)
	}
	if res.PartitionsCompacted != 1 {
		t.Errorf("settled partition was not compacted: %+v", res)
	}
	if got := f.count(t); got != 60 {
		t.Errorf("row count = %d, want 60", got)
	}
	f.assertCatalogHealthy(t)
}

// TestMaxRowGroupBytesLeavesOversizedPartitionAlone covers the guard that keeps
// the merge from being OOM-killed. A parquet row group is the unit held in memory
// and is sized in rows, so wide rows make it arbitrarily large: real Homer files
// with 1.5GB row groups needed ~10GB RSS to merge. Rather than synthesise such a
// file, the budget is set below the fixture's row groups.
func TestMaxRowGroupBytesLeavesOversizedPartitionAlone(t *testing.T) {
	f := newLakeFixture(t)
	for i := int64(0); i < 6; i++ {
		f.insertBatch(t, "2026-05-30", i*10, i*10+10)
	}
	filesBefore := f.activeFiles(t)
	rowsBefore := f.count(t)

	opts := f.options()
	opts.MaxRowGroupBytes = 1 // smaller than any real row group
	res, err := CompactTable(context.Background(), opts, f.table)
	if err != nil {
		t.Fatalf("CompactTable: %v", err)
	}
	if res.PartitionsSkippedLarge != 1 {
		t.Errorf("oversized partition was not left alone: %+v", res)
	}
	if res.PartitionsCompacted != 0 {
		t.Errorf("partition was merged despite the budget: %+v", res)
	}
	if got := f.activeFiles(t); got != filesBefore {
		t.Errorf("active files changed: %d -> %d", filesBefore, got)
	}

	// With the budget lifted the same partition must compact, proving the skip
	// came from the budget and nothing else.
	opts.MaxRowGroupBytes = 1 << 30
	res, err = CompactTable(context.Background(), opts, f.table)
	if err != nil {
		t.Fatalf("second CompactTable: %v", err)
	}
	if res.PartitionsCompacted != 1 {
		t.Errorf("partition was not compacted with a generous budget: %+v", res)
	}
	if got := f.count(t); got != rowsBefore {
		t.Errorf("row count = %d, want %d", got, rowsBefore)
	}
	f.assertCatalogHealthy(t)
}

// TestPartitionQuietFor covers the age helper, including the case where the
// catalog gives no snapshot time and the age must be reported as unknown so the
// caller does not skip work on a guess.
func TestPartitionQuietFor(t *testing.T) {
	age, ok := partitionQuietFor([]sourceFile{
		{ageSec: sql.NullInt64{Int64: 3600, Valid: true}},
		{ageSec: sql.NullInt64{Int64: 60, Valid: true}},
		{ageSec: sql.NullInt64{Valid: false}},
	})
	if !ok {
		t.Fatal("expected a known age")
	}
	// Must follow the newest file, not the oldest.
	if age != time.Minute {
		t.Errorf("age = %s, want 1m", age)
	}

	if _, ok := partitionQuietFor([]sourceFile{{ageSec: sql.NullInt64{Valid: false}}}); ok {
		t.Error("age reported as known when no file carries a snapshot time")
	}
	if _, ok := partitionQuietFor(nil); ok {
		t.Error("age reported as known for an empty partition")
	}

	// A snapshot timestamped in the future must read as "just written", never as
	// an old partition that is safe to merge.
	future, ok := partitionQuietFor([]sourceFile{{ageSec: sql.NullInt64{Int64: -30, Valid: true}}})
	if !ok || future != 0 {
		t.Errorf("future snapshot: age = %s, ok = %v; want 0 and true", future, ok)
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
