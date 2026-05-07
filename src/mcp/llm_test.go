package mcp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/sipcapture/homer-core/src/config"
)

func newTestLLMServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, func()) {
	t.Helper()
	ts := httptest.NewServer(handler)
	return ts, ts.Close
}

func chatJSON(content string) string {
	resp := chatResponse{
		Choices: []struct {
			Message chatMessage `json:"message"`
		}{
			{Message: chatMessage{Role: "assistant", Content: content}},
		},
	}
	b, _ := json.Marshal(resp)
	return string(b)
}

func TestLLMClientParseSuccess(t *testing.T) {
	var (
		gotAuth   string
		gotMethod string
		gotPath   string
	)

	ts, stop := newTestLLMServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotMethod = r.Method
		gotPath = r.URL.Path

		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, chatJSON(`{"method":"INVITE","src_ip":"10.1.2.3","time_range_label":"last_hour","from_ms":1000,"to_ms":2000}`))
	})
	defer stop()

	client := NewLLMClient(&config.MCPLLMConfig{
		Enable:     true,
		BaseURL:    ts.URL,
		APIKey:     "secret-key",
		Model:      "gpt-4o-mini",
		TimeoutSec: 5,
	})
	if client == nil {
		t.Fatal("expected non-nil client")
	}

	res, err := client.Parse(context.Background(), "find INVITE in the last hour from 10.1.2.3", 1700000000000)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if res.Filters.Method != "INVITE" {
		t.Fatalf("expected INVITE, got %q", res.Filters.Method)
	}
	if res.Filters.SrcIP != "10.1.2.3" {
		t.Fatalf("expected 10.1.2.3, got %q", res.Filters.SrcIP)
	}
	if res.Filters.FromMS != 1000 || res.Filters.ToMS != 2000 {
		t.Fatalf("unexpected time range: %d..%d", res.Filters.FromMS, res.Filters.ToMS)
	}
	if res.Model != "gpt-4o-mini" {
		t.Fatalf("expected model echo, got %q", res.Model)
	}
	if gotMethod != http.MethodPost {
		t.Fatalf("expected POST, got %s", gotMethod)
	}
	if gotPath != "/chat/completions" {
		t.Fatalf("expected /chat/completions, got %s", gotPath)
	}
	if gotAuth != "Bearer secret-key" {
		t.Fatalf("expected Bearer secret-key, got %q", gotAuth)
	}
}

func TestLLMClientParseInvalidJSON(t *testing.T) {
	ts, stop := newTestLLMServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, chatJSON("totally not json"))
	})
	defer stop()

	client := NewLLMClient(&config.MCPLLMConfig{
		Enable:     true,
		BaseURL:    ts.URL,
		Model:      "test",
		TimeoutSec: 5,
	})
	_, err := client.Parse(context.Background(), "show me bye", 1700000000000)
	if err == nil {
		t.Fatal("expected error for invalid JSON content")
	}
}

func TestLLMClientParseTimeout(t *testing.T) {
	ts, stop := newTestLLMServer(t, func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, chatJSON("{}"))
	})
	defer stop()

	client := NewLLMClient(&config.MCPLLMConfig{
		Enable:     true,
		BaseURL:    ts.URL,
		Model:      "test",
		TimeoutSec: 1,
	})
	client.http.Timeout = 50 * time.Millisecond

	_, err := client.Parse(context.Background(), "anything", 1700000000000)
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestLLMClientNoAuthHeaderWhenEmpty(t *testing.T) {
	var gotAuth string
	ts, stop := newTestLLMServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, chatJSON("{}"))
	})
	defer stop()

	client := NewLLMClient(&config.MCPLLMConfig{
		Enable:     true,
		BaseURL:    ts.URL,
		APIKey:     "",
		Model:      "llama3.1",
		TimeoutSec: 5,
	})
	_, err := client.Parse(context.Background(), "anything", 1700000000000)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if gotAuth != "" {
		t.Fatalf("expected no Authorization header, got %q", gotAuth)
	}
}

func TestLLMClientDisabledReturnsNil(t *testing.T) {
	if got := NewLLMClient(&config.MCPLLMConfig{Enable: false}); got != nil {
		t.Fatalf("expected nil client when Enable=false, got %#v", got)
	}
	if got := NewLLMClient(nil); got != nil {
		t.Fatalf("expected nil client for nil cfg, got %#v", got)
	}
}

func TestExtractJSONObjectStripsMarkdownFence(t *testing.T) {
	in := "```json\n{\"method\":\"INVITE\"}\n```"
	out, err := extractJSONObject(in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != `{"method":"INVITE"}` {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestExtractJSONObjectFindsObjectInProse(t *testing.T) {
	in := "Here is the result: {\"src_ip\":\"1.1.1.1\"} hope it helps"
	out, err := extractJSONObject(in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != `{"src_ip":"1.1.1.1"}` {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestExtractJSONObjectErrorsWhenMissing(t *testing.T) {
	if _, err := extractJSONObject("no braces here"); err == nil {
		t.Fatal("expected error when no JSON object is present")
	}
}
