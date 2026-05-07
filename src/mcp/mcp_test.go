package mcp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/sipcapture/homer-core/src/config"
)

func newTestModule(t *testing.T) *Module {
	t.Helper()
	mod, err := New(&config.MCPConfig{
		Mode:            "hybrid",
		HomerBaseURL:    "http://127.0.0.1:8080",
		HomerToken:      "token",
		DefaultLimit:    100,
		SQLDefaultLimit: 100,
	})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	return mod
}

func TestBuildStructuredPayloadInviteLastHour(t *testing.T) {
	mod := newTestModule(t)
	now := int64(1740656400000)
	payload, normalized := mod.buildStructuredPayload("find INVITE in the last hour src ip 10.1.2.3", now, 200)

	if payload.Filter.Method != "INVITE" {
		t.Fatalf("expected method INVITE, got %q", payload.Filter.Method)
	}
	if payload.Filter.SrcIP != "10.1.2.3" {
		t.Fatalf("expected src_ip 10.1.2.3, got %q", payload.Filter.SrcIP)
	}
	if payload.Timestamp.From != now-60*60*1000 {
		t.Fatalf("unexpected timestamp.from: %d", payload.Timestamp.From)
	}
	if payload.Timestamp.To != now {
		t.Fatalf("unexpected timestamp.to: %d", payload.Timestamp.To)
	}
	if payload.Param.Limit != 200 {
		t.Fatalf("expected limit 200, got %d", payload.Param.Limit)
	}
	if normalized["time_range"] != "last_hour" {
		t.Fatalf("expected time_range=last_hour, got %#v", normalized["time_range"])
	}
}

func TestValidateSQLAllowsCallTableName(t *testing.T) {
	sql := "SELECT * FROM homer_lake.main.hep_proto_1_call WHERE method = 'INVITE' ORDER BY timestamp DESC LIMIT 10"
	if err := validateSQL(sql); err != nil {
		t.Fatalf("expected SQL to be valid, got error: %v", err)
	}
}

func TestValidateSQLRejectsSemicolon(t *testing.T) {
	sql := "SELECT * FROM homer_lake.main.hep_proto_1_call WHERE method = 'INVITE';"
	if err := validateSQL(sql); err == nil {
		t.Fatalf("expected semicolon SQL to be rejected")
	}
}

func TestValidateSQLRejectsDropToken(t *testing.T) {
	sql := "SELECT * FROM homer_lake.main.hep_proto_1_call WHERE note = 'drop table'"
	if err := validateSQL(sql); err == nil {
		t.Fatalf("expected DROP token SQL to be rejected")
	}
}

func TestValidateSQLRejectsForeignTable(t *testing.T) {
	sql := "SELECT * FROM some_other.table WHERE method = 'INVITE'"
	if err := validateSQL(sql); err == nil {
		t.Fatalf("expected foreign table SQL to be rejected")
	}
}

func TestRunHybridAutoModeRouting(t *testing.T) {
	var (
		mu      sync.Mutex
		visited []string
	)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		visited = append(visited, r.URL.Path)
		mu.Unlock()

		if r.Header.Get("Authorization") != "Bearer token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"items": []map[string]any{{"id": 1}},
				"keys":  []string{"id"},
			},
			"meta": map[string]any{"ok": true},
		})
	})

	ts := httptest.NewServer(handler)
	defer ts.Close()

	mod, err := New(&config.MCPConfig{
		Mode:            "hybrid",
		HomerBaseURL:    ts.URL,
		HomerToken:      "token",
		DefaultLimit:    100,
		SQLDefaultLimit: 100,
	})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	_, err = mod.runHybrid(context.Background(), hybridArgs{QueryText: "find INVITE in the last hour", Mode: "auto"})
	if err != nil {
		t.Fatalf("runHybrid structured error: %v", err)
	}
	_, err = mod.runHybrid(context.Background(), hybridArgs{QueryText: "show sql INVITE in the last hour", Mode: "auto"})
	if err != nil {
		t.Fatalf("runHybrid sql error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(visited) != 2 {
		t.Fatalf("expected 2 API calls, got %d", len(visited))
	}
	if visited[0] != "/api/v4/transactions/search" {
		t.Fatalf("expected first call to /api/v4/transactions/search, got %s", visited[0])
	}
	if visited[1] != "/api/v4/query" {
		t.Fatalf("expected second call to /api/v4/query, got %s", visited[1])
	}
}

func TestParseQueryRegexOnlyWhenLLMDisabled(t *testing.T) {
	mod := newTestModule(t)
	if mod.llm != nil {
		t.Fatalf("expected nil llm client, got %#v", mod.llm)
	}

	_, _, meta, err := mod.parseQuery(context.Background(), "find INVITE in the last hour", 1740656400000, 0, "auto")
	if err != nil {
		t.Fatalf("parseQuery error: %v", err)
	}
	if meta.Used != parserRegex {
		t.Fatalf("expected parser_used=regex, got %q", meta.Used)
	}
}

func TestParseQueryStrictLLMErrorsWhenDisabled(t *testing.T) {
	mod := newTestModule(t)
	_, _, _, err := mod.parseQuery(context.Background(), "show me bye", 1740656400000, 0, "llm")
	if err == nil {
		t.Fatal("expected error when parser=llm and LLM is disabled")
	}
}

func newModuleWithLLM(t *testing.T, llmURL string) *Module {
	t.Helper()
	mod, err := New(&config.MCPConfig{
		Mode:            "hybrid",
		HomerBaseURL:    "http://127.0.0.1:8080",
		HomerToken:      "token",
		DefaultLimit:    100,
		SQLDefaultLimit: 100,
		LLM: config.MCPLLMConfig{
			Enable:     true,
			BaseURL:    llmURL,
			APIKey:     "k",
			Model:      "test-model",
			TimeoutSec: 5,
		},
	})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	if mod.llm == nil {
		t.Fatal("expected non-nil llm client")
	}
	return mod
}

func TestParseQueryUsesLLMWhenAvailable(t *testing.T) {
	llm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, chatJSON(`{"method":"BYE","src_ip":"9.9.9.9"}`))
	}))
	defer llm.Close()

	mod := newModuleWithLLM(t, llm.URL)
	payload, normalized, meta, err := mod.parseQuery(context.Background(), "give me everything", 1740656400000, 0, "auto")
	if err != nil {
		t.Fatalf("parseQuery error: %v", err)
	}
	if meta.Used != parserLLM {
		t.Fatalf("expected parser_used=llm, got %q", meta.Used)
	}
	if meta.Model != "test-model" {
		t.Fatalf("expected model=test-model, got %q", meta.Model)
	}
	if payload.Filter.Method != "BYE" {
		t.Fatalf("expected method=BYE, got %q", payload.Filter.Method)
	}
	if payload.Filter.SrcIP != "9.9.9.9" {
		t.Fatalf("expected src_ip=9.9.9.9, got %q", payload.Filter.SrcIP)
	}
	if normalized["method"] != "BYE" {
		t.Fatalf("expected normalized.method=BYE, got %#v", normalized["method"])
	}
}

func TestParseQueryFallsBackOnLLMFailure(t *testing.T) {
	llm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer llm.Close()

	mod := newModuleWithLLM(t, llm.URL)
	payload, _, meta, err := mod.parseQuery(context.Background(), "find INVITE in the last hour src ip 10.1.2.3", 1740656400000, 0, "auto")
	if err != nil {
		t.Fatalf("parseQuery error: %v", err)
	}
	if meta.Used != "regex_fallback" {
		t.Fatalf("expected parser_used=regex_fallback, got %q", meta.Used)
	}
	if meta.Error == "" {
		t.Fatal("expected non-empty meta.error after fallback")
	}
	if payload.Filter.Method != "INVITE" || payload.Filter.SrcIP != "10.1.2.3" {
		t.Fatalf("regex fallback did not extract expected fields: %+v", payload.Filter)
	}
}

func TestParseQueryRegexOverrideIgnoresLLM(t *testing.T) {
	called := false
	llm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, chatJSON(`{"method":"BYE"}`))
	}))
	defer llm.Close()

	mod := newModuleWithLLM(t, llm.URL)
	payload, _, meta, err := mod.parseQuery(context.Background(), "find INVITE last hour", 1740656400000, 0, "regex")
	if err != nil {
		t.Fatalf("parseQuery error: %v", err)
	}
	if called {
		t.Fatal("expected LLM to not be called when parser=regex")
	}
	if meta.Used != parserRegex {
		t.Fatalf("expected parser_used=regex, got %q", meta.Used)
	}
	if payload.Filter.Method != "INVITE" {
		t.Fatalf("expected method=INVITE from regex, got %q", payload.Filter.Method)
	}
}

func TestParseQueryStrictLLMErrorsWhenLLMFails(t *testing.T) {
	llm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "nope", http.StatusBadGateway)
	}))
	defer llm.Close()

	mod := newModuleWithLLM(t, llm.URL)
	_, _, _, err := mod.parseQuery(context.Background(), "anything", 1740656400000, 0, "llm")
	if err == nil {
		t.Fatal("expected error when parser=llm and LLM fails")
	}
}

func TestParseQueryLLMTimeRangeOverridesRegex(t *testing.T) {
	llm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, chatJSON(`{"method":"INVITE","from_ms":1000,"to_ms":5000,"time_range_label":"custom_range"}`))
	}))
	defer llm.Close()

	mod := newModuleWithLLM(t, llm.URL)
	payload, normalized, _, err := mod.parseQuery(context.Background(), "anything", 1740656400000, 0, "auto")
	if err != nil {
		t.Fatalf("parseQuery error: %v", err)
	}
	if payload.Timestamp.From != 1000 || payload.Timestamp.To != 5000 {
		t.Fatalf("expected llm time range, got %d..%d", payload.Timestamp.From, payload.Timestamp.To)
	}
	if normalized["time_range"] != "custom_range" {
		t.Fatalf("expected time_range=custom_range, got %#v", normalized["time_range"])
	}
}

func TestRunHybridForcedStructuredIgnoresSQLRequest(t *testing.T) {
	var visited string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		visited = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{"items": []map[string]any{}, "keys": []string{}},
			"meta": map[string]any{},
		})
	}))
	defer ts.Close()

	mod, err := New(&config.MCPConfig{
		Mode:            "structured",
		HomerBaseURL:    ts.URL,
		HomerToken:      "token",
		DefaultLimit:    100,
		SQLDefaultLimit: 100,
	})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	_, err = mod.runHybrid(context.Background(), hybridArgs{QueryText: "show sql INVITE", Mode: "sql"})
	if err != nil {
		t.Fatalf("runHybrid error: %v", err)
	}
	if visited != "/api/v4/transactions/search" {
		t.Fatalf("expected forced structured route, got %s", visited)
	}
}
