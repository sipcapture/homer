// Copyright (C) 2026 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package writer

import (
	"errors"
	"testing"
)

func TestMaintenanceFailureEscalation(t *testing.T) {
	svc := &CompactionService{lakeName: "lake"}
	boom := errors.New("Invalid type in column \"path\"")

	for i, wantEscalated := range []bool{false, false, true, true} {
		if got := svc.recordMaintenanceResult("cleanup_old_files", boom); got != wantEscalated {
			t.Errorf("failure %d: escalated = %v, want %v", i+1, got, wantEscalated)
		}
	}
	if got := svc.GetStats()["maintenance_consecutive_failures"].(map[string]int)["cleanup_old_files"]; got != 4 {
		t.Errorf("stats consecutive failures = %d, want 4", got)
	}
	if svc.recordMaintenanceResult("delete_orphaned_files", boom) {
		t.Error("failures of one operation escalated another")
	}

	svc.recordMaintenanceResult("cleanup_old_files", nil)
	if got := svc.recordMaintenanceResult("cleanup_old_files", boom); got {
		t.Error("counter not reset after a successful run")
	}
}
