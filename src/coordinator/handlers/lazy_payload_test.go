package handlers

import (
	"strings"
	"testing"
)

func TestLazyPayloadEligible(t *testing.T) {
	mk := func(sel, group string, proto int) *SearchObjectV4 {
		r := &SearchObjectV4{}
		r.Param.Select = sel
		r.Param.GroupBy = group
		r.Filter.ProtoType = proto
		return r
	}
	cases := []struct {
		name string
		on   bool
		req  *SearchObjectV4
		want bool
	}{
		{"default SIP", true, mk("", "", 1), true},
		{"proto unset defaults to SIP", true, mk("", "", 0), true},
		{"disabled by flag", false, mk("", "", 1), false},
		{"custom select", true, mk("uuid, caller", "", 1), false},
		{"group by", true, mk("", "method", 1), false},
	}
	for _, c := range cases {
		h := &SearchHandler{lazyPayloadHydration: c.on}
		if got := h.lazyPayloadEligible(c.req); got != c.want {
			t.Errorf("%s: lazyPayloadEligible=%v want %v", c.name, got, c.want)
		}
	}
}

func TestLazyPayloadEligibleOTLPAndLP(t *testing.T) {
	h := &SearchHandler{lazyPayloadHydration: true}
	// OTLP / LP virtual proto types have no uuid/payload pair and must be excluded.
	for _, proto := range []int{otlpHepIDTraces, otlpHepIDMetrics, otlpHepIDLogs, lpHepID} {
		req := &SearchObjectV4{}
		req.Filter.ProtoType = proto
		if h.lazyPayloadEligible(req) {
			t.Errorf("proto %d (OTLP/LP) should not be eligible", proto)
		}
	}
}

func TestNarrowProjectionUUIDTimestampOnly(t *testing.T) {
	req := &SearchObjectV4{}
	req.Filter.ProtoType = 1
	req.Timestamp.From = 1_700_000_000_000
	req.Timestamp.To = 1_700_003_600_000
	req.Param.Limit = 50

	sql, err := buildSearchSQLV4WithOpts("homer_lake", req, nil, searchSQLOpts{narrowNoHeavy: true})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	wantProj := "SELECT " + narrowProjectionExpr + " FROM"
	if !strings.Contains(sql, wantProj) {
		t.Errorf("narrow SQL missing %q: %s", wantProj, sql)
	}
	if strings.Contains(sql, "caller") || strings.Contains(sql, "session_id") {
		t.Errorf("narrow SQL must not project light columns: %s", sql)
	}

	// Without the opt the default SELECT * is preserved.
	full, err := buildSearchSQLV4WithOpts("homer_lake", req, nil, searchSQLOpts{})
	if err != nil {
		t.Fatalf("build full: %v", err)
	}
	if !strings.Contains(full, "SELECT * FROM") {
		t.Errorf("default SQL should select *: %s", full)
	}
}

func TestNarrowProjectionIgnoredWithCustomSelect(t *testing.T) {
	req := &SearchObjectV4{}
	req.Filter.ProtoType = 1
	req.Param.Select = "uuid, caller, callee"
	req.Param.Limit = 50

	sql, err := buildSearchSQLV4WithOpts("homer_lake", req, nil, searchSQLOpts{narrowNoHeavy: true})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if strings.Contains(sql, narrowProjectionExpr) {
		t.Errorf("custom select must win over narrow projection: %s", sql)
	}
	if !strings.Contains(sql, "uuid, caller, callee") {
		t.Errorf("custom select columns missing: %s", sql)
	}
}
