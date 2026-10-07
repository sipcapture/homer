package ducklake

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	_ "github.com/duckdb/duckdb-go/v2"
	_ "modernc.org/sqlite"
)

// newDatePartitionLake attaches a real DuckLake as "lake". Skipped when the
// DuckDB extensions are unavailable (offline CI).
func newDatePartitionLake(t *testing.T) *sql.DB {
	t.Helper()
	dir := t.TempDir()
	data := filepath.Join(dir, "data")
	if err := os.MkdirAll(data, 0o755); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("duckdb", "")
	if err != nil {
		t.Skipf("duckdb unavailable: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	db.SetMaxOpenConns(1)
	for _, stmt := range []string{"LOAD ducklake;", "LOAD sqlite;"} {
		if _, err := db.Exec(stmt); err != nil {
			t.Skipf("extension unavailable (%q): %v", stmt, err)
		}
	}
	if _, err := db.Exec(fmt.Sprintf("ATTACH 'ducklake:sqlite:%s' AS lake (DATA_PATH '%s');",
		filepath.Join(dir, "catalog.sqlite"), data)); err != nil {
		t.Skipf("ATTACH failed: %v", err)
	}
	return db
}

func execAll(t *testing.T, db *sql.DB, stmts ...string) {
	t.Helper()
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("%s: %v", s, err)
		}
	}
}

func TestEnsureDatePartition(t *testing.T) {
	ctx := context.Background()
	db := newDatePartitionLake(t)
	execAll(t, db,
		`CREATE TABLE lake.cpu (ts TIMESTAMP, host VARCHAR)`,
		`INSERT INTO lake.cpu VALUES (TIMESTAMP '2026-10-01 10:00:00', 'old')`,
		`CREATE TABLE lake.cdr (ts TIMESTAMP, date VARCHAR)`,
		`CREATE TABLE lake.lookup (name VARCHAR)`,
		`CREATE TABLE lake.done (date DATE, ts TIMESTAMP)`,
		`ALTER TABLE lake.done SET PARTITIONED BY (date)`,
	)

	tests := []struct {
		table    string
		wantFill bool
		wantKeys []string
	}{
		{"cpu", true, []string{"date"}},
		{"cdr", false, nil},
		{"lookup", false, nil},
		{"done", true, []string{"date"}},
	}
	for _, tc := range tests {
		t.Run(tc.table, func(t *testing.T) {
			fill, err := EnsureDatePartition(ctx, db, "lake", "main", tc.table, "ts")
			if err != nil {
				t.Fatalf("EnsureDatePartition: %v", err)
			}
			if fill != tc.wantFill {
				t.Errorf("fill = %v, want %v", fill, tc.wantFill)
			}
			keys, ok := partitionColumns(ctx, db, "lake", "main", tc.table)
			if !ok {
				t.Fatal("partitionColumns: not a DuckLake catalog")
			}
			if fmt.Sprint(keys) != fmt.Sprint(tc.wantKeys) {
				t.Errorf("partition keys = %v, want %v", keys, tc.wantKeys)
			}
		})
	}

	var nulls int
	if err := db.QueryRow(`SELECT count(*) FROM lake.cpu WHERE date IS NULL`).Scan(&nulls); err != nil {
		t.Fatal(err)
	}
	if nulls != 1 {
		t.Errorf("pre-existing cpu rows with NULL date = %d, want 1", nulls)
	}
}

func TestEnsureDatePartitionPlainDuckDB(t *testing.T) {
	db, err := sql.Open("duckdb", "")
	if err != nil {
		t.Skipf("duckdb unavailable: %v", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	execAll(t, db, `ATTACH ':memory:' AS lake`, `CREATE TABLE lake.cpu (ts TIMESTAMP)`)

	fill, err := EnsureDatePartition(context.Background(), db, "lake", "main", "cpu", "ts")
	if err != nil || fill {
		t.Fatalf("EnsureDatePartition = %v, %v; want false, nil", fill, err)
	}
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM information_schema.columns
		WHERE table_name = 'cpu' AND column_name = 'date'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Error("date column added to a non-DuckLake table")
	}
}

func TestDatePartitionedTables(t *testing.T) {
	db := newDatePartitionLake(t)
	execAll(t, db,
		`CREATE TABLE lake.cpu (date DATE, ts TIMESTAMP, v DOUBLE)`,
		`ALTER TABLE lake.cpu SET PARTITIONED BY (date)`,
		`CREATE TABLE lake.otlp_logs (date DATE, timestamp TIMESTAMP)`,
		`ALTER TABLE lake.otlp_logs SET PARTITIONED BY (date)`,
		`CREATE TABLE lake.plain (date DATE, ts TIMESTAMP)`,
		`CREATE TABLE lake.no_time (date DATE, v BIGINT)`,
		`ALTER TABLE lake.no_time SET PARTITIONED BY (date)`,
		`CREATE TABLE lake.by_host (host VARCHAR, ts TIMESTAMP, date DATE)`,
		`ALTER TABLE lake.by_host SET PARTITIONED BY (host)`,
	)

	got, err := DatePartitionedTables(context.Background(), db, "lake")
	if err != nil {
		t.Fatalf("DatePartitionedTables: %v", err)
	}
	want := []DatePartitionedTable{{"cpu", "ts"}, {"otlp_logs", "timestamp"}}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("got %v, want %v", got, want)
	}
}
