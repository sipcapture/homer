package mcp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestBuildStructuredPayloadResponseCodeRejectedWith(t *testing.T) {
	mod := newTestModule(t)
	now := int64(1740656400000)
	payload, normalized := mod.buildStructuredPayload("find all calls in the last 15 minutes that were rejected with 608", now, 0)

	if payload.Filter.ResponseCode != "608" {
		t.Fatalf("expected response_code 608, got %q", payload.Filter.ResponseCode)
	}
	if normalized["response_code"] != "608" {
		t.Fatalf("expected normalized.response_code=608, got %#v", normalized["response_code"])
	}
	// "last 15 minutes" isn't parsed by parseTimeRange today (unrelated,
	// pre-existing limitation) - documenting it here so this test doesn't
	// silently assume it's handled.
	if normalized["time_range"] != "default_last_hour" {
		t.Fatalf("expected time_range=default_last_hour, got %#v", normalized["time_range"])
	}
}

func TestBuildStructuredPayloadResponseCodeMultiple(t *testing.T) {
	mod := newTestModule(t)
	payload, _ := mod.buildStructuredPayload("show 608 or 486 responses", 0, 0)
	if payload.Filter.ResponseCode != "608,486" {
		t.Fatalf("expected response_code 608,486, got %q", payload.Filter.ResponseCode)
	}
}

func TestBuildStructuredPayloadResponseCodeCommaSeparated(t *testing.T) {
	mod := newTestModule(t)
	payload, _ := mod.buildStructuredPayload("find calls with response code 404, 486", 0, 0)
	if payload.Filter.ResponseCode != "404,486" {
		t.Fatalf("expected response_code 404,486, got %q", payload.Filter.ResponseCode)
	}
}

func TestBuildStructuredPayloadResponseCodeIgnoresPortAndMinutes(t *testing.T) {
	mod := newTestModule(t)
	payload, _ := mod.buildStructuredPayload("find INVITE on port 5060 in the last 15 minutes", 0, 0)
	if payload.Filter.ResponseCode != "" {
		t.Fatalf("expected empty response_code, got %q", payload.Filter.ResponseCode)
	}
}

func TestBuildStructuredPayloadResponseCodeIgnoresPartialDigitRun(t *testing.T) {
	mod := newTestModule(t)
	payload, _ := mod.buildStructuredPayload("response 5060 latency test", 0, 0)
	if payload.Filter.ResponseCode != "" {
		t.Fatalf("expected empty response_code (must not extract 506 from 5060), got %q", payload.Filter.ResponseCode)
	}
}

func TestBuildStructuredPayloadCallIDAcceptsSessionId(t *testing.T) {
	mod := newTestModule(t)
	payload, _ := mod.buildStructuredPayload("session id abc-999", 0, 0)
	if payload.Filter.CallID != "abc-999" {
		t.Fatalf("expected call_id abc-999, got %q", payload.Filter.CallID)
	}
}

func TestBuildStructuredPayloadCIDIsSeparateFromCallID(t *testing.T) {
	mod := newTestModule(t)
	payload, normalized := mod.buildStructuredPayload("find cid abc-123-xyz", 0, 0)
	if payload.Filter.CID != "abc-123-xyz" {
		t.Fatalf("expected cid abc-123-xyz, got %q", payload.Filter.CID)
	}
	if payload.Filter.CallID != "" {
		t.Fatalf("expected empty call_id (cid must not alias into call_id), got %q", payload.Filter.CallID)
	}
	if normalized["cid"] != "abc-123-xyz" {
		t.Fatalf("expected normalized.cid=abc-123-xyz, got %#v", normalized["cid"])
	}
}

func TestValidateSQLAllowsCallTableName(t *testing.T) {
	sql := "SELECT * FROM homer_lake.main.hep_proto_1_call WHERE method = 'INVITE' ORDER BY timestamp DESC LIMIT 10"
	if err := validateSQL(sql); err != nil {
		t.Fatalf("expected SQL to be valid, got error: %v", err)
	}
}

func TestValidateSQLAllowsForbiddenWordsInLiterals(t *testing.T) {
	sql := "SELECT * FROM homer_lake.main.hep_proto_1_call WHERE session_id = 'foo-call-bar' OR note = 'drop table'"
	if err := validateSQL(sql); err != nil {
		t.Fatalf("expected keywords inside string literals to be allowed, got: %v", err)
	}
}

func TestValidateSQLRejectsSemicolon(t *testing.T) {
	sql := "SELECT * FROM homer_lake.main.hep_proto_1_call WHERE method = 'INVITE';"
	if err := validateSQL(sql); err == nil {
		t.Fatalf("expected semicolon SQL to be rejected")
	}
}

func TestBuildSQL_SemicolonSeparatedOR(t *testing.T) {
	payload := searchPayload{}
	payload.Timestamp.From = 1
	payload.Timestamp.To = 2
	payload.Filter.ToUser = "112;110"
	payload.Param.Limit = 10
	sql := buildSQL(payload)
	if err := validateSQL(sql); err != nil {
		t.Fatalf("generated SQL rejected: %v\n%s", err, sql)
	}
	if !containsAll(sql, "callee LIKE '%112%'", "callee LIKE '%110%'") {
		t.Fatalf("expected OR of LIKE tokens, got:\n%s", sql)
	}
}

func TestBuildSQL_ResponseCodeSingle(t *testing.T) {
	payload := searchPayload{}
	payload.Timestamp.From = 1
	payload.Timestamp.To = 2
	payload.Filter.ResponseCode = "608"
	sql := buildSQL(payload)
	if err := validateSQL(sql); err != nil {
		t.Fatalf("generated SQL rejected: %v\n%s", err, sql)
	}
	if !containsAll(sql, "response_code = '608'") {
		t.Fatalf("expected response_code equality clause, got:\n%s", sql)
	}
}

func TestBuildSQL_ResponseCodeMultipleIN(t *testing.T) {
	payload := searchPayload{}
	payload.Timestamp.From = 1
	payload.Timestamp.To = 2
	payload.Filter.ResponseCode = "608,486"
	sql := buildSQL(payload)
	if err := validateSQL(sql); err != nil {
		t.Fatalf("generated SQL rejected: %v\n%s", err, sql)
	}
	if !containsAll(sql, "response_code IN (", "'608'", "'486'") {
		t.Fatalf("expected response_code IN clause, got:\n%s", sql)
	}
}

func TestBuildSQL_ResponseCodeEscapesQuotes(t *testing.T) {
	payload := searchPayload{}
	payload.Timestamp.From = 1
	payload.Timestamp.To = 2
	payload.Filter.ResponseCode = "608'; DROP TABLE x --"
	sql := buildSQL(payload)
	if err := validateSQL(sql); err != nil {
		t.Fatalf("generated SQL rejected: %v\n%s", err, sql)
	}
	// The embedded ';' is consumed as a value separator (same rationale as
	// mcpLikeAny, #1008), producing two IN-list values rather than one
	// value with an embedded semicolon - the semicolon never survives into
	// the final SQL string.
	if !containsAll(sql, "response_code IN (", "'608'''", "'DROP TABLE x --'") {
		t.Fatalf("expected escaped two-value IN clause, got:\n%s", sql)
	}
}

func TestBuildSQL_CIDMatchesOnlyCIDColumn(t *testing.T) {
	payload := searchPayload{}
	payload.Timestamp.From = 1
	payload.Timestamp.To = 2
	payload.Filter.CID = "abc-123"
	sql := buildSQL(payload)
	if err := validateSQL(sql); err != nil {
		t.Fatalf("generated SQL rejected: %v\n%s", err, sql)
	}
	if !containsAll(sql, "cid LIKE '%abc-123%'") {
		t.Fatalf("expected cid LIKE clause, got:\n%s", sql)
	}
	if strings.Contains(sql, "session_id") {
		t.Fatalf("cid filter must not also match session_id, got:\n%s", sql)
	}
}

func containsAll(s string, parts ...string) bool {
	for _, p := range parts {
		if !strings.Contains(s, p) {
			return false
		}
	}
	return true
}

func TestValidateSQLRejectsDropToken(t *testing.T) {
	// Bare DROP identifier must still be rejected; words inside string
	// literals are allowed (see TestValidateSQLAllowsForbiddenWordsInLiterals).
	sql := "SELECT * FROM homer_lake.main.hep_proto_1_call WHERE DROP"
	if err := validateSQL(sql); err == nil {
		t.Fatalf("expected DROP identifier SQL to be rejected")
	}
}

func TestValidateSQLRejectsCallStatement(t *testing.T) {
	sql := "SELECT * FROM homer_lake.main.hep_proto_1_call WHERE 1=1 CALL some_proc()"
	if err := validateSQL(sql); err == nil {
		t.Fatalf("expected CALL statement SQL to be rejected")
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

func TestRunHybridUsesStaticAuthTokenHeaderWhenConfigured(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Auth-Token") != "static-secret" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if r.Header.Get("Authorization") != "" {
			http.Error(w, "unexpected authorization header", http.StatusBadRequest)
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
		HomerToken:      "static-secret",
		HomerAuthHeader: "Auth-Token",
		DefaultLimit:    100,
		SQLDefaultLimit: 100,
	})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	if _, err := mod.runHybrid(context.Background(), hybridArgs{QueryText: "find INVITE in the last hour", Mode: "auto"}); err != nil {
		t.Fatalf("runHybrid structured error: %v", err)
	}
	if _, err := mod.runHybrid(context.Background(), hybridArgs{QueryText: "show sql INVITE in the last hour", Mode: "auto"}); err != nil {
		t.Fatalf("runHybrid sql error: %v", err)
	}
}

func TestPostJSONRefusesCrossHostRedirect(t *testing.T) {
	var attackerHit bool
	attacker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attackerHit = true
		if r.Header.Get("Auth-Token") != "" {
			t.Errorf("Auth-Token header leaked to redirect target: %q", r.Header.Get("Auth-Token"))
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer attacker.Close()

	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, attacker.URL+"/steal", http.StatusFound)
	}))
	defer primary.Close()

	mod, err := New(&config.MCPConfig{
		Mode:            "hybrid",
		HomerBaseURL:    primary.URL,
		HomerToken:      "static-secret",
		HomerAuthHeader: "Auth-Token",
		DefaultLimit:    100,
		SQLDefaultLimit: 100,
	})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	_, err = mod.runHybrid(context.Background(), hybridArgs{QueryText: "find INVITE in the last hour", Mode: "auto"})
	if err == nil {
		t.Fatal("expected runHybrid to fail when the Coordinator redirects cross-host, got nil error")
	}
	if attackerHit {
		t.Fatal("expected the cross-host redirect target to never be requested")
	}
}

func TestRunHybridDefaultsToBearerWhenAuthHeaderEmpty(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

	if _, err := mod.runHybrid(context.Background(), hybridArgs{QueryText: "find INVITE in the last hour", Mode: "auto"}); err != nil {
		t.Fatalf("runHybrid error: %v", err)
	}
}

func TestNewTrimsHomerAuthHeader(t *testing.T) {
	mod, err := New(&config.MCPConfig{
		Mode:            "hybrid",
		HomerBaseURL:    "http://127.0.0.1:8080",
		HomerToken:      "token",
		HomerAuthHeader: "  Auth-Token  ",
		DefaultLimit:    100,
		SQLDefaultLimit: 100,
	})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	if mod.cfg.HomerAuthHeader != "Auth-Token" {
		t.Fatalf("expected trimmed HomerAuthHeader %q, got %q", "Auth-Token", mod.cfg.HomerAuthHeader)
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

func TestParseQueryLLMResponseCodeOverridesRegex(t *testing.T) {
	llm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, chatJSON(`{"method":"BYE","response_code":"608,486"}`))
	}))
	defer llm.Close()

	mod := newModuleWithLLM(t, llm.URL)
	payload, normalized, _, err := mod.parseQuery(context.Background(), "give me everything", 1740656400000, 0, "auto")
	if err != nil {
		t.Fatalf("parseQuery error: %v", err)
	}
	if payload.Filter.ResponseCode != "608,486" {
		t.Fatalf("expected response_code=608,486, got %q", payload.Filter.ResponseCode)
	}
	if normalized["response_code"] != "608,486" {
		t.Fatalf("expected normalized.response_code=608,486, got %#v", normalized["response_code"])
	}
}

func TestParseQueryLLMEmptyResponseCodeFallsBackToRegex(t *testing.T) {
	llm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, chatJSON(`{"method":"INVITE"}`))
	}))
	defer llm.Close()

	mod := newModuleWithLLM(t, llm.URL)
	payload, _, meta, err := mod.parseQuery(context.Background(), "find all calls in the last 15 minutes that were rejected with 608", 1740656400000, 0, "auto")
	if err != nil {
		t.Fatalf("parseQuery error: %v", err)
	}
	if meta.Used != parserLLM {
		t.Fatalf("expected parser_used=llm, got %q", meta.Used)
	}
	if payload.Filter.ResponseCode != "608" {
		t.Fatalf("expected regex-derived response_code=608 to survive the merge, got %q", payload.Filter.ResponseCode)
	}
}

func TestParseQueryLLMCIDDistinctFromCallID(t *testing.T) {
	llm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, chatJSON(`{"call_id":"call-1","cid":"cid-1"}`))
	}))
	defer llm.Close()

	mod := newModuleWithLLM(t, llm.URL)
	payload, _, _, err := mod.parseQuery(context.Background(), "give me everything", 1740656400000, 0, "auto")
	if err != nil {
		t.Fatalf("parseQuery error: %v", err)
	}
	if payload.Filter.CallID != "call-1" {
		t.Fatalf("expected call_id=call-1, got %q", payload.Filter.CallID)
	}
	if payload.Filter.CID != "cid-1" {
		t.Fatalf("expected cid=cid-1, got %q", payload.Filter.CID)
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
