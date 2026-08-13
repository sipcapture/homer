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

func TestCreateDashboard_AllowsSameIDForDifferentUsers(t *testing.T) {
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
