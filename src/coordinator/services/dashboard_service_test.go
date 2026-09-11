// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published
// by the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package services

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sipcapture/homer-core/src/config"
)

// TestResetDashboards_SeedsDefaultsIncludingGames pins the set of dashboards
// a brand-new user gets on first login: Home, Smart Search, Games, NetGames.
// The Games tab must contain every single-player game widget; NetGames must
// contain the multiplayer ones. The layout numbers themselves are tweakable —
// the test only asserts the wiring, not pixel-perfect grid coords.
func TestResetDashboards_SeedsDefaultsIncludingGames(t *testing.T) {
	dir := t.TempDir()
	db, err := OpenSettingsDB(filepath.Join(dir, "settings.duckdb"))
	if err != nil {
		t.Fatalf("OpenSettingsDB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := EnsureSettingsSchema(db); err != nil {
		t.Fatalf("EnsureSettingsSchema: %v", err)
	}

	svc := NewDashboardService(db, config.DefaultWidgetControl())
	ctx := context.Background()
	const user = "alice"

	if err := svc.ResetDashboards(ctx, user); err != nil {
		t.Fatalf("ResetDashboards: %v", err)
	}

	settings, err := svc.ListDashboards(ctx, user)
	if err != nil {
		t.Fatalf("ListDashboards: %v", err)
	}

	byParam := make(map[string]json.RawMessage, len(settings))
	for _, s := range settings {
		byParam[s.Param] = s.Data
	}

	expectedDashboards := []string{"home", "smartsearch", "games", "netgames"}
	for _, p := range expectedDashboards {
		if _, ok := byParam[p]; !ok {
			t.Fatalf("default dashboard %q missing; got %v", p, keys(byParam))
		}
	}

	games := decodeWidgets(t, byParam["games"])
	wantGames := map[string]bool{
		"packet_defender":    false,
		"sip_dialog_master":  false,
		"jitter_buffer_hero": false,
		"sipetris":           false,
		"chess":              false,
	}
	for _, w := range games {
		if _, ok := wantGames[w]; ok {
			wantGames[w] = true
		}
	}
	for w, present := range wantGames {
		if !present {
			t.Errorf("Games dashboard missing widget type %q (got %v)", w, games)
		}
	}

	netGames := decodeWidgets(t, byParam["netgames"])
	wantNet := map[string]bool{"netris": false, "netchess": false}
	for _, w := range netGames {
		if _, ok := wantNet[w]; ok {
			wantNet[w] = true
		}
	}
	for w, present := range wantNet {
		if !present {
			t.Errorf("NetGames dashboard missing widget type %q (got %v)", w, netGames)
		}
	}
}

func TestResetDashboards_SkipsGamesWhenDisabled(t *testing.T) {
	dir := t.TempDir()
	db, err := OpenSettingsDB(filepath.Join(dir, "settings.duckdb"))
	if err != nil {
		t.Fatalf("OpenSettingsDB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := EnsureSettingsSchema(db); err != nil {
		t.Fatalf("EnsureSettingsSchema: %v", err)
	}

	control := config.NormalizeWidgetControl(map[string]bool{"games": false})
	svc := NewDashboardService(db, control)
	ctx := context.Background()

	if err := svc.ResetDashboards(ctx, "carol"); err != nil {
		t.Fatalf("ResetDashboards: %v", err)
	}

	settings, err := svc.ListDashboards(ctx, "carol")
	if err != nil {
		t.Fatalf("ListDashboards: %v", err)
	}

	byParam := make(map[string]json.RawMessage, len(settings))
	for _, s := range settings {
		byParam[s.Param] = s.Data
	}

	expected := []string{"home", "smartsearch"}
	for _, p := range expected {
		if _, ok := byParam[p]; !ok {
			t.Fatalf("dashboard %q missing; got %v", p, keys(byParam))
		}
	}
	if _, ok := byParam["games"]; ok {
		t.Fatal("games dashboard should not be seeded when disabled")
	}
	if _, ok := byParam["netgames"]; ok {
		t.Fatal("netgames dashboard should not be seeded when disabled")
	}
}

// TestResetDashboards_IsIdempotent confirms a second call replaces the
// dashboards in place rather than duplicating them — the API handler calls
// Reset whenever ListDashboards returns nothing, so a partially-deleted user
// must converge on the seed set.
func TestResetDashboards_IsIdempotent(t *testing.T) {
	dir := t.TempDir()
	db, err := OpenSettingsDB(filepath.Join(dir, "settings.duckdb"))
	if err != nil {
		t.Fatalf("OpenSettingsDB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := EnsureSettingsSchema(db); err != nil {
		t.Fatalf("EnsureSettingsSchema: %v", err)
	}

	svc := NewDashboardService(db, config.DefaultWidgetControl())
	ctx := context.Background()

	if err := svc.ResetDashboards(ctx, "bob"); err != nil {
		t.Fatalf("first ResetDashboards: %v", err)
	}
	first, err := svc.ListDashboards(ctx, "bob")
	if err != nil {
		t.Fatalf("first ListDashboards: %v", err)
	}

	if err := svc.ResetDashboards(ctx, "bob"); err != nil {
		t.Fatalf("second ResetDashboards: %v", err)
	}
	second, err := svc.ListDashboards(ctx, "bob")
	if err != nil {
		t.Fatalf("second ListDashboards: %v", err)
	}

	if len(first) != len(second) {
		t.Fatalf("dashboard count drifted: first=%d second=%d", len(first), len(second))
	}
}

// TestResetDashboards_SeedsIndependentDefaultsPerUser is issue #936: default
// IDs (home, smartsearch, …) are per-user, not globally unique. Seeding for
// alice must not block bob's auto-seed or reset.
func TestResetDashboards_SeedsIndependentDefaultsPerUser(t *testing.T) {
	dir := t.TempDir()
	db, err := OpenSettingsDB(filepath.Join(dir, "settings.duckdb"))
	if err != nil {
		t.Fatalf("OpenSettingsDB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := EnsureSettingsSchema(db); err != nil {
		t.Fatalf("EnsureSettingsSchema: %v", err)
	}

	svc := NewDashboardService(db, config.DefaultWidgetControl())
	ctx := context.Background()

	if err := svc.ResetDashboards(ctx, "alice"); err != nil {
		t.Fatalf("ResetDashboards alice: %v", err)
	}
	if err := svc.ResetDashboards(ctx, "bob"); err != nil {
		t.Fatalf("ResetDashboards bob: %v", err)
	}

	for _, user := range []string{"alice", "bob"} {
		settings, err := svc.ListDashboards(ctx, user)
		if err != nil {
			t.Fatalf("ListDashboards %s: %v", user, err)
		}
		byParam := make(map[string]string, len(settings))
		for _, s := range settings {
			byParam[s.Param] = s.UserName
		}
		for _, id := range []string{"home", "smartsearch", "games", "netgames"} {
			owner, ok := byParam[id]
			if !ok {
				t.Fatalf("%s missing default dashboard %q; got %v", user, id, keys(byParam))
			}
			if !strings.EqualFold(owner, user) {
				t.Errorf("%s listed %q owned by %q", user, id, owner)
			}
		}

		got, err := svc.GetDashboard(ctx, user, "home")
		if err != nil {
			t.Fatalf("GetDashboard %s/home: %v", user, err)
		}
		if got == nil {
			t.Fatalf("GetDashboard %s/home: nil", user)
		}
		if !strings.EqualFold(got.UserName, user) {
			t.Errorf("GetDashboard %s/home owner=%q", user, got.UserName)
		}
	}
}

func newTestDashboardService(t *testing.T) *DashboardService {
	t.Helper()
	dir := t.TempDir()
	db, err := OpenSettingsDB(filepath.Join(dir, "settings.duckdb"))
	if err != nil {
		t.Fatalf("OpenSettingsDB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := EnsureSettingsSchema(db); err != nil {
		t.Fatalf("EnsureSettingsSchema: %v", err)
	}
	return NewDashboardService(db, config.DefaultWidgetControl())
}

func TestCreateDashboard_AllowsSameIDForDifferentUsers(t *testing.T) {
	svc := newTestDashboardService(t)
	ctx := context.Background()
	payload := json.RawMessage(`{"name":"Custom","param":"custom","shared":false}`)

	if _, err := svc.CreateDashboard(ctx, "alice", "custom", payload); err != nil {
		t.Fatalf("CreateDashboard alice: %v", err)
	}
	if _, err := svc.CreateDashboard(ctx, "bob", "custom", payload); err != nil {
		t.Fatalf("CreateDashboard bob: %v", err)
	}
	if _, err := svc.CreateDashboard(ctx, "alice", "custom", payload); err == nil {
		t.Fatal("expected error creating duplicate dashboard for same user")
	}
}

func TestUpdateDashboard_OwnerCanUpdateOwnShared(t *testing.T) {
	svc := newTestDashboardService(t)
	ctx := context.Background()
	if _, err := svc.CreateDashboard(ctx, "alice", "ops", json.RawMessage(`{"name":"Before","shared":true}`)); err != nil {
		t.Fatalf("CreateDashboard: %v", err)
	}

	guid, err := svc.UpdateDashboard(ctx, "alice", "ops", json.RawMessage(`{"name":"After","shared":true}`), false)
	if err != nil {
		t.Fatalf("UpdateDashboard: %v", err)
	}
	if guid == "" {
		t.Fatal("owner update returned empty guid")
	}

	got, err := svc.GetDashboard(ctx, "alice", "ops")
	if err != nil {
		t.Fatalf("GetDashboard: %v", err)
	}
	if got == nil || !strings.Contains(string(got.Data), `"After"`) {
		t.Fatalf("owner update did not persist: %#v", got)
	}
}

func TestUpdateDashboard_AdminCanUpdateSharedDashboard(t *testing.T) {
	svc := newTestDashboardService(t)
	ctx := context.Background()
	if _, err := svc.CreateDashboard(ctx, "alice", "ops", json.RawMessage(`{"name":"Before","owner":"alice","shared":true}`)); err != nil {
		t.Fatalf("CreateDashboard: %v", err)
	}

	guid, err := svc.UpdateDashboard(ctx, "bob", "ops", json.RawMessage(`{"name":"After","owner":"bob","shared":true}`), true)
	if err != nil {
		t.Fatalf("UpdateDashboard: %v", err)
	}
	if guid == "" {
		t.Fatal("admin update of shared dashboard returned empty guid")
	}

	got, err := svc.GetDashboard(ctx, "carol", "ops")
	if err != nil {
		t.Fatalf("GetDashboard: %v", err)
	}
	if got == nil {
		t.Fatal("GetDashboard: nil")
	}
	if !strings.EqualFold(got.UserName, "alice") {
		t.Errorf("admin update stole username column: got %q", got.UserName)
	}
	if dashboardJSONField(t, got.Data, "owner") != "alice" {
		t.Errorf("admin update stole JSON owner: %s", got.Data)
	}
	if dashboardJSONField(t, got.Data, "name") != "After" {
		t.Fatalf("shared dashboard was not updated: %s", got.Data)
	}
}

func TestUpdateDashboard_NonAdminCannotUpdateSharedDashboard(t *testing.T) {
	svc := newTestDashboardService(t)
	ctx := context.Background()
	if _, err := svc.CreateDashboard(ctx, "alice", "ops", json.RawMessage(`{"name":"Before","shared":true}`)); err != nil {
		t.Fatalf("CreateDashboard: %v", err)
	}

	guid, err := svc.UpdateDashboard(ctx, "bob", "ops", json.RawMessage(`{"name":"After","shared":true}`), false)
	if guid != "" {
		t.Fatalf("non-admin update returned guid %q", guid)
	}
	if err != ErrDashboardNotWritable {
		t.Fatalf("UpdateDashboard err=%v want ErrDashboardNotWritable", err)
	}

	got, err := svc.GetDashboard(ctx, "bob", "ops")
	if err != nil {
		t.Fatalf("GetDashboard: %v", err)
	}
	if got == nil || strings.Contains(string(got.Data), `"After"`) {
		t.Fatalf("non-admin must not mutate shared dashboard: %#v", got)
	}
}

func TestUpdateDashboard_AdminCannotUpdatePrivateDashboard(t *testing.T) {
	svc := newTestDashboardService(t)
	ctx := context.Background()
	if _, err := svc.CreateDashboard(ctx, "alice", "private", json.RawMessage(`{"name":"Before","shared":false}`)); err != nil {
		t.Fatalf("CreateDashboard: %v", err)
	}

	guid, err := svc.UpdateDashboard(ctx, "bob", "private", json.RawMessage(`{"name":"After","shared":false}`), true)
	if err != nil {
		t.Fatalf("UpdateDashboard: %v", err)
	}
	if guid != "" {
		t.Fatal("admin must not update another user's private dashboard")
	}

	got, err := svc.GetDashboard(ctx, "alice", "private")
	if err != nil {
		t.Fatalf("GetDashboard: %v", err)
	}
	if got == nil || strings.Contains(string(got.Data), `"After"`) {
		t.Fatalf("private dashboard was mutated: %#v", got)
	}
}

func TestUpdateDashboard_SameIDDoesNotClobberOtherUser(t *testing.T) {
	svc := newTestDashboardService(t)
	ctx := context.Background()
	if _, err := svc.CreateDashboard(ctx, "alice", "home", json.RawMessage(`{"name":"Alice Home","shared":false}`)); err != nil {
		t.Fatalf("CreateDashboard alice: %v", err)
	}
	if _, err := svc.CreateDashboard(ctx, "bob", "home", json.RawMessage(`{"name":"Bob Home","shared":true}`)); err != nil {
		t.Fatalf("CreateDashboard bob: %v", err)
	}

	guid, err := svc.UpdateDashboard(ctx, "carol", "home", json.RawMessage(`{"name":"Admin Edit","shared":true}`), true)
	if err != nil {
		t.Fatalf("UpdateDashboard: %v", err)
	}
	if guid == "" {
		t.Fatal("admin update of shared home returned empty guid")
	}

	alice, err := svc.GetDashboard(ctx, "alice", "home")
	if err != nil {
		t.Fatalf("GetDashboard alice: %v", err)
	}
	if alice == nil || !strings.Contains(string(alice.Data), `"Alice Home"`) {
		t.Fatalf("alice private home was clobbered: %#v", alice)
	}
	if strings.Contains(string(alice.Data), `"Admin Edit"`) {
		t.Fatal("alice private home picked up admin edit")
	}

	bob, err := svc.GetDashboard(ctx, "bob", "home")
	if err != nil {
		t.Fatalf("GetDashboard bob: %v", err)
	}
	if bob == nil || !strings.Contains(string(bob.Data), `"Admin Edit"`) {
		t.Fatalf("bob shared home was not updated: %#v", bob)
	}
	if !strings.EqualFold(bob.UserName, "bob") {
		t.Errorf("bob home owner drifted to %q", bob.UserName)
	}
}

func dashboardJSONField(t *testing.T, raw json.RawMessage, key string) string {
	t.Helper()
	var data map[string]interface{}
	if err := json.Unmarshal(raw, &data); err != nil {
		t.Fatalf("decode dashboard data: %v", err)
	}
	v, _ := data[key].(string)
	return v
}

func decodeWidgets(t *testing.T, raw json.RawMessage) []string {
	t.Helper()
	var doc struct {
		Widgets []struct {
			Type string `json:"type"`
		} `json:"widgets"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("decode dashboard data: %v", err)
	}
	out := make([]string, 0, len(doc.Widgets))
	for _, w := range doc.Widgets {
		out = append(out, w.Type)
	}
	return out
}

func keys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
