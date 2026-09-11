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
	"github.com/sipcapture/homer-core/src/config"
	"github.com/sipcapture/homer-core/src/coordinator/services"
)

func newTestDashboardsHandler(t *testing.T) *DashboardsHandler {
	t.Helper()
	db, err := services.OpenSettingsDB(filepath.Join(t.TempDir(), "settings.duckdb"))
	if err != nil {
		t.Fatalf("OpenSettingsDB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := services.EnsureSettingsSchema(db); err != nil {
		t.Fatalf("EnsureSettingsSchema: %v", err)
	}
	return NewDashboardsHandler(services.NewDashboardService(db, config.DefaultWidgetControl()))
}

func setDashboardJWT(c echo.Context, username string, admin bool) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, &JWTClaims{
		Username: username,
		Admin:    admin,
	})
	c.Set("user", token)
}

func putDashboard(t *testing.T, h *DashboardsHandler, username string, admin bool, dashboardID, body string) *httptest.ResponseRecorder {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest(http.MethodPut, "/api/v4/dashboards/"+dashboardID, strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("dashboardId")
	c.SetParamValues(dashboardID)
	setDashboardJWT(c, username, admin)
	if err := h.V4DashboardsUpdate(c); err != nil {
		t.Fatal(err)
	}
	return rec
}

func TestV4DashboardsUpdate_AdminCanSaveSharedDashboard(t *testing.T) {
	h := newTestDashboardsHandler(t)
	ctx := context.Background()
	if _, err := h.service.CreateDashboard(ctx, "alice", "ops", json.RawMessage(`{"name":"Before","owner":"alice","shared":true}`)); err != nil {
		t.Fatalf("CreateDashboard: %v", err)
	}

	rec := putDashboard(t, h, "bob", true, "ops", `{"name":"After","owner":"bob","shared":true}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200 body=%s", rec.Code, rec.Body.String())
	}

	got, err := h.service.GetDashboard(ctx, "carol", "ops")
	if err != nil || got == nil {
		t.Fatalf("GetDashboard: err=%v got=%v", err, got)
	}
	if !strings.EqualFold(got.UserName, "alice") {
		t.Errorf("username column: got %q want alice", got.UserName)
	}
	var data map[string]interface{}
	if err := json.Unmarshal(got.Data, &data); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if data["name"] != "After" {
		t.Errorf("name: got %#v", data["name"])
	}
	if data["owner"] != "alice" {
		t.Errorf("JSON owner: got %#v want alice", data["owner"])
	}
}

func TestV4DashboardsUpdate_NonAdminGets403OnSharedDashboard(t *testing.T) {
	h := newTestDashboardsHandler(t)
	ctx := context.Background()
	if _, err := h.service.CreateDashboard(ctx, "alice", "ops", json.RawMessage(`{"name":"Before","shared":true}`)); err != nil {
		t.Fatalf("CreateDashboard: %v", err)
	}

	rec := putDashboard(t, h, "bob", false, "ops", `{"name":"After","shared":true}`)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status: got %d want 403 body=%s", rec.Code, rec.Body.String())
	}
}

func TestV4DashboardsUpdate_MissingDashboardIs404(t *testing.T) {
	h := newTestDashboardsHandler(t)
	rec := putDashboard(t, h, "bob", true, "missing", `{"name":"Nope"}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status: got %d want 404 body=%s", rec.Code, rec.Body.String())
	}
}
