// Copyright (C) 2026 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package ducklake

import (
	"database/sql"
	"strings"
	"sync"
)

// instanceFailure carries the first error after which DuckDB invalidated a
// writer instance. All shards of a ShardedWriter share one, so a single
// receiver learns about any of them. It is sent at most once.
type instanceFailure struct {
	once sync.Once
	ch   chan error
}

func newInstanceFailure() *instanceFailure {
	return &instanceFailure{ch: make(chan error, 1)}
}

func (f *instanceFailure) report(err error) {
	f.once.Do(func() { f.ch <- err })
}

// instanceInvalidated reports whether DuckDB has invalidated the instance
// behind db. It reads probeTable, an in-memory buffer table: once the
// instance is invalidated every statement on it fails with "database has
// been invalidated", while an ordinary out-of-memory error leaves it usable.
func instanceInvalidated(db *sql.DB, probeTable string) bool {
	rows, err := db.Query("SELECT 1 FROM " + probeTable + " LIMIT 0")
	if err == nil {
		err = rows.Close()
	}
	return err != nil && strings.Contains(err.Error(), "database has been invalidated")
}

// noteFlushError is called when a flush of tw fails with an error that looks
// fatal. isDuckDBFatalError also matches "failed to pin block", which DuckDB
// raises for ordinary out-of-memory errors too, so the instance is probed
// before the failure is reported.
func (mtw *MultiTableWriter) noteFlushError(tw *TableWriter, err error) {
	if mtw.failure == nil || !instanceInvalidated(mtw.db, tw.memTables[0]) {
		return
	}
	mtw.failure.report(err)
}

// Invalidated returns a channel that receives one error when a fatal DuckDB
// error has invalidated the writer's instance. The instance cannot be used
// again: every later append, flush and search on it fails until the process
// restarts.
func (mtw *MultiTableWriter) Invalidated() <-chan error {
	return mtw.failure.ch
}
