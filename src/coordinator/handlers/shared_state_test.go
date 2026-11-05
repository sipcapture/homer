package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"
	"github.com/sipcapture/homer-core/src/coordinator/services"
)

func sharedStateReplicas(t *testing.T) (*sql.DB, *services.RuntimeState, *sql.DB, *services.RuntimeState) {
	t.Helper()
	dsn := os.Getenv("HOMER_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set HOMER_TEST_POSTGRES_DSN for PostgreSQL integration tests")
	}
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	name := "homer_http_state_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		_ = admin.Close(ctx)
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = admin.Exec(ctx, "DROP DATABASE "+name+" WITH (FORCE)"); _ = admin.Close(ctx) })
	installer, err := sql.Open("duckdb", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := installer.Exec("LOAD postgres"); err != nil {
		if _, err := installer.Exec("INSTALL postgres FROM 'https://extensions.duckdb.org'"); err != nil {
			_ = installer.Close()
			t.Fatal(err)
		}
	}
	_ = installer.Close()
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	u := url.URL{Scheme: "postgresql", Host: net.JoinHostPort(cfg.Host, strconv.Itoa(int(cfg.Port))), User: url.UserPassword(cfg.User, cfg.Password), Path: "/" + name, RawQuery: "sslmode=disable"}
	open := func() (*sql.DB, *services.RuntimeState) {
		db, err := services.OpenSettingsDB(u.String(), "postgres")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = db.Close() })
		state, err := services.NewRuntimeState(db)
		if err != nil {
			t.Fatal(err)
		}
		return db, state
	}
	a, first := open()
	b, second := open()
	return a, first, b, second
}

func testSharedAuth(state *services.RuntimeState) *AuthHandler {
	h := &AuthHandler{sessionStore: NewSessionStore(), oneTimeStore: NewOneTimeTokenStore(), oauthState: NewOAuthStateStore(), avatarStore: NewAvatarStore(time.Hour)}
	h.SetRuntimeState(state)
	return h
}

func TestPostgresSharedAuthentication(t *testing.T) {
	firstDB, first, secondDB, second := sharedStateReplicas(t)
	a, b := testSharedAuth(first), testSharedAuth(second)
	c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/", nil), httptest.NewRecorder())
	if err := a.putOAuthState(c, "csrf", "provider", "pkce-verifier"); err != nil {
		t.Fatal(err)
	}
	entry, ok := b.takeOAuthState(c, "csrf")
	if !ok || entry.ProviderName != "provider" || entry.PKCEVerifier != "pkce-verifier" {
		t.Fatalf("OAuth handoff lost: %+v %v", entry, ok)
	}
	if _, ok := a.takeOAuthState(c, "csrf"); ok {
		t.Fatal("OAuth state replay accepted")
	}
	if err := a.putOneTimeToken(c, "login-token", "alice", true); err != nil {
		t.Fatal(err)
	}
	username, admin, ok := b.consumeOneTimeToken(c, "login-token")
	if !ok || username != "alice" || !admin {
		t.Fatal("OAuth login handoff lost")
	}
	if _, _, ok := a.consumeOneTimeToken(c, "login-token"); ok {
		t.Fatal("OAuth login replay accepted")
	}
	a.avatarStore.Put("alice", []byte("photo"), "image/png")
	if payload, typ, ok := b.avatarStore.Get("alice"); !ok || string(payload) != "photo" || typ != "image/png" {
		t.Fatal("avatar not shared")
	}
	if revoked, err := b.sessionIsRevoked(c, "session"); err != nil || revoked {
		t.Fatal("fresh session considered revoked")
	}
	if err := a.revokeSession(c, "session", time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	_ = firstDB.Close()
	if revoked, err := b.sessionIsRevoked(c, "session"); err != nil || !revoked {
		t.Fatal("revocation lost after replica shutdown")
	}
	if revoked, err := b.sessionIsRevoked(c, "active-session"); err != nil || revoked {
		t.Fatal("fresh active session failed")
	}
	_ = secondDB.Close()
	if revoked, err := b.sessionIsRevoked(c, "active-session"); err != nil || revoked {
		t.Fatal("recent answer lost during outage")
	}
	if revoked, err := b.sessionIsRevoked(c, "session"); err != nil || !revoked {
		t.Fatal("recent revocation lost during outage")
	}
	b.revocations.mu.Lock()
	b.revocations.entries["active-session"] = revocationAnswer{expires: time.Now().Add(-time.Second)}
	b.revocations.mu.Unlock()
	if _, err := b.sessionIsRevoked(c, "active-session"); err == nil {
		t.Fatal("expired answer masked outage")
	}

	if _, err := b.sessionIsRevoked(c, "unknown-session"); err == nil {
		t.Fatal("session lookup outage did not return an error")
	}
	if err := b.revokeSession(c, "another-session", time.Now().Add(time.Hour)); err == nil {
		t.Fatal("revocation reported success during database outage")
	}
}

func TestPostgresSharedExports(t *testing.T) {
	firstDB, first, _, second := sharedStateReplicas(t)
	a, b := NewExportsHandler(NewExportStore()), NewExportsHandler(NewExportStore())
	a.SetRuntimeState(first)
	b.SetRuntimeState(second)
	e := echo.New()
	makeContext := func(method string, body string) (echo.Context, *httptest.ResponseRecorder) {
		req := httptest.NewRequest(method, "/", strings.NewReader(body))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.Set("user", &jwt.Token{Claims: &JWTClaims{Username: "alice"}})
		return c, rec
	}
	c, rec := makeContext(http.MethodPost, `{"type":"text","query":{"callid":"shared-call"}}`)
	if err := a.V4ExportsCreate(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusCreated {
		t.Fatalf("create failed: %d %s", rec.Code, rec.Body.String())
	}
	var created ExportResponseV4
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	_ = firstDB.Close()
	c, rec = makeContext(http.MethodGet, "")
	c.SetParamNames("exportId")
	c.SetParamValues(created.Data.ExportID)
	if err := b.V4ExportsGet(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "completed") {
		t.Fatalf("status lost: %s", rec.Body.String())
	}
	c, rec = makeContext(http.MethodGet, "")
	c.SetParamNames("exportId")
	c.SetParamValues(created.Data.ExportID)
	if err := b.V4ExportsDownload(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "shared-call") {
		t.Fatalf("download lost: %s", rec.Body.String())
	}
}
