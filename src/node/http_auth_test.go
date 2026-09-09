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

	"github.com/sipcapture/homer-core/src/storage/ducklake"
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

func TestHandleQueryRejectsInsert(t *testing.T) {
	node := &Node{}
	body, _ := json.Marshal(QueryRequest{SQL: `INSERT INTO t VALUES (1)`})
	req := httptest.NewRequest(http.MethodPost, "/query", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	node.handleQuery(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for INSERT on /query, got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestHandleExecRejectsRawSQL(t *testing.T) {
	node := &Node{}

	t.Run("sql field", func(t *testing.T) {
		body, _ := json.Marshal(QueryRequest{SQL: `INSERT INTO homer_lake.main.hep_proto_1_call (id) VALUES (1)`})
		req := httptest.NewRequest(http.MethodPost, "/exec", bytes.NewReader(body))
		rr := httptest.NewRecorder()
		node.handleExec(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d body=%s", rr.Code, rr.Body.String())
		}
		if !bytes.Contains(rr.Body.Bytes(), []byte("raw SQL")) {
			t.Fatalf("expected raw SQL rejection, body=%s", rr.Body.String())
		}
	})

	t.Run("insert select", func(t *testing.T) {
		body, _ := json.Marshal(QueryRequest{SQL: `INSERT INTO t SELECT content FROM read_text('/etc/passwd')`})
		req := httptest.NewRequest(http.MethodPost, "/exec", bytes.NewReader(body))
		rr := httptest.NewRecorder()
		node.handleExec(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d body=%s", rr.Code, rr.Body.String())
		}
	})
}

func TestHandleExecRowsContract(t *testing.T) {
	node := &Node{}

	t.Run("unknown table", func(t *testing.T) {
		body, _ := json.Marshal(ExecInsertRequest{
			ProtoType: 99,
			SubType:   "nope",
			Rows:      [][]interface{}{{1}},
		})
		req := httptest.NewRequest(http.MethodPost, "/exec", bytes.NewReader(body))
		rr := httptest.NewRecorder()
		node.handleExec(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d body=%s", rr.Code, rr.Body.String())
		}
	})

	t.Run("empty rows", func(t *testing.T) {
		body, _ := json.Marshal(ExecInsertRequest{
			ProtoType: 1,
			SubType:   "call",
			Rows:      [][]interface{}{},
		})
		req := httptest.NewRequest(http.MethodPost, "/exec", bytes.NewReader(body))
		rr := httptest.NewRecorder()
		node.handleExec(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d body=%s", rr.Code, rr.Body.String())
		}
	})

	t.Run("valid rows without db is 503", func(t *testing.T) {
		n := len(ducklake.InsertColumnNamesForKey(ducklake.TableKey{ProtoType: ducklake.ProtoTypeSIP, SubType: ducklake.SIPTypeCall}))
		row := make([]interface{}, n)
		for i := range row {
			row[i] = "x"
		}
		body, _ := json.Marshal(ExecInsertRequest{
			ProtoType: ducklake.ProtoTypeSIP,
			SubType:   ducklake.SIPTypeCall,
			Rows:      [][]interface{}{row},
		})
		req := httptest.NewRequest(http.MethodPost, "/exec", bytes.NewReader(body))
		rr := httptest.NewRecorder()
		node.handleExec(rr, req)
		if rr.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected 503 after schema check, got %d body=%s", rr.Code, rr.Body.String())
		}
	})
}

func TestBearerTokenFromRequest(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer  abc ")
	if got := bearerTokenFromRequest(req); got != "abc" {
		t.Fatalf("got %q", got)
	}
}
