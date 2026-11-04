// Copyright (C) 2026 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package ducklake

import (
	"database/sql"
	"errors"
	"strings"
	"testing"
)

// invalidateInstance makes DuckDB invalidate the in-memory instance behind db
// the way docs/OOM.md describes: a checkpoint that runs out of memory with
// nowhere to spill.
func invalidateInstance(t *testing.T, db *sql.DB, probeTable string) {
	t.Helper()
	for _, q := range []string{
		"SET memory_limit = '64MB'",
		"SET temp_directory = ''",
		"CREATE TABLE fill (id BIGINT, s VARCHAR)",
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(q, err)
		}
	}
	for i := 0; i < 400; i++ {
		if _, err := db.Exec("INSERT INTO fill SELECT range, md5(range::VARCHAR) || repeat('y', 100) FROM range(20000)"); err != nil {
			break
		}
	}
	_, err := db.Exec("FORCE CHECKPOINT")
	if !instanceInvalidated(db, probeTable) {
		t.Skipf("the checkpoint did not invalidate the DuckDB instance (%v); this DuckDB version recovers from it", err)
	}
}

// newInvalidationWriter returns a TableWriter on plain in-memory tables whose
// fatal flush errors go to a MultiTableWriter, as getOrCreateTable wires them.
func newInvalidationWriter(t *testing.T) (*MultiTableWriter, *TableWriter) {
	t.Helper()
	tw := newFlushRaceTableWriter(t)
	mtw := &MultiTableWriter{db: tw.db, failure: newInstanceFailure()}
	tw.onFatalFlush = func(err error) { mtw.noteFlushError(tw, err) }
	return mtw, tw
}

// After DuckDB invalidates the writer's instance, a flush fails and the
// writer reports it once on Invalidated.
func TestFlushAfterInvalidationReportsWriter(t *testing.T) {
	mtw, tw := newInvalidationWriter(t)
	if _, err := tw.db.Exec("INSERT INTO mem_a VALUES (1)"); err != nil {
		t.Fatal(err)
	}
	tw.flushSlotDirect(0, nil)
	assertLakeRows(t, tw.db, 1)
	select {
	case err := <-mtw.Invalidated():
		t.Fatalf("a successful flush reported the instance as invalidated: %v", err)
	default:
	}

	invalidateInstance(t, tw.db, tw.memTables[0])
	tw.flushSlotDirect(0, nil)
	select {
	case err := <-mtw.Invalidated():
		if !strings.Contains(err.Error(), "invalidated") {
			t.Fatalf("reported error %q does not say the instance was invalidated", err)
		}
	default:
		t.Fatal("a flush on an invalidated instance was not reported")
	}

	tw.flushSlotDirect(1, nil)
	select {
	case err := <-mtw.Invalidated():
		t.Fatalf("the invalidation was reported twice: %v", err)
	default:
	}
}

// DuckDB also says "failed to pin block" in ordinary out-of-memory errors that
// leave the instance usable. Those are not reported.
func TestOrdinaryOutOfMemoryErrorIsNotReported(t *testing.T) {
	mtw, tw := newInvalidationWriter(t)
	oom := errors.New("Out of Memory Error: failed to pin block of size 256.0 KiB (1.9 GiB/2.0 GiB used)")
	if !isDuckDBFatalError(oom.Error()) {
		t.Fatal("isDuckDBFatalError no longer matches this error; the probe in noteFlushError may be unnecessary")
	}
	mtw.noteFlushError(tw, oom)
	select {
	case err := <-mtw.Invalidated():
		t.Fatalf("an out-of-memory error on a usable instance was reported: %v", err)
	default:
	}
}
