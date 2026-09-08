package writer

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "github.com/duckdb/duckdb-go/v2"
	_ "modernc.org/sqlite"
)

// newOTLPRetentionFixture builds a real DuckLake holding one HEP table and one
// OTLP table, both with the retention-relevant shape the writers give them:
// a date partition column and a `timestamp` column. Each table gets `days`
// rows, one per day, ending today.
//
// The OTLP shape mirrors storage/ducklake/otlp_storage.go (otlpLogsTableSQL +
// SET PARTITIONED BY (date) / SET SORTED BY (timestamp ASC)).
func newOTLPRetentionFixture(t *testing.T, cfg CompactionConfig, days int) (*CompactionService, *sql.DB) {
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
		`CREATE TABLE lake.hep_proto_1_call (date DATE, timestamp TIMESTAMP, id BIGINT)`,
		`CREATE TABLE lake.otlp_logs (date DATE, timestamp TIMESTAMP, body VARCHAR)`,
		`ALTER TABLE lake.hep_proto_1_call SET PARTITIONED BY (date)`,
		`ALTER TABLE lake.otlp_logs SET PARTITIONED BY (date)`,
		`ALTER TABLE lake.otlp_logs SET SORTED BY (timestamp ASC)`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}

	for d := 0; d < days; d++ {
		ts := time.Now().AddDate(0, 0, -d).UTC().Format("2006-01-02 15:04:05")
		for _, stmt := range []string{
			fmt.Sprintf(`INSERT INTO lake.hep_proto_1_call VALUES (DATE '%s', TIMESTAMP '%s', %d)`, ts[:10], ts, d),
			fmt.Sprintf(`INSERT INTO lake.otlp_logs VALUES (DATE '%s', TIMESTAMP '%s', 'kamailio line %d')`, ts[:10], ts, d),
		} {
			if _, err := db.Exec(stmt); err != nil {
				t.Fatalf("%s: %v", stmt, err)
			}
		}
	}

	svc := NewCompactionService(db, "lake", data, catalog, cfg, nil, nil, nil)
	return svc, db
}

func rowCount(t *testing.T, db *sql.DB, table string) int {
	t.Helper()
	var n int
	if err := db.QueryRow("SELECT count(*) FROM " + table).Scan(&n); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return n
}

// TestDiscoverTablesIncludesOTLPTables asserts the discovery set contains the
// OTLP signal tables, so both the retention and the merge phase can reach them.
//
// Before the predicate was widened this failed: discovery was scoped to
// hep_proto_%, so OTLP tables in the same catalog were invisible to both.
func TestDiscoverTablesIncludesOTLPTables(t *testing.T) {
	svc, _ := newOTLPRetentionFixture(t, CompactionConfig{Enable: true}, 3)

	if err := svc.discoverTables(); err != nil {
		t.Fatalf("discoverTables: %v", err)
	}

	svc.mu.Lock()
	tables := append([]string(nil), svc.tables...)
	svc.mu.Unlock()

	t.Logf("discovered: %v", tables)
	var sawHEP, sawOTLP bool
	for _, tbl := range tables {
		switch tableNameFromFQN(tbl) {
		case "hep_proto_1_call":
			sawHEP = true
		case "otlp_logs":
			sawOTLP = true
		}
	}
	if !sawHEP {
		t.Errorf("hep_proto_1_call missing from discovery set: %v", tables)
	}
	if !sawOTLP {
		t.Errorf("otlp_logs missing from discovery set: %v", tables)
	}
}

// TestRetentionOverrideAppliesToOTLPTable is the regression test for the bug.
//
// The config is exactly what docs/RETENTION.md describes for per-table TTL, and
// what docs/OTLP.md tells operators to use for capacity planning on otlp_*:
// keep HEP 7 days, keep OTLP logs 1 day.
//
// Today this FAILS: the override resolves, retentionEnabled() returns true and
// the cycle reports success, but otlp_logs is never iterated, so nothing is
// deleted from it.
func TestRetentionOverrideAppliesToOTLPTable(t *testing.T) {
	cfg := CompactionConfig{
		Enable:               true,
		RetentionDays:        7,
		RetentionDaysByTable: map[string]int{"otlp_logs": 1},
	}
	svc, db := newOTLPRetentionFixture(t, cfg, 30)

	// Sanity: the override does resolve, and it does arm the retention phase.
	if got := svc.retentionValueForTable("lake.main.otlp_logs"); got != 1 {
		t.Fatalf("retentionValueForTable(otlp_logs) = %d, want 1", got)
	}
	if !svc.retentionEnabled() {
		t.Fatal("retentionEnabled() = false, want true")
	}

	beforeHEP := rowCount(t, db, "lake.hep_proto_1_call")
	beforeOTLP := rowCount(t, db, "lake.otlp_logs")

	svc.runCompaction()

	afterHEP := rowCount(t, db, "lake.hep_proto_1_call")
	afterOTLP := rowCount(t, db, "lake.otlp_logs")

	t.Logf("hep_proto_1_call: %d -> %d rows", beforeHEP, afterHEP)
	t.Logf("otlp_logs:        %d -> %d rows", beforeOTLP, afterOTLP)

	// The HEP table proves the cycle really ran and retention really works.
	if afterHEP >= beforeHEP {
		t.Fatalf("HEP retention did not run: %d -> %d rows", beforeHEP, afterHEP)
	}

	// The OTLP table is the bug.
	if afterOTLP >= beforeOTLP {
		t.Errorf("retention_days_by_table{\"otlp_logs\": 1} was a silent no-op: "+
			"otlp_logs still has %d of %d rows after a cycle in which the override "+
			"resolved to 1 day and retention was enabled", afterOTLP, beforeOTLP)
	}
	if afterOTLP > 2 {
		t.Errorf("otlp_logs has %d rows, want <= 2 for a 1-day TTL over 30 days of data", afterOTLP)
	}
}
