package node

import (
	"database/sql"
	"testing"

	_ "github.com/duckdb/duckdb-go/v2"
)

func TestNodeTimeRange(t *testing.T) {
	db, err := sql.Open("duckdb", "")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	// Two hep_proto_* tables with known timestamps.
	mustExec(t, db, `CREATE TABLE hep_proto_1_call (timestamp TIMESTAMP)`)
	mustExec(t, db, `INSERT INTO hep_proto_1_call VALUES (TIMESTAMP '2025-01-22 00:00:00')`)
	mustExec(t, db, `CREATE TABLE hep_proto_1_default (timestamp TIMESTAMP)`)
	mustExec(t, db, `INSERT INTO hep_proto_1_default VALUES (TIMESTAMP '2025-01-23 00:00:00')`)

	minNs, maxNs, err := nodeTimeRange(db)
	if err != nil {
		t.Fatal(err)
	}
	wantMin := int64(1737504000000000000) // 2025-01-22T00:00:00Z
	wantMax := int64(1737590400000000000) // 2025-01-23T00:00:00Z
	if minNs != wantMin || maxNs != wantMax {
		t.Fatalf("got min=%d max=%d, want min=%d max=%d", minNs, maxNs, wantMin, wantMax)
	}
}

func TestNodeTimeRangeEmpty(t *testing.T) {
	db, _ := sql.Open("duckdb", "")
	defer db.Close()
	minNs, maxNs, err := nodeTimeRange(db)
	if err != nil {
		t.Fatal(err)
	}
	if minNs != 0 || maxNs != 0 {
		t.Fatalf("empty node should report 0/0, got %d/%d", minNs, maxNs)
	}
}

func mustExec(t *testing.T, db *sql.DB, q string) {
	t.Helper()
	if _, err := db.Exec(q); err != nil {
		t.Fatalf("exec %q: %v", q, err)
	}
}
