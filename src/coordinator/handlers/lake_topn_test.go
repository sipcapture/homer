package handlers

import (
	"testing"
	"time"
)

func TestSearchHandlerLakeTopNStrategy(t *testing.T) {
	cases := map[string]string{
		"":        topNStream,
		"stream":  topNStream,
		"chunked": topNChunked,
		"full":    topNFull,
		"bogus":   topNStream,
	}
	for in, want := range cases {
		h := &SearchHandler{lakeTopNStrategyCfg: in}
		if got := h.lakeTopNStrategy(); got != want {
			t.Errorf("lakeTopNStrategy(%q)=%q want %q", in, got, want)
		}
	}
}

func TestOuterChunkMs(t *testing.T) {
	if got := (&SearchHandler{}).outerChunkMs(); got != transactionSearchChunkMs {
		t.Errorf("default got %d want %d (24h)", got, transactionSearchChunkMs)
	}
	if got := (&SearchHandler{lakeChunkSec: 0}).outerChunkMs(); got != transactionSearchChunkMs {
		t.Errorf("zero got %d want %d (24h)", got, transactionSearchChunkMs)
	}
	if got := (&SearchHandler{lakeChunkSec: 300}).outerChunkMs(); got != 300_000 {
		t.Errorf("300s got %d want 300000", got)
	}
}

func TestTransactionSearchTopNShape(t *testing.T) {
	mk := func(sel, group, order string) *SearchObjectV4 {
		r := &SearchObjectV4{}
		r.Param.Select = sel
		r.Param.GroupBy = group
		r.Param.OrderBy = order
		return r
	}
	if !transactionSearchTopNShape(mk("", "", "")) {
		t.Error("default top-N shape should be true")
	}
	if !transactionSearchTopNShape(mk("", "", "timestamp DESC")) {
		t.Error("timestamp DESC should be true")
	}
	if transactionSearchTopNShape(mk("count(*)", "", "")) {
		t.Error("custom select should be false")
	}
	if transactionSearchTopNShape(mk("", "method", "")) {
		t.Error("group by should be false")
	}
	if transactionSearchTopNShape(mk("", "", "caller ASC")) {
		t.Error("custom order should be false")
	}
}

func TestSortRowsByTimestampDescLimit(t *testing.T) {
	base := time.Date(2026, 6, 15, 10, 0, 0, 0, time.UTC)
	rows := []map[string]interface{}{
		{"timestamp": base.Add(1 * time.Minute), "id": 2},
		{"timestamp": "2026-06-15 10:03:00", "id": 4},
		{"timestamp": base.Add(0 * time.Minute), "id": 1},
		{"timestamp": base.Add(2 * time.Minute), "id": 3},
	}
	got := sortRowsByTimestampDescLimit(rows, 2)
	if len(got) != 2 || got[0]["id"] != 4 || got[1]["id"] != 3 {
		t.Errorf("got %v,%v want newest 4,3", get(got, 0), get(got, 1))
	}
}

func get(rows []map[string]interface{}, i int) interface{} {
	if i < len(rows) {
		return rows[i]["id"]
	}
	return nil
}
