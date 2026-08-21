package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v4"
	"github.com/sipcapture/homer-core/src/coordinator/services"
	"github.com/sipcapture/homer-core/src/passwordhash"

	_ "github.com/duckdb/duckdb-go/v2"
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

func newTestAuthHandlerWithUsers(t *testing.T) (*AuthHandler, *services.UserService) {
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
	h, err := passwordhash.Hash("testpass")
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.ExecContext(ctx, `
		INSERT INTO users (username, password_hash, email, full_name, is_admin, is_active, created_at, updated_at)
		VALUES ('admin', '`+h+`', 'admin@example.com', 'Admin', true, true, current_timestamp, current_timestamp)`)
	if err != nil {
		t.Fatal(err)
	}
	userSvc := services.NewUserService(db)
	auth := &AuthHandler{
		jwtSecret:   "test-secret-minimum-32-characters-long",
		expireHours: 24,
		userService: userSvc,
	}
	return auth, userSvc
}

func setJWTOnContext(t *testing.T, c echo.Context, h *AuthHandler, username string, admin bool) {
	setJWTOnContextWithMustChange(t, c, h, username, admin, false)
}

func setJWTOnContextWithMustChange(t *testing.T, c echo.Context, h *AuthHandler, username string, admin, mustChange bool) {
	t.Helper()
	tokenStr, _, err := h.generateToken(username, admin, mustChange)
	if err != nil {
		t.Fatal(err)
	}
	token, err := jwt.ParseWithClaims(tokenStr, &JWTClaims{}, func(token *jwt.Token) (interface{}, error) {
		return []byte(h.jwtSecret), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	c.Set("user", token)
}

func TestV4PatchMe_UpdatesPasswordAndEmail(t *testing.T) {
	h, userSvc := newTestAuthHandlerWithUsers(t)
	e := echo.New()

	body := `{"email":"new@example.com","password":"newsecret"}`
	req := httptest.NewRequest(http.MethodPatch, "/api/v4/me", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	setJWTOnContext(t, c, h, "admin", true)

	if err := h.V4PatchMe(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d body=%s", rec.Code, rec.Body.String())
	}

	var resp UserProfileResponseV4
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Data.Email != "new@example.com" {
		t.Fatalf("email: got %q want new@example.com", resp.Data.Email)
	}

	u, err := userSvc.Authenticate(context.Background(), "admin", "newsecret")
	if err != nil || u == nil {
		t.Fatalf("Authenticate with new password: err=%v user=%v", err, u)
	}
}

func TestV4PatchMe_NoFields(t *testing.T) {
	h, _ := newTestAuthHandlerWithUsers(t)
	e := echo.New()

	req := httptest.NewRequest(http.MethodPatch, "/api/v4/me", strings.NewReader(`{}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	setJWTOnContext(t, c, h, "admin", true)

	if err := h.V4PatchMe(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d want 400", rec.Code)
	}
}

func TestV4PatchMe_Unauthenticated(t *testing.T) {
	h, _ := newTestAuthHandlerWithUsers(t)
	e := echo.New()

	req := httptest.NewRequest(http.MethodPatch, "/api/v4/me", strings.NewReader(`{"password":"x"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := h.V4PatchMe(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status: got %d want 401", rec.Code)
	}
}

func TestJWTTokenFromAuthTokenItem_AdminUserGroupGrantsAdmin(t *testing.T) {
	h := &AuthHandler{}
	item := &services.AuthTokenItem{
		GUID:       "tok-1",
		UserObject: json.RawMessage(`{"username":"alice","user_group":"admin"}`),
	}

	tok := h.jwtTokenFromAuthTokenItem(item)
	claims, ok := tok.Claims.(*JWTClaims)
	if !ok {
		t.Fatalf("claims type: got %T", tok.Claims)
	}
	if claims.Username != "alice" {
		t.Fatalf("username: got %q want %q", claims.Username, "alice")
	}
	if !claims.Admin {
		t.Fatal("admin: got false want true (user_group=admin should grant admin scope)")
	}
}

func TestJWTTokenFromAuthTokenItem_NonAdminUserGroup(t *testing.T) {
	h := &AuthHandler{}
	item := &services.AuthTokenItem{
		GUID:       "tok-2",
		UserObject: json.RawMessage(`{"username":"bob","user_group":"user"}`),
	}

	tok := h.jwtTokenFromAuthTokenItem(item)
	claims := tok.Claims.(*JWTClaims)
	if claims.Username != "bob" {
		t.Fatalf("username: got %q want %q", claims.Username, "bob")
	}
	if claims.Admin {
		t.Fatal("admin: got true want false")
	}
}

func newTestAuthHandlerWithSipcaptureHash(t *testing.T) *AuthHandler {
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
	_, err = db.ExecContext(ctx, `
		INSERT INTO users (username, password_hash, email, full_name, is_admin, is_active, created_at, updated_at)
		VALUES ('admin', '`+passwordhash.LegacySHA256SipcaptureHash+`', '', '', true, true, current_timestamp, current_timestamp)`)
	if err != nil {
		t.Fatal(err)
	}
	return &AuthHandler{
		jwtSecret:   "test-secret-minimum-32-characters-long",
		expireHours: 24,
		userService: services.NewUserService(db),
	}
}

func TestV4CreateSession_SipcaptureSetsMustChangePassword(t *testing.T) {
	h := newTestAuthHandlerWithSipcaptureHash(t)
	e := echo.New()
	body := `{"username":"admin","password":"sipcapture","type":"internal"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v4/auth/sessions", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := h.V4CreateSession(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusCreated {
		t.Fatalf("status: got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp LoginResponseV4
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.Data.User.MustChangePassword {
		t.Fatal("must_change_password: want true")
	}
	tok, err := jwt.ParseWithClaims(resp.Data.Token, &JWTClaims{}, func(token *jwt.Token) (interface{}, error) {
		return []byte(h.jwtSecret), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	claims := tok.Claims.(*JWTClaims)
	if !claims.MustChangePassword {
		t.Fatal("JWT must_change_password: want true")
	}
}

func TestJWTMiddlewareV4_BlocksUntilPasswordChanged(t *testing.T) {
	h := newTestAuthHandler(false)
	e := echo.New()
	g := e.Group("/api/v4")
	g.Use(h.JWTMiddlewareV4())
	g.GET("/me", func(c echo.Context) error { return c.NoContent(http.StatusOK) })
	g.GET("/dashboards", func(c echo.Context) error { return c.NoContent(http.StatusOK) })

	token, _, err := h.generateToken("admin", true, true)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v4/dashboards", nil)
	req.Header.Set(echo.HeaderAuthorization, "Bearer "+token)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("dashboards: got %d want 403 body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v4/me", nil)
	req.Header.Set(echo.HeaderAuthorization, "Bearer "+token)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("me: got %d want 200 body=%s", rec.Code, rec.Body.String())
	}
}

func TestV4PatchMe_MustChangeRequiresPassword(t *testing.T) {
	h, _ := newTestAuthHandlerWithUsers(t)
	e := echo.New()
	req := httptest.NewRequest(http.MethodPatch, "/api/v4/me", strings.NewReader(`{"email":"x@example.com"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	setJWTOnContextWithMustChange(t, c, h, "admin", true, true)

	if err := h.V4PatchMe(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestV4PatchMe_RejectsSipcapturePassword(t *testing.T) {
	h, _ := newTestAuthHandlerWithUsers(t)
	e := echo.New()
	req := httptest.NewRequest(http.MethodPatch, "/api/v4/me", strings.NewReader(`{"password":"sipcapture"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	setJWTOnContext(t, c, h, "admin", true)

	if err := h.V4PatchMe(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d body=%s", rec.Code, rec.Body.String())
	}
}
