// Copyright (C) 2026 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package ducklake

import (
	"bytes"
	"context"
	"database/sql"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	logger "github.com/sipcapture/homer-core/src/utils/logging"
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

// startTieredFixture starts a hot local + cold volume manager and returns the cold volume.
func startTieredFixture(t *testing.T, cold Volume) (*TieredStorageManager, *Volume) {
	t.Helper()
	dir := t.TempDir()
	cold.Name, cold.Priority, cold.LakeName = "cold", 1, "homer_lake_cold"
	tsm, err := NewTieredStorageManager(TieredStorageConfig{
		CatalogPath: filepath.Join(dir, "homer_catalog.sqlite"),
		Volumes: []Volume{
			{Name: "hot", Type: VolumeTypeLocal, Path: filepath.Join(dir, "hot") + "/", Priority: 0, LakeName: "homer_lake_hot"},
			cold,
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
	for _, v := range tsm.GetVolumes() {
		if v.Name == "cold" {
			return tsm, v
		}
	}
	t.Fatal("cold volume not found")
	return nil, nil
}

func localColdVolume(t *testing.T) Volume {
	return Volume{Type: VolumeTypeLocal, Path: filepath.Join(t.TempDir(), "cold") + "/"}
}

// scheduleColdFiles writes two parquet files to the cold lake, deletes their
// rows and expires the snapshots so both files are scheduled for deletion.
func scheduleColdFiles(t *testing.T, db *sql.DB) {
	t.Helper()
	for _, stmt := range []string{
		"CALL homer_lake_cold.set_option('data_inlining_row_limit', 0)",
		"CREATE TABLE homer_lake_cold.main.t (id INTEGER)",
		"INSERT INTO homer_lake_cold.main.t VALUES (1)",
		"INSERT INTO homer_lake_cold.main.t VALUES (2)",
		"DELETE FROM homer_lake_cold.main.t",
		"CALL ducklake_expire_snapshots('homer_lake_cold', older_than => NOW())",
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
}

// captureWarnings routes the project logger into a buffer for the test.
func captureWarnings(t *testing.T) *syncBuffer {
	t.Helper()
	buf := &syncBuffer{}
	prev := logger.Logger
	logger.Logger = slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelWarn}))
	t.Cleanup(func() { logger.Logger = prev })
	return buf
}

type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// TestFileCleanupDBRemovesScheduledFiles proves that the maintenance instance
// attaches the same catalog as the tiering instance and physically removes
// files scheduled for deletion there.
func TestFileCleanupDBRemovesScheduledFiles(t *testing.T) {
	tsm, cold := startTieredFixture(t, localColdVolume(t))
	scheduleColdFiles(t, tsm.GetDB())
	if n := countParquet(t, cold.Path); n == 0 {
		t.Fatal("expected parquet files before cleanup")
	}
	logs := captureWarnings(t)

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
	if strings.Contains(logs.String(), "did not reduce") {
		t.Fatalf("successful cleanup must not warn, got: %s", logs.String())
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

// TestRunVolumeMaintenanceS3CleanupDoesNotBlockTieringDB is the regression
// test for sipcapture/homer#1037: while S3 file cleanup is in progress, the
// tiering connection that serves Node hot+cold reads must stay available.
func TestRunVolumeMaintenanceS3CleanupDoesNotBlockTieringDB(t *testing.T) {
	// ATTACH and the catalog-only steps never touch the endpoint; the file
	// cleanup statements are replaced below, so no S3 server is needed.
	tsm, cold := startTieredFixture(t, Volume{
		Type:        VolumeTypeS3,
		Path:        "s3://homer-test/cold/",
		S3Endpoint:  "127.0.0.1:1",
		S3AccessKey: "test",
		S3SecretKey: "test",
	})

	started := make(chan *sql.DB, 1)
	release := make(chan struct{})
	tsm.fileCleanupExec = func(db *sql.DB, _ CatalogLocker, stmt string) error {
		if !strings.Contains(stmt, "cleanup_old_files") {
			return nil
		}
		// Hold the connection like a long per-object sweep would.
		conn, err := db.Conn(context.Background())
		if err != nil {
			return err
		}
		defer conn.Close()
		started <- db
		<-release
		return nil
	}

	done := make(chan error, 1)
	go func() { done <- tsm.RunVolumeMaintenance(cold, 1) }()

	var cleanupDB *sql.DB
	select {
	case cleanupDB = <-started:
	case err := <-done:
		t.Fatalf("maintenance finished before file cleanup started: %v", err)
	case <-time.After(30 * time.Second):
		t.Fatal("file cleanup did not start")
	}
	if cleanupDB == tsm.GetDB() {
		close(release)
		t.Fatal("S3 file cleanup ran on the tiering connection")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	var one int
	err := tsm.GetDB().QueryRowContext(ctx, "SELECT 1").Scan(&one)
	close(release)
	if err != nil {
		t.Fatalf("tiering connection blocked during S3 file cleanup: %v", err)
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("RunVolumeMaintenance: %v", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("RunVolumeMaintenance did not finish")
	}
}

// TestRunVolumeMaintenanceWarnsWhenCleanupRemovesNothing covers the object
// store refusing deletes (e.g. no s3:DeleteObject): cleanup "succeeds" but the
// scheduled list does not shrink, and the operator must be told why.
func TestRunVolumeMaintenanceWarnsWhenCleanupRemovesNothing(t *testing.T) {
	tsm, cold := startTieredFixture(t, localColdVolume(t))
	scheduleColdFiles(t, tsm.GetDB())
	tsm.fileCleanupExec = func(*sql.DB, CatalogLocker, string) error { return nil }
	logs := captureWarnings(t)

	if err := tsm.RunVolumeMaintenance(cold, 0); err != nil {
		t.Fatalf("RunVolumeMaintenance: %v", err)
	}
	out := logs.String()
	if !strings.Contains(out, "did not reduce files scheduled for deletion") {
		t.Fatalf("expected no-progress warning, got: %s", out)
	}
	if !strings.Contains(out, "s3:DeleteObject") {
		t.Fatalf("warning must hint at delete permission, got: %s", out)
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
