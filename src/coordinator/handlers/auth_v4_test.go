package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
)

func newTestAuthHandler(disablePassword bool) *AuthHandler {
	return &AuthHandler{
		jwtSecret:            "test-secret-minimum-32-characters-long",
		expireHours:          24,
		disablePasswordLogin: disablePassword,
		providers: []OAuthProvider{
			{Enable: true, Name: "azure", AutoRedirect: true},
		},
	}
}

func TestV4ListProviders_DisablePasswordLogin(t *testing.T) {
	h := newTestAuthHandler(true)
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v4/auth/providers", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := h.V4ListProviders(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200", rec.Code)
	}

	var resp ProvidersResponseV4
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Data.Internal.Enable {
		t.Fatal("internal should be disabled")
	}
	if resp.Data.Ldap.Enable {
		t.Fatal("ldap should be disabled")
	}
	if len(resp.Data.Oauth2) != 1 || resp.Data.Oauth2[0].Name != "azure" {
		t.Fatalf("oauth2: got %#v", resp.Data.Oauth2)
	}
}

func TestV4ListProviders_PasswordLoginEnabled(t *testing.T) {
	h := newTestAuthHandler(false)
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v4/auth/providers", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := h.V4ListProviders(c); err != nil {
		t.Fatal(err)
	}

	var resp ProvidersResponseV4
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.Data.Internal.Enable {
		t.Fatal("internal should be enabled")
	}
}

func TestV4CreateSession_DisablePasswordLogin(t *testing.T) {
	h := newTestAuthHandler(true)
	e := echo.New()
	body := `{"username":"admin","password":"sipcapture"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v4/auth/sessions", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := h.V4CreateSession(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status: got %d want 403", rec.Code)
	}
}
