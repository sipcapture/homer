// Copyright (C) 2026 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package ducklake

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestInProcessSQLiteRefusedWhileCatalogAttached(t *testing.T) {
	dir := t.TempDir()
	catalog := filepath.Join(dir, "catalog.sqlite")
	newTestCatalog(t, catalog, 3)

	release := MarkCatalogAttached(catalog)

	if _, err := BackupCatalog(catalog, 2); !errors.Is(err, ErrCatalogAttachedInProcess) {
		t.Fatalf("BackupCatalog on attached catalog: err = %v, want ErrCatalogAttachedInProcess", err)
	}
	if _, err := RepairCatalogSnapshots(catalog); !errors.Is(err, ErrCatalogAttachedInProcess) {
		t.Fatalf("RepairCatalogSnapshots on attached catalog: err = %v, want ErrCatalogAttachedInProcess", err)
	}
	// A relative spelling of the same file must be recognised too.
	wd, _ := os.Getwd()
	if rel, err := filepath.Rel(wd, catalog); err == nil {
		if !IsCatalogAttachedInProcess(rel) {
			t.Fatalf("relative path %q not recognised as attached", rel)
		}
	}

	release()
	if IsCatalogAttachedInProcess(catalog) {
		t.Fatal("catalog still reported attached after release")
	}
	if _, err := BackupCatalog(catalog, 2); err != nil {
		t.Fatalf("BackupCatalog after release: %v", err)
	}
}

func TestMarkCatalogAttachedIsRefCounted(t *testing.T) {
	catalog := filepath.Join(t.TempDir(), "c.sqlite")
	r1 := MarkCatalogAttached(catalog)
	r2 := MarkCatalogAttached(catalog)
	r1()
	r1() // double release must not drop the second holder
	if !IsCatalogAttachedInProcess(catalog) {
		t.Fatal("second holder lost after first release")
	}
	r2()
	if IsCatalogAttachedInProcess(catalog) {
		t.Fatal("catalog still attached after all releases")
	}
}
