// Copyright (C) 2026 Homer Server Contributors
// SPDX-License-Identifier: AGPL-3.0-or-later

package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v4"
)

type exportMemoryState struct {
	payload []byte
	writes  int
}

func (s *exportMemoryState) Get(context.Context, string, string, bool) ([]byte, bool, error) {
	return s.payload, len(s.payload) > 0, nil
}
func (s *exportMemoryState) Put(_ context.Context, _, _ string, p []byte, _ time.Time) error {
	s.payload = p
	s.writes++
	return nil
}

func exportContext(method, body, user string) (echo.Context, *httptest.ResponseRecorder) {
	req := httptest.NewRequest(method, "/", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.Set("user", &jwt.Token{Claims: &JWTClaims{Username: user}})
	c.SetParamNames("exportId")
	c.SetParamValues("test-id")
	return c, rec
}

func TestSharedExportOwnershipAndSize(t *testing.T) {
	state := &exportMemoryState{}
	a, b := NewExportsHandler(NewExportStore()), NewExportsHandler(NewExportStore())
	a.shared = state
	b.shared = state
	c, rec := exportContext(http.MethodPost, `{"type":"text","query":{"callid":"private"}}`, "alice")
	if err := a.V4ExportsCreate(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusCreated {
		t.Fatal(rec.Code, rec.Body.String())
	}
	for _, user := range []string{"alice", "bob"} {
		for _, handler := range []func(echo.Context) error{b.V4ExportsGet, b.V4ExportsDownload} {
			c, rec = exportContext(http.MethodGet, "", user)
			if err := handler(c); err != nil {
				t.Fatal(err)
			}
			want := http.StatusOK
			if user == "bob" {
				want = http.StatusNotFound
			}
			if rec.Code != want {
				t.Fatalf("%s: got %d want %d", user, rec.Code, want)
			}
		}
	}
	c, rec = exportContext(http.MethodPost, `{"type":"text","query":"`+strings.Repeat("x", maxExportRequestBytes)+`"}`, "alice")
	if err := a.V4ExportsCreate(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusRequestEntityTooLarge || state.writes != 1 {
		t.Fatalf("oversized export: status=%d writes=%d body=%s", rec.Code, state.writes, rec.Body.String())
	}
}

func TestLocalExportOwnership(t *testing.T) {
	h := NewExportsHandler(NewExportStore())
	h.store.Update(exportJob{ID: "test-id", Owner: "alice", Status: "completed", FilePath: "/unused"})
	for _, handler := range []func(echo.Context) error{h.V4ExportsGet, h.V4ExportsDownload} {
		c, rec := exportContext(http.MethodGet, "", "bob")
		if err := handler(c); err != nil {
			t.Fatal(err)
		}
		if rec.Code != http.StatusNotFound {
			t.Fatalf("local export exposed: %d", rec.Code)
		}
	}
}
