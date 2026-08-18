// Copyright (C) 2026 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package ducklake

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func newTestCatalog(t *testing.T, path string, rows int) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE ducklake_snapshot(snapshot_id BIGINT PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < rows; i++ {
		if _, err := db.Exec(`INSERT INTO ducklake_snapshot VALUES (?)`, i); err != nil {
			t.Fatal(err)
		}
	}
}

func TestBackupCatalogCopiesAndPrunes(t *testing.T) {
	dir := t.TempDir()
	catalog := filepath.Join(dir, "catalog.sqlite")
	newTestCatalog(t, catalog, 5)

	const keep = 2
	var made []string
	for i := 0; i < 4; i++ {
		// Backup names carry a one-second timestamp, so make them distinct.
		dest, err := BackupCatalog(catalog, keep)
		if err != nil {
			t.Fatalf("BackupCatalog #%d: %v", i, err)
		}
		made = append(made, dest)
		if i < 3 {
			renameForDistinctTimestamp(t, dest, i)
		}
	}

	// The newest copy must be a readable catalog with the same contents.
	newest := made[len(made)-1]
	db, err := sql.Open("sqlite", "file:"+newest)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM ducklake_snapshot`).Scan(&n); err != nil {
		t.Fatalf("read backup: %v", err)
	}
	if n != 5 {
		t.Errorf("backup holds %d rows, want 5", n)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var backups int
	for _, e := range entries {
		if strings.Contains(e.Name(), catalogBackupSuffix) {
			backups++
		}
	}
	if backups > keep {
		t.Errorf("kept %d backups, want at most %d", backups, keep)
	}
}

// renameForDistinctTimestamp rewrites a backup's timestamp so the retention test
// does not depend on wall-clock seconds elapsing between calls.
func renameForDistinctTimestamp(t *testing.T, dest string, i int) {
	t.Helper()
	idx := strings.LastIndex(dest, catalogBackupSuffix)
	older := dest[:idx+len(catalogBackupSuffix)] + fmt.Sprintf("20200101T00000%dZ", i)
	if err := os.Rename(dest, older); err != nil {
		t.Fatal(err)
	}
}

func TestBackupCatalogMissingFile(t *testing.T) {
	if _, err := BackupCatalog(filepath.Join(t.TempDir(), "nope.sqlite"), 3); err == nil {
		t.Error("expected an error for a missing catalog")
	}
	if _, err := BackupCatalog("", 3); err == nil {
		t.Error("expected an error for an empty path")
	}
}

func TestBackupCatalogKeepZeroRetainsAll(t *testing.T) {
	dir := t.TempDir()
	catalog := filepath.Join(dir, "catalog.sqlite")
	newTestCatalog(t, catalog, 2)

	for i := 0; i < 3; i++ {
		dest, err := BackupCatalog(catalog, 0)
		if err != nil {
			t.Fatalf("BackupCatalog #%d: %v", i, err)
		}
		renameForDistinctTimestamp(t, dest, i)
	}
	list, err := ListCatalogBackups(catalog)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 3 {
		t.Fatalf("keep=0 retained %d backups, want 3", len(list))
	}
}

func TestBackupCatalogToCustomPath(t *testing.T) {
	dir := t.TempDir()
	catalog := filepath.Join(dir, "catalog.sqlite")
	newTestCatalog(t, catalog, 4)
	dest := filepath.Join(dir, "copies", "manual.sqlite")
	got, err := BackupCatalogTo(catalog, dest)
	if err != nil {
		t.Fatalf("BackupCatalogTo: %v", err)
	}
	if got != dest {
		if abs, _ := filepath.Abs(dest); got != abs {
			t.Fatalf("dest = %q, want %q", got, dest)
		}
	}
	assertSnapshotCount(t, got, 4)
}

func TestRestoreCatalogRoundTrip(t *testing.T) {
	dir := t.TempDir()
	catalog := filepath.Join(dir, "catalog.sqlite")
	newTestCatalog(t, catalog, 7)

	backup, err := BackupCatalog(catalog, 3)
	if err != nil {
		t.Fatalf("BackupCatalog: %v", err)
	}

	// Mutate the live catalog after the snapshot.
	db, err := sql.Open("sqlite", "file:"+catalog)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO ducklake_snapshot VALUES (99)`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	assertSnapshotCount(t, catalog, 8)

	previous, err := RestoreCatalog(catalog, backup)
	if err != nil {
		t.Fatalf("RestoreCatalog: %v", err)
	}
	if previous == "" {
		t.Fatal("expected previous catalog path")
	}
	assertSnapshotCount(t, catalog, 7)
	assertSnapshotCount(t, previous, 8)
}

func TestRestoreCatalogNewestWhenFromEmpty(t *testing.T) {
	dir := t.TempDir()
	catalog := filepath.Join(dir, "catalog.sqlite")
	newTestCatalog(t, catalog, 1)
	if _, err := BackupCatalog(catalog, 3); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", "file:"+catalog)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO ducklake_snapshot VALUES (1)`); err != nil {
		t.Fatal(err)
	}
	db.Close()

	if _, err := RestoreCatalog(catalog, ""); err != nil {
		t.Fatalf("RestoreCatalog newest: %v", err)
	}
	assertSnapshotCount(t, catalog, 1)
}

func TestRestoreCatalogRejectsNonCatalog(t *testing.T) {
	dir := t.TempDir()
	catalog := filepath.Join(dir, "catalog.sqlite")
	newTestCatalog(t, catalog, 1)
	junk := filepath.Join(dir, "junk.sqlite")
	db, err := sql.Open("sqlite", "file:"+junk)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE t(x INT)`); err != nil {
		t.Fatal(err)
	}
	db.Close()

	if _, err := RestoreCatalog(catalog, junk); err == nil {
		t.Fatal("expected error for a non-DuckLake sqlite file")
	}
	assertSnapshotCount(t, catalog, 1)
}

func TestRestoreCatalogRefusesWriterLock(t *testing.T) {
	dir := t.TempDir()
	catalog := filepath.Join(dir, "catalog.sqlite")
	newTestCatalog(t, catalog, 1)
	backup, err := BackupCatalog(catalog, 3)
	if err != nil {
		t.Fatal(err)
	}
	lock, err := acquireCatalogWriterLock(catalog)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseCatalogWriterLock(lock)

	if _, err := RestoreCatalog(catalog, backup); err == nil {
		t.Fatal("expected error while the writer lock is held")
	}
}

func TestRestoreCatalogRefusesLivePath(t *testing.T) {
	dir := t.TempDir()
	catalog := filepath.Join(dir, "catalog.sqlite")
	newTestCatalog(t, catalog, 1)
	if _, err := RestoreCatalog(catalog, catalog); err == nil {
		t.Fatal("expected error restoring the live catalog onto itself")
	}
}

func TestListCatalogBackupsNewestFirst(t *testing.T) {
	dir := t.TempDir()
	catalog := filepath.Join(dir, "catalog.sqlite")
	newTestCatalog(t, catalog, 1)
	for i := 0; i < 2; i++ {
		dest, err := BackupCatalog(catalog, 0)
		if err != nil {
			t.Fatal(err)
		}
		renameForDistinctTimestamp(t, dest, i)
	}
	list, err := ListCatalogBackups(catalog)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("got %d backups, want 2", len(list))
	}
	if list[0].Path < list[1].Path {
		t.Fatalf("expected newest-first order: %q then %q", list[0].Path, list[1].Path)
	}
}

func assertSnapshotCount(t *testing.T, path string, want int) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM ducklake_snapshot`).Scan(&n); err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if n != want {
		t.Fatalf("%s holds %d snapshot rows, want %d", path, n, want)
	}
}
