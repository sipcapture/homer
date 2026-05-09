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
	"strings"
	"sync/atomic"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/sipcapture/homer-core/src/config"
	"github.com/sipcapture/homer-core/src/coordinator/services"
	mcppkg "github.com/sipcapture/homer-core/src/mcp"
)

// chatCompletionsBody is the minimum subset of the OpenAI chat-completions
// response shape we need to fabricate in tests; mirrored locally so tests
// don't depend on unexported types in src/mcp.
type chatCompletionsBody struct {
	Choices []struct {
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

// writeChatCompletion replies with a single-choice chat-completions response
// whose assistant message carries the given content.
func writeChatCompletion(w http.ResponseWriter, content string) {
	resp := chatCompletionsBody{}
	resp.Choices = append(resp.Choices, struct {
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
	}{})
	resp.Choices[0].Message.Role = "assistant"
	resp.Choices[0].Message.Content = content
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// newLLMTestHandler builds a SearchHandler whose LLM client is wired to a
// caller-supplied http handler. The flight service is constructed with no
// nodes which is fine for tests that don't run real queries (we only need
// `LakeName()` to keep the SQL prompt template valid).
func newLLMTestHandler(t *testing.T, llmHandler http.HandlerFunc, apiKey string) (*SearchHandler, func()) {
	t.Helper()
	ts := httptest.NewServer(llmHandler)

	cfg := &config.MCPConfig{}
	cfg.LLM = config.MCPLLMConfig{
		Enable:     true,
		Provider:   "ollama",
		BaseURL:    ts.URL,
		APIKey:     apiKey,
		Model:      "llama3.1",
		TimeoutSec: 5,
	}

	fs := services.NewFlightService(nil, 0)
	h := NewSearchHandler(fs, nil, cfg, nil, nil, 0, nil)
	return h, ts.Close
}

func TestNormalizeParserHint(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"", "auto"},
		{"  ", "auto"},
		{"AUTO", "auto"},
		{"llm", "llm"},
		{"LLM", "llm"},
		{"regex", "regex"},
		{"rule", "regex"},
		{"rules", "regex"},
		{"sql", "auto"}, // unknown → auto
	}
	for _, tc := range cases {
		if got := normalizeParserHint(tc.in); got != tc.want {
			t.Errorf("normalizeParserHint(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestTryLLMStructured_Success(t *testing.T) {
	var gotAuth string
	h, stop := newLLMTestHandler(t, func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		writeChatCompletion(w, `{"filter":{"method":"INVITE","src_ip":"10.0.0.1"},"timestamp":{"from":1000,"to":2000},"limit":250}`)
	}, "")
	defer stop()

	req, res := h.tryLLMStructured(context.Background(), "find INVITE from 10.0.0.1", 100, 200, 100)
	if !res.Used {
		t.Fatalf("expected Used=true, got %#v", res)
	}
	if res.Err != nil {
		t.Fatalf("unexpected error: %v", res.Err)
	}
	if req == nil {
		t.Fatal("expected non-nil SearchObjectV4")
	}
	if req.Filter.Method != "INVITE" || req.Filter.SrcIP != "10.0.0.1" {
		t.Fatalf("unexpected filter: %+v", req.Filter)
	}
	if req.Timestamp.From != 1000 || req.Timestamp.To != 2000 {
		t.Fatalf("unexpected timestamp: %+v", req.Timestamp)
	}
	// limit=250 is below the 1000-cap so it should be preserved as-is.
	if req.Param.Limit != 250 {
		t.Fatalf("unexpected limit: %d", req.Param.Limit)
	}
	if res.Model != "llama3.1" {
		t.Fatalf("expected model echo 'llama3.1', got %q", res.Model)
	}
	// Ollama scenario: empty API key → no Authorization header should be sent.
	if gotAuth != "" {
		t.Fatalf("expected no Authorization header for empty api_key, got %q", gotAuth)
	}
}

func TestTryLLMStructured_FallbackToFallbackTime(t *testing.T) {
	h, stop := newLLMTestHandler(t, func(w http.ResponseWriter, _ *http.Request) {
		// Model omits time range — handler must use fallbackFrom/fallbackTo.
		writeChatCompletion(w, `{"filter":{"method":"BYE"}}`)
	}, "key")
	defer stop()

	req, res := h.tryLLMStructured(context.Background(), "find BYE", 555, 999, 50)
	if !res.Used || req == nil {
		t.Fatalf("expected used=true, got %#v / %v", res, req)
	}
	if req.Timestamp.From != 555 || req.Timestamp.To != 999 {
		t.Fatalf("expected fallback timestamps, got %+v", req.Timestamp)
	}
	if req.Param.Limit != 50 {
		t.Fatalf("expected fallback limit 50, got %d", req.Param.Limit)
	}
}

func TestTryLLMStructured_DisabledReturnsZeroResult(t *testing.T) {
	cfg := &config.MCPConfig{}
	cfg.LLM = config.MCPLLMConfig{Enable: false}
	fs := services.NewFlightService(nil, 0)
	h := NewSearchHandler(fs, nil, cfg, nil, nil, 0, nil)

	req, res := h.tryLLMStructured(context.Background(), "anything", 0, 0, 0)
	if req != nil {
		t.Fatalf("expected nil request when LLM disabled, got %+v", req)
	}
	if res.Used || res.Err != nil || res.Model != "" {
		t.Fatalf("expected zero llmRunResult, got %#v", res)
	}
}

func TestTryLLMStructured_HTTPErrorReturnsErrInResult(t *testing.T) {
	h, stop := newLLMTestHandler(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, "model is angry")
	}, "")
	defer stop()

	req, res := h.tryLLMStructured(context.Background(), "anything", 1, 2, 10)
	if req != nil {
		t.Fatalf("expected nil request on http error, got %+v", req)
	}
	if res.Err == nil {
		t.Fatal("expected non-nil error in llmRunResult")
	}
	if res.Used {
		t.Fatalf("expected Used=false, got %#v", res)
	}
}

func TestTryLLMSQL_Success(t *testing.T) {
	h, stop := newLLMTestHandler(t, func(w http.ResponseWriter, _ *http.Request) {
		writeChatCompletion(w, `{"sql":"SELECT * FROM homer_lake.main.hep_proto_1_call WHERE method = 'INVITE' ORDER BY timestamp DESC LIMIT 100"}`)
	}, "key")
	defer stop()

	sql, res := h.tryLLMSQL(context.Background(), "find INVITE", 100)
	if !res.Used || res.Err != nil {
		t.Fatalf("expected Used=true err=nil, got %#v", res)
	}
	if !strings.Contains(sql, "SELECT") || !strings.Contains(sql, "INVITE") {
		t.Fatalf("unexpected sql: %s", sql)
	}
}

func TestTryLLMSQL_EmptySQLReturnsErr(t *testing.T) {
	h, stop := newLLMTestHandler(t, func(w http.ResponseWriter, _ *http.Request) {
		writeChatCompletion(w, `{"sql":""}`)
	}, "key")
	defer stop()

	sql, res := h.tryLLMSQL(context.Background(), "find anything", 50)
	if sql != "" {
		t.Fatalf("expected empty sql, got %q", sql)
	}
	if res.Used {
		t.Fatalf("expected Used=false, got %#v", res)
	}
	if res.Err == nil {
		t.Fatal("expected error 'llm returned empty sql'")
	}
}

func TestV4MCPLLMStatus_OllamaPingNoAPIKey(t *testing.T) {
	var pingHits int32
	var sawAuth string

	// Spin a fake Ollama-like server: GET /models replies 200 OK and we
	// remember whether an Authorization header was sent.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/models" && r.Method == http.MethodGet {
			atomic.AddInt32(&pingHits, 1)
			sawAuth = r.Header.Get("Authorization")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"data":[]}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	cfg := &config.MCPConfig{}
	cfg.LLM = config.MCPLLMConfig{
		Enable:     true,
		Provider:   "ollama",
		BaseURL:    srv.URL,
		APIKey:     "",
		Model:      "llama3.1",
		TimeoutSec: 2,
	}
	fs := services.NewFlightService(nil, 0)
	h := NewSearchHandler(fs, nil, cfg, nil, nil, 0, nil)

	e := echo.New()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v4/mcp/llm/status", nil)
	c := e.NewContext(req, rec)

	if err := h.V4MCPLLMStatus(c); err != nil {
		t.Fatalf("V4MCPLLMStatus: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}
	if atomic.LoadInt32(&pingHits) != 1 {
		t.Fatalf("expected 1 ping hit, got %d", pingHits)
	}
	if sawAuth != "" {
		t.Fatalf("expected no Authorization header for empty api_key, got %q", sawAuth)
	}

	var body struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v (raw=%s)", err, rec.Body.String())
	}
	if got, _ := body.Data["enabled"].(bool); !got {
		t.Errorf("expected enabled=true, got %v", body.Data["enabled"])
	}
	if got, _ := body.Data["reachable"].(bool); !got {
		t.Errorf("expected reachable=true, got %v", body.Data["reachable"])
	}
	if got, _ := body.Data["key_configured"].(bool); got {
		t.Errorf("expected key_configured=false, got %v", body.Data["key_configured"])
	}
	if got, _ := body.Data["provider_supported"].(bool); !got {
		t.Errorf("expected provider_supported=true (any OpenAI-compatible), got %v", body.Data["provider_supported"])
	}
}

func TestNewSearchHandler_LLMDisabledMeansNilClient(t *testing.T) {
	cfg := &config.MCPConfig{}
	cfg.LLM = config.MCPLLMConfig{Enable: false}
	fs := services.NewFlightService(nil, 0)
	h := NewSearchHandler(fs, nil, cfg, nil, nil, 0, nil)
	if h.llm != nil {
		t.Fatalf("expected nil LLM client when Enable=false, got %#v", h.llm)
	}
	// Sanity: nil-receiver Enabled() is safe.
	var nilClient *mcppkg.LLMClient
	if nilClient.Enabled() {
		t.Fatal("nil-receiver Enabled() must return false")
	}
}
