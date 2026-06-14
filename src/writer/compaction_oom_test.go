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
			MaxFileSizeBytes:  8192,
			MaxCompactedFiles: 25,
		},
	}
	sqlWithLimit := c.buildMergeSQLWithLimit("hep_proto_1_call", 5)
	if !strings.Contains(sqlWithLimit, "max_compacted_files => 5") {
		t.Fatalf("expected explicit max_compacted_files in SQL, got: %s", sqlWithLimit)
	}
	sqlNoLimit := c.buildMergeSQLWithLimit("hep_proto_1_call", 0)
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
