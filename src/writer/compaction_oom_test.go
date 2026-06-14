package writer

import (
	"strings"
	"testing"
)

func TestOOMRetryLimits(t *testing.T) {
	c := &CompactionService{}
	cases := []struct {
		initial int
		want    []int
	}{
		{initial: 10, want: []int{5, 2, 1}},
		{initial: 25, want: []int{10, 5, 2, 1}},
		{initial: 2, want: []int{1}},
		{initial: 1, want: nil},
	}
	for _, tc := range cases {
		got := c.oomRetryLimits(tc.initial)
		if len(got) != len(tc.want) {
			t.Fatalf("initial=%d got=%v want=%v", tc.initial, got, tc.want)
		}
		for i := range tc.want {
			if got[i] != tc.want[i] {
				t.Fatalf("initial=%d got=%v want=%v", tc.initial, got, tc.want)
			}
		}
	}
}

func TestBuildMergeSQLWithLimit(t *testing.T) {
	c := &CompactionService{
		lakeName: "homer_lake",
		config: CompactionConfig{
			MinFileSizeBytes:  1024,
			MaxCompactedFiles: 25,
		},
	}
	sqlWithLimit := c.buildMergeSQLWithLimit("hep_proto_1_call", 5, 8192)
	if !strings.Contains(sqlWithLimit, "max_compacted_files => 5") {
		t.Fatalf("expected explicit max_compacted_files in SQL, got: %s", sqlWithLimit)
	}
	if !strings.Contains(sqlWithLimit, "max_file_size => 8192") {
		t.Fatalf("expected explicit max_file_size in SQL, got: %s", sqlWithLimit)
	}
	sqlNoLimit := c.buildMergeSQLWithLimit("hep_proto_1_call", 0, 0)
	if strings.Contains(sqlNoLimit, "max_compacted_files") {
		t.Fatalf("did not expect max_compacted_files in SQL when limit=0, got: %s", sqlNoLimit)
	}
}

func TestCompactTempTableNameSanitizes(t *testing.T) {
	name := compactTempTableName("hep.proto-1/call")
	if !strings.HasPrefix(name, "__compact_hep_proto_1_call_") {
		t.Fatalf("unexpected compact temp table name: %s", name)
	}
}

func TestParseDuckDBByteSize(t *testing.T) {
	cases := []struct {
		in   string
		want int64
	}{
		{in: "4GB", want: 4_000_000_000},
		{in: "3.7 GiB", want: 3972844748},
		{in: "1500MB", want: 1_500_000_000},
	}
	for _, tc := range cases {
		got, err := parseDuckDBByteSize(tc.in)
		if err != nil {
			t.Fatalf("parse %q: %v", tc.in, err)
		}
		if got != tc.want {
			t.Fatalf("parse %q got=%d want=%d", tc.in, got, tc.want)
		}
	}
}
