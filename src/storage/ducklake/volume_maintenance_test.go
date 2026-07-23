// Copyright (C) 2026 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package ducklake

import (
	"strings"
	"testing"
)

func TestVolumeMaintenanceSQL(t *testing.T) {
	stmts := volumeMaintenanceSQL("homer_lake_cold", 1800)
	if len(stmts) != 3 {
		t.Fatalf("expected 3 maintenance statements, got %d", len(stmts))
	}
	if !strings.Contains(stmts[0], "ducklake_expire_snapshots('homer_lake_cold'") {
		t.Errorf("first statement must expire snapshots, got: %s", stmts[0])
	}
	if !strings.Contains(stmts[0], "INTERVAL '1800 seconds'") {
		t.Errorf("expire must use configured window, got: %s", stmts[0])
	}
	if !strings.Contains(stmts[1], "ducklake_cleanup_old_files('homer_lake_cold', cleanup_all => true)") {
		t.Errorf("second statement must cleanup old files, got: %s", stmts[1])
	}
	if !strings.Contains(stmts[2], "ducklake_delete_orphaned_files('homer_lake_cold', cleanup_all => true)") {
		t.Errorf("third statement must delete orphaned files, got: %s", stmts[2])
	}
}

func TestVolumeMaintenanceSQLDefaultWindow(t *testing.T) {
	stmts := volumeMaintenanceSQL("homer_lake_hot", 0)
	if !strings.Contains(stmts[0], "INTERVAL '3600 seconds'") {
		t.Errorf("zero window must default to 3600s, got: %s", stmts[0])
	}
}
