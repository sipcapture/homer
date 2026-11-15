package ducklake

import (
	"database/sql"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Exercise connection, appender construction, AppendRow, and Close failures
// using the real DuckDB driver. Transient failures retain rows; deterministic failures discard atomically.
func TestAppenderFailuresRecoverOrDiscardBatch(t *testing.T) {
	for _, failure := range []string{"connection", "create", "row", "close"} {
		t.Run(failure, func(t *testing.T) {
			tw := newBufferTestWriter(t)
			tw.batchSize = 100
			original := tw.db
			switch failure {
			case "connection":
				closed, err := sql.Open("duckdb", "")
				if err != nil {
					t.Fatal(err)
				}
				_ = closed.Close()
				tw.db = closed
				defer func() { tw.db = original }()
			case "create":
				if _, err := original.Exec("DROP TABLE buffer_a"); err != nil {
					t.Fatal(err)
				}
			case "close":
				if _, err := original.Exec("DROP TABLE buffer_a; CREATE TABLE buffer_a (id BIGINT PRIMARY KEY)"); err != nil {
					t.Fatal(err)
				}
			}
			if err := tw.Write([]interface{}{int64(1)}); err != nil {
				t.Fatal(err)
			}
			value := interface{}(int64(2))
			if failure == "row" {
				value = "invalid integer"
			}
			if failure == "close" {
				value = int64(1)
			}
			if err := tw.Write([]interface{}{value}); err != nil {
				t.Fatal(err)
			}
			if err := tw.flushBatch(); err == nil {
				t.Fatalf("expected %s failure", failure)
			}
			if failure == "connection" && (len(tw.batch) != 2 || tw.batch[0][0] != int64(1) || tw.batch[1][0] != value) {
				t.Fatalf("failed batch was discarded or mutated: %#v", tw.batch)
			}
			tw.db = original
			if failure == "create" {
				if _, err := original.Exec("CREATE TABLE buffer_a (id BIGINT)"); err != nil {
					t.Fatal(err)
				}
			}
			var rows int
			if err := original.QueryRow("SELECT count(*) FROM buffer_a").Scan(&rows); err != nil || rows != 0 {
				t.Fatalf("partial batch committed: rows=%d err=%v", rows, err)
			}
			if failure != "connection" {
				if len(tw.batch) != 0 {
					t.Fatal("deterministic failure retained poisoned batch")
				}
				_ = tw.Write([]interface{}{int64(1)})
				_ = tw.Write([]interface{}{int64(2)})
			} else {
				tw.batch[1][0] = int64(2)
			}
			if err := tw.flushBatch(); err != nil {
				t.Fatal(err)
			}
			if err := tw.flushSlotDirect(0, tw.catalogMu); err != nil {
				t.Fatal(err)
			}
			var distinct int
			if err := original.QueryRow("SELECT count(*), count(DISTINCT id) FROM saved").Scan(&rows, &distinct); err != nil || rows != 2 || distinct != 2 {
				t.Fatalf("retry lost/duplicated rows: %d/%d %v", rows, distinct, err)
			}
		})
	}
}

func TestSingleAppenderConcurrentWrites(t *testing.T) {
	tw := newBufferTestWriter(t)
	const workers, rows = 8, 128
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < rows; i++ {
				if err := tw.Write([]interface{}{int64(w*rows + i)}); err != nil {
					t.Error(fmt.Errorf("worker %d: %w", w, err))
					return
				}
			}
		}(w)
	}
	wg.Wait()
	if err := tw.flushSlotDirect(0, tw.catalogMu); err != nil {
		t.Fatal(err)
	}
	var count, distinct int
	if err := tw.db.QueryRow("SELECT count(*), count(DISTINCT id) FROM saved").Scan(&count, &distinct); err != nil || count != workers*rows || distinct != count {
		t.Fatalf("concurrent appends lost/duplicated rows: %d/%d %v", count, distinct, err)
	}
}

func TestAppenderRetryBound(t *testing.T) {
	tw := newBufferTestWriter(t)
	original := tw.db
	closed, err := sql.Open("duckdb", "")
	if err != nil {
		t.Fatal(err)
	}
	_ = closed.Close()
	tw.db = closed
	defer func() { tw.db = original }()
	for i := 0; i < appendMaxFailures; i++ {
		if err := tw.Write([]interface{}{int64(i)}); err == nil {
			t.Fatal("expected connection failure")
		}
	}
	if len(tw.batch) != 0 {
		t.Fatalf("exhausted batch retained %d rows", len(tw.batch))
	}
	tw.db = original
	if err := tw.Write([]interface{}{int64(99)}); err != nil {
		t.Fatal(err)
	}
}

func TestAppenderQueueBound(t *testing.T) {
	tw := newBufferTestWriter(t)
	tw.batchSize = 2048
	tw.batch = make([][]interface{}, 4096)
	start := time.Now()
	if err := tw.writeWithWait([]interface{}{int64(1)}, 25*time.Millisecond); err == nil {
		t.Fatal("full queue accepted row")
	}
	if time.Since(start) < 25*time.Millisecond {
		t.Fatal("full queue did not apply backpressure")
	}
	if len(tw.batch) != 4096 {
		t.Fatal("queue grew past bound")
	}
	tw.batch = nil
}

func TestAppenderQueueWaitsAndRecovers(t *testing.T) {
	tw := newBufferTestWriter(t)
	tw.batchSize = 2048
	for i := 0; i < 4096; i++ {
		tw.batch = append(tw.batch, []interface{}{int64(i)})
	}
	done := make(chan error, 1)
	go func() { done <- tw.Write([]interface{}{int64(4096)}) }()
	select {
	case err := <-done:
		t.Fatalf("writer did not wait: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	if err := tw.flushBatch(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("writer not woken after queue drained")
	}
	if err := tw.flushBatch(); err != nil {
		t.Fatal(err)
	}
	if err := tw.flushSlotDirect(0, tw.catalogMu); err != nil {
		t.Fatal(err)
	}
	var count, distinct int
	if err := tw.db.QueryRow("SELECT count(*),count(DISTINCT id) FROM saved").Scan(&count, &distinct); err != nil || count != 4097 || distinct != 4097 {
		t.Fatalf("backpressure lost rows: %d/%d %v", count, distinct, err)
	}
}

func TestAppenderDropLogRateLimit(t *testing.T) {
	tw := &TableWriter{}
	now := time.Now()
	var logged atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 128; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if tw.shouldLogBatchDrop(now) {
				logged.Add(1)
			}
		}()
	}
	wg.Wait()
	if logged.Load() != 1 {
		t.Fatalf("log storm: %d logs", logged.Load())
	}
	if !tw.shouldLogBatchDrop(now.Add(10 * time.Second)) {
		t.Fatal("log limiter did not recover")
	}
}

func TestAppenderStalledSlotBackpressure(t *testing.T) {
	tw := newBufferTestWriter(t)
	tw.memMu[0].Lock()
	var release sync.Once
	unlock := func() { release.Do(tw.memMu[0].Unlock) }
	defer unlock()
	const rows = 1100
	done := make(chan error, rows)
	for i := 0; i < rows; i++ {
		go func(i int) { done <- tw.Write([]interface{}{int64(i)}) }(i)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		tw.batchMu.Lock()
		waiting := tw.batchSpace != nil
		tw.batchMu.Unlock()
		if waiting {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("writers did not fill stalled queue")
		}
		time.Sleep(time.Millisecond)
	}
	select {
	case err := <-done:
		t.Fatalf("stalled writer returned early: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	unlock()
	for i := 0; i < rows; i++ {
		select {
		case err := <-done:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("writer did not recover")
		}
	}
	if err := tw.flushSlotDirect(0, tw.catalogMu); err != nil {
		t.Fatal(err)
	}
	var count, distinct int
	if err := tw.db.QueryRow("SELECT count(*),count(DISTINCT id) FROM saved").Scan(&count, &distinct); err != nil || count != rows || distinct != rows {
		t.Fatalf("stalled slot lost rows: %d/%d %v", count, distinct, err)
	}
}
