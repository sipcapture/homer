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
