// Copyright (C) 2025 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package services

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	_ "github.com/duckdb/duckdb-go/v2"
)

func TestScriptsService_UpdatePreservesLongScript(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.db")
	db, err := OpenSettingsDB(path)
	if err != nil {
		t.Fatalf("OpenSettingsDB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := EnsureSettingsSchema(db); err != nil {
		t.Fatalf("EnsureSettingsSchema: %v", err)
	}

	ctx := context.Background()
	svc := NewScriptsService(db)

	longLua := "-- x\n" + strings.Repeat("local a = 1\n", 400) // >> 1000 chars (SafeString limit)
	wantLen := len(longLua)

	guid, err := svc.Create(ctx, HepScript{
		Profile:  "call",
		HepAlias: "SIP",
		Type:     "correlation",
		HepID:    1,
		Status:   false,
		Script:   "function correlate() return {} end",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	_, err = svc.Update(ctx, guid, HepScript{
		Profile:  "call",
		HepAlias: "SIP",
		Type:     "correlation",
		HepID:    1,
		Status:   false,
		Script:   longLua,
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, err := svc.GetByGUID(ctx, guid)
	if err != nil {
		t.Fatalf("GetByGUID: %v", err)
	}
	if got == nil {
		t.Fatal("expected row after update")
	}
	if len(got.Script) != wantLen {
		t.Fatalf("script length: got %d want %d (truncation bug if want > 1000)", len(got.Script), wantLen)
	}
	if got.Script != longLua {
		t.Fatal("script content mismatch")
	}
}
