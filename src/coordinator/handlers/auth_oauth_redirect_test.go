// Copyright (C) 2025 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package handlers

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/labstack/echo/v4"
)

func TestResolveOAuthClientRedirect_relativeOnlyWithoutCallback(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/cb?redirect_uri=/login", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	u, err := resolveOAuthClientRedirect(c, &OAuthProvider{})
	if err != nil {
		t.Fatal(err)
	}
	if u.Path != "/login" {
		t.Fatalf("path: got %q", u.Path)
	}
}

func TestResolveOAuthClientRedirect_rejectsExternalWithoutCallback(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/cb?redirect_uri=https://evil.example/steal", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	_, err := resolveOAuthClientRedirect(c, &OAuthProvider{})
	if err == nil {
		t.Fatal("expected error for external redirect")
	}
}

func TestResolveOAuthClientRedirect_allowsConfiguredOrigin(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/cb", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	provider := &OAuthProvider{CallbackURL: "https://homer.example/app/oauth"}
	u, err := resolveOAuthClientRedirect(c, provider)
	if err != nil {
		t.Fatal(err)
	}
	if u.String() != provider.CallbackURL {
		t.Fatalf("got %q", u.String())
	}
}

func TestOAuthErrorClientRedirect_ignoresRedirectURI(t *testing.T) {
	provider := &OAuthProvider{CallbackURL: "https://homer.example/app/oauth"}
	u, err := oauthErrorClientRedirect(provider)
	if err != nil {
		t.Fatal(err)
	}
	if u.String() != provider.CallbackURL {
		t.Fatalf("got %q", u.String())
	}
}

func TestOAuthErrorClientRedirect_defaultPathWithoutCallback(t *testing.T) {
	u, err := oauthErrorClientRedirect(&OAuthProvider{})
	if err != nil {
		t.Fatal(err)
	}
	if u.Path != "/" {
		t.Fatalf("path: got %q", u.Path)
	}
}

func TestOAuthRedirectOriginsMatch(t *testing.T) {
	a, _ := url.Parse("https://homer.example/app")
	b, _ := url.Parse("https://homer.example/other")
	if !oauthRedirectOriginsMatch(a, b) {
		t.Fatal("expected same origin")
	}
	c, _ := url.Parse("https://evil.example/app")
	if oauthRedirectOriginsMatch(a, c) {
		t.Fatal("expected different origin")
	}
}
