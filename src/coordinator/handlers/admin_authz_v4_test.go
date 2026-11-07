package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/sipcapture/homer-core/src/coordinator/services"
)

func newTestSettingsDB(t *testing.T) *services.AlertsService {
	t.Helper()
	db, err := services.OpenSettingsDB(filepath.Join(t.TempDir(), "settings.duckdb"))
	if err != nil {
		t.Fatalf("OpenSettingsDB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := services.EnsureSettingsSchema(db); err != nil {
		t.Fatalf("EnsureSettingsSchema: %v", err)
	}
	return services.NewAlertsService(db)
}

func newTestAgentSubsHandler(t *testing.T) *AgentSubscriptionsHandler {
	t.Helper()
	db, err := services.OpenSettingsDB(filepath.Join(t.TempDir(), "settings.duckdb"))
	if err != nil {
		t.Fatalf("OpenSettingsDB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := services.EnsureSettingsSchema(db); err != nil {
		t.Fatalf("EnsureSettingsSchema: %v", err)
	}
	return NewAgentSubscriptionsHandler(services.NewAgentSubscriptionService(db))
}

func callJSON(t *testing.T, method, target, body string, admin *bool, handler func(echo.Context) error, paramName, paramValue string) *httptest.ResponseRecorder {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	if paramName != "" {
		c.SetParamNames(paramName)
		c.SetParamValues(paramValue)
	}
	if admin != nil {
		setDashboardJWT(c, "alice", *admin)
	}
	if err := handler(c); err != nil {
		t.Fatal(err)
	}
	return rec
}

func TestV4AgentSubsWriteRequiresAdmin(t *testing.T) {
	h := newTestAgentSubsHandler(t)
	body := `{"type":"hepic","host":"10.0.0.8","port":9060,"protocol":"udp"}`
	nonAdmin := false
	admin := true

	rec := callJSON(t, http.MethodPost, "/api/v4/agents/subscriptions", body, nil, h.V4AgentSubsCreate, "", "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("no token: got %d want 401 body=%s", rec.Code, rec.Body.String())
	}

	rec = callJSON(t, http.MethodPost, "/api/v4/agents/subscriptions", body, &nonAdmin, h.V4AgentSubsCreate, "", "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("non-admin create: got %d want 403 body=%s", rec.Code, rec.Body.String())
	}

	rec = callJSON(t, http.MethodPut, "/api/v4/agents/subscriptions/missing", body, &nonAdmin, h.V4AgentSubsUpdate, "subscriptionId", "missing")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("non-admin update: got %d want 403 body=%s", rec.Code, rec.Body.String())
	}

	rec = callJSON(t, http.MethodDelete, "/api/v4/agents/subscriptions/missing", "", &nonAdmin, h.V4AgentSubsDelete, "subscriptionId", "missing")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("non-admin delete: got %d want 403 body=%s", rec.Code, rec.Body.String())
	}

	rec = callJSON(t, http.MethodPost, "/api/v4/agents/subscriptions", body, &admin, h.V4AgentSubsCreate, "", "")
	if rec.Code != http.StatusCreated {
		t.Fatalf("admin create: got %d want 201 body=%s", rec.Code, rec.Body.String())
	}
}

func TestV4AlertsDeleteAllRequiresAdmin(t *testing.T) {
	svc := newTestSettingsDB(t)
	h := NewAlertsHandler(svc)
	ctx := context.Background()
	if _, err := svc.Insert(ctx, "critical", "asr", "drop", nil); err != nil {
		t.Fatal(err)
	}

	nonAdmin := false
	admin := true
	body := `{"title":"from grafana","message":"asr drop"}`

	rec := callJSON(t, http.MethodDelete, "/api/v4/alerts", "", nil, h.V4AlertsDeleteAll, "", "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("no token: got %d want 401 body=%s", rec.Code, rec.Body.String())
	}

	rec = callJSON(t, http.MethodDelete, "/api/v4/alerts", "", &nonAdmin, h.V4AlertsDeleteAll, "", "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("non-admin delete: got %d want 403 body=%s", rec.Code, rec.Body.String())
	}
	items, err := svc.List(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("alerts after forbidden delete: got %d want 1", len(items))
	}

	rec = callJSON(t, http.MethodPost, "/api/v4/alerts", body, &nonAdmin, h.V4AlertsCreate, "", "")
	if rec.Code != http.StatusCreated {
		t.Fatalf("non-admin create: got %d want 201 body=%s", rec.Code, rec.Body.String())
	}

	rec = callJSON(t, http.MethodDelete, "/api/v4/alerts", "", &admin, h.V4AlertsDeleteAll, "", "")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("admin delete: got %d want 204 body=%s", rec.Code, rec.Body.String())
	}
	items, err = svc.List(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Fatalf("alerts after admin delete: got %d want 0", len(items))
	}
}
