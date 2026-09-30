// Copyright (C) 2026 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package writer

import (
	"database/sql"
	"fmt"

	"github.com/sipcapture/homer-core/src/storage/ducklake"
	logger "github.com/sipcapture/homer-core/src/utils/logging"
)

// maintenanceDBOpener opens a fresh DuckDB instance with the lake attached and
// returns the function that closes it.
type maintenanceDBOpener func() (*sql.DB, func() error, error)

// maxMergeFatalsPerTable is how many times a table's merge may invalidate the
// compaction instance before that table is left out of the DuckDB merge until
// restart. The same partition would otherwise crash the merge every cycle.
const maxMergeFatalsPerTable = 2

// UseMaintenanceDB moves compaction onto its own DuckDB instance so a fatal
// DuckLake error in a merge invalidates only that instance, not the one serving
// ingest and search (sipcapture/homer#1048). The instance is reopened before the
// next cycle after such an error. Call before Start.
func (c *CompactionService) UseMaintenanceDB(open maintenanceDBOpener) error {
	db, closeFn, err := open()
	if err != nil {
		return err
	}
	c.dbMu.Lock()
	c.db, c.closeDB, c.openDB = db, closeFn, open
	c.dbMu.Unlock()
	return nil
}

// database returns the DuckDB instance compaction currently runs on.
func (c *CompactionService) database() *sql.DB {
	c.dbMu.RLock()
	defer c.dbMu.RUnlock()
	return c.db
}

// noteDBError reports whether err invalidated the compaction DuckDB instance,
// and if so marks it for reopening.
func (c *CompactionService) noteDBError(err error) bool {
	if !ducklake.IsDuckDBFatalError(err) {
		return false
	}
	if !c.dbBroken.CompareAndSwap(false, true) {
		return true
	}
	c.dbMu.RLock()
	isolated := c.openDB != nil
	c.dbMu.RUnlock()
	if isolated {
		logger.Error("CompactionService: compaction DuckDB instance was invalidated by a fatal error; "+
			"ingest and search are unaffected, the instance is reopened before the next cycle",
			"lake", c.lakeName, "error", err)
	} else {
		logger.Error("CompactionService: DuckDB instance shared with ingest and search was invalidated "+
			"by a fatal error during compaction; restart homer to recover",
			"lake", c.lakeName, "error", err)
	}
	return true
}

// ensureDB reopens the compaction instance after a fatal error. It reports
// false when there is no usable instance for this cycle.
func (c *CompactionService) ensureDB() bool {
	if !c.dbBroken.Load() {
		return true
	}
	c.dbMu.RLock()
	open := c.openDB
	c.dbMu.RUnlock()
	if open == nil {
		return false
	}
	db, closeFn, err := open()
	if err != nil {
		logger.Error("CompactionService: reopening the compaction DuckDB instance failed; skipping this cycle",
			"lake", c.lakeName, "error", err)
		return false
	}
	c.dbMu.Lock()
	oldClose := c.closeDB
	c.db, c.closeDB = db, closeFn
	c.dbBroken.Store(false)
	c.dbMu.Unlock()
	if oldClose != nil {
		if err := oldClose(); err != nil {
			logger.Warn("CompactionService: closing the invalidated compaction DuckDB instance", "error", err)
		}
	}
	logger.Info("CompactionService: compaction DuckDB instance reopened", "lake", c.lakeName)
	return true
}

// closeMaintenanceDB closes the dedicated instance, if compaction owns one.
func (c *CompactionService) closeMaintenanceDB() {
	c.dbMu.Lock()
	closeFn := c.closeDB
	c.closeDB = nil
	c.dbMu.Unlock()
	if closeFn != nil {
		if err := closeFn(); err != nil {
			logger.Warn("CompactionService: closing compaction DuckDB instance", "error", err)
		}
	}
}

// recordMergeFatal counts a fatal merge error for table and leaves the table
// out of the DuckDB merge once it reaches maxMergeFatalsPerTable.
func (c *CompactionService) recordMergeFatal(table string) {
	c.mu.Lock()
	if c.mergeFatals == nil {
		c.mergeFatals = make(map[string]int)
	}
	c.mergeFatals[table]++
	n := c.mergeFatals[table]
	c.mu.Unlock()
	if n < maxMergeFatalsPerTable {
		return
	}
	reason := fmt.Sprintf("merge invalidated DuckDB %d times", n)
	c.mergeQuarantined.Store(table, reason)
	logger.Error("CompactionService: table left out of merge_adjacent_files until restart",
		"table", table, "reason", reason)
}
