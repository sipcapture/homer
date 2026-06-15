package node

import (
	"testing"

	"github.com/sipcapture/homer-core/src/config"
)

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
		"":        lakeTopNChunked,
		"chunked": lakeTopNChunked,
		"stream":  lakeTopNStream,
		"full":    lakeTopNFull,
		"bogus":   lakeTopNChunked,
	}
	for in, want := range cases {
		n := &Node{config: &config.NodeConfig{
			DuckLake: config.DuckLakeConfig{Search: config.SearchConfig{LakeTopNStrategy: in}},
		}}
		if got := n.lakeTopNStrategy(); got != want {
			t.Errorf("lakeTopNStrategy(%q)=%q want %q", in, got, want)
		}
	}
	if got := (&Node{}).lakeTopNStrategy(); got != lakeTopNChunked {
		t.Errorf("nil config: got %q want %q", got, lakeTopNChunked)
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
