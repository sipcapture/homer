// Copyright (C) 2026 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package ducklake

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	_ "github.com/duckdb/duckdb-go/v2"
)

func TestVqrtcpWriteRowHoldsCatalogLock(t *testing.T) {
	db, err := sql.Open("duckdb", "")
	if err != nil {
		t.Skipf("duckdb unavailable: %v", err)
	}
	defer db.Close()
	if _, err := db.Exec(fmt.Sprintf("CREATE TABLE %s (%s);", vqrtcpTableName, vqrtcpTableSQL)); err != nil {
		t.Fatalf("create table: %v", err)
	}

	locker := &fakeCatalogLocker{}
	s := &VqrtcpStorage{db: db, locker: locker}
	for _, leg := range []string{"A", "B"} {
		row := VqrtcpRow{
			CallID:    "call-1",
			MosLQ:     4.2,
			Event:     "VQSessionReport",
			SIPMethod: "PUBLISH",
			Message:   map[string]any{"local_id": "leg-" + leg},
			Timestamp: time.Now(),
		}
		if err := s.WriteRow(context.Background(), row); err != nil {
			t.Fatalf("WriteRow leg %s: %v", leg, err)
		}
	}

	if got := locker.lockCount.Load(); got != 2 {
		t.Fatalf("catalog lock taken %d times, want 2 (once per INSERT)", got)
	}
	var n int
	if err := db.QueryRow(fmt.Sprintf("SELECT count(*) FROM %s WHERE callid = 'call-1'", vqrtcpTableName)).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 2 {
		t.Fatalf("rows = %d, want 2", n)
	}
}

func TestVqrtcpWriteRowWithoutLocker(t *testing.T) {
	db, err := sql.Open("duckdb", "")
	if err != nil {
		t.Skipf("duckdb unavailable: %v", err)
	}
	defer db.Close()
	if _, err := db.Exec(fmt.Sprintf("CREATE TABLE %s (%s);", vqrtcpTableName, vqrtcpTableSQL)); err != nil {
		t.Fatalf("create table: %v", err)
	}
	s := &VqrtcpStorage{db: db}
	if err := s.WriteRow(context.Background(), VqrtcpRow{CallID: "call-2", MosLQ: 4.0}); err != nil {
		t.Fatalf("WriteRow: %v", err)
	}
}

type mutexCatalogLocker struct{ mu sync.Mutex }

func (m *mutexCatalogLocker) CatalogLock()   { m.mu.Lock() }
func (m *mutexCatalogLocker) CatalogUnlock() { m.mu.Unlock() }

// newVqrtcpLake attaches a real DuckLake (SQLite catalog) as "lake" without
// pinning the pool to one connection, so concurrent writers really contend
// on the catalog like the receiver workers do in production.
func newVqrtcpLake(t *testing.T) *sql.DB {
	t.Helper()
	dir := t.TempDir()
	data := filepath.Join(dir, "data")
	if err := os.MkdirAll(data, 0o755); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("duckdb", "")
	if err != nil {
		t.Skipf("duckdb unavailable: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	for _, stmt := range []string{"LOAD ducklake;", "LOAD sqlite;"} {
		if _, err := db.Exec(stmt); err != nil {
			t.Skipf("extension unavailable (%q): %v", stmt, err)
		}
	}
	if _, err := db.Exec(fmt.Sprintf("ATTACH 'ducklake:sqlite:%s' AS lake (DATA_PATH '%s');",
		filepath.Join(dir, "catalog.sqlite"), data)); err != nil {
		t.Skipf("ATTACH failed: %v", err)
	}
	return db
}

// Concurrent receiver workers must not lose reports to catalog contention
// ("database is locked" / transaction conflicts on the SQLite catalog).
func TestVqrtcpWriteRowConcurrentDuckLake(t *testing.T) {
	db := newVqrtcpLake(t)
	s := &VqrtcpStorage{db: db, lakeName: "lake", locker: &mutexCatalogLocker{}}
	if err := s.EnsureVqrtcpSchema(context.Background()); err != nil {
		t.Fatalf("schema: %v", err)
	}

	const workers, perWorker = 4, 25
	var wg sync.WaitGroup
	errs := make(chan error, workers*perWorker)
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < perWorker; i++ {
				row := VqrtcpRow{CallID: fmt.Sprintf("call-%d-%d", w, i), MosLQ: 4.2, Event: "VQSessionReport"}
				if err := s.WriteRow(context.Background(), row); err != nil {
					errs <- err
				}
			}
		}(w)
	}
	wg.Wait()
	close(errs)

	failed := 0
	var first error
	for err := range errs {
		if first == nil {
			first = err
		}
		failed++
	}
	if failed > 0 {
		t.Fatalf("%d/%d writes failed, first: %v", failed, workers*perWorker, first)
	}
	var n int
	if err := db.QueryRow("SELECT count(*) FROM lake." + vqrtcpTableName).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != workers*perWorker {
		t.Fatalf("rows = %d, want %d", n, workers*perWorker)
	}
}
