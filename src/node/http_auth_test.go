// Copyright (C) 2026 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package node

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWithBearerAuthDisabled(t *testing.T) {
	called := false
	h := withBearerAuth("", func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	})
	rr := httptest.NewRecorder()
	h(rr, httptest.NewRequest(http.MethodPost, "/query", nil))
	if !called || rr.Code != http.StatusNoContent {
		t.Fatalf("empty token should skip auth: called=%v code=%d", called, rr.Code)
	}
}

func TestWithBearerAuthRequired(t *testing.T) {
	h := withBearerAuth("secret-token", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	t.Run("missing", func(t *testing.T) {
		rr := httptest.NewRecorder()
		h(rr, httptest.NewRequest(http.MethodPost, "/query", nil))
		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("code=%d", rr.Code)
		}
	})

	t.Run("wrong", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/query", nil)
		req.Header.Set("Authorization", "Bearer wrong")
		rr := httptest.NewRecorder()
		h(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("code=%d", rr.Code)
		}
	})

	t.Run("ok", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/query", nil)
		req.Header.Set("Authorization", "Bearer secret-token")
		rr := httptest.NewRecorder()
		h(rr, req)
		if rr.Code != http.StatusNoContent {
			t.Fatalf("code=%d", rr.Code)
		}
	})
}

func TestHandleQueryValidatesSQL(t *testing.T) {
	// Validation runs before DB access, so a bare Node is enough.
	node := &Node{}
	body, _ := json.Marshal(QueryRequest{SQL: `SELECT content FROM read_text('/etc/passwd') LIMIT 1`})
	req := httptest.NewRequest(http.MethodPost, "/query", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	node.handleQuery(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for read_text, got %d body=%s", rr.Code, rr.Body.String())
	}
	var resp QueryResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Success || resp.Error == "" {
		t.Fatalf("expected validation failure, got %+v", resp)
	}
	if !bytes.Contains([]byte(resp.Error), []byte("SQL validation failed")) {
		t.Fatalf("unexpected error: %q", resp.Error)
	}
}

func TestHandleQueryRejectsDML(t *testing.T) {
	node := &Node{}
	body, _ := json.Marshal(QueryRequest{SQL: `DROP TABLE t`})
	req := httptest.NewRequest(http.MethodPost, "/query", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	node.handleQuery(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestBearerTokenFromRequest(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer  abc ")
	if got := bearerTokenFromRequest(req); got != "abc" {
		t.Fatalf("got %q", got)
	}
}
