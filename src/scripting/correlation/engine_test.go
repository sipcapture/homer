// Copyright (C) 2026 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package correlation

import (
	"context"
	"errors"
	"reflect"
	"sort"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sipcapture/homer-core/src/config"
	"github.com/sipcapture/homer-core/src/coordinator/sqlvalidator"
)

// fakeExecutor records SQL it receives and returns a scripted response.
type fakeExecutor struct {
	called     int32
	lastSQL    string
	returnErr  error
	returnRows []map[string]interface{}
}

func (f *fakeExecutor) Query(_ context.Context, sql string) ([]map[string]interface{}, error) {
	atomic.AddInt32(&f.called, 1)
	f.lastSQL = sql
	if f.returnErr != nil {
		return nil, f.returnErr
	}
	return f.returnRows, nil
}

// fakeLoader serves a static in-memory script list.
type fakeLoader struct {
	items []LoadedScript
	err   error
}

func (f *fakeLoader) LoadActiveCorrelation(_ context.Context) ([]LoadedScript, error) {
	return f.items, f.err
}

func newTestEngine(t *testing.T, script string, exec SQLExecutor) *CorrelationEngine {
	t.Helper()
	cfg := config.CorrelationScriptingConfig{
		Enable:          true,
		SQLTimeoutMS:    500,
		ScriptTimeoutMS: 1500,
		MaxSQLCalls:     4,
		MaxSQLRows:      64,
		SyncIntervalSec: 0,
	}
	loader := &fakeLoader{items: []LoadedScript{{HepID: 1, Profile: "call", Script: script, GUID: "t1"}}}
	e := NewEngine(cfg, loader, exec, "homer_lake")
	if e == nil {
		t.Fatalf("NewEngine returned nil despite Enable=true")
	}
	if err := e.ReloadFromDB(context.Background()); err != nil {
		t.Fatalf("ReloadFromDB: %v", err)
	}
	return e
}

func TestEngineDisabledReturnsNil(t *testing.T) {
	cfg := config.CorrelationScriptingConfig{Enable: false}
	if e := NewEngine(cfg, nil, nil, ""); e != nil {
		t.Fatalf("expected nil engine when disabled, got %#v", e)
	}
}

func TestCorrelateReturnsExtras(t *testing.T) {
	script := `
function correlate(data, nodes)
  return {"abc", "def", "abc"} -- duplicate must be removed
end
`
	e := newTestEngine(t, script, nil)

	res := e.Correlate(context.Background(), CorrelationInput{HepID: 1, Profile: "call"})
	if res == nil {
		t.Fatal("expected non-nil result")
	}

	got := append([]string(nil), res.ExtraSessionIDs...)
	sort.Strings(got)
	want := []string{"abc", "def"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("extras = %v, want %v", got, want)
	}
}

func TestCorrelateScriptRuntimeError(t *testing.T) {
	// error() during call — Correlate must swallow and return nil.
	script := `
function correlate(data, nodes)
  error("boom")
end
`
	e := newTestEngine(t, script, nil)
	if res := e.Correlate(context.Background(), CorrelationInput{HepID: 1, Profile: "call"}); res != nil {
		t.Fatalf("expected nil on script error, got %#v", res)
	}
}

func TestCorrelateMissingCorrelateFunction(t *testing.T) {
	// DoString succeeds but `correlate` is absent — must not crash.
	e := newTestEngine(t, `local x = 1`, nil)
	if res := e.Correlate(context.Background(), CorrelationInput{HepID: 1, Profile: "call"}); res != nil {
		t.Fatalf("expected nil when correlate() is missing, got %#v", res)
	}
}

func TestCorrelateExecuteSQLForwardsAndBlocks(t *testing.T) {
	exec := &fakeExecutor{returnRows: []map[string]interface{}{{"session_id": "X"}, {"session_id": "Y"}}}

	script := `
function correlate(data, nodes)
  local rows = executeSQL("SELECT session_id FROM homer_lake.hep_proto_1_call WHERE x_call_id = 'abc'")
  local bad  = executeSQL("UPDATE hep_proto_1_call SET x=1") -- must be rejected
  local out = {}
  if rows ~= nil then
    for i = 1, #rows do
      out[#out+1] = rows[i].session_id
    end
  end
  if bad ~= nil and #bad > 0 then
    out[#out+1] = "LEAKED"
  end
  return out
end
`
	e := newTestEngine(t, script, exec)
	res := e.Correlate(context.Background(), CorrelationInput{HepID: 1, Profile: "call"})
	if res == nil {
		t.Fatal("expected result")
	}

	got := append([]string(nil), res.ExtraSessionIDs...)
	sort.Strings(got)
	want := []string{"X", "Y"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
	if atomic.LoadInt32(&exec.called) != 1 {
		t.Fatalf("executor must run exactly once (blocked SQL should skip Query); got %d", exec.called)
	}
}

func TestCorrelateRespectsMaxSQLCalls(t *testing.T) {
	exec := &fakeExecutor{returnRows: []map[string]interface{}{{"session_id": "X"}}}
	script := `
function correlate(data, nodes)
  for i = 1, 20 do
    executeSQL("SELECT session_id FROM homer_lake.hep_proto_1_call LIMIT 1")
  end
  return {}
end
`
	e := newTestEngine(t, script, exec)
	_ = e.Correlate(context.Background(), CorrelationInput{HepID: 1, Profile: "call"})
	if got := atomic.LoadInt32(&exec.called); got > int32(e.cfg.MaxSQLCalls) {
		t.Fatalf("executor called %d times, must be <= MaxSQLCalls=%d", got, e.cfg.MaxSQLCalls)
	}
}

func TestCorrelateSQLExecutorErrorIsSwallowed(t *testing.T) {
	exec := &fakeExecutor{returnErr: errors.New("node down")}
	// Lua scripts must use #rows == 0 rather than `== nil` because luar
	// proxies a Go nil slice as userdata, which is not Lua nil.
	script := `
function correlate(data, nodes)
  local rows = executeSQL("SELECT 1")
  if rows == nil or #rows == 0 then
    return {"fallback"}
  end
  return {"unreachable"}
end
`
	e := newTestEngine(t, script, exec)
	res := e.Correlate(context.Background(), CorrelationInput{HepID: 1, Profile: "call"})
	if res == nil || len(res.ExtraSessionIDs) != 1 || res.ExtraSessionIDs[0] != "fallback" {
		t.Fatalf("expected fallback result, got %#v", res)
	}
}

func TestCorrelateReadsBaseRows(t *testing.T) {
	script := `
function correlate(data, nodes)
  local out = {}
  for i = 1, #data do
    local xcid = data[i].x_call_id
    if xcid ~= nil then
      out[#out+1] = xcid
    end
  end
  return out
end
`
	e := newTestEngine(t, script, nil)
	res := e.Correlate(context.Background(), CorrelationInput{
		HepID:   1,
		Profile: "call",
		BaseRows: []map[string]interface{}{
			{"session_id": "s1", "x_call_id": "A"},
			{"session_id": "s2", "x_call_id": "B"},
			{"session_id": "s3"},
		},
	})
	if res == nil {
		t.Fatal("expected result")
	}
	got := append([]string(nil), res.ExtraSessionIDs...)
	sort.Strings(got)
	want := []string{"A", "B"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

// TestCorrelateReceivesContext verifies that the third argument passed to
// correlate() carries the request-level context (timerange, session_ids,
// hepid, profile). A script can "export" these values by returning them
// as fake session_ids, which lets us assert on exact values.
func TestCorrelateReceivesContext(t *testing.T) {
	script := `
function correlate(data, nodes, ctx)
  if type(ctx) ~= "table" then
    return {"ctx_missing"}
  end
  local out = {}
  out[#out+1] = "from=" .. tostring(ctx.time_from)
  out[#out+1] = "to="   .. tostring(ctx.time_to)
  out[#out+1] = "hepid=" .. tostring(ctx.hepid)
  out[#out+1] = "profile=" .. tostring(ctx.profile)
  out[#out+1] = "proto_type=" .. tostring(ctx.proto_type)
  out[#out+1] = "event_type=" .. tostring(ctx.event_type)
  if type(ctx.session_ids) == "table" then
    for i = 1, #ctx.session_ids do
      out[#out+1] = "sid=" .. tostring(ctx.session_ids[i])
    end
  end
  return out
end
`
	e := newTestEngine(t, script, nil)
	res := e.Correlate(context.Background(), CorrelationInput{
		HepID:      1,
		Profile:    "call",
		ProtoType:  1,
		EventType:  "call",
		SessionIDs: []string{"sidA", "sidB"},
		TimeFrom:   1700000000000,
		TimeTo:     1700000060000,
	})
	if res == nil {
		t.Fatal("expected non-nil result")
	}
	got := append([]string(nil), res.ExtraSessionIDs...)
	sort.Strings(got)
	want := []string{
		"event_type=call",
		"from=1700000000000",
		"hepid=1",
		"profile=call",
		"proto_type=1",
		"sid=sidA",
		"sid=sidB",
		"to=1700000060000",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("context exposed to Lua differs:\n got  %v\n want %v", got, want)
	}
}

// TestCorrelateLegacy2ArgScriptStillWorks asserts that scripts written
// against the pre-ctx signature `function correlate(data, nodes)` keep
// working — Lua silently drops extra arguments, so the engine passing a
// third argument must not break them.
func TestCorrelateLegacy2ArgScriptStillWorks(t *testing.T) {
	script := `
function correlate(data, nodes)
  return {"legacy_ok"}
end
`
	e := newTestEngine(t, script, nil)
	res := e.Correlate(context.Background(), CorrelationInput{
		HepID:    1,
		Profile:  "call",
		TimeFrom: 1, TimeTo: 2,
	})
	if res == nil || len(res.ExtraSessionIDs) != 1 || res.ExtraSessionIDs[0] != "legacy_ok" {
		t.Fatalf("legacy 2-arg script broken: %#v", res)
	}
}

func TestHasLookup(t *testing.T) {
	e := newTestEngine(t, `function correlate(d, n) return {} end`, nil)
	if !e.Has(1, "call") {
		t.Fatal("Has(1, 'call') expected true after reload")
	}
	if e.Has(1, "CALL") {
		// Key is normalised to lowercase; the handler will pass lowercase.
		// Documenting the behaviour here.
	}
	if e.Has(99, "call") {
		t.Fatal("Has(99, 'call') expected false")
	}
}

func TestSyncLoopStopsOnCancel(t *testing.T) {
	e := newTestEngine(t, `function correlate(d, n) return {} end`, nil)
	e.cfg.SyncIntervalSec = 1
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		e.SyncLoop(ctx)
		close(done)
	}()
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("SyncLoop did not exit within 2s after context cancel")
	}
}

func TestDedupNonEmpty(t *testing.T) {
	got := dedupNonEmpty([]string{" a ", "b", "", "a", "  "})
	want := []string{"a", "b"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestIsSafeIdent(t *testing.T) {
	cases := map[string]bool{
		"x_call_id":       true,
		"session_id":      true,
		"Col_123":         true,
		"drop table x":    false,
		"id; DROP":        false,
		"":                false,
		"a'b":             false,
		"x.y":             false,
		"col-with-hyphen": false,
	}
	for in, want := range cases {
		if got := isSafeIdent(in); got != want {
			t.Fatalf("isSafeIdent(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestEnsureLimitAppendsOnlyWhenMissing(t *testing.T) {
	cases := []struct {
		in  string
		max int
		out string
	}{
		{"SELECT * FROM t", 10, "SELECT * FROM t LIMIT 10"},
		{"SELECT * FROM t LIMIT 5", 10, "SELECT * FROM t LIMIT 5"},
		{"WITH c AS (SELECT 1) SELECT * FROM c", 7, "WITH c AS (SELECT 1) SELECT * FROM c LIMIT 7"},
		{"SHOW TABLES", 10, "SHOW TABLES"}, // not LIMIT-able
	}
	for _, c := range cases {
		if got := sqlvalidator.EnsureLimit(c.in, c.max); got != c.out {
			t.Fatalf("EnsureLimit(%q, %d) = %q, want %q", c.in, c.max, got, c.out)
		}
	}
}
