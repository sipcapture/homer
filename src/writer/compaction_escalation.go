// Copyright (C) 2026 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package writer

import (
	"fmt"

	logger "github.com/sipcapture/homer-core/src/utils/logging"
)

// maintenanceFailureEscalation is the number of consecutive failures of one
// maintenance operation after which it is logged as an error. A cleanup that
// keeps failing lets the disk fill at the ingest rate (sipcapture/homer#1048:
// 25 days of warnings, then a full disk).
const maintenanceFailureEscalation = 3

// recordMaintenanceResult tracks consecutive failures of op and reports
// whether this failure was escalated to an error.
func (c *CompactionService) recordMaintenanceResult(op string, err error) bool {
	c.mu.Lock()
	if c.maintenanceFailures == nil {
		c.maintenanceFailures = make(map[string]int)
	}
	if err == nil {
		prev := c.maintenanceFailures[op]
		delete(c.maintenanceFailures, op)
		c.mu.Unlock()
		if prev >= maintenanceFailureEscalation {
			logger.Info("CompactionService: maintenance operation recovered", "op", op, "failed_runs", prev)
		}
		return false
	}
	c.maintenanceFailures[op]++
	n := c.maintenanceFailures[op]
	c.mu.Unlock()
	if n < maintenanceFailureEscalation {
		return false
	}

	args := []any{"lake", c.lakeName, "op", op, "consecutive_failures", n, "error", err}
	if queued, qerr := c.scheduledDeletionCount(); qerr == nil {
		args = append(args, "files_scheduled_for_deletion", queued)
	}
	logger.Error("CompactionService: maintenance operation keeps failing; parquet files are not being "+
		"deleted and disk usage grows with ingest until it is fixed", args...)
	return true
}

// scheduledDeletionCount returns the length of DuckLake's file deletion queue.
func (c *CompactionService) scheduledDeletionCount() (int64, error) {
	if c.database() == nil {
		return 0, fmt.Errorf("no database")
	}
	var n int64
	err := c.database().QueryRow(fmt.Sprintf(
		"SELECT count(*) FROM __ducklake_metadata_%s.ducklake_files_scheduled_for_deletion", c.lakeName)).Scan(&n)
	return n, err
}

// maintenanceFailureStats copies the consecutive-failure counters. The caller
// holds c.mu.
func (c *CompactionService) maintenanceFailureStats() map[string]int {
	out := make(map[string]int, len(c.maintenanceFailures))
	for op, n := range c.maintenanceFailures {
		out[op] = n
	}
	return out
}
