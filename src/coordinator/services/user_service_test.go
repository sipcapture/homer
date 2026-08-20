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

// sha256("testpass") — not the well-known sipcapture digest.
const testLegacySHA256 = "13d249f2cb4127b40cfa757866850278793f814ded3c587fe5889e889a7a9f6c"

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
	h := testLegacySHA256
	_, err = db.ExecContext(ctx, `
		INSERT INTO users (username, password_hash, email, full_name, is_admin, is_active, created_at, updated_at)
		VALUES ('legacy', '`+h+`', '', '', true, NULL, current_timestamp, current_timestamp)`)
	if err != nil {
		t.Fatal(err)
	}
	svc := NewUserService(db)
	u, err := svc.Authenticate(ctx, "legacy", "testpass")
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
	h := testLegacySHA256
	_, err = db.ExecContext(ctx, `
		INSERT INTO users (username, password_hash, email, full_name, is_admin, is_active, created_at, updated_at)
		VALUES ('blocked', '`+h+`', '', '', true, false, current_timestamp, current_timestamp)`)
	if err != nil {
		t.Fatal(err)
	}
	svc := NewUserService(db)
	_, err = svc.Authenticate(ctx, "blocked", "testpass")
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
	h := strings.ToUpper(testLegacySHA256)
	_, err = db.ExecContext(ctx, `
		INSERT INTO users (username, password_hash, email, full_name, is_admin, is_active, created_at, updated_at)
		VALUES ('mixedcase', '`+h+`', '', '', true, true, current_timestamp, current_timestamp)`)
	if err != nil {
		t.Fatal(err)
	}
	svc := NewUserService(db)
	u, err := svc.Authenticate(ctx, "mixedcase", "testpass")
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
	h := testLegacySHA256
	_, err = db.ExecContext(ctx, `
		INSERT INTO users (username, password_hash, email, full_name, is_admin, is_active, created_at, updated_at)
		VALUES ('admin', '`+h+`', '', '', true, true, current_timestamp, current_timestamp)`)
	if err != nil {
		t.Fatal(err)
	}
	svc := NewUserService(db)
	u, err := svc.Authenticate(ctx, "  admin\t", "  testpass\n")
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
	h := testLegacySHA256
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
	_, err = svc.Authenticate(ctx, "admin", "testpass")
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
}

func TestAuthenticate_RejectsSipcaptureDefaultHash(t *testing.T) {
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
	h := "883ffc1f37fd0fe542b0fb9740035c4383e7d976c411161d24e62edace280f90"
	_, err = db.ExecContext(ctx, `
		INSERT INTO users (username, password_hash, email, full_name, is_admin, is_active, created_at, updated_at)
		VALUES ('admin', '`+h+`', '', '', true, true, current_timestamp, current_timestamp)`)
	if err != nil {
		t.Fatal(err)
	}
	svc := NewUserService(db)
	if _, err := svc.Authenticate(ctx, "admin", "sipcapture"); err == nil {
		t.Fatal("sipcapture default hash must not authenticate")
	}
}
