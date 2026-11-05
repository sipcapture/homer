package ducklake

import (
	"database/sql"
	"sync"
	"testing"
	"time"
)

func newBufferTestWriter(t *testing.T) *TableWriter {
	t.Helper()
	db, err := sql.Open("duckdb", "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec("CREATE TABLE saved (id BIGINT); CREATE TABLE buffer_a (id BIGINT); CREATE TABLE buffer_b (id BIGINT)"); err != nil {
		t.Fatal(err)
	}
	tw := &TableWriter{db: db, tableFQN: "saved", memTables: [2]string{"buffer_a", "buffer_b"}, batchSize: 1, flushCh: make(chan flushJob, 2), catalogMu: &sync.Mutex{}}
	for i, name := range tw.memTables {
		tw.flushInsertSQL[i] = "INSERT INTO saved SELECT * FROM " + name
		tw.flushTruncateSQL[i] = "TRUNCATE TABLE " + name
	}
	tw.flushWg.Add(1)
	go tw.flushWorker()
	t.Cleanup(func() {
		if err := tw.Close(); err != nil {
			t.Error(err)
		}
	})
	return tw
}

func TestBufferReuseContinuesDuringQueuedFlush(t *testing.T) {
	tw := newBufferTestWriter(t)
	tw.catalogMu.Lock()
	var once sync.Once
	release := func() { once.Do(tw.catalogMu.Unlock) }
	defer release()
	if err := tw.Write([]interface{}{int64(1)}); err != nil {
		t.Fatal(err)
	}
	tw.SwapAndFlush()
	// The other buffer can accept data while the first flush is blocked.
	if err := tw.Write([]interface{}{int64(2)}); err != nil {
		t.Fatal(err)
	}
	tw.SwapAndFlush()
	writeDone := make(chan error, 1)
	go func() { writeDone <- tw.Write([]interface{}{int64(3)}) }()
	select {
	case err := <-writeDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("queued flush blocked reuse before acquiring the catalog lock")
	}
	release()
	// Direct flushes also drain any rows not picked up by the queued jobs.
	for slot := range tw.memTables {
		if err := tw.flushSlotDirect(slot, tw.catalogMu); err != nil {
			t.Fatal(err)
		}
	}
	var n, distinct int
	if err := tw.db.QueryRow("SELECT count(*), count(DISTINCT id) FROM saved").Scan(&n, &distinct); err != nil {
		t.Fatal(err)
	}
	if n != 3 || distinct != 3 {
		t.Fatalf("rows=%d unique=%d, want 3 of each", n, distinct)
	}
}

func TestFailedBufferInsertRetainsRows(t *testing.T) {
	tw := newBufferTestWriter(t)
	if err := tw.Write([]interface{}{int64(1)}); err != nil {
		t.Fatal(err)
	}
	insert := tw.flushInsertSQL[0]
	tw.flushInsertSQL[0] = "INSERT INTO missing_table SELECT * FROM buffer_a"
	if err := tw.flushSlotDirect(0, tw.catalogMu); err == nil {
		t.Fatal("expected insert failure")
	}
	var n int
	if err := tw.db.QueryRow("SELECT count(*) FROM buffer_a").Scan(&n); err != nil || n != 1 {
		t.Fatalf("failed insert discarded rows: %d %v", n, err)
	}
	tw.flushInsertSQL[0] = insert
	if err := tw.flushSlotDirect(0, tw.catalogMu); err != nil {
		t.Fatal(err)
	}
	if err := tw.db.QueryRow("SELECT count(*) FROM saved").Scan(&n); err != nil || n != 1 {
		t.Fatalf("retry lost rows: %d %v", n, err)
	}
}

func TestFailedBufferClearDoesNotReinsert(t *testing.T) {
	tw := newBufferTestWriter(t)
	if err := tw.Write([]interface{}{int64(1)}); err != nil {
		t.Fatal(err)
	}
	truncate := tw.flushTruncateSQL[0]
	tw.flushTruncateSQL[0] = "TRUNCATE TABLE missing_table"
	if err := tw.flushSlotDirect(0, tw.catalogMu); err == nil {
		t.Fatal("expected clear failure")
	}
	tw.flushTruncateSQL[0] = truncate
	// Appending again must clear committed rows first, preserving only new data
	// for the next insert rather than duplicating the earlier successful insert.
	if err := tw.Write([]interface{}{int64(2)}); err != nil {
		t.Fatal(err)
	}
	if err := tw.flushSlotDirect(0, tw.catalogMu); err != nil {
		t.Fatal(err)
	}
	var n, distinct int
	if err := tw.db.QueryRow("SELECT count(*), count(DISTINCT id) FROM saved").Scan(&n, &distinct); err != nil {
		t.Fatal(err)
	}
	if n != 2 || distinct != 2 {
		t.Fatalf("clear retry duplicated rows: %d unique=%d", n, distinct)
	}
}

// While a failed TRUNCATE remains broken, an append must preserve its Go batch
// and keep new rows out of the committed memory table.
func TestFailedBufferClearRetainsNewBatch(t *testing.T) {
	tw := newBufferTestWriter(t)
	if err := tw.Write([]interface{}{int64(1)}); err != nil {
		t.Fatal(err)
	}
	truncate := tw.flushTruncateSQL[0]
	tw.flushTruncateSQL[0] = "TRUNCATE TABLE missing_table"
	if err := tw.flushSlotDirect(0, tw.catalogMu); err == nil {
		t.Fatal("expected clear failure")
	}
	if err := tw.Write([]interface{}{int64(2)}); err == nil {
		t.Fatal("append should fail while clear is broken")
	}
	var rows int
	if err := tw.db.QueryRow("SELECT count(*) FROM buffer_a").Scan(&rows); err != nil || rows != 1 {
		t.Fatalf("new rows mixed with committed rows: %d %v", rows, err)
	}
	tw.flushTruncateSQL[0] = truncate
	if err := tw.Write([]interface{}{int64(3)}); err != nil {
		t.Fatal(err)
	}
	if err := tw.flushSlotDirect(0, tw.catalogMu); err != nil {
		t.Fatal(err)
	}
	var distinct int
	if err := tw.db.QueryRow("SELECT count(*), count(DISTINCT id) FROM saved").Scan(&rows, &distinct); err != nil || rows != 3 || distinct != 3 {
		t.Fatalf("retained batch lost or duplicated: rows=%d distinct=%d err=%v", rows, distinct, err)
	}
}

func TestFailedBufferClearRetryWithoutAppend(t *testing.T) {
	tw := newBufferTestWriter(t)
	if err := tw.Write([]interface{}{int64(1)}); err != nil {
		t.Fatal(err)
	}
	truncate := tw.flushTruncateSQL[0]
	tw.flushTruncateSQL[0] = "TRUNCATE TABLE missing_table"
	if err := tw.flushSlotDirect(0, tw.catalogMu); err == nil {
		t.Fatal("expected clear failure")
	}
	tw.flushTruncateSQL[0] = truncate
	if err := tw.flushSlotDirect(0, tw.catalogMu); err != nil {
		t.Fatal(err)
	}
	var saved, buffered int
	if err := tw.db.QueryRow("SELECT (SELECT count(*) FROM saved), (SELECT count(*) FROM buffer_a)").Scan(&saved, &buffered); err != nil || saved != 1 || buffered != 0 {
		t.Fatalf("retry should only clear: saved=%d buffered=%d err=%v", saved, buffered, err)
	}
}
