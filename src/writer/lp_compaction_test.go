package writer

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "github.com/duckdb/duckdb-go/v2"
	_ "modernc.org/sqlite"
)

// newLPCompactionFixture builds a real DuckLake holding a Line Protocol table
// shaped the way lineprotoreceiver creates it (measurement name, `ts`, daily
// `date` partition) with `days` rows, one per day ending today, each in its
// own file.
func newLPCompactionFixture(t *testing.T, cfg CompactionConfig, days int) (*CompactionService, *sql.DB) {
	t.Helper()
	dir := t.TempDir()
	data := filepath.Join(dir, "data")
	if err := os.MkdirAll(data, 0o755); err != nil {
		t.Fatal(err)
	}
	catalog := filepath.Join(dir, "catalog.sqlite")
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
	if _, err := db.Exec(fmt.Sprintf(
		"ATTACH 'ducklake:sqlite:%s' AS lake (DATA_PATH '%s');", catalog, data)); err != nil {
		t.Skipf("ATTACH failed: %v", err)
	}
	for _, stmt := range []string{
		`CALL lake.set_option('data_inlining_row_limit', 0)`,
		`CREATE TABLE lake.cpu (date DATE, host VARCHAR, ts TIMESTAMP, usage DOUBLE)`,
		`ALTER TABLE lake.cpu SET PARTITIONED BY (date)`,
		`ALTER TABLE lake.cpu SET SORTED BY (ts ASC)`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	for d := 0; d < days; d++ {
		ts := time.Now().AddDate(0, 0, -d).UTC().Format("2006-01-02 15:04:05")
		stmt := fmt.Sprintf(`INSERT INTO lake.cpu VALUES (DATE '%s', 'h', TIMESTAMP '%s', %d)`, ts[:10], ts, d)
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	return NewCompactionService(db, "lake", data, catalog, cfg, nil, nil, nil), db
}

func TestDiscoverTablesIncludesLineProtocolTables(t *testing.T) {
	svc, _ := newLPCompactionFixture(t, CompactionConfig{Enable: true}, 2)
	if err := svc.discoverTables(); err != nil {
		t.Fatalf("discoverTables: %v", err)
	}
	svc.mu.Lock()
	tables := append([]string(nil), svc.tables...)
	svc.mu.Unlock()
	found := false
	for _, tbl := range tables {
		if tableNameFromFQN(tbl) == "cpu" {
			found = true
		}
	}
	if !found {
		t.Errorf("Line Protocol table cpu missing from discovery set: %v", tables)
	}
}

func TestRetentionAppliesToLineProtocolTable(t *testing.T) {
	svc, db := newLPCompactionFixture(t, CompactionConfig{Enable: true, RetentionDays: 3}, 10)
	before := rowCount(t, db, "lake.cpu")
	svc.runCompaction()
	after := rowCount(t, db, "lake.cpu")
	t.Logf("cpu: %d -> %d rows", before, after)
	if after >= before || after > 4 {
		t.Errorf("retention_days=3 left %d of %d cpu rows, want <= 4", after, before)
	}
}

// TestNativeEngineMergesUnpartitionedTable: the native compactor cannot swap
// a table without an identity partition, so such a table must still reach the
// DuckDB merge instead of growing one file per flush.
func TestNativeEngineMergesUnpartitionedTable(t *testing.T) {
	svc, db := newLPCompactionFixture(t, CompactionConfig{Enable: true, Engine: EngineNativeGo}, 1)
	for _, stmt := range []string{
		`CREATE TABLE lake.otlp_old (timestamp TIMESTAMP, body VARCHAR)`,
		`INSERT INTO lake.otlp_old VALUES (now(), 'a')`,
		`INSERT INTO lake.otlp_old VALUES (now(), 'b')`,
		`INSERT INTO lake.otlp_old VALUES (now(), 'c')`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	if !svc.useNativeEngine() {
		t.Skip("native engine unavailable in this DuckLake build")
	}
	svc.backupCatalog = func(context.Context, string) (string, error) { return "test", nil }
	before, err := svc.getTableFileCount("otlp_old")
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.runMerge([]string{"lake.main.otlp_old"}); err != nil {
		t.Fatalf("runMerge: %v", err)
	}
	after, err := svc.getTableFileCount("otlp_old")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("otlp_old files: %d -> %d", before, after)
	if before < 3 || after >= before {
		t.Errorf("unpartitioned table not merged under the native engine: %d -> %d files", before, after)
	}
	if n := rowCount(t, db, "lake.otlp_old"); n != 3 {
		t.Errorf("otlp_old rows = %d, want 3", n)
	}
}
