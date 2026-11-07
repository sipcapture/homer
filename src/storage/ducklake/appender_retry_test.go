package ducklake

import (
	"database/sql"
	"fmt"
	"sync"
	"testing"
)

// Exercise connection, appender construction, AppendRow, and Close failures
// using the real DuckDB driver. No failed batch may be pooled or partly commit.
func TestAppenderFailuresRetainEntireBatch(t *testing.T) {
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
			if len(tw.batch) != 2 || tw.batch[0][0] != int64(1) || tw.batch[1][0] != value {
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
			tw.batch[1][0] = int64(2)
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
