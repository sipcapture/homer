package compactor

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
)

func nstr(s string) sql.NullString { return sql.NullString{String: s, Valid: true} }
func nint(i int64) sql.NullInt64   { return sql.NullInt64{Int64: i, Valid: true} }

func TestPlanBatches(t *testing.T) {
	const target = 512 // 512 "bytes" for readability
	tests := []struct {
		name   string
		sizes  []int64
		target int64
		want   [][]int
	}{
		{
			name:   "pairs under target",
			sizes:  []int64{200, 200, 200, 200},
			target: target,
			want:   [][]int{{0, 1}, {2, 3}},
		},
		{
			name:   "trailing single skipped",
			sizes:  []int64{200, 200, 200},
			target: target,
			want:   [][]int{{0, 1}},
		},
		{
			name:   "single file no batch",
			sizes:  []int64{200},
			target: target,
			want:   nil,
		},
		{
			name:   "oversize file isolated and skipped",
			sizes:  []int64{600, 100, 100},
			target: target,
			want:   [][]int{{1, 2}},
		},
		{
			name:   "six 77 into one plus leftover",
			sizes:  []int64{77, 77, 77, 77, 77, 77, 77},
			target: 512,
			want:   [][]int{{0, 1, 2, 3, 4, 5}},
		},
		{
			name:   "zero target",
			sizes:  []int64{100, 100},
			target: 0,
			want:   nil,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := planBatches(tc.sizes, tc.target)
			if !equalBatches(got, tc.want) {
				t.Fatalf("planBatches(%v, %d) = %v, want %v", tc.sizes, tc.target, got, tc.want)
			}
		})
	}
}

func equalBatches(a, b [][]int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if len(a[i]) != len(b[i]) {
			return false
		}
		for j := range a[i] {
			if a[i][j] != b[i][j] {
				return false
			}
		}
	}
	return true
}

func TestIsNumericType(t *testing.T) {
	numeric := []string{"uint32", "bigint", "INTEGER", "double", "decimal(18,3)", "hugeint"}
	nonNumeric := []string{"varchar", "date", "timestamp", "json", "uuid", "blob"}
	for _, x := range numeric {
		if !isNumericType(x) {
			t.Errorf("isNumericType(%q) = false, want true", x)
		}
	}
	for _, x := range nonNumeric {
		if isNumericType(x) {
			t.Errorf("isNumericType(%q) = true, want false", x)
		}
	}
}

func TestStatLess(t *testing.T) {
	// Numeric: "80" < "5060" by value, not lexicographically.
	if !statLess("80", "5060", "uint32") {
		t.Error("numeric statLess: 80 should be < 5060")
	}
	if statLess("5060", "80", "uint32") {
		t.Error("numeric statLess: 5060 should not be < 80")
	}
	// Strings compare lexicographically.
	if !statLess("2026-05-30", "2026-05-31", "date") {
		t.Error("string statLess: earlier date should be < later date")
	}
	if !statLess("", "100", "varchar") {
		t.Error("string statLess: empty should be < non-empty")
	}
}

func TestAggregateColumnStats(t *testing.T) {
	colTypes := map[int64]string{
		9:  "uint32",  // port: numeric
		3:  "timestamp",
		12: "varchar", // response_code (some empty)
	}
	perFile := [][]colStat{
		{
			{columnID: 9, columnSizeBytes: 100, valueCount: 10, nullCount: 0, minValue: nstr("5060"), maxValue: nstr("5060")},
			{columnID: 3, columnSizeBytes: 200, valueCount: 10, nullCount: 1, minValue: nstr("2026-05-30 01:00:00.000"), maxValue: nstr("2026-05-30 02:00:00.000")},
			{columnID: 12, columnSizeBytes: 50, valueCount: 10, nullCount: 0, minValue: nstr("200"), maxValue: nstr("487")},
		},
		{
			{columnID: 9, columnSizeBytes: 120, valueCount: 8, nullCount: 0, minValue: nstr("80"), maxValue: nstr("5160")},
			{columnID: 3, columnSizeBytes: 220, valueCount: 8, nullCount: 0, minValue: nstr("2026-05-30 00:30:00.000"), maxValue: nstr("2026-05-30 03:00:00.000")},
			{columnID: 12, columnSizeBytes: 60, valueCount: 8, nullCount: 2, minValue: nstr(""), maxValue: nstr("500")},
		},
	}
	got := aggregateColumnStats(perFile, colTypes)
	by := map[int64]aggregatedColStat{}
	for _, g := range got {
		by[g.columnID] = g
	}

	port := by[9]
	if port.valueCount != 18 || port.nullCount != 0 || port.columnSizeBytes != 220 {
		t.Errorf("port counts wrong: %+v", port)
	}
	if *port.minValue != "80" || *port.maxValue != "5160" {
		t.Errorf("port min/max = %s/%s, want 80/5160", *port.minValue, *port.maxValue)
	}

	ts := by[3]
	if *ts.minValue != "2026-05-30 00:30:00.000" || *ts.maxValue != "2026-05-30 03:00:00.000" {
		t.Errorf("timestamp min/max = %s/%s", *ts.minValue, *ts.maxValue)
	}
	if ts.nullCount != 1 {
		t.Errorf("timestamp nullCount = %d, want 1", ts.nullCount)
	}

	rc := by[12]
	if *rc.minValue != "" || *rc.maxValue != "500" {
		t.Errorf("response_code min/max = %q/%q, want \"\"/500", *rc.minValue, *rc.maxValue)
	}
}

// --- catalog commit/reap on a temp sqlite ---

const ducklakeSchema = `
CREATE TABLE ducklake_snapshot(snapshot_id BIGINT PRIMARY KEY, snapshot_time VARCHAR, schema_version BIGINT, next_catalog_id BIGINT, next_file_id BIGINT);
CREATE TABLE ducklake_snapshot_changes(snapshot_id BIGINT PRIMARY KEY, changes_made VARCHAR, author VARCHAR, commit_message VARCHAR, commit_extra_info VARCHAR);
CREATE TABLE ducklake_data_file(data_file_id BIGINT PRIMARY KEY, table_id BIGINT, begin_snapshot BIGINT, end_snapshot BIGINT, file_order BIGINT, path VARCHAR, path_is_relative BIGINT, file_format VARCHAR, record_count BIGINT, file_size_bytes BIGINT, footer_size BIGINT, row_id_start BIGINT, partition_id BIGINT, encryption_key VARCHAR, mapping_id BIGINT, partial_max BIGINT);
CREATE TABLE ducklake_file_column_stats(data_file_id BIGINT, table_id BIGINT, column_id BIGINT, column_size_bytes BIGINT, value_count BIGINT, null_count BIGINT, min_value VARCHAR, max_value VARCHAR, contains_nan BIGINT, extra_stats VARCHAR);
CREATE TABLE ducklake_file_partition_value(data_file_id BIGINT, table_id BIGINT, partition_key_index BIGINT, partition_value VARCHAR);
CREATE TABLE ducklake_table(table_id BIGINT, table_uuid VARCHAR, begin_snapshot BIGINT, end_snapshot BIGINT, schema_id BIGINT, table_name VARCHAR, path VARCHAR, path_is_relative BIGINT);
CREATE TABLE ducklake_column(column_id BIGINT, begin_snapshot BIGINT, end_snapshot BIGINT, table_id BIGINT, column_order BIGINT, column_name VARCHAR, column_type VARCHAR, initial_default VARCHAR, default_value VARCHAR, nulls_allowed BIGINT, parent_column BIGINT, default_value_type VARCHAR, default_value_dialect VARCHAR);
CREATE TABLE ducklake_table_stats(table_id BIGINT, record_count BIGINT, next_row_id BIGINT, file_size_bytes BIGINT);
CREATE TABLE ducklake_files_scheduled_for_deletion(data_file_id BIGINT, path VARCHAR, path_is_relative BIGINT, schedule_start VARCHAR);
CREATE TABLE ducklake_delete_file(delete_file_id BIGINT PRIMARY KEY, table_id BIGINT, begin_snapshot BIGINT, end_snapshot BIGINT, data_file_id BIGINT, path VARCHAR, path_is_relative BIGINT, format VARCHAR, delete_count BIGINT, file_size_bytes BIGINT, footer_size BIGINT, encryption_key VARCHAR, partial_max BIGINT);
`

func seedCatalog(t *testing.T, db *sql.DB) {
	t.Helper()
	if _, err := db.Exec(ducklakeSchema); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	exec := func(q string, args ...any) {
		if _, err := db.Exec(q, args...); err != nil {
			t.Fatalf("seed exec %q: %v", q, err)
		}
	}
	// snapshot 10: next_file_id=4 (ids 0..3 used)
	exec(`INSERT INTO ducklake_snapshot VALUES (10, ?, 1, 5, 4)`, nowDuckLakeTime())
	exec(`INSERT INTO ducklake_snapshot_changes VALUES (10, 'created_table:1', '', '', '')`)
	exec(`INSERT INTO ducklake_table VALUES (1, 'uuid-1', 1, NULL, 0, 'tbl', 'tbl/', 1)`)
	exec(`INSERT INTO ducklake_column VALUES (1, 1, NULL, 1, 1, 'timestamp', 'timestamp', NULL, NULL, 1, NULL, NULL, NULL)`)
	exec(`INSERT INTO ducklake_table_stats VALUES (1, 40, 40, 400)`)
	for i := int64(0); i < 4; i++ {
		exec(`INSERT INTO ducklake_data_file
			(data_file_id, table_id, begin_snapshot, end_snapshot, file_order, path, path_is_relative, file_format, record_count, file_size_bytes, footer_size, row_id_start, partition_id, encryption_key, mapping_id, partial_max)
			VALUES (?, 1, ?, NULL, NULL, ?, 1, 'parquet', 10, 100, 50, ?, 2, NULL, NULL, NULL)`,
			i, 6+i, filepath.Join("date=2026-05-30", "src"+itoa(i)+".parquet"), i*10)
		exec(`INSERT INTO ducklake_file_column_stats VALUES (?, 1, 1, 64, 10, 0, '2026-05-30 00:00:00.000', '2026-05-30 01:00:00.000', 0, NULL)`, i)
		exec(`INSERT INTO ducklake_file_partition_value VALUES (?, 1, 0, '2026-05-30')`, i)
	}
}

func itoa(i int64) string {
	return string(rune('0' + i))
}

func openTempCatalog(t *testing.T) (*Catalog, string) {
	t.Helper()
	dir := t.TempDir()
	catPath := filepath.Join(dir, "catalog.sqlite")
	cat, err := OpenCatalog(catPath)
	if err != nil {
		t.Fatalf("OpenCatalog: %v", err)
	}
	t.Cleanup(func() { cat.Close() })
	seedCatalog(t, cat.db)
	return cat, dir
}

func TestCommitRegistersAndRetires(t *testing.T) {
	cat, _ := openTempCatalog(t)

	in := commitInput{
		tableID: 1,
		newFiles: []newFile{{
			relPath:        filepath.Join("date=2026-05-30", "merged.parquet"),
			fileFormat:     "parquet",
			recordCount:    20,
			fileSizeBytes:  190,
			footerSize:     60,
			partitionID:    nint(2),
			partitionValue: "2026-05-30",
			stats: []aggregatedColStat{{
				columnID: 1, columnSizeBytes: 120, valueCount: 20, nullCount: 0,
				minValue: strptr("2026-05-30 00:00:00.000"), maxValue: strptr("2026-05-30 01:00:00.000"),
			}},
		}},
		retired: []retiredFile{
			{dataFileID: 0, path: filepath.Join("date=2026-05-30", "src0.parquet"), pathIsRel: 1},
			{dataFileID: 1, path: filepath.Join("date=2026-05-30", "src1.parquet"), pathIsRel: 1},
		},
		netSizeDelta: 190 - 200,
	}

	snap, err := cat.commit(in)
	if err != nil {
		t.Fatalf("commit: %v", err)
	}
	if snap != 11 {
		t.Fatalf("new snapshot = %d, want 11", snap)
	}

	// New data file: id 4 (from next_file_id), begin=11, row_id_start=40.
	var begin, rowStart, recCount int64
	if err := cat.db.QueryRow(`SELECT begin_snapshot, row_id_start, record_count FROM ducklake_data_file WHERE data_file_id=4`).
		Scan(&begin, &rowStart, &recCount); err != nil {
		t.Fatalf("read new file: %v", err)
	}
	if begin != 11 || rowStart != 40 || recCount != 20 {
		t.Errorf("new file begin/rowstart/rec = %d/%d/%d, want 11/40/20", begin, rowStart, recCount)
	}

	// Retired files have end_snapshot=11.
	var retired int
	if err := cat.db.QueryRow(`SELECT COUNT(*) FROM ducklake_data_file WHERE data_file_id IN (0,1) AND end_snapshot=11`).Scan(&retired); err != nil {
		t.Fatal(err)
	}
	if retired != 2 {
		t.Errorf("retired count = %d, want 2", retired)
	}

	// next_file_id advanced to 5; new snapshot present.
	var nextFileID int64
	if err := cat.db.QueryRow(`SELECT next_file_id FROM ducklake_snapshot WHERE snapshot_id=11`).Scan(&nextFileID); err != nil {
		t.Fatal(err)
	}
	if nextFileID != 5 {
		t.Errorf("next_file_id = %d, want 5", nextFileID)
	}

	// table_stats: next_row_id += 20, file_size_bytes += -10, record_count unchanged.
	var rc, nrid, fsz int64
	if err := cat.db.QueryRow(`SELECT record_count, next_row_id, file_size_bytes FROM ducklake_table_stats WHERE table_id=1`).
		Scan(&rc, &nrid, &fsz); err != nil {
		t.Fatal(err)
	}
	if rc != 40 || nrid != 60 || fsz != 390 {
		t.Errorf("table_stats = %d/%d/%d, want 40/60/390", rc, nrid, fsz)
	}

	// scheduled_for_deletion has the two retired files.
	var sched int
	if err := cat.db.QueryRow(`SELECT COUNT(*) FROM ducklake_files_scheduled_for_deletion`).Scan(&sched); err != nil {
		t.Fatal(err)
	}
	if sched != 2 {
		t.Errorf("scheduled_for_deletion = %d, want 2", sched)
	}
}

func TestReapRemovesSupersededFiles(t *testing.T) {
	cat, dir := openTempCatalog(t)

	// Commit a merge retiring files 0,1.
	in := commitInput{
		tableID: 1,
		newFiles: []newFile{{
			relPath: filepath.Join("date=2026-05-30", "merged.parquet"), fileFormat: "parquet",
			recordCount: 20, fileSizeBytes: 190, footerSize: 60, partitionID: nint(2), partitionValue: "2026-05-30",
			stats: []aggregatedColStat{{columnID: 1, valueCount: 20}},
		}},
		retired: []retiredFile{
			{dataFileID: 0, path: filepath.Join("date=2026-05-30", "src0.parquet"), pathIsRel: 1},
			{dataFileID: 1, path: filepath.Join("date=2026-05-30", "src1.parquet"), pathIsRel: 1},
		},
		netSizeDelta: -10,
	}
	if _, err := cat.commit(in); err != nil {
		t.Fatalf("commit: %v", err)
	}

	// Create the physical retired parquet files so reap can unlink them.
	partDir := filepath.Join(dir, "main", "tbl", "date=2026-05-30")
	if err := os.MkdirAll(partDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"src0.parquet", "src1.parquet"} {
		if err := os.WriteFile(filepath.Join(partDir, n), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// window=0 keeps only the latest snapshot; files retired at <=latest are reaped.
	filesDeleted, snapsPruned, err := cat.reap(dir, 0)
	if err != nil {
		t.Fatalf("reap: %v", err)
	}
	if filesDeleted != 2 {
		t.Errorf("filesDeleted = %d, want 2", filesDeleted)
	}
	if snapsPruned == 0 {
		t.Errorf("expected some snapshots pruned, got 0")
	}

	// Physical files gone.
	for _, n := range []string{"src0.parquet", "src1.parquet"} {
		if _, err := os.Stat(filepath.Join(partDir, n)); !os.IsNotExist(err) {
			t.Errorf("expected %s removed", n)
		}
	}
	// Catalog rows for retired files gone.
	var remaining int
	if err := cat.db.QueryRow(`SELECT COUNT(*) FROM ducklake_data_file WHERE data_file_id IN (0,1)`).Scan(&remaining); err != nil {
		t.Fatal(err)
	}
	if remaining != 0 {
		t.Errorf("retired data_file rows = %d, want 0", remaining)
	}
}

func strptr(s string) *string { return &s }
