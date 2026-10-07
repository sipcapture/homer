// Copyright (C) 2026 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package lineprotoreceiver

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "github.com/duckdb/duckdb-go/v2"
	_ "modernc.org/sqlite"
)

// newLPDuckLake attaches a real DuckLake as "lake". Skipped when the DuckDB
// extensions are unavailable (offline CI).
func newLPDuckLake(t *testing.T) *sql.DB {
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

func lpPartitionKeys(t *testing.T, db *sql.DB, table string) string {
	t.Helper()
	rows, err := db.Query(`
		SELECT c.column_name || ':' || pc.transform
		FROM __ducklake_metadata_lake.ducklake_partition_info pi
		JOIN __ducklake_metadata_lake.ducklake_partition_column pc
		  ON pc.partition_id = pi.partition_id AND pc.table_id = pi.table_id
		JOIN __ducklake_metadata_lake.ducklake_column c
		  ON c.table_id = pi.table_id AND c.column_id = pc.column_id AND c.end_snapshot IS NULL
		JOIN __ducklake_metadata_lake.ducklake_table t
		  ON t.table_id = pi.table_id AND t.end_snapshot IS NULL
		WHERE pi.end_snapshot IS NULL AND t.table_name = ?
		ORDER BY pc.partition_key_index`, table)
	if err != nil {
		t.Fatalf("partition keys: %v", err)
	}
	defer rows.Close()
	var keys []string
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			t.Fatal(err)
		}
		keys = append(keys, k)
	}
	return strings.Join(keys, ",")
}

func lpColumn(t *testing.T, db *sql.DB, query string) string {
	t.Helper()
	rows, err := db.Query(query)
	if err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var v sql.NullString
		if err := rows.Scan(&v); err != nil {
			t.Fatal(err)
		}
		if !v.Valid {
			v.String = "NULL"
		}
		out = append(out, v.String)
	}
	return strings.Join(out, ",")
}

func TestIngester_NewTablePartitionedByDate(t *testing.T) {
	db := newLPDuckLake(t)
	ing := NewIngester(db, "lake", lpTestCfg())

	// 2026-10-06T23:59:59Z and 2026-10-07T00:00:01Z
	body := []byte("cpu,host=a usage=1 1791331199000000000\ncpu,host=b usage=2 1791331201000000000\n")
	if _, err := ing.Ingest(context.Background(), "", body, PrecisionNanoseconds); err != nil {
		t.Fatalf("ingest: %v", err)
	}

	if got := lpPartitionKeys(t, db, "cpu"); got != "date:identity" {
		t.Errorf("partition keys = %q, want date:identity", got)
	}
	if got := lpColumn(t, db, `SELECT CAST(date AS VARCHAR) FROM lake.cpu ORDER BY ts`); got != "2026-10-06,2026-10-07" {
		t.Errorf("date values = %q", got)
	}
	if got := lpColumn(t, db, `SELECT data_type FROM information_schema.columns
		WHERE table_catalog = 'lake' AND table_name = 'cpu' AND column_name = 'date'`); got != "DATE" {
		t.Errorf("date type = %q, want DATE", got)
	}
}

func TestIngester_ExistingTableGainsDatePartition(t *testing.T) {
	db := newLPDuckLake(t)
	if _, err := db.Exec(`CREATE TABLE lake.mem (host VARCHAR, ts TIMESTAMP, used BIGINT)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO lake.mem VALUES ('old', TIMESTAMP '2026-10-01 10:00:00', 1)`); err != nil {
		t.Fatal(err)
	}
	ing := NewIngester(db, "lake", lpTestCfg())
	body := []byte("mem,host=new used=2i 1791331201000000000\n")
	if _, err := ing.Ingest(context.Background(), "", body, PrecisionNanoseconds); err != nil {
		t.Fatalf("ingest: %v", err)
	}

	if got := lpPartitionKeys(t, db, "mem"); got != "date:identity" {
		t.Errorf("partition keys = %q, want date:identity", got)
	}
	if got := lpColumn(t, db, `SELECT host || ':' || coalesce(CAST(date AS VARCHAR), 'NULL') FROM lake.mem ORDER BY ts`); got != "old:NULL,new:2026-10-07" {
		t.Errorf("rows = %q", got)
	}
}

func TestIngester_DateFieldIsNotOverwritten(t *testing.T) {
	db := newLPDuckLake(t)
	ing := NewIngester(db, "lake", lpTestCfg())
	body := []byte(`cdr,host=a date="20261007",v=1i 1791331201000000000` + "\n")
	if _, err := ing.Ingest(context.Background(), "", body, PrecisionNanoseconds); err != nil {
		t.Fatalf("ingest: %v", err)
	}
	if got := lpPartitionKeys(t, db, "cdr"); got != "" {
		t.Errorf("partition keys = %q, want none: date is a user field", got)
	}
	if got := lpColumn(t, db, `SELECT date FROM lake.cdr`); got != "20261007" {
		t.Errorf("date = %q, want the field value", got)
	}
}

func TestIngester_PlainDuckDBGetsNoDateColumn(t *testing.T) {
	db, lake := newLPDB(t)
	ing := NewIngester(db, lake, lpTestCfg())
	if _, err := ing.Ingest(context.Background(), "", []byte("cpu usage=1 1791331201000000000\n"), PrecisionNanoseconds); err != nil {
		t.Fatalf("ingest: %v", err)
	}
	if got := lpColumn(t, db, `SELECT count(*) FROM information_schema.columns
		WHERE table_name = 'cpu' AND column_name = 'date'`); got != "0" {
		t.Errorf("date columns = %s, want 0", got)
	}
}
