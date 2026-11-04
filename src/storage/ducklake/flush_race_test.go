// Copyright (C) 2026 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package ducklake

import (
	"database/sql"
	"sync"
	"testing"
	"time"

	"github.com/sipcapture/homer-core/src/utils/metrics"
)

// newFlushRaceTableWriter returns a TableWriter whose memory tables and lake
// table are plain in-memory DuckDB tables. With batchSize 1 every Write
// appends to the active memory table at once. The pool has as many
// connections as the real writer.
func newFlushRaceTableWriter(t *testing.T) *TableWriter {
	t.Helper()
	db, err := sql.Open("duckdb", "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	db.SetMaxOpenConns(writerPoolConns)
	for _, q := range []string{
		"CREATE TABLE mem_a (id BIGINT)",
		"CREATE TABLE mem_b (id BIGINT)",
		"CREATE TABLE lake (id BIGINT)",
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	tw := &TableWriter{
		db:        db,
		tableFQN:  "lake",
		memTables: [2]string{"mem_a", "mem_b"},
		batchSize: 1,
		flushCh:   make(chan flushJob, 2),
		catalogMu: &sync.Mutex{},
	}
	for i, mem := range tw.memTables {
		tw.flushInsertSQL[i] = "INSERT INTO lake SELECT * FROM " + mem
		tw.flushTruncateSQL[i] = "TRUNCATE TABLE " + mem
	}
	return tw
}

func waitForWriters(t *testing.T, wg *sync.WaitGroup) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Minute):
		t.Fatal("writers did not finish: appends and flushes deadlocked")
	}
}

func assertLakeRows(t *testing.T, db *sql.DB, want int) {
	t.Helper()
	var rows, distinct int
	if err := db.QueryRow("SELECT count(*), count(DISTINCT id) FROM lake").Scan(&rows, &distinct); err != nil {
		t.Fatal(err)
	}
	if rows != want || distinct != want {
		t.Fatalf("appended %d rows, lake has %d rows (%d distinct): %d lost", want, rows, distinct, want-distinct)
	}
}

// A flush swaps the memory tables, then copies the old one to the lake with
// INSERT ... SELECT and empties it with TRUNCATE. An append that read the old
// slot just before the swap must not commit between those two statements,
// or the TRUNCATE deletes a row the INSERT never copied.
func TestFlushSlotDirectKeepsRowsAppendedDuringFlush(t *testing.T) {
	tw := newFlushRaceTableWriter(t)
	const n = 2000
	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		defer close(stop)
		for i := 1; i <= n; i++ {
			if err := tw.Write([]interface{}{int64(i)}); err != nil {
				t.Error(err)
				return
			}
		}
	}()
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			old := int(tw.activeIdx.Load())
			tw.activeIdx.Store(int32(1 - old))
			tw.flushSlotDirect(old, tw.catalogMu)
		}
	}()
	waitForWriters(t, &wg)
	tw.flushSlotDirect(0, tw.catalogMu)
	tw.flushSlotDirect(1, tw.catalogMu)
	assertLakeRows(t, tw.db, n)
}

// A flush that waits for the catalog lock, held by compaction, must not hold
// the lock of its memory table. Otherwise, once the next swap makes that
// table active again, every append waits until compaction is done. The check
// is timing-based: if the flush has not reached the catalog lock within the
// sleep, the test passes without exercising the wait, so it does not prove the
// lock order. The loss tests above cover the race.
func TestFlushWaitingForCatalogLockDoesNotBlockAppends(t *testing.T) {
	tw := newFlushRaceTableWriter(t)
	if err := tw.Write([]interface{}{int64(1)}); err != nil {
		t.Fatal(err)
	}

	tw.catalogMu.Lock()
	catalogLocked := true
	defer func() {
		if catalogLocked {
			tw.catalogMu.Unlock()
		}
	}()
	tw.activeIdx.Store(1)
	flushed := make(chan struct{})
	go func() {
		defer close(flushed)
		tw.flushSlotDirect(0, tw.catalogMu)
	}()
	// Give the flush time to reach the catalog lock, then swap back so the
	// table it waits to flush is active again.
	time.Sleep(100 * time.Millisecond)
	tw.activeIdx.Store(0)

	const n = 10
	appended := make(chan error, 1)
	go func() {
		for i := 2; i <= n; i++ {
			if err := tw.Write([]interface{}{int64(i)}); err != nil {
				appended <- err
				return
			}
		}
		appended <- nil
	}()
	select {
	case err := <-appended:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("appends blocked while a flush waited for the catalog lock")
	}

	tw.catalogMu.Unlock()
	catalogLocked = false
	<-flushed
	tw.flushSlotDirect(0, tw.catalogMu)
	tw.flushSlotDirect(1, tw.catalogMu)
	assertLakeRows(t, tw.db, n)
}

// The same race through MultiTableWriter, with the central flush queue (the
// default for a SQLite catalog) and with the per-table flush worker. The
// flush loop swaps every millisecond while four goroutines write, so queued
// jobs fall behind the swaps and often copy and clear a memory table that is
// active again. After Stop every row must be in the lake exactly once.
func TestMultiTableWriterFlushKeepsConcurrentAppends(t *testing.T) {
	for _, tc := range []struct {
		name       string
		flushQueue bool
	}{
		{"flush queue", true},
		{"per-table flush worker", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tw := newFlushRaceTableWriter(t)
			key := TableKey{ProtoType: 1, SubType: "call"}
			mtw := &MultiTableWriter{
				tables:        map[TableKey]*TableWriter{key: tw},
				stopChan:      make(chan struct{}),
				flushInterval: time.Millisecond,
			}
			tw.catalogMu = &mtw.catalogMu
			if tc.flushQueue {
				mtw.flushQueueCh = make(chan centralFlushJob, 64)
			}
			tw.flushWg.Add(1)
			go tw.flushWorker()
			mtw.Start()

			const writers, perWriter = 4, 500
			var wg sync.WaitGroup
			for w := 0; w < writers; w++ {
				wg.Add(1)
				go func(first int) {
					defer wg.Done()
					for i := 0; i < perWriter; i++ {
						if err := mtw.WriteRecord(key, []interface{}{int64(first + i)}); err != nil {
							t.Error(err)
							return
						}
					}
				}(w * perWriter)
			}
			waitForWriters(t, &wg)
			if err := mtw.Stop(); err != nil {
				t.Fatal(err)
			}
			assertLakeRows(t, tw.db, writers*perWriter)
		})
	}
}

// A flush of an empty memory table records its duration but no rows: idle
// tables flush on every flush_interval_sec and must not create a flushed-rows
// series or log a flush. DeleteLabelValues reports whether a series existed
// without creating one.
func TestEmptyFlushRecordsNoRows(t *testing.T) {
	tw := newFlushRaceTableWriter(t)
	tw.tableFQN = "empty_flush_" + time.Now().Format("150405.000000000")
	tw.flushSlotDirect(0, tw.catalogMu)
	tw.flushSlotDirect(1, nil)
	if metrics.DucklakeTableFlushedRows.DeleteLabelValues(tw.tableFQN) {
		t.Fatal("empty flush recorded flushed rows")
	}
	if !metrics.DucklakeTableFlushDuration.DeleteLabelValues(tw.tableFQN) {
		t.Fatal("empty flush did not record its duration")
	}
	if err := tw.Write([]interface{}{int64(1)}); err != nil {
		t.Fatal(err)
	}
	tw.flushSlotDirect(int(tw.activeIdx.Load()), tw.catalogMu)
	if !metrics.DucklakeTableFlushedRows.DeleteLabelValues(tw.tableFQN) {
		t.Fatal("flush of one row recorded no flushed rows")
	}
	assertLakeRows(t, tw.db, 1)
}
