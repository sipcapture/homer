package compactor

import (
	"context"
	"fmt"
	"testing"
)

// newRetentionFixture builds a table shaped like a HEP table: partitioned by date
// but also carrying the timestamp column retention actually filters on.
func newRetentionFixture(t *testing.T) *lakeFixture {
	t.Helper()
	f := newLakeFixture(t)
	f.mustExec(t, `CREATE TABLE lake.hep (
	    date DATE, "timestamp" TIMESTAMP, id BIGINT, payload VARCHAR)`)
	f.mustExec(t, `ALTER TABLE lake.hep SET PARTITIONED BY (date)`)
	// Two days, several files each, timestamps spread across each day.
	for _, day := range []string{"2026-05-29", "2026-05-30"} {
		for i := int64(0); i < 3; i++ {
			f.mustExec(t, fmt.Sprintf(
				`INSERT INTO lake.hep
				 SELECT DATE '%[1]s',
				        TIMESTAMP '%[1]s 00:00:00' + INTERVAL (i * 3600) SECOND,
				        i, repeat('x', 100)
				   FROM range(%[2]d, %[3]d) tbl(i)`, day, i*8, i*8+8))
		}
	}
	return f
}

func (f *lakeFixture) activeDeleteFiles(t *testing.T) int64 {
	t.Helper()
	var n int64
	if err := f.db.QueryRow(
		`SELECT COUNT(*) FROM __ducklake_metadata_lake.ducklake_delete_file
		  WHERE end_snapshot IS NULL`).Scan(&n); err != nil {
		t.Fatalf("count delete files: %v", err)
	}
	return n
}

// TestRetentionMidDayCutoffOnlyBlocksItsOwnPartition covers how retention and the
// native engine interact. Retention runs `DELETE ... WHERE timestamp < cutoff`
// while the table is partitioned by date, so a cutoff inside a day removes part of
// a file's rows and DuckLake records a row-level delete file. That partition
// cannot be retired wholesale, but every other partition still compacts — which is
// what keeps retention and native compaction usable together.
func TestRetentionMidDayCutoffOnlyBlocksItsOwnPartition(t *testing.T) {
	f := newRetentionFixture(t)
	if got := f.activeDeleteFiles(t); got != 0 {
		t.Fatalf("fixture already has %d delete files", got)
	}

	// Mirrors runRetention with a cutoff that lands inside 2026-05-29.
	f.mustExec(t, `DELETE FROM lake.hep WHERE "timestamp" < TIMESTAMP '2026-05-29 01:30:00'`)

	deletes := f.activeDeleteFiles(t)
	t.Logf("active row-level delete files after a mid-day cutoff: %d", deletes)
	if deletes == 0 {
		t.Skip("this DuckLake version retired whole files instead of recording " +
			"row-level deletes, so there is nothing to block")
	}
	rowsBefore := f.countTable(t, "hep")

	res, err := CompactTable(context.Background(), f.options(), "hep")
	if err != nil {
		t.Fatalf("CompactTable: %v", err)
	}
	if res.Skipped {
		t.Fatalf("whole table was skipped over one partition: %s", res.SkipReason)
	}
	if res.PartitionsSkippedDeletes != 1 {
		t.Errorf("the partition holding the cutoff was not left alone: %+v", res)
	}
	if res.PartitionsCompacted != 1 {
		t.Errorf("the partition retention did not touch was not compacted: %+v", res)
	}
	if got := f.countTable(t, "hep"); got != rowsBefore {
		t.Errorf("row count = %d, want %d", got, rowsBefore)
	}
	// Rows retention removed must not come back.
	var expired int64
	if err := f.db.QueryRow(
		`SELECT COUNT(*) FROM lake.hep WHERE "timestamp" < TIMESTAMP '2026-05-29 01:30:00'`).
		Scan(&expired); err != nil {
		t.Fatal(err)
	}
	if expired != 0 {
		t.Errorf("%d expired rows came back after compaction", expired)
	}
	// The delete file here is intentional, so only the snapshot invariants apply.
	f.assertSnapshotsHealthy(t)
}

func (f *lakeFixture) countTable(t *testing.T, table string) int64 {
	t.Helper()
	var n int64
	if err := f.db.QueryRow(fmt.Sprintf(`SELECT COUNT(*) FROM lake.%s`, quoteIdent(table))).Scan(&n); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return n
}

// TestRetentionDayAlignedCutoffKeepsCompactionWorking checks the other end: when
// the cutoff falls on a partition boundary the delete covers whole files, so no
// row-level delete file is produced and the remaining data still compacts. This is
// what makes a date-aligned retention cutoff compatible with the native engine.
func TestRetentionDayAlignedCutoffKeepsCompactionWorking(t *testing.T) {
	f := newRetentionFixture(t)

	f.mustExec(t, `DELETE FROM lake.hep WHERE "timestamp" < TIMESTAMP '2026-05-30 00:00:00'`)

	if got := f.activeDeleteFiles(t); got != 0 {
		t.Errorf("day-aligned cutoff produced %d row-level delete files, expected none", got)
	}

	var rows int64
	if err := f.db.QueryRow(`SELECT COUNT(*) FROM lake.hep`).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 24 {
		t.Errorf("rows after retention = %d, want 24 (only 2026-05-30 remains)", rows)
	}

	res, err := CompactTable(context.Background(), f.options(), "hep")
	if err != nil {
		t.Fatalf("CompactTable: %v", err)
	}
	if res.Skipped {
		t.Fatalf("compaction was skipped after a day-aligned retention: %s", res.SkipReason)
	}
	if res.PartitionsCompacted != 1 {
		t.Errorf("remaining partition was not compacted: %+v", res)
	}

	var after int64
	if err := f.db.QueryRow(`SELECT COUNT(*) FROM lake.hep`).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if after != rows {
		t.Errorf("row count changed by compaction: %d -> %d", rows, after)
	}
	f.assertCatalogHealthy(t)
}
