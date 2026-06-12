package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/sipcapture/homer-core/src/config"
	"github.com/sipcapture/homer-core/src/coordinator/services"

	_ "github.com/duckdb/duckdb-go/v2"
)

func newCookieAuthHandler(t *testing.T) *AuthHandler {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.duckdb")
	db, err := services.OpenSettingsDB(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := services.EnsureSettingsSchema(db); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	h := config.DefaultInternalAuthPasswordHash
	_, err = db.ExecContext(ctx, `
		INSERT INTO users (username, password_hash, email, full_name, is_admin, is_active, created_at, updated_at)
		VALUES ('admin', '`+h+`', 'admin@example.com', 'Admin', true, true, current_timestamp, current_timestamp)`)
	if err != nil {
		t.Fatal(err)
	}
	userSvc := services.NewUserService(db)
	enable := true
	return NewAuthHandlerWithUserService(
		"test-secret-minimum-32-characters-long",
		24,
		config.JWTConfig{CookieEnable: &enable, CookieName: "homer_session", CookieSameSite: "Lax"},
		userSvc,
		nil,
		nil,
		config.APISettingsConfig{},
		nil,
		"",
		false,
	)
}

func TestV4CreateSession_SetsHttpOnlyCookie(t *testing.T) {
	h := newCookieAuthHandler(t)
	e := echo.New()
	body := `{"username":"admin","password":"sipcapture"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v4/auth/sessions", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := h.V4CreateSession(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusCreated {
		t.Fatalf("status: got %d want 201", rec.Code)
	}
	setCookie := rec.Header().Get("Set-Cookie")
	if !strings.Contains(setCookie, "homer_session=") {
		t.Fatalf("expected homer_session cookie, got %q", setCookie)
	}
	if !strings.Contains(setCookie, "HttpOnly") {
		t.Fatalf("expected HttpOnly cookie, got %q", setCookie)
	}
}

func TestJWTMiddlewareV4_AcceptsSessionCookie(t *testing.T) {
	h := newCookieAuthHandler(t)
	token, _, err := h.generateToken("admin", true)
	if err != nil {
		t.Fatal(err)
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v4/me", nil)
	req.AddCookie(&http.Cookie{Name: "homer_session", Value: token})
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	mw := h.JWTMiddlewareV4()
	handler := mw(func(c echo.Context) error {
		return c.NoContent(http.StatusNoContent)
	})
	if err := handler(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status: got %d want 204", rec.Code)
	}
}

func TestV4LogoutCurrentSession_ClearsCookie(t *testing.T) {
	h := newCookieAuthHandler(t)
	token, _, err := h.generateToken("admin", true)
	if err != nil {
		t.Fatal(err)
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodDelete, "/api/v4/auth/sessions/current", nil)
	req.AddCookie(&http.Cookie{Name: "homer_session", Value: token})
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	mw := h.JWTMiddlewareV4()
	handler := mw(func(c echo.Context) error {
		return h.V4LogoutCurrentSession(c)
	})
	if err := handler(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status: got %d want 204", rec.Code)
	}
	setCookie := rec.Header().Get("Set-Cookie")
	if !strings.Contains(setCookie, "homer_session=") || !strings.Contains(setCookie, "Max-Age=0") {
		t.Fatalf("expected cleared cookie, got %q", setCookie)
	}
}
