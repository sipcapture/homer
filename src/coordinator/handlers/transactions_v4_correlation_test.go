// Copyright (C) 2026 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package handlers

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/sipcapture/homer-core/src/config"
	"github.com/sipcapture/homer-core/src/coordinator/services"
)

func TestMergeSessionIDs_DedupsAndCaps(t *testing.T) {
	got := mergeSessionIDs([]string{"a", " b ", "a"}, []string{"b", "", "c"})
	want := []string{"a", "b", "c"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestMergeSessionIDs_RespectsCap(t *testing.T) {
	base := make([]string, maxTransactionSessionIDs)
	for i := range base {
		base[i] = "sid" + strconv.Itoa(i)
	}
	extras := []string{"extra1", "extra2"}
	got := mergeSessionIDs(base, extras)
	if len(got) != maxTransactionSessionIDs {
		t.Fatalf("expected len=%d (cap), got %d", maxTransactionSessionIDs, len(got))
	}
	for _, e := range extras {
		for _, g := range got {
			if g == e {
				t.Fatalf("extra %q should not be added once cap is reached", e)
			}
		}
	}
}

// fakeCorrelator lets us observe whether queryTransactionMessages consults
// the engine and with which inputs, without spinning up a real Lua VM.
type fakeCorrelator struct {
	hasReturn  bool
	called     int32
	correlates int32
	lastInput  CorrelationInput
	result     *CorrelationResult
}

func (f *fakeCorrelator) Has(hepid int, profile string) bool {
	atomic.AddInt32(&f.called, 1)
	return f.hasReturn
}

func (f *fakeCorrelator) Correlate(_ context.Context, in CorrelationInput) *CorrelationResult {
	atomic.AddInt32(&f.correlates, 1)
	f.lastInput = in
	return f.result
}

// newTestSearchHandler builds a SearchHandler wired to a local httptest
// server that stands in for a data node. Every POST /query call appends the
// received SQL to `capturedSQL` and replies with `rowsPerCall`. Index i in
// rowsPerCall maps to the i-th call; when callCount exceeds len(rowsPerCall)
// the last element is reused.
func newTestSearchHandler(t *testing.T, rowsPerCall [][]map[string]interface{}, capturedSQL *[]string) (*SearchHandler, func()) {
	t.Helper()

	var callIdx int32
	// /query lives on FlightServer.Port+1, so we spin one test server and
	// derive the "flight port" from its URL.
	queryMux := http.NewServeMux()
	queryMux.HandleFunc("/query", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			SQL string `json:"sql"`
		}
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		*capturedSQL = append(*capturedSQL, body.SQL)
		idx := int(atomic.AddInt32(&callIdx, 1) - 1)
		if idx >= len(rowsPerCall) {
			idx = len(rowsPerCall) - 1
		}
		rows := rowsPerCall[idx]
		resp := map[string]interface{}{
			"success": true,
			"data":    rows,
			"count":   len(rows),
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})
	// /health keeps ConnectAll happy.
	queryMux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	srv := httptest.NewServer(queryMux)
	u, err := url.Parse(srv.URL)
	if err != nil {
		srv.Close()
		t.Fatalf("parse server url: %v", err)
	}
	host := u.Hostname()
	httpPort, _ := strconv.Atoi(u.Port())
	// FlightService derives the HTTP port as flightPort+1, so we point it
	// to httpPort-1 as the "flight port".
	flightPort := httpPort - 1

	fs := services.NewFlightService([]config.NodeEndpoint{{
		Name: "n1", Host: host, Port: flightPort,
	}}, 0, false)
	_ = fs.ConnectAll()
	fs.SetLakeName("homer_lake")

	h := NewSearchHandler(fs, nil, &config.MCPConfig{}, nil, nil, 0, nil, "", 0)

	return h, func() {
		srv.Close()
		fs.CloseAll()
	}
}

func TestQueryTransactionMessages_CorrelationOff_NoSecondCall(t *testing.T) {
	var sqls []string
	h, done := newTestSearchHandler(t, [][]map[string]interface{}{
		{{"session_id": "s1"}},
	}, &sqls)
	defer done()

	// No correlator attached — behaviour must be unchanged.
	res, err := h.queryTransactionMessages(context.Background(), &TransactionSessionRequestV4{
		SessionIDs: []string{"s1"},
		ProtoType:  1,
		EventType:  "call",
	})
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if len(res) != 1 {
		t.Fatalf("expected 1 row, got %d", len(res))
	}
	if len(sqls) != 1 {
		t.Fatalf("expected 1 SQL call, got %d: %v", len(sqls), sqls)
	}
}

func TestQueryTransactionMessages_CorrelationOn_NoExtras_NoSecondCall(t *testing.T) {
	var sqls []string
	h, done := newTestSearchHandler(t, [][]map[string]interface{}{
		{{"session_id": "s1"}},
	}, &sqls)
	defer done()

	corr := &fakeCorrelator{hasReturn: true, result: &CorrelationResult{ExtraSessionIDs: nil}}
	h.SetCorrelator(corr)

	_, err := h.queryTransactionMessages(context.Background(), &TransactionSessionRequestV4{
		SessionIDs: []string{"s1"},
		ProtoType:  1,
		EventType:  "call",
	})
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if atomic.LoadInt32(&corr.correlates) != 1 {
		t.Fatalf("Correlate should be called exactly once, got %d", corr.correlates)
	}
	if len(sqls) != 1 {
		t.Fatalf("expected 1 SQL call when no extras, got %d", len(sqls))
	}
}

func TestQueryTransactionMessages_CorrelationOn_ExtrasTriggerSecondCall(t *testing.T) {
	var sqls []string
	// First call returns the base row, second call (with expanded ids) adds two more.
	h, done := newTestSearchHandler(t, [][]map[string]interface{}{
		{{"session_id": "s1"}},
		{{"session_id": "s1"}, {"session_id": "s2"}, {"session_id": "s3"}},
	}, &sqls)
	defer done()

	corr := &fakeCorrelator{hasReturn: true, result: &CorrelationResult{ExtraSessionIDs: []string{"s2", "s3"}}}
	h.SetCorrelator(corr)

	rows, err := h.queryTransactionMessages(context.Background(), &TransactionSessionRequestV4{
		SessionIDs: []string{"s1"},
		ProtoType:  1,
		EventType:  "call",
	})
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("expected 3 merged rows, got %d: %v", len(rows), rows)
	}
	if len(sqls) != 2 {
		t.Fatalf("expected 2 SQL calls (base + expanded), got %d: %v", len(sqls), sqls)
	}

	// The second SQL must reference the extra session_ids.
	second := sqls[1]
	for _, want := range []string{"s1", "s2", "s3"} {
		if !strings.Contains(second, "session_id = '"+want+"'") {
			t.Fatalf("expanded SQL missing %q:\n%s", want, second)
		}
	}

	// The input passed to Correlate should carry the base rows and the seed ids.
	gotIDs := append([]string(nil), corr.lastInput.SessionIDs...)
	sort.Strings(gotIDs)
	if !reflect.DeepEqual(gotIDs, []string{"s1"}) {
		t.Fatalf("Correlate seed ids = %v, want [s1]", gotIDs)
	}
	if corr.lastInput.HepID != 1 || corr.lastInput.Profile != "call" {
		t.Fatalf("Correlate key wrong: hepid=%d profile=%q", corr.lastInput.HepID, corr.lastInput.Profile)
	}
	if len(corr.lastInput.BaseRows) != 1 {
		t.Fatalf("Correlate BaseRows len = %d, want 1", len(corr.lastInput.BaseRows))
	}
}

func TestQueryTransactionMessages_CorrelationOn_NoKeyMatch_NoCorrelateCall(t *testing.T) {
	var sqls []string
	h, done := newTestSearchHandler(t, [][]map[string]interface{}{
		{{"session_id": "s1"}},
	}, &sqls)
	defer done()

	corr := &fakeCorrelator{hasReturn: false} // engine present but no script registered
	h.SetCorrelator(corr)

	_, err := h.queryTransactionMessages(context.Background(), &TransactionSessionRequestV4{
		SessionIDs: []string{"s1"},
		ProtoType:  1,
		EventType:  "call",
	})
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if atomic.LoadInt32(&corr.called) == 0 {
		t.Fatal("Has() must be consulted")
	}
	if atomic.LoadInt32(&corr.correlates) != 0 {
		t.Fatalf("Correlate must NOT be called when Has()=false, got %d", corr.correlates)
	}
	if len(sqls) != 1 {
		t.Fatalf("expected exactly 1 SQL call, got %d", len(sqls))
	}
}

func TestFetchTransactionRows_Correlate_ExpandsExportToCorrelatedLegs(t *testing.T) {
	var sqls []string
	// 1st call: base rows for the clicked callee leg (correlation input).
	// 2nd call: export query over the expanded set.
	h, done := newTestSearchHandler(t, [][]map[string]interface{}{
		{{"session_id": "b1"}},
		{{"session_id": "a"}, {"session_id": "b1"}, {"session_id": "b2"}},
	}, &sqls)
	defer done()

	corr := &fakeCorrelator{hasReturn: true, result: &CorrelationResult{ExtraSessionIDs: []string{"a", "b2"}}}
	h.SetCorrelator(corr)

	c := echo.New().NewContext(httptest.NewRequest(http.MethodPost, "/", nil), httptest.NewRecorder())
	rows, filename, err := h.fetchTransactionRowsWithRequest(c, &TransactionSessionRequestV4{
		SessionID: "b1",
		ProtoType: 1,
		EventType: "call",
		Correlate: true,
	}, "pcap")
	if err != nil {
		t.Fatalf("fetch failed: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}
	if len(sqls) != 2 {
		t.Fatalf("expected 2 SQL calls, got %d: %v", len(sqls), sqls)
	}
	for _, sid := range []string{"a", "b1", "b2"} {
		if !strings.Contains(sqls[1], "'"+sid+"'") {
			t.Fatalf("export SQL missing session %q: %s", sid, sqls[1])
		}
	}
	if filename != "homer_b1.pcap" {
		t.Fatalf("filename should follow the clicked session, got %q", filename)
	}
}

func TestFetchTransactionRows_CorrelateOff_IgnoresCorrelator(t *testing.T) {
	var sqls []string
	h, done := newTestSearchHandler(t, [][]map[string]interface{}{
		{{"session_id": "b1"}},
	}, &sqls)
	defer done()

	corr := &fakeCorrelator{hasReturn: true, result: &CorrelationResult{ExtraSessionIDs: []string{"a"}}}
	h.SetCorrelator(corr)

	c := echo.New().NewContext(httptest.NewRequest(http.MethodPost, "/", nil), httptest.NewRecorder())
	_, _, err := h.fetchTransactionRowsWithRequest(c, &TransactionSessionRequestV4{
		SessionID: "b1",
		ProtoType: 1,
		EventType: "call",
	}, "txt")
	if err != nil {
		t.Fatalf("fetch failed: %v", err)
	}
	if atomic.LoadInt32(&corr.correlates) != 0 {
		t.Fatalf("Correlate must not run without correlate=true")
	}
	if len(sqls) != 1 || strings.Contains(sqls[0], "'a'") {
		t.Fatalf("expected one query for b1 only, got %v", sqls)
	}
}

func TestExpandCorrelatedSessionIDs_FailOpen(t *testing.T) {
	ids := []string{"b1"}
	req := &TransactionSessionRequestV4{ProtoType: 1, EventType: "call", Correlate: true}

	t.Run("no correlator", func(t *testing.T) {
		var sqls []string
		h, done := newTestSearchHandler(t, [][]map[string]interface{}{{{"session_id": "b1"}}}, &sqls)
		defer done()
		if got := h.expandCorrelatedSessionIDs(context.Background(), req, ids); !reflect.DeepEqual(got, ids) {
			t.Fatalf("got %v", got)
		}
		if len(sqls) != 0 {
			t.Fatalf("no query expected, got %v", sqls)
		}
	})

	t.Run("event_type all skips correlation", func(t *testing.T) {
		var sqls []string
		h, done := newTestSearchHandler(t, [][]map[string]interface{}{{{"session_id": "b1"}}}, &sqls)
		defer done()
		corr := &fakeCorrelator{hasReturn: true, result: &CorrelationResult{ExtraSessionIDs: []string{"a"}}}
		h.SetCorrelator(corr)
		allReq := *req
		allReq.EventType = "all"
		if got := h.expandCorrelatedSessionIDs(context.Background(), &allReq, ids); !reflect.DeepEqual(got, ids) {
			t.Fatalf("got %v", got)
		}
		if atomic.LoadInt32(&corr.correlates) != 0 {
			t.Fatalf("Correlate must not run for event_type all")
		}
	})

	t.Run("script not registered", func(t *testing.T) {
		var sqls []string
		h, done := newTestSearchHandler(t, [][]map[string]interface{}{{{"session_id": "b1"}}}, &sqls)
		defer done()
		h.SetCorrelator(&fakeCorrelator{hasReturn: false})
		if got := h.expandCorrelatedSessionIDs(context.Background(), req, ids); !reflect.DeepEqual(got, ids) {
			t.Fatalf("got %v", got)
		}
	})

	t.Run("nil result", func(t *testing.T) {
		var sqls []string
		h, done := newTestSearchHandler(t, [][]map[string]interface{}{{{"session_id": "b1"}}}, &sqls)
		defer done()
		h.SetCorrelator(&fakeCorrelator{hasReturn: true, result: nil})
		if got := h.expandCorrelatedSessionIDs(context.Background(), req, ids); !reflect.DeepEqual(got, ids) {
			t.Fatalf("got %v", got)
		}
	})
}

func TestTransactionSessionRequestV4_correlateJSON(t *testing.T) {
	var req TransactionSessionRequestV4
	if err := json.Unmarshal([]byte(`{"session_id":"x","correlate":true}`), &req); err != nil {
		t.Fatal(err)
	}
	if !req.Correlate {
		t.Fatal("correlate not parsed")
	}
}

func TestShareExportPayload_correlateRoundTrip(t *testing.T) {
	// Share links store the raw POST body and replay it through the same fetch path.
	var sqls []string
	h, done := newTestSearchHandler(t, [][]map[string]interface{}{
		{{"session_id": "b1"}},
		{{"session_id": "a"}, {"session_id": "b1"}},
	}, &sqls)
	defer done()
	h.SetCorrelator(&fakeCorrelator{hasReturn: true, result: &CorrelationResult{ExtraSessionIDs: []string{"a"}}})

	stored := []byte(`{"session_id":"b1","proto_type":1,"event_type":"call","correlate":true}`)
	var req TransactionSessionRequestV4
	if err := json.Unmarshal(stored, &req); err != nil {
		t.Fatal(err)
	}
	c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/", nil), httptest.NewRecorder())
	rows, _, err := h.fetchTransactionRowsWithRequest(c, &req, "txt")
	if err != nil {
		t.Fatalf("replay failed: %v", err)
	}
	if len(rows) != 2 || !strings.Contains(sqls[len(sqls)-1], "'a'") {
		t.Fatalf("share replay did not include correlated leg: rows=%d sqls=%v", len(rows), sqls)
	}
}

func TestExpandCorrelatedSessionIDs_BaseQueryFailureFailsOpen(t *testing.T) {
	// A node that answers every query with an error: QueryWithRange returns an error
	// because no node succeeded.
	mux := http.NewServeMux()
	mux.HandleFunc("/query", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	})
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	srv := httptest.NewServer(mux)
	defer srv.Close()
	u, _ := url.Parse(srv.URL)
	httpPort, _ := strconv.Atoi(u.Port())
	fs := services.NewFlightService([]config.NodeEndpoint{{Name: "n1", Host: u.Hostname(), Port: httpPort - 1}}, 0, false)
	_ = fs.ConnectAll()
	defer fs.CloseAll()
	fs.SetLakeName("homer_lake")
	h := NewSearchHandler(fs, nil, &config.MCPConfig{}, nil, nil, 0, nil, "", 0)
	corr := &fakeCorrelator{hasReturn: true, result: &CorrelationResult{ExtraSessionIDs: []string{"a"}}}
	h.SetCorrelator(corr)

	ids := []string{"b1"}
	got := h.expandCorrelatedSessionIDs(context.Background(),
		&TransactionSessionRequestV4{ProtoType: 1, EventType: "call", Correlate: true}, ids)
	if !reflect.DeepEqual(got, ids) {
		t.Fatalf("expected requested ids on base query failure, got %v", got)
	}
	if atomic.LoadInt32(&corr.correlates) != 0 {
		t.Fatalf("Correlate must not run when the base query failed")
	}
}

func TestExpandCorrelatedSessionIDs_PassesBaseRowsAndWindowToCorrelator(t *testing.T) {
	var sqls []string
	h, done := newTestSearchHandler(t, [][]map[string]interface{}{{{"session_id": "b1", "x_call_id": "a"}}}, &sqls)
	defer done()
	corr := &fakeCorrelator{hasReturn: true, result: &CorrelationResult{ExtraSessionIDs: []string{"a", "b2"}}}
	h.SetCorrelator(corr)

	req := &TransactionSessionRequestV4{ProtoType: 1, EventType: "call", Correlate: true}
	req.Timestamp.From, req.Timestamp.To = 1000, 2000
	got := h.expandCorrelatedSessionIDs(context.Background(), req, []string{"b1"})

	if want := []string{"b1", "a", "b2"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	in := corr.lastInput
	if len(in.BaseRows) != 1 || in.TimeFrom != 1000 || in.TimeTo != 2000 || !reflect.DeepEqual(in.SessionIDs, []string{"b1"}) {
		t.Fatalf("unexpected correlator input: %+v", in)
	}
}

func TestV4TransactionExportText_Correlate_ThroughHandler(t *testing.T) {
	var sqls []string
	h, done := newTestSearchHandler(t, [][]map[string]interface{}{
		{{"session_id": "b1"}},
		{
			{"session_id": "a", "src_ip": "10.0.0.1", "dst_ip": "10.0.0.2", "src_port": 5060, "dst_port": 5060, "payload": "INVITE A"},
			{"session_id": "b1", "src_ip": "10.0.0.2", "dst_ip": "10.0.0.11", "src_port": 5060, "dst_port": 5060, "payload": "INVITE B1"},
		},
	}, &sqls)
	defer done()
	h.SetCorrelator(&fakeCorrelator{hasReturn: true, result: &CorrelationResult{ExtraSessionIDs: []string{"a"}}})

	post := func(body string) string {
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		if err := h.V4TransactionExportText(echo.New().NewContext(req, rec)); err != nil {
			t.Fatalf("handler error: %v", err)
		}
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
		}
		return rec.Body.String()
	}

	out := post(`{"session_id":"b1","proto_type":1,"event_type":"call","correlate":true}`)
	if !strings.Contains(out, "INVITE A") || !strings.Contains(out, "INVITE B1") {
		t.Fatalf("correlated export missing legs: %q", out)
	}
}
