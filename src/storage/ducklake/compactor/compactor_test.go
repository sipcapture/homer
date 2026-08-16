package compactor

import (
	"path/filepath"
	"testing"
)

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
			name:   "trailing single still covered",
			sizes:  []int64{200, 200, 200},
			target: target,
			want:   [][]int{{0, 1}, {2}},
		},
		{
			name:   "single file covered",
			sizes:  []int64{200},
			target: target,
			want:   [][]int{{0}},
		},
		{
			name:   "oversize file gets its own batch",
			sizes:  []int64{600, 100, 100},
			target: target,
			want:   [][]int{{0}, {1, 2}},
		},
		{
			name:   "six 77 into one plus leftover",
			sizes:  []int64{77, 77, 77, 77, 77, 77, 77},
			target: 512,
			want:   [][]int{{0, 1, 2, 3, 4, 5}, {6}},
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

// TestPlanBatchesCoversEveryFile is the invariant the partition swap depends on:
// the retirement DELETE drops the whole partition, so a file left out of the
// batching would be data loss.
func TestPlanBatchesCoversEveryFile(t *testing.T) {
	cases := [][]int64{
		{200, 200, 200},
		{600, 100, 100},
		{1, 1, 1, 1, 1, 1, 1, 1, 1},
		{900, 900},
		{5, 900, 5, 900, 5},
		{512, 512, 1},
	}
	for _, sizes := range cases {
		batches := planBatches(sizes, 512)
		seen := make([]int, len(sizes))
		for _, b := range batches {
			for _, i := range b {
				seen[i]++
			}
		}
		for i, n := range seen {
			if n != 1 {
				t.Errorf("sizes=%v: file %d covered %d times, want exactly 1 (batches=%v)",
					sizes, i, n, batches)
			}
		}
	}
}

func TestPlanPartition(t *testing.T) {
	tests := []struct {
		name      string
		sizes     []int64
		target    int64
		wantOK    bool
		wantFiles int // number of output batches when compacting
	}{
		{
			name:      "many small files are consolidated",
			sizes:     []int64{100, 100, 100, 100},
			target:    512,
			wantOK:    true,
			wantFiles: 1,
		},
		{
			name:   "single file is never worth rewriting",
			sizes:  []int64{100},
			target: 512,
			wantOK: false,
		},
		{
			name:   "already-compacted partition is skipped",
			sizes:  []int64{600, 700},
			target: 512,
			wantOK: false,
		},
		{
			name:   "one big file dwarfing the small ones is deferred",
			sizes:  []int64{5000, 10, 10, 10},
			target: 512,
			wantOK: false,
		},
		{
			name:      "big file is absorbed once the small files justify it",
			sizes:     []int64{600, 100, 100, 100, 100, 100, 100, 100},
			target:    512,
			wantOK:    true,
			wantFiles: 3,
		},
		{
			name:   "same big file is left alone while few small files exist",
			sizes:  []int64{600, 100, 100},
			target: 512,
			wantOK: false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			batches, ok := planPartition(tc.sizes, tc.target)
			if ok != tc.wantOK {
				t.Fatalf("planPartition(%v, %d) ok = %v, want %v (batches=%v)",
					tc.sizes, tc.target, ok, tc.wantOK, batches)
			}
			if !ok {
				if batches != nil {
					t.Errorf("expected no batches when skipping, got %v", batches)
				}
				return
			}
			if len(batches) != tc.wantFiles {
				t.Errorf("got %d batches, want %d: %v", len(batches), tc.wantFiles, batches)
			}
			// Coverage must hold whenever we decide to compact.
			seen := make([]int, len(tc.sizes))
			for _, b := range batches {
				for _, i := range b {
					seen[i]++
				}
			}
			for i, n := range seen {
				if n != 1 {
					t.Errorf("file %d covered %d times, want 1", i, n)
				}
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

func TestPartitionKeyUsable(t *testing.T) {
	tests := []struct {
		name string
		key  partitionKey
		want bool
	}{
		{"identity date", partitionKey{columnName: "date", columnType: "date", transform: "identity"}, true},
		{"identity varchar", partitionKey{columnName: "d", columnType: "varchar", transform: "identity"}, true},
		{"decimal type", partitionKey{columnName: "d", columnType: "decimal(18,3)", transform: "identity"}, true},
		{"no partitioning", partitionKey{}, false},
		{"non-identity transform", partitionKey{columnName: "ts", columnType: "timestamp", transform: "month"}, false},
		{"unsafe type string", partitionKey{columnName: "d", columnType: "date; DROP TABLE x", transform: "identity"}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.key.usable(); got != tc.want {
				t.Errorf("usable() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestQuoteIdentAndLiteral(t *testing.T) {
	if got := quoteIdent(`we"ird`); got != `"we""ird"` {
		t.Errorf("quoteIdent = %s", got)
	}
	if got := quoteLiteral(`it's`); got != `'it''s'` {
		t.Errorf("quoteLiteral = %s", got)
	}
	if got := tableRef("homer_lake", "hep_proto_1_call"); got != `"homer_lake"."main"."hep_proto_1_call"` {
		t.Errorf("tableRef = %s", got)
	}
}

func TestNewOutputRelPathStaysInPartitionDir(t *testing.T) {
	// DuckLake derives the added file's partition value from its directory, so
	// the merged file must be written next to its sources.
	got := newOutputRelPath(filepath.Join("date=2026-05-30", "ducklake-abc.parquet"))
	if dir := filepath.Dir(got); dir != "date=2026-05-30" {
		t.Errorf("output dir = %q, want date=2026-05-30 (got path %q)", dir, got)
	}
	if filepath.Ext(got) != ".parquet" {
		t.Errorf("output is not parquet: %q", got)
	}

	flat := newOutputRelPath("ducklake-abc.parquet")
	if filepath.Dir(flat) != "." {
		t.Errorf("unpartitioned output should stay at the root, got %q", flat)
	}
}

func TestHiveLikePathComponent(t *testing.T) {
	if got := HiveLikePathComponent("/data/homer/parquet"); got != "" {
		t.Errorf("clean path flagged %q", got)
	}
	if got := HiveLikePathComponent("/srv/env=prod/homer"); got != "env=prod" {
		t.Errorf("got %q, want env=prod", got)
	}
}

func TestTableFileAbs(t *testing.T) {
	abs := tableFileAbs("/data", "hep_proto_1_call/", "date=2026-05-30/f.parquet", 1)
	want := filepath.Join("/data", "main", "hep_proto_1_call", "date=2026-05-30", "f.parquet")
	if abs != want {
		t.Errorf("relative entry = %q, want %q", abs, want)
	}
	if got := tableFileAbs("/data", "t/", "/elsewhere/f.parquet", 0); got != "/elsewhere/f.parquet" {
		t.Errorf("absolute entry rewritten to %q", got)
	}
}
