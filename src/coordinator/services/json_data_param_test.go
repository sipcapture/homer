// Copyright (C) 2026 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"testing"

	_ "github.com/duckdb/duckdb-go/v2"
)

const injectionLikeText = `O'Reilly \ C:\tmp '); DROP TABLE dashboard_settings; -- \u0027`

func TestJSONDataParam_StoresSameValueAsEscapedLiteral(t *testing.T) {
	db, err := sql.Open("duckdb", filepath.Join(t.TempDir(), "t.duckdb"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	payloads := []string{
		`{"a":"hello\nworld","p":"C:\\tmp","q":"O'Reilly"}`,
		`x\' OR 1=1 --`,
		`hello\`,
		"nul\x00byte",
		"bad\xffutf8",
		injectionLikeText,
	}
	for _, payload := range payloads {
		var literal, bound string
		if err := db.QueryRow(`SELECT '` + escapeJSONData(payload) + `'`).Scan(&literal); err != nil {
			t.Fatalf("literal %q: %v", payload, err)
		}
		if err := db.QueryRow(`SELECT ?::VARCHAR`, jsonDataParam(payload)).Scan(&bound); err != nil {
			t.Fatalf("bound %q: %v", payload, err)
		}
		if bound != literal {
			t.Fatalf("payload %q: bound %q != literal %q", payload, bound, literal)
		}
	}
}

func dashboardName(t *testing.T, raw json.RawMessage) string {
	t.Helper()
	var v struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatalf("unmarshal dashboard data %s: %v", raw, err)
	}
	return v.Name
}

func TestDashboard_PayloadWithQuotesRoundTrips(t *testing.T) {
	svc := newTestDashboardService(t)
	ctx := context.Background()

	create, err := json.Marshal(map[string]any{"name": injectionLikeText, "param": "q", "shared": false})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateDashboard(ctx, "alice", "quotes", create); err != nil {
		t.Fatalf("CreateDashboard: %v", err)
	}
	got, err := svc.GetDashboard(ctx, "alice", "quotes")
	if err != nil || got == nil {
		t.Fatalf("GetDashboard after create: %v %v", got, err)
	}
	if name := dashboardName(t, got.Data); name != injectionLikeText {
		t.Fatalf("created name = %q, want %q", name, injectionLikeText)
	}

	updatedName := injectionLikeText + " v2 ''"
	update, err := json.Marshal(map[string]any{"name": updatedName, "param": "q", "shared": false})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.UpdateDashboard(ctx, "alice", "quotes", update, false); err != nil {
		t.Fatalf("UpdateDashboard: %v", err)
	}
	got, err = svc.GetDashboard(ctx, "alice", "quotes")
	if err != nil || got == nil {
		t.Fatalf("GetDashboard after update: %v %v", got, err)
	}
	if name := dashboardName(t, got.Data); name != updatedName {
		t.Fatalf("updated name = %q, want %q", name, updatedName)
	}
}

func TestAuthTokenCreate_UserObjectWithQuotesRoundTrips(t *testing.T) {
	db, err := OpenSettingsDB(filepath.Join(t.TempDir(), "settings.duckdb"))
	if err != nil {
		t.Fatalf("OpenSettingsDB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := EnsureSettingsSchema(db); err != nil {
		t.Fatalf("EnsureSettingsSchema: %v", err)
	}
	svc := NewAuthTokenService(db)
	ctx := context.Background()

	userObject, err := json.Marshal(map[string]string{"username": injectionLikeText})
	if err != nil {
		t.Fatal(err)
	}
	created, err := svc.Create(ctx, AuthTokenItem{Name: "quotes", UserObject: userObject, Active: true})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, err := svc.GetByGUID(ctx, created.GUID)
	if err != nil || got == nil {
		t.Fatalf("GetByGUID: %v %v", got, err)
	}
	var obj map[string]string
	if err := json.Unmarshal(got.UserObject, &obj); err != nil {
		t.Fatalf("unmarshal user_object %s: %v", got.UserObject, err)
	}
	if obj["username"] != injectionLikeText {
		t.Fatalf("user_object username = %q, want %q", obj["username"], injectionLikeText)
	}
}
