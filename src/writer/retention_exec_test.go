package writer

import (
	"database/sql"
	"testing"

	_ "github.com/duckdb/duckdb-go/v2"
)

// TestRunRetentionRespectsUnit exercises runRetention against a real
// in-memory DuckDB connection to confirm the "hours" unit actually deletes
// on an hours-based cutoff rather than days, and that the default ("days")
// behavior is unchanged.
func TestRunRetentionRespectsUnit(t *testing.T) {
	db, err := sql.Open("duckdb", "")
	if err != nil {
		t.Skipf("duckdb unavailable: %v", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)

	if _, err := db.Exec(`CREATE TABLE t (id INTEGER, timestamp TIMESTAMP)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	// One row 10 hours old (older than a 4h retention, newer than a 1-day retention),
	// one row 10 days old (older than both).
	if _, err := db.Exec(`INSERT INTO t VALUES
		(1, now() - INTERVAL 10 HOUR),
		(2, now() - INTERVAL 10 DAY),
		(3, now())`); err != nil {
		t.Fatalf("insert rows: %v", err)
	}

	svc := &CompactionService{db: db, config: CompactionConfig{RetentionUnit: "hours"}}
	deleted, err := svc.runRetention("t", 4)
	if err != nil {
		t.Fatalf("runRetention(hours) failed: %v", err)
	}
	if deleted != 2 {
		t.Fatalf("runRetention(4 hours) deleted %d rows, want 2", deleted)
	}

	var remaining int
	if err := db.QueryRow(`SELECT count(*) FROM t`).Scan(&remaining); err != nil {
		t.Fatalf("count remaining: %v", err)
	}
	if remaining != 1 {
		t.Fatalf("remaining rows = %d, want 1", remaining)
	}
}

// TestRunRetentionDefaultUnitIsDays confirms omitting RetentionUnit (empty
// string) still behaves like "days", matching pre-existing behavior.
func TestRunRetentionDefaultUnitIsDays(t *testing.T) {
	db, err := sql.Open("duckdb", "")
	if err != nil {
		t.Skipf("duckdb unavailable: %v", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)

	if _, err := db.Exec(`CREATE TABLE t (id INTEGER, timestamp TIMESTAMP)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO t VALUES
		(1, now() - INTERVAL 10 HOUR),
		(2, now() - INTERVAL 10 DAY)`); err != nil {
		t.Fatalf("insert rows: %v", err)
	}

	svc := &CompactionService{db: db, config: CompactionConfig{}}
	deleted, err := svc.runRetention("t", 4)
	if err != nil {
		t.Fatalf("runRetention(default) failed: %v", err)
	}
	// A 4-day cutoff only catches the 10-day-old row, not the 10-hour-old one.
	if deleted != 1 {
		t.Fatalf("runRetention(4, default unit) deleted %d rows, want 1", deleted)
	}
}
