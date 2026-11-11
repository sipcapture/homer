package services

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestPostgresRuntimeStateAcrossReplicas(t *testing.T) {
	db := openPostgresSettingsForTest(t)
	state, err := NewRuntimeState(db)
	if err != nil {
		t.Fatal(err)
	}
	// A separate DuckDB client models an independent coordinator process.
	otherDB, err := OpenSettingsDB(db.Driver().(*settingsPostgresDriver).config.ConnString(), "postgres")
	if err != nil {
		t.Fatal(err)
	}
	defer otherDB.Close()
	other, err := NewRuntimeState(otherDB)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := state.Put(ctx, "oauth", "one-time-token", []byte("profile"), time.Now().Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	var successes atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			payload, found, err := other.Get(ctx, "oauth", "one-time-token", true)
			if err != nil {
				t.Error(err)
				return
			}
			if found {
				if string(payload) != "profile" {
					t.Errorf("wrong payload %q", payload)
				}
				successes.Add(1)
			}
		}()
	}
	wg.Wait()
	if successes.Load() != 1 {
		t.Fatalf("token consumed %d times", successes.Load())
	}
	if err := state.Put(ctx, "session", "session-id", []byte("revoked"), time.Now().Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	_ = db.Close() // A replica restart must not erase revocations.
	payload, found, err := other.Get(ctx, "session", "session-id", false)
	if err != nil || !found || string(payload) != "revoked" {
		t.Fatalf("revocation lost: %q %v %v", payload, found, err)
	}
	if err := other.Put(ctx, "expiry", "expired", []byte("expired"), time.Now().Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, found, err := other.Get(ctx, "expiry", "expired", true); err != nil || found {
		t.Fatalf("expired entry returned: %v %v", found, err)
	}
	// Upsert replaces an expired entry without competing with cleanup for the same row.
	if err := other.Put(ctx, "expiry", "expired", []byte("renewed"), time.Now().Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if payload, found, err := other.Get(ctx, "expiry", "expired", false); err != nil || !found || string(payload) != "renewed" {
		t.Fatalf("renewal failed: %q %v %v", payload, found, err)
	}
}

func TestPostgresSettingsBootstrapLock(t *testing.T) {
	db := openPostgresSettingsForTest(t)
	release, err := LockSettingsBootstrap(db)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	var acquired bool
	if err := postgresQueryRow(db)(context.Background(), "SELECT pg_try_advisory_lock(724193507)").Scan(&acquired); err != nil {
		t.Fatal(err)
	}
	if acquired {
		t.Fatal("another replica bypassed bootstrap lock")
	}
	release()
	if err := postgresQueryRow(db)(context.Background(), "SELECT pg_try_advisory_lock(724193507)").Scan(&acquired); err != nil {
		t.Fatal(err)
	}
	if !acquired {
		t.Fatal("bootstrap lock leaked")
	}
}

func TestPostgresRuntimeStatePoolReuseAndClose(t *testing.T) {
	db := openPostgresSettingsForTest(t)
	state, err := NewRuntimeState(db)
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	ctx := context.Background()
	state.native.SetMaxOpenConns(1)
	if err := state.Put(ctx, "pool", "token", []byte("payload"), time.Now().Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	var initialPID int
	if err := state.native.QueryRowContext(ctx, "SELECT pg_backend_pid()").Scan(&initialPID); err != nil {
		t.Fatal(err)
	}
	// Every operation must respect this pool's connection limit.
	conn, err := state.native.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	for name, op := range map[string]func(context.Context) error{
		"put": func(ctx context.Context) error {
			return state.Put(ctx, "pool", "other", []byte("x"), time.Now().Add(time.Minute))
		},
		"get":     func(ctx context.Context) error { _, _, err := state.Get(ctx, "pool", "token", false); return err },
		"consume": func(ctx context.Context) error { _, _, err := state.Get(ctx, "pool", "token", true); return err },
	} {
		t.Run(name+" shares bounded pool", func(t *testing.T) {
			ctx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
			defer cancel()
			if err := op(ctx); !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("operation bypassed occupied pool: %v", err)
			}
		})
	}
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 20; i++ {
		if err := state.Put(ctx, "pool", "token", []byte("payload"), time.Now().Add(time.Minute)); err != nil {
			t.Fatal(err)
		}
		if payload, found, err := state.Get(ctx, "pool", "token", false); err != nil || !found || string(payload) != "payload" {
			t.Fatalf("pooled lookup: %q %v %v", payload, found, err)
		}
	}
	var reusedPID int
	if err := state.native.QueryRowContext(ctx, "SELECT pg_backend_pid()").Scan(&reusedPID); err != nil || reusedPID != initialPID {
		t.Fatalf("native session not reused: %d -> %d err=%v", initialPID, reusedPID, err)
	}
	other, err := NewRuntimeState(db)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	if err := state.Close(); err != nil {
		t.Fatal(err)
	}
	if _, _, err := state.Get(ctx, "pool", "token", false); err == nil {
		t.Fatal("closed runtime pool accepted query")
	}
	if _, found, err := other.Get(ctx, "pool", "token", false); err != nil || !found {
		t.Fatalf("closing one runtime closed the other: %v %v", found, err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if _, _, err := other.Get(ctx, "pool", "token", false); err == nil {
		t.Fatal("settings closure left runtime pool usable")
	}
	if stats := other.native.Stats(); stats.OpenConnections != 0 {
		t.Fatalf("native connections leaked: %+v", stats)
	}
}
