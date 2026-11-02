// Copyright (C) 2026 Homer Server Contributors
// SPDX-License-Identifier: AGPL-3.0-or-later
package writer

import "testing"

func TestPostgresCatalogUsesDuckLakeCompaction(t *testing.T) {
	c := NewCompactionService(nil, "lake", "/tmp/data", "postgres://homer:private@localhost/lake", CompactionConfig{Engine: EngineNativeGo}, nil, nil, nil)
	if c.useNativeEngine() {
		t.Fatal("PostgreSQL selected SQLite-backed native safety routines")
	}
	if !c.verifyCatalogIntegrity() {
		t.Fatal("PostgreSQL invoked SQLite integrity checks")
	}
	c.quarantineScheduledDeletions()
}
