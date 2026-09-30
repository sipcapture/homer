// Copyright (C) 2026 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package writer

import (
	"errors"
	"strings"

	"github.com/sipcapture/homer-core/src/storage/ducklake"
	logger "github.com/sipcapture/homer-core/src/utils/logging"
)

// catalogIntegrityOK reports whether compaction may keep writing to the
// catalog. It latches false once a check finds damage.
func (c *CompactionService) catalogIntegrityOK() bool {
	return !c.catalogCorrupt.Load()
}

// stopOnCatalogDamage stops compaction until restart. Every further cycle would
// write to a damaged catalog and rotate the good pre-merge backups away.
func (c *CompactionService) stopOnCatalogDamage(msg string, args ...any) {
	if !c.catalogCorrupt.CompareAndSwap(false, true) {
		return
	}
	args = append([]any{"lake", c.lakeName, "catalog", c.catalogPath}, args...)
	logger.Error("CompactionService: "+msg+"; compaction stopped until restart to keep the last good "+
		"catalog backups. Stop homer, then `homer catalog restore` or `homer system --rebuild-catalog`",
		args...)
}

// verifyCatalogIntegrity runs after the merge, before the maintenance calls
// write to the catalog again.
func (c *CompactionService) verifyCatalogIntegrity() bool {
	if !c.catalogIntegrityOK() {
		return false
	}
	path := strings.TrimSpace(c.catalogPath)
	if path == "" {
		return true
	}

	n, err := ducklake.CountCorruptDataFileTableIDsLive(path, c.database())
	switch {
	case err != nil:
		logger.Warn("CompactionService: ducklake_data_file.table_id check failed", "error", err)
	case n > 0:
		c.stopOnCatalogDamage("ducklake_data_file has non-integer table_id values", "corrupt_rows", n)
		return false
	}

	report, err := ducklake.QuickCheckCatalog(path)
	switch {
	case errors.Is(err, ducklake.ErrSQLiteCLIUnavailable):
		logger.Debug("CompactionService: sqlite3 CLI not installed, skipping catalog quick_check")
	case err != nil:
		logger.Warn("CompactionService: catalog quick_check could not run", "error", err)
	case report != "":
		c.stopOnCatalogDamage("SQLite quick_check reports catalog damage", "quick_check", report)
		return false
	}
	return true
}

// quarantineScheduledDeletions removes deletion-queue rows DuckLake cannot
// parse; one such row otherwise blocks all file cleanup. Caller holds the
// catalog lock.
func (c *CompactionService) quarantineScheduledDeletions() {
	path := strings.TrimSpace(c.catalogPath)
	if path == "" {
		return
	}
	res, err := ducklake.QuarantineMalformedScheduledDeletions(path, c.database())
	if err != nil {
		logger.Warn("CompactionService: scan of ducklake_files_scheduled_for_deletion failed", "error", err)
		return
	}
	if res.Removed > 0 {
		logger.Error("CompactionService: removed malformed rows from ducklake_files_scheduled_for_deletion "+
			"so file cleanup can run; the files they referenced are reclaimed by delete_orphaned_files",
			"lake", c.lakeName, "removed", res.Removed, "saved_to", res.File)
	}
}
