package services

import (
	"context"
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
