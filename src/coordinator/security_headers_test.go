// Copyright (C) 2025 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package coordinator

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
)

func TestSecurityHeaders_CSP(t *testing.T) {
	e := echo.New()
	e.Use(securityHeaders())
	e.GET("/", func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	csp := rec.Header().Get("Content-Security-Policy")
	if csp != UIContentSecurityPolicy {
		t.Fatalf("CSP mismatch:\n got %q\nwant %q", csp, UIContentSecurityPolicy)
	}
	for _, needle := range []string{
		"script-src 'self'",
		"object-src 'none'",
		"frame-ancestors 'self'",
	} {
		if !strings.Contains(csp, needle) {
			t.Errorf("CSP missing %q", needle)
		}
	}
	if strings.Contains(csp, "script-src") && strings.Contains(csp, "unsafe-eval") {
		t.Error("script-src must not allow unsafe-eval")
	}
	if strings.Contains(csp, "unsafe-inline") && !strings.Contains(csp, "style-src") {
		t.Error("unsafe-inline must not apply outside style-src")
	}
	// script-src token list must not include unsafe-inline
	for _, dir := range strings.Split(csp, ";") {
		d := strings.TrimSpace(dir)
		if strings.HasPrefix(d, "script-src") && strings.Contains(d, "unsafe-inline") {
			t.Errorf("script-src allows unsafe-inline: %s", d)
		}
	}
	if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("X-Content-Type-Options: %q", rec.Header().Get("X-Content-Type-Options"))
	}
	if rec.Header().Get("X-Frame-Options") != "SAMEORIGIN" {
		t.Errorf("X-Frame-Options: %q", rec.Header().Get("X-Frame-Options"))
	}
}
