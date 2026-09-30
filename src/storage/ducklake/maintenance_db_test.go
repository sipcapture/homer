// Copyright (C) 2026 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package ducklake

import (
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// openTestWriterDB brings up a writer-like DuckDB with a fresh lake attached.
// Skipped when the DuckDB extensions are unavailable (offline CI).
func openTestWriterDB(t *testing.T) (Config, *sql.DB) {
	t.Helper()
	dir := t.TempDir()
	data := filepath.Join(dir, "data")
	if err := os.MkdirAll(data, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := Config{
		CatalogPath:         filepath.Join(dir, "catalog.sqlite"),
		DataPath:            data,
		LakeName:            "lake",
		TuningTempDirectory: filepath.Join(dir, "spill"),
		TuningThreads:       2,
	}
	db, err := newLakeDuckDB(cfg, 2)
	if err != nil {
		t.Skipf("duckdb unavailable: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := loadLakeExtensions(db); err != nil {
		t.Skipf("extensions unavailable: %v", err)
	}
	if _, err := db.Exec(buildLakeAttachSQL(cfg)); err != nil {
		t.Skipf("ATTACH failed: %v", err)
	}
	t.Cleanup(MarkCatalogAttached(cfg.CatalogPath))
	return cfg, db
}

func TestOpenMaintenanceDBSeesWriterLake(t *testing.T) {
	cfg, writer := openTestWriterDB(t)
	if _, err := writer.Exec(`CREATE TABLE lake.t (d DATE, id BIGINT)`); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Exec(`INSERT INTO lake.t VALUES (DATE '2026-09-30', 1)`); err != nil {
		t.Fatal(err)
	}

	m, err := OpenMaintenanceDB(cfg, "", writer)
	if err != nil {
		t.Fatalf("OpenMaintenanceDB: %v", err)
	}
	var n int
	if err := m.DB.QueryRow(`SELECT count(*) FROM lake.t`).Scan(&n); err != nil {
		t.Fatalf("maintenance read: %v", err)
	}
	if n != 1 {
		t.Errorf("maintenance instance sees %d rows, want 1", n)
	}
	if _, err := os.Stat(filepath.Join(cfg.TuningTempDirectory, "compaction")); err != nil {
		t.Errorf("separate spill directory not created: %v", err)
	}

	if err := m.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if !IsCatalogAttachedInProcess(cfg.CatalogPath) {
		t.Error("closing the maintenance instance dropped the writer's attach mark")
	}
}

// TestMaintenanceDBConcurrentWithWriterFlush runs flushes on the writer
// instance while the maintenance instance merges, expires and cleans up, with
// the catalog lock serialising them as in production. The catalog must stay
// consistent: no duplicate snapshot ids, no lost rows, and SQLite reports no
// page damage.
func TestMaintenanceDBConcurrentWithWriterFlush(t *testing.T) {
	cfg, writer := openTestWriterDB(t)
	if _, err := writer.Exec(`CREATE TABLE lake.t (d DATE, id BIGINT)`); err != nil {
		t.Fatal(err)
	}
	m, err := OpenMaintenanceDB(cfg, "", writer)
	if err != nil {
		t.Fatalf("OpenMaintenanceDB: %v", err)
	}
	defer m.Close()

	var catalogMu sync.Mutex
	const batches = 40
	errs := make(chan error, 2)
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < batches; i++ {
			catalogMu.Lock()
			_, err := writer.Exec(fmt.Sprintf(
				`INSERT INTO lake.t SELECT DATE '2026-09-30', range FROM range(%d, %d)`, i*10, i*10+10))
			catalogMu.Unlock()
			if err != nil {
				errs <- fmt.Errorf("flush %d: %w", i, err)
				return
			}
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 10; i++ {
			for _, stmt := range []string{
				`CALL ducklake_merge_adjacent_files('lake', 't', schema => 'main')`,
				`CALL ducklake_expire_snapshots('lake', older_than => NOW()::TIMESTAMPTZ)`,
				`CALL ducklake_cleanup_old_files('lake', cleanup_all => true)`,
			} {
				catalogMu.Lock()
				rows, err := m.DB.Query(stmt)
				if err == nil {
					for rows.Next() {
					}
					err = rows.Err()
					rows.Close()
				}
				catalogMu.Unlock()
				if err != nil {
					errs <- fmt.Errorf("%s: %w", stmt, err)
					return
				}
			}
		}
	}()

	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}

	for name, db := range map[string]*sql.DB{"writer": writer, "maintenance": m.DB} {
		var n int
		if err := db.QueryRow(`SELECT count(*) FROM lake.t`).Scan(&n); err != nil {
			t.Fatalf("%s count: %v", name, err)
		}
		if n != batches*10 {
			t.Errorf("%s sees %d rows, want %d", name, n, batches*10)
		}
	}

	var dup int
	if err := writer.QueryRow(`SELECT count(*) FROM (
		SELECT snapshot_id FROM __ducklake_metadata_lake.ducklake_snapshot
		GROUP BY snapshot_id HAVING count(*) > 1)`).Scan(&dup); err != nil {
		t.Fatalf("snapshot check: %v", err)
	}
	if dup != 0 {
		t.Errorf("%d duplicated snapshot ids", dup)
	}

	if _, err := exec.LookPath("sqlite3"); err == nil {
		out, err := exec.Command("sqlite3", cfg.CatalogPath, "PRAGMA quick_check;").CombinedOutput()
		if err != nil || strings.TrimSpace(string(out)) != "ok" {
			t.Errorf("quick_check: %v: %s", err, out)
		}
	}
}
