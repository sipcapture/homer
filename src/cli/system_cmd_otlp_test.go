package cli

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	_ "github.com/duckdb/duckdb-go/v2"
	_ "modernc.org/sqlite"
)

func newCLIDiscoveryFixture(t *testing.T) *sql.DB {
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
		`CREATE TABLE lake.hep_proto_1_call (date DATE, timestamp TIMESTAMP)`,
		`CREATE TABLE lake.otlp_logs (date DATE, timestamp TIMESTAMP, body VARCHAR)`,
		`CREATE TABLE lake.otlp_traces (date DATE, timestamp TIMESTAMP, name VARCHAR)`,
		`CREATE TABLE lake.cpu (date DATE, timestamp TIMESTAMP, usage DOUBLE)`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	return db
}

func TestDiscoverDuckLakeTablesIncludesOTLP(t *testing.T) {
	db := newCLIDiscoveryFixture(t)

	tables, err := discoverDuckLakeTables(db, "lake")
	if err != nil {
		t.Fatalf("discoverDuckLakeTables: %v", err)
	}

	saw := map[string]bool{}
	for _, tbl := range tables {
		saw[tableNameFromFQN(tbl)] = true
	}
	for _, want := range []string{"hep_proto_1_call", "otlp_logs", "otlp_traces"} {
		if !saw[want] {
			t.Errorf("%s missing from CLI discovery set: %v", want, tables)
		}
	}
	if saw["cpu"] {
		t.Errorf("Line Protocol table cpu should not be in CLI discovery set: %v", tables)
	}
}
