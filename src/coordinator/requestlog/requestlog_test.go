// Copyright (C) 2026 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package requestlog

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
)

func TestRedactURI(t *testing.T) {
	cases := map[string]string{
		"/api/v4/me":                                    "/api/v4/me",
		"/api/v4/me?access_token=abc.def.ghi":           "/api/v4/me?access_token=REDACTED",
		"/api/v4/stream/hep?proto=1&access_token=x&m=I": "/api/v4/stream/hep?proto=1&access_token=REDACTED&m=I",
		"/cb?code=c0de&state=st&x=1":                    "/cb?code=REDACTED&state=REDACTED&x=1",
		"/x?Access_Token=abc":                           "/x?Access_Token=REDACTED",
		"/x?access%5Ftoken=abc":                         "/x?access%5Ftoken=REDACTED",
		"/x?proto=1":                                    "/x?proto=1",
	}
	for in, want := range cases {
		if got := redactURI(in); got != want {
			t.Errorf("redactURI(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRequestLogger_DoesNotLogAccessToken(t *testing.T) {
	var buf bytes.Buffer
	e := echo.New()
	e.Use(Middleware(&buf))
	e.GET("/api/v4/me", func(c echo.Context) error { return c.NoContent(http.StatusUnauthorized) })

	req := httptest.NewRequest(http.MethodGet, "/api/v4/me?access_token=SECRETJWT", nil)
	e.ServeHTTP(httptest.NewRecorder(), req)

	line := buf.String()
	if strings.Contains(line, "SECRETJWT") {
		t.Fatalf("token leaked into log: %q", line)
	}
	if !strings.Contains(line, "GET /api/v4/me?access_token=REDACTED 401") {
		t.Fatalf("unexpected log line: %q", line)
	}
}
