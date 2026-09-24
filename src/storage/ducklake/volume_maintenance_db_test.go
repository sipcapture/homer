// Copyright (C) 2026 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package ducklake

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestUsesDedicatedFileCleanupDB(t *testing.T) {
	cases := []struct {
		name string
		vol  *Volume
		want bool
	}{
		{"nil", nil, false},
		{"local", &Volume{Type: VolumeTypeLocal}, false},
		{"s3", &Volume{Type: VolumeTypeS3}, true},
		{"azure", &Volume{Type: VolumeTypeAzure}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := usesDedicatedFileCleanupDB(tc.vol); got != tc.want {
				t.Fatalf("usesDedicatedFileCleanupDB = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestOrphanScanDue(t *testing.T) {
	var m maintenanceDB
	now := time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)
	local := &Volume{Type: VolumeTypeLocal, LakeName: "homer_lake_hot"}
	cold := &Volume{Type: VolumeTypeS3, LakeName: "homer_lake_cold"}

	m.markOrphanScan(local, now)
	if !m.orphanScanDue(local, now.Add(time.Minute)) {
		t.Fatal("local volumes must scan for orphans every cycle")
	}

	if !m.orphanScanDue(cold, now) {
		t.Fatal("first object-store scan must be due")
	}
	m.markOrphanScan(cold, now)
	if m.orphanScanDue(cold, now.Add(time.Hour)) {
		t.Fatal("object-store scan must not repeat within the interval")
	}
	if !m.orphanScanDue(cold, now.Add(objectStoreOrphanScanInterval)) {
		t.Fatal("object-store scan must be due once the interval elapses")
	}
}

// TestFileCleanupDBRemovesScheduledFiles proves that the maintenance instance
// attaches the same catalog as the tiering instance and physically removes
// files scheduled for deletion there.
func TestFileCleanupDBRemovesScheduledFiles(t *testing.T) {
	dir := t.TempDir()
	tsm, err := NewTieredStorageManager(TieredStorageConfig{
		CatalogPath: filepath.Join(dir, "homer_catalog.sqlite"),
		Volumes: []Volume{
			{Name: "hot", Type: VolumeTypeLocal, Path: filepath.Join(dir, "hot") + "/", Priority: 0, LakeName: "homer_lake_hot"},
			{Name: "cold", Type: VolumeTypeLocal, Path: filepath.Join(dir, "cold") + "/", Priority: 1, LakeName: "homer_lake_cold"},
		},
		TuningThreads:     1,
		TuningMemoryLimit: "256MB",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := tsm.Start(); err != nil {
		if strings.Contains(err.Error(), "extension") {
			t.Skipf("ducklake/sqlite extension unavailable: %v", err)
		}
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tsm.Stop() })

	var cold *Volume
	for _, v := range tsm.GetVolumes() {
		if v.Name == "cold" {
			cold = v
		}
	}
	if cold == nil {
		t.Fatal("cold volume not found")
	}

	for _, stmt := range []string{
		"CALL homer_lake_cold.set_option('data_inlining_row_limit', 0)",
		"CREATE TABLE homer_lake_cold.main.t (id INTEGER)",
		"INSERT INTO homer_lake_cold.main.t VALUES (1)",
		"INSERT INTO homer_lake_cold.main.t VALUES (2)",
		"DELETE FROM homer_lake_cold.main.t",
		"CALL ducklake_expire_snapshots('homer_lake_cold', older_than => NOW())",
	} {
		if _, err := tsm.GetDB().Exec(stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	if n := countParquet(t, cold.Path); n == 0 {
		t.Fatal("expected parquet files before cleanup")
	}

	db, err := tsm.fileCleanupDB(cold)
	if err != nil {
		t.Fatal(err)
	}
	if db == tsm.GetDB() {
		t.Fatal("file cleanup must not use the tiering connection")
	}
	before, err := scheduledFileCount(db, cold.LakeName)
	if err != nil {
		t.Fatal(err)
	}
	if before == 0 {
		t.Fatal("expected files scheduled for deletion")
	}

	var stepErr error
	record := func(_ string, err error) {
		if err != nil && stepErr == nil {
			stepErr = err
		}
	}
	stmts := volumeMaintenanceSQL(cold.LakeName, 0)
	tsm.runFileCleanup(db, cold, nil, stmts[1], stmts[2], record)
	if stepErr != nil {
		t.Fatal(stepErr)
	}

	after, err := scheduledFileCount(db, cold.LakeName)
	if err != nil {
		t.Fatal(err)
	}
	if after != 0 {
		t.Fatalf("scheduled files after cleanup = %d, want 0", after)
	}
	if n := countParquet(t, cold.Path); n != 0 {
		t.Fatalf("parquet files after cleanup = %d, want 0", n)
	}

	again, err := tsm.fileCleanupDB(cold)
	if err != nil {
		t.Fatal(err)
	}
	if again != db {
		t.Fatal("maintenance DuckDB must be reused across cycles")
	}
	var rows int
	if err := tsm.GetDB().QueryRow("SELECT COUNT(*) FROM homer_lake_cold.main.t").Scan(&rows); err != nil {
		t.Fatalf("tiering connection must still read the cold lake: %v", err)
	}
}

func countParquet(t *testing.T, root string) int {
	t.Helper()
	n := 0
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(path, ".parquet") {
			n++
		}
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return n
}
