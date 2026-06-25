// Copyright (C) 2025 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package services

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sipcapture/homer-core/src/config"

	_ "github.com/duckdb/duckdb-go/v2"
)

func TestAuthenticate_IsActiveNullMeansActive(t *testing.T) {
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
	h := config.LegacySHA256SipcaptureHash
	_, err = db.ExecContext(ctx, `
		INSERT INTO users (username, password_hash, email, full_name, is_admin, is_active, created_at, updated_at)
		VALUES ('legacy', '`+h+`', '', '', true, NULL, current_timestamp, current_timestamp)`)
	if err != nil {
		t.Fatal(err)
	}
	svc := NewUserService(db)
	u, err := svc.Authenticate(ctx, "legacy", "sipcapture")
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if u == nil || u.Username != "legacy" {
		t.Fatalf("user: %+v", u)
	}
}

func TestAuthenticate_IsActiveFalseRejected(t *testing.T) {
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
	h := config.LegacySHA256SipcaptureHash
	_, err = db.ExecContext(ctx, `
		INSERT INTO users (username, password_hash, email, full_name, is_admin, is_active, created_at, updated_at)
		VALUES ('blocked', '`+h+`', '', '', true, false, current_timestamp, current_timestamp)`)
	if err != nil {
		t.Fatal(err)
	}
	svc := NewUserService(db)
	_, err = svc.Authenticate(ctx, "blocked", "sipcapture")
	if err == nil {
		t.Fatal("want error for disabled user")
	}
}

func TestAuthenticate_PasswordHashHexCaseInsensitive(t *testing.T) {
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
	h := strings.ToUpper(config.LegacySHA256SipcaptureHash)
	_, err = db.ExecContext(ctx, `
		INSERT INTO users (username, password_hash, email, full_name, is_admin, is_active, created_at, updated_at)
		VALUES ('mixedcase', '`+h+`', '', '', true, true, current_timestamp, current_timestamp)`)
	if err != nil {
		t.Fatal(err)
	}
	svc := NewUserService(db)
	u, err := svc.Authenticate(ctx, "mixedcase", "sipcapture")
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if u == nil {
		t.Fatal("nil user")
	}
}

func TestAuthenticate_TrimsUsernameAndPassword(t *testing.T) {
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
	h := config.LegacySHA256SipcaptureHash
	_, err = db.ExecContext(ctx, `
		INSERT INTO users (username, password_hash, email, full_name, is_admin, is_active, created_at, updated_at)
		VALUES ('admin', '`+h+`', '', '', true, true, current_timestamp, current_timestamp)`)
	if err != nil {
		t.Fatal(err)
	}
	svc := NewUserService(db)
	u, err := svc.Authenticate(ctx, "  admin\t", "  sipcapture\n")
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if u.Username != "admin" {
		t.Fatalf("username: %q", u.Username)
	}
}

func TestGetUserByUsername_CaseInsensitive(t *testing.T) {
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
	h := config.LegacySHA256SipcaptureHash
	_, err = db.ExecContext(ctx, `
		INSERT INTO users (username, password_hash, email, full_name, is_admin, is_active, created_at, updated_at)
		VALUES ('ADMIN', '`+h+`', '', '', true, true, current_timestamp, current_timestamp)`)
	if err != nil {
		t.Fatal(err)
	}
	svc := NewUserService(db)
	u, err := svc.GetUserByUsername(ctx, "admin")
	if err != nil {
		t.Fatalf("GetUserByUsername: %v", err)
	}
	if u == nil || u.Username != "ADMIN" {
		t.Fatalf("user %+v", u)
	}
	_, err = svc.Authenticate(ctx, "admin", "sipcapture")
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
}
