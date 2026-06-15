package node

import (
	"testing"
	"time"

	"github.com/sipcapture/homer-core/src/config"
)

func TestSortRowsByTimestampDescLimit(t *testing.T) {
	base := time.Date(2026, 6, 15, 10, 0, 0, 0, time.UTC)
	rows := []map[string]interface{}{
		{"timestamp": base.Add(1 * time.Minute), "id": 2},
		{"timestamp": base.Add(3 * time.Minute), "id": 4},
		{"timestamp": base.Add(0 * time.Minute), "id": 1},
		{"timestamp": base.Add(2 * time.Minute), "id": 3},
	}
	got := sortRowsByTimestampDescLimit(rows, 2)
	if len(got) != 2 {
		t.Fatalf("len=%d want 2", len(got))
	}
	if got[0]["id"] != 4 || got[1]["id"] != 3 {
		t.Errorf("got newest ids %v,%v want 4,3", got[0]["id"], got[1]["id"])
	}

	// limit 0 = no trim, still sorted desc.
	all := sortRowsByTimestampDescLimit(rows, 0)
	if len(all) != 4 || all[0]["id"] != 4 || all[3]["id"] != 1 {
		t.Errorf("unexpected order/len: %+v", all)
	}
}

func TestStripOrderByForStream(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "order by desc with limit",
			in:   "SELECT * FROM t WHERE timestamp >= 1 AND timestamp < 2 ORDER BY timestamp DESC LIMIT 500",
			want: "SELECT * FROM t WHERE timestamp >= 1 AND timestamp < 2 LIMIT 500",
		},
		{
			name: "no order by is unchanged",
			in:   "SELECT * FROM t WHERE timestamp >= 1 LIMIT 10",
			want: "SELECT * FROM t WHERE timestamp >= 1 LIMIT 10",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := stripOrderByForStream(tc.in); got != tc.want {
				t.Fatalf("stripOrderByForStream()\n got=%q\nwant=%q", got, tc.want)
			}
		})
	}
}

func TestLakeTopNStrategy(t *testing.T) {
	cases := map[string]string{
		"":        lakeTopNStream,
		"chunked": lakeTopNChunked,
		"stream":  lakeTopNStream,
		"full":    lakeTopNFull,
		"bogus":   lakeTopNStream,
	}
	for in, want := range cases {
		n := &Node{config: &config.NodeConfig{
			DuckLake: config.DuckLakeConfig{Search: config.SearchConfig{LakeTopNStrategy: in}},
		}}
		if got := n.lakeTopNStrategy(); got != want {
			t.Errorf("lakeTopNStrategy(%q)=%q want %q", in, got, want)
		}
	}
	if got := (&Node{}).lakeTopNStrategy(); got != lakeTopNStream {
		t.Errorf("nil config: got %q want %q", got, lakeTopNStream)
	}
}

func TestSqlHasNonTimestampFilter(t *testing.T) {
	tsOnly := "SELECT *, 'homer_lake' AS storage_lake FROM t WHERE timestamp >= (to_timestamp(1 / 1000.0) AT TIME ZONE 'UTC') AND timestamp <= (to_timestamp(2 / 1000.0) AT TIME ZONE 'UTC')"
	if sqlHasNonTimestampFilter(tsOnly) {
		t.Error("timestamp-only WHERE should not be a filter")
	}
	filtered := tsOnly + " AND (session_id LIKE '%x%' OR cid LIKE '%x%')"
	if !sqlHasNonTimestampFilter(filtered) {
		t.Error("session_id LIKE should be detected as a filter")
	}
	if sqlHasNonTimestampFilter("SELECT * FROM t") {
		t.Error("no WHERE should not be a filter")
	}

	// A filtered long-range query must NOT be split (single efficient scan).
	ob, lim, base := extractOrderLimit(filtered + " ORDER BY timestamp DESC LIMIT 50")
	if shouldSplitLakeAndMem(filtered, base, ob, lim) {
		t.Error("filtered query should not be split (top-N path)")
	}
}

func TestLakeChunkMs(t *testing.T) {
	if got := (&Node{}).lakeChunkMs(); got != defaultLakeTimeChunkMs {
		t.Errorf("nil config: got %d want %d", got, defaultLakeTimeChunkMs)
	}
	n := &Node{config: &config.NodeConfig{
		DuckLake: config.DuckLakeConfig{Search: config.SearchConfig{LakeChunkSec: 0}},
	}}
	if got := n.lakeChunkMs(); got != defaultLakeTimeChunkMs {
		t.Errorf("zero: got %d want %d", got, defaultLakeTimeChunkMs)
	}
	n.config.DuckLake.Search.LakeChunkSec = 300
	if got := n.lakeChunkMs(); got != 300_000 {
		t.Errorf("300s: got %d want 300000", got)
	}
}
