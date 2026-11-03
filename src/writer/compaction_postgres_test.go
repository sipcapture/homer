// Copyright (C) 2026 Homer Server Contributors
// SPDX-License-Identifier: AGPL-3.0-or-later
package writer

import (
	"github.com/sipcapture/homer-core/src/storage/ducklake"
	"testing"
)

func TestPostgresCatalogUsesDuckLakeCompaction(t *testing.T) {
	c := NewCompactionService(nil, "lake", "/tmp/data", "postgres://homer:private@localhost/lake", CompactionConfig{CatalogType: ducklake.CatalogPostgres, Engine: EngineNativeGo}, nil, nil, nil)
	if c.useNativeEngine() {
		t.Fatal("PostgreSQL selected SQLite-backed native safety routines")
	}
	if !c.verifyCatalogIntegrity() {
		t.Fatal("PostgreSQL invoked SQLite integrity checks")
	}
	c.quarantineScheduledDeletions()
}

func TestPostgresCompactionUsesConfiguredType(t *testing.T) {
	// Eligibility must follow the supplied type, independently of path syntax.
	c := NewCompactionService(nil, "lake", "/tmp/data", "catalog.sqlite", CompactionConfig{CatalogType: ducklake.CatalogPostgres, Engine: EngineNativeGo}, nil, nil, nil)
	if c.useNativeEngine() {
		t.Fatal("configured PostgreSQL enabled native SQLite compaction")
	}
	if !c.verifyCatalogIntegrity() {
		t.Fatal("configured PostgreSQL ran SQLite integrity checks")
	}
	c.quarantineScheduledDeletions()
}
