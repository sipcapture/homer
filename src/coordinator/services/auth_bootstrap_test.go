// Copyright (C) 2025 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package services

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/sipcapture/homer-core/src/config"

	_ "github.com/duckdb/duckdb-go/v2"
)

func TestEnsureBootstrapAdminUser_Idempotent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.duckdb")
	db, err := OpenSettingsDB(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := EnsureSettingsSchema(db); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	hash := config.LegacySHA256SipcaptureHash
	if _, err := EnsureBootstrapAdminUser(ctx, db, "admin", hash); err != nil {
		t.Fatalf("first: %v", err)
	}
	if _, err := EnsureBootstrapAdminUser(ctx, db, "admin", hash); err != nil {
		t.Fatalf("second: %v", err)
	}
	var n int64
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE username = 'admin'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("count: want 1, got %d", n)
	}
}

func TestEnsureBootstrapAdminUser_GeneratesPasswordWhenHashEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.duckdb")
	db, err := OpenSettingsDB(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := EnsureSettingsSchema(db); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	pwd, err := EnsureBootstrapAdminUser(ctx, db, "admin", "")
	if err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	if pwd == "" {
		t.Fatal("expected generated bootstrap password")
	}
	svc := NewUserService(db)
	u, err := svc.Authenticate(ctx, "admin", pwd)
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if u == nil || u.Username != "admin" {
		t.Fatalf("user %+v", u)
	}
	if _, err := EnsureBootstrapAdminUser(ctx, db, "admin", ""); err != nil {
		t.Fatalf("second bootstrap: %v", err)
	}
}

func TestResetOrInsertAdminPassword_UpdateThenInsert(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.duckdb")
	db, err := OpenSettingsDB(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := EnsureSettingsSchema(db); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	h1 := config.LegacySHA256SipcaptureHash
	if _, err := EnsureBootstrapAdminUser(ctx, db, "admin", h1); err != nil {
		t.Fatal(err)
	}
	h2 := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	if err := ResetOrInsertAdminPassword(ctx, db, "admin", h2); err != nil {
		t.Fatalf("update: %v", err)
	}
	var got string
	if err := db.QueryRowContext(ctx, `SELECT password_hash FROM users WHERE username = 'admin'`).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != h2 {
		t.Fatalf("password_hash: want %s, got %s", h2, got)
	}
	if err := ResetOrInsertAdminPassword(ctx, db, "other", h1); err != nil {
		t.Fatalf("insert other: %v", err)
	}
	var nOther int64
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE username = 'other'`).Scan(&nOther); err != nil {
		t.Fatal(err)
	}
	if nOther != 1 {
		t.Fatalf("other user count: want 1, got %d", nOther)
	}
}

func TestEnsureBootstrapAdminUser_RepairsEmptyPasswordHash(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.duckdb")
	db, err := OpenSettingsDB(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := EnsureSettingsSchema(db); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	hash := config.LegacySHA256SipcaptureHash
	_, err = db.ExecContext(ctx, `
		INSERT INTO users (username, password_hash, email, full_name, is_admin, is_active, created_at, updated_at)
		VALUES ('admin', NULL, '', '', true, true, current_timestamp, current_timestamp)`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := EnsureBootstrapAdminUser(ctx, db, "admin", hash); err != nil {
		t.Fatalf("bootstrap repair: %v", err)
	}
	var got string
	if err := db.QueryRowContext(ctx, `SELECT password_hash FROM users WHERE username = 'admin'`).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != hash {
		t.Fatalf("password_hash: want %s, got %s", hash, got)
	}
	svc := NewUserService(db)
	u, err := svc.Authenticate(ctx, "admin", "sipcapture")
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if u == nil || u.Username != "admin" {
		t.Fatalf("user %+v", u)
	}
}
