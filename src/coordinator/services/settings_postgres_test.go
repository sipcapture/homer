// Copyright (C) 2026 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// The opt-in DSN must grant CREATE DATABASE on a disposable test server.
// Each run owns a separate database, never the configured application's tables.
func openPostgresSettingsForTest(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("HOMER_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set HOMER_TEST_POSTGRES_DSN for PostgreSQL integration tests")
	}
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	name := "homer_settings_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		_ = admin.Close(ctx)
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec(ctx, "DROP DATABASE "+name+" WITH (FORCE)")
		_ = admin.Close(ctx)
	})
	installer, err := sql.Open("duckdb", "")
	if err != nil {
		t.Fatal(err)
	}
	_, installErr := installer.Exec("LOAD postgres")
	if installErr != nil {
		_, installErr = installer.Exec("INSTALL postgres FROM 'https://extensions.duckdb.org'; LOAD postgres")
	}
	_ = installer.Close()
	if installErr != nil {
		t.Fatal(installErr)
	}
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	// Exclude public from search_path to verify schema DDL qualifies every
	// table, sequence and sequence default rather than relying on session state.
	testDSN := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable options='-c search_path=pg_catalog'",
		libpqTestValue(cfg.Host), cfg.Port, libpqTestValue(cfg.User), libpqTestValue(cfg.Password), name)
	db, err := OpenSettingsDB(testDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := EnsureSettingsSchema(db); err != nil {
		t.Fatal(err)
	}
	// Verify bootstrap is idempotent and refreshes extension metadata.
	if err := EnsureSettingsSchema(db); err != nil {
		t.Fatal(err)
	}
	// Exercise URL detection as well as the keyword DSN above.
	uri := url.URL{Scheme: "postgresql", Host: net.JoinHostPort(cfg.Host, strconv.Itoa(int(cfg.Port))),
		User: url.UserPassword(cfg.User, cfg.Password), Path: "/" + name, RawQuery: "sslmode=disable"}
	uriDB, err := OpenSettingsDB(uri.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = uriDB.Close() })
	var catalog string
	if err := uriDB.QueryRow("SELECT current_database()").Scan(&catalog); err != nil || catalog != settingsPostgresCatalog {
		t.Fatalf("URL selected wrong backend: %q %v", catalog, err)
	}
	return db
}

func TestOpenSettingsDBRejectsMalformedPostgresString(t *testing.T) {
	_, err := OpenSettingsDB("postgres://user:private-password@host:%zz/settings")
	if err == nil || !strings.Contains(err.Error(), "settings_db_path") || strings.Contains(err.Error(), "private-password") {
		t.Fatalf("expected a credential-free connection string error, got: %v", err)
	}
}

func libpqTestValue(s string) string {
	return "'" + strings.ReplaceAll(strings.ReplaceAll(s, `\`, `\\`), "'", `\'`) + "'"
}

func TestSettingsReturningLocalDuckDB(t *testing.T) {
	db, err := OpenSettingsDB(filepath.Join(t.TempDir(), "settings.duckdb"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := EnsureSettingsSchema(db); err != nil {
		t.Fatal(err)
	}
	var id int64
	if err := db.QueryRowContext(context.Background(),
		"INSERT INTO users (username) VALUES ($1) RETURNING id", "local").Scan(&id); err != nil || id <= 0 {
		t.Fatalf("local RETURNING: id=%d err=%v", id, err)
	}
	if err := db.QueryRowContext(context.Background(),
		"DELETE FROM users WHERE id = $1 RETURNING id", -1).Scan(&id); err != sql.ErrNoRows {
		t.Fatalf("missing local row: %v", err)
	}
	// Services must use their normal DuckDB branches, without entering the
	// native helper (which now rejects local databases).
	if _, err := NewUserService(db).CreateUser(context.Background(), "local-service", "test-password-739", "", "Local", false, true); err != nil {
		t.Fatal(err)
	}
	if _, err := NewAlertsService(db).Insert(context.Background(), "info", "local", "", nil); err != nil {
		t.Fatal(err)
	}
}

func TestPostgresSettingsDSNVariants(t *testing.T) {
	db := openPostgresSettingsForTest(t)
	cfg := db.Driver().(*settingsPostgresDriver).config
	u := url.URL{Scheme: "postgresql", Host: net.JoinHostPort(cfg.Host, strconv.Itoa(int(cfg.Port))),
		User: url.UserPassword(cfg.User, cfg.Password), Path: "/" + cfg.Database, RawQuery: "sslmode=disable"}
	for _, dsn := range []string{
		u.String(), strings.Replace(u.String(), "postgresql://", "postgres://", 1),
		strings.Replace(u.String(), "postgresql://", "POSTGRES://", 1), cfg.ConnString(),
	} {
		client, err := OpenSettingsDB(dsn)
		if err != nil {
			t.Fatal(err)
		}
		defer client.Close()
		var catalog, schema string
		if err := client.QueryRow("SELECT current_database(), current_schema()").Scan(&catalog, &schema); err != nil || catalog != settingsPostgresCatalog || schema != "public" {
			t.Fatalf("DuckDB settings path: catalog=%q schema=%q error=%v", catalog, schema, err)
		}
		if err := postgresQueryRow(client)(context.Background(), "SELECT current_database(), current_schema()").Scan(&catalog, &schema); err != nil || catalog != cfg.Database || schema != "public" {
			t.Fatalf("native settings path: catalog=%q schema=%q error=%v", catalog, schema, err)
		}
	}
}

func TestPostgresSettingsServices(t *testing.T) {
	db := openPostgresSettingsForTest(t)
	ctx := context.Background()

	t.Run("users", func(t *testing.T) {
		s := NewUserService(db)
		id, err := s.CreateUser(ctx, "alice", "test-password-739", "alice@example.test", "Alice", false, true)
		if err != nil || id <= 0 {
			t.Fatalf("create: %d %v", id, err)
		}
		u, err := s.Authenticate(ctx, "alice", "test-password-739")
		if err != nil || u.ID != id {
			t.Fatalf("read after create: %v", err)
		}
		email := "updated@example.test"
		if ok, err := s.UpdateUser(ctx, id, &email, nil, nil, nil, nil); err != nil || !ok {
			t.Fatalf("update: %t %v", ok, err)
		}
		u, err = s.GetUserByID(ctx, id)
		if err != nil || u.Email != email {
			t.Fatalf("read after update: %+v %v", u, err)
		}
		if ok, err := s.DeleteUser(ctx, id); err != nil || !ok {
			t.Fatalf("delete: %t %v", ok, err)
		}
		if ok, err := s.DeleteUser(ctx, id); err != nil || ok {
			t.Fatalf("missing delete: %t %v", ok, err)
		}
		if ok, err := s.UpdateUser(ctx, id, &email, nil, nil, nil, nil); err != nil || ok {
			t.Fatalf("missing update: %t %v", ok, err)
		}
	})

	t.Run("preferences", func(t *testing.T) {
		s := NewUserSettingsService(db)
		guid, err := s.UpsertByCategory(ctx, "alice", "theme", "color", json.RawMessage(`{"value":"blue"}`), 10)
		if err != nil || guid == "" {
			t.Fatalf("insert: %q %v", guid, err)
		}
		updated, err := s.UpsertByCategory(ctx, "alice", "theme", "color", json.RawMessage(`{"value":"red"}`), 10)
		if err != nil || updated != guid {
			t.Fatalf("upsert existing: %q %v", updated, err)
		}
		if ok, err := s.DeleteByCategoryAndParam(ctx, "alice", "theme", "color"); err != nil || !ok {
			t.Fatalf("delete: %t %v", ok, err)
		}
		if ok, err := s.DeleteByCategory(ctx, "alice", "theme"); err != nil || ok {
			t.Fatalf("missing delete: %t %v", ok, err)
		}
	})

	t.Run("user mappings", func(t *testing.T) {
		s := NewUserMappingService(db)
		guid, err := s.Upsert(ctx, "alice", "sip", json.RawMessage(`{"field":"a"}`), 10)
		if err != nil || guid == "" {
			t.Fatalf("insert: %q %v", guid, err)
		}
		updated, err := s.Upsert(ctx, "alice", "sip", json.RawMessage(`{"field":"b"}`), 10)
		if err != nil || updated != guid {
			t.Fatalf("upsert existing: %q %v", updated, err)
		}
		if ok, err := s.Delete(ctx, "alice", "sip"); err != nil || !ok {
			t.Fatalf("delete: %t %v", ok, err)
		}
	})

	t.Run("dashboards", func(t *testing.T) {
		s := NewDashboardService(db, nil)
		payload := json.RawMessage(`{"name":"O'Reilly \\ café $1","shared":false}`)
		guid, err := s.CreateDashboard(ctx, "alice", "test", payload)
		if err != nil || guid == "" {
			t.Fatalf("create: %q %v", guid, err)
		}
		if _, err := s.UpdateDashboard(ctx, "alice", "test", payload, false); err != nil {
			t.Fatal(err)
		}
		got, err := s.GetDashboard(ctx, "alice", "test")
		if err != nil || got == nil || !strings.Contains(string(got.Data), "O'Reilly") {
			t.Fatalf("read: %+v %v", got, err)
		}
		if ok, err := s.DeleteDashboard(ctx, "alice", "test"); err != nil || !ok {
			t.Fatalf("delete: %t %v", ok, err)
		}
	})

	t.Run("global settings", func(t *testing.T) {
		s := NewGlobalSettingsService(db)
		item := GlobalSetting{Category: "test", Param: "x", Data: json.RawMessage(`{"value":1}`)}
		guid, err := s.Create(ctx, item)
		if err != nil || guid == "" {
			t.Fatalf("create: %q %v", guid, err)
		}
		if got, err := s.Update(ctx, guid, item); err != nil || got != guid {
			t.Fatalf("update: %q %v", got, err)
		}
		if ok, err := s.Delete(ctx, guid); err != nil || !ok {
			t.Fatalf("delete: %t %v", ok, err)
		}
	})

	t.Run("scripts", func(t *testing.T) {
		s := NewScriptsService(db)
		item := HepScript{Profile: "call", HepID: 1, Script: "return 'hello'\n" + strings.Repeat("-- test\\path\n", 200)}
		guid, err := s.Create(ctx, item)
		if err != nil || guid == "" {
			t.Fatalf("create: %q %v", guid, err)
		}
		if got, err := s.Update(ctx, guid, item); err != nil || got != guid {
			t.Fatalf("update: %q %v", got, err)
		}
		got, err := s.GetByGUID(ctx, guid)
		if err != nil || got == nil || got.Script != item.Script {
			t.Fatalf("script roundtrip: %v", err)
		}
		if ok, err := s.Delete(ctx, guid); err != nil || !ok {
			t.Fatalf("delete: %t %v", ok, err)
		}
	})

	t.Run("alerts", func(t *testing.T) {
		s := NewAlertsService(db)
		id, err := s.Insert(ctx, "info", "title", "message", json.RawMessage(`{"path":"C:\\tmp","name":"O'Reilly"}`))
		if err != nil || id <= 0 {
			t.Fatalf("insert: %d %v", id, err)
		}
		rows, err := s.List(ctx, 10)
		if err != nil || len(rows) != 1 || rows[0].ID != id {
			t.Fatalf("read: %+v %v", rows, err)
		}
	})

	t.Run("token opens remain atomic", func(t *testing.T) {
		s := NewTransactionViewTokenService(db)
		id, err := s.Create(ctx, "alice", []byte(`{"token":"payload"}`), time.Hour, 3)
		if err != nil {
			t.Fatal(err)
		}
		var successes atomic.Int32
		var wg sync.WaitGroup
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				payload, err := s.ConsumePayload(ctx, id)
				if err == nil {
					if string(payload) != `{"token":"payload"}` {
						t.Errorf("unexpected payload: %s", payload)
					}
					successes.Add(1)
				} else if !strings.Contains(err.Error(), "maximum opens reached") {
					t.Errorf("unexpected consume error: %v", err)
				}
			}()
		}
		wg.Wait()
		if successes.Load() != 3 {
			t.Fatalf("successful opens=%d, want 3", successes.Load())
		}
	})

	t.Run("pooled sessions use PostgreSQL", func(t *testing.T) {
		first, err := db.Conn(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer first.Close()
		var catalog string
		if err := db.QueryRow("SELECT current_database()").Scan(&catalog); err != nil || catalog != settingsPostgresCatalog {
			t.Fatalf("catalog=%q, err=%v", catalog, err)
		}
	})

	t.Run("canceled write and native connection cleanup", func(t *testing.T) {
		canceled, cancel := context.WithCancel(ctx)
		cancel()
		var id int64
		if err := postgresQueryRow(db)(canceled, "INSERT INTO users (username) VALUES ('canceled') RETURNING id").Scan(&id); err == nil {
			t.Fatal("canceled query succeeded")
		}
		var count int
		if err := db.QueryRow("SELECT count(*) FROM users WHERE username = 'canceled'").Scan(&count); err != nil || count != 0 {
			t.Fatalf("canceled write: count=%d err=%v", count, err)
		}
		// Capture the native backend PID; Scan must close it before returning.
		if err := postgresQueryRow(db)(ctx, "SELECT pg_backend_pid()").Scan(&id); err != nil {
			t.Fatal(err)
		}
		if err := postgresQueryRow(db)(ctx, "SELECT count(*) FROM pg_stat_activity WHERE pid = $1", id).Scan(&count); err != nil || count != 0 {
			t.Fatalf("native connection leaked: count=%d err=%v", count, err)
		}
		var invalidDestination chan string
		if err := postgresQueryRow(db)(ctx, "SELECT pg_backend_pid(), 'text'::text").Scan(&id, &invalidDestination); err == nil {
			t.Fatal("expected a scan conversion error")
		}
		if err := postgresQueryRow(db)(ctx, "SELECT count(*) FROM pg_stat_activity WHERE pid = $1", id).Scan(&count); err != nil || count != 0 {
			t.Fatalf("native connection leaked after scan error: count=%d err=%v", count, err)
		}
	})

	t.Run("closed database cannot open a native connection", func(t *testing.T) {
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
		var id int64
		if err := postgresQueryRow(db)(ctx, "INSERT INTO users (username) VALUES ('closed') RETURNING id").Scan(&id); err == nil {
			t.Fatalf("write after close: %v", err)
		}
	})
}
