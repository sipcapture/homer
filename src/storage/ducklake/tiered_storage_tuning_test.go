// Copyright (C) 2026 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package ducklake

import (
	"database/sql"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	_ "github.com/duckdb/duckdb-go/v2"
)

func TestTieredStorageManagerApplyDuckDBTuning(t *testing.T) {
	newManager := func(t *testing.T, catalog string, threads int, memoryLimit, tempDirectory string) *TieredStorageManager {
		t.Helper()
		tsm, err := NewTieredStorageManager(TieredStorageConfig{
			CatalogType:         CatalogSQLite,
			CatalogPath:         catalog,
			TuningThreads:       threads,
			TuningMemoryLimit:   memoryLimit,
			TuningTempDirectory: tempDirectory,
			Volumes:             []Volume{{Name: "hot", Type: VolumeTypeLocal}},
		})
		if err != nil {
			t.Fatalf("NewTieredStorageManager: %v", err)
		}
		return tsm
	}

	openDB := func(t *testing.T) *sql.DB {
		t.Helper()
		db, err := sql.Open("duckdb", "")
		if err != nil {
			t.Fatalf("open duckdb: %v", err)
		}
		t.Cleanup(func() { _ = db.Close() })
		return db
	}

	t.Run("explicit operator settings", func(t *testing.T) {
		root := t.TempDir()
		tempDir := filepath.Join(root, "spill")
		if err := os.MkdirAll(tempDir, 0o755); err != nil {
			t.Fatalf("create temp directory: %v", err)
		}
		tsm := newManager(t, filepath.Join(root, "catalog.sqlite"), 1, "256MiB", tempDir)
		db := openDB(t)

		tsm.applyDuckDBTuning(db)

		if got := getSetting(t, db, "threads"); got != "1" {
			t.Fatalf("threads = %q, want 1", got)
		}
		if got := strings.ToLower(getSetting(t, db, "memory_limit")); !strings.Contains(got, "256") {
			t.Fatalf("memory_limit = %q, want configured 256MiB", got)
		}
		if got := getSetting(t, db, "temp_directory"); !strings.Contains(got, tempDir) {
			t.Fatalf("temp_directory = %q, want path containing %q", got, tempDir)
		}
		if got := getSetting(t, db, "preserve_insertion_order"); got != "false" {
			t.Fatalf("preserve_insertion_order = %q, want false", got)
		}
	})

	t.Run("safe defaults", func(t *testing.T) {
		root := t.TempDir()
		catalog := filepath.Join(root, "catalog.sqlite")
		tsm := newManager(t, catalog, 0, "", "")
		db := openDB(t)
		expected := openDB(t)
		if _, err := expected.Exec("SET memory_limit = '2GB'"); err != nil {
			t.Fatalf("set expected memory limit: %v", err)
		}
		wantMemoryLimit := getSetting(t, expected, "memory_limit")

		tsm.applyDuckDBTuning(db)

		if got, want := getSetting(t, db, "threads"), strconv.Itoa(AutoThreads()); got != want {
			t.Fatalf("threads = %q, want auto default %q", got, want)
		}
		if got := getSetting(t, db, "memory_limit"); got != wantMemoryLimit {
			t.Fatalf("memory_limit = %q, want 2GB setting %q", got, wantMemoryLimit)
		}
		wantTemp := filepath.Join(root, ".duckdb_spill")
		if got := getSetting(t, db, "temp_directory"); !strings.Contains(got, wantTemp) {
			t.Fatalf("temp_directory = %q, want default path containing %q", got, wantTemp)
		}
		if got := getSetting(t, db, "preserve_insertion_order"); got != "false" {
			t.Fatalf("preserve_insertion_order = %q, want false", got)
		}
	})
}
