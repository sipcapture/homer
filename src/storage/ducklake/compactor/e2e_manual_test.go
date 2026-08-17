package compactor

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"

	_ "github.com/duckdb/duckdb-go/v2"
)

// TestE2EManual runs the native compactor against a real, isolated copy of a
// DuckLake catalog + data tree. Gated by env vars so it is skipped in CI.
//
//	HOMER_E2E_CATALOG=/path/catalog.sqlite
//	HOMER_E2E_DATA=/path/parquet
//	HOMER_E2E_TABLE=hep_proto_1_call
func TestE2EManual(t *testing.T) {
	cat := os.Getenv("HOMER_E2E_CATALOG")
	data := os.Getenv("HOMER_E2E_DATA")
	table := os.Getenv("HOMER_E2E_TABLE")
	if cat == "" || data == "" || table == "" {
		t.Skip("set HOMER_E2E_CATALOG, HOMER_E2E_DATA, HOMER_E2E_TABLE to run")
	}

	db, err := sql.Open("duckdb", "")
	if err != nil {
		t.Fatalf("open duckdb: %v", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	for _, stmt := range []string{"LOAD ducklake;", "LOAD sqlite;"} {
		if _, err := db.Exec(stmt); err != nil {
			t.Skipf("extension unavailable (%q): %v", stmt, err)
		}
	}
	if _, err := db.Exec(fmt.Sprintf(
		"ATTACH 'ducklake:sqlite:%s' AS lake (DATA_PATH '%s');", cat, data)); err != nil {
		t.Fatalf("attach: %v", err)
	}

	before := inspectLake(t, db, data, table)
	t.Logf("before: %+v", before)

	res, err := CompactTable(context.Background(), Options{
		DB:                  db,
		LakeName:            "lake",
		DataPath:            data,
		TargetFileSizeBytes: 512 << 20,
	}, table)
	if err != nil {
		t.Fatalf("CompactTable: %v", err)
	}
	t.Logf("result: %+v", res)

	after := inspectLake(t, db, data, table)
	t.Logf("after: %+v", after)

	if after.rows != before.rows {
		t.Errorf("row count changed: %d -> %d", before.rows, after.rows)
	}
	if after.checksum != before.checksum {
		t.Errorf("contents changed:\n before: %s\n after:  %s", before.checksum, after.checksum)
	}
	if after.missingFiles != 0 {
		t.Errorf("%d active catalog entries point at missing files", after.missingFiles)
	}
	if after.latestSnapshotRows != 1 {
		t.Errorf("latest snapshot matches %d rows, want 1 (catalog is corrupt)", after.latestSnapshotRows)
	}
}

// TestE2EVerifyManual inspects a real lake without touching it, so it can be run
// against a live writer to confirm a cycle left the catalog sound. Uses the same
// env vars as TestE2EManual.
func TestE2EVerifyManual(t *testing.T) {
	cat := os.Getenv("HOMER_E2E_CATALOG")
	data := os.Getenv("HOMER_E2E_DATA")
	table := os.Getenv("HOMER_E2E_TABLE")
	if cat == "" || data == "" || table == "" {
		t.Skip("set HOMER_E2E_CATALOG, HOMER_E2E_DATA, HOMER_E2E_TABLE to run")
	}

	db, err := sql.Open("duckdb", "")
	if err != nil {
		t.Fatalf("open duckdb: %v", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	for _, stmt := range []string{"LOAD ducklake;", "LOAD sqlite;"} {
		if _, err := db.Exec(stmt); err != nil {
			t.Skipf("extension unavailable (%q): %v", stmt, err)
		}
	}
	if _, err := db.Exec(fmt.Sprintf(
		"ATTACH 'ducklake:sqlite:%s' AS lake (DATA_PATH '%s', READ_ONLY);", cat, data)); err != nil {
		t.Fatalf("attach: %v", err)
	}

	got := inspectLake(t, db, data, table)
	t.Logf("lake: %+v", got)
	if got.missingFiles != 0 {
		t.Errorf("%d active catalog entries point at missing files", got.missingFiles)
	}
	if got.latestSnapshotRows != 1 {
		t.Errorf("latest snapshot matches %d rows, want 1 (catalog is corrupt)", got.latestSnapshotRows)
	}
}

// lakeState is the observable state used to decide whether a cycle was lossless.
type lakeState struct {
	rows               int64
	activeFiles        int64
	missingFiles       int
	latestSnapshotRows int64
	checksum           string
}

func inspectLake(t *testing.T, db *sql.DB, dataPath, table string) lakeState {
	t.Helper()
	var st lakeState

	if err := db.QueryRow(fmt.Sprintf(`SELECT COUNT(*) FROM lake.%s`, quoteIdent(table))).Scan(&st.rows); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	// Summing a per-row hash gives a schema-independent, order-insensitive
	// fingerprint, so it proves the merge preserved values without assuming any
	// particular column exists or that row order is stable.
	var sum sql.NullString
	if err := db.QueryRow(fmt.Sprintf(
		`SELECT SUM(hash(to_json(t)::VARCHAR)::HUGEINT)::VARCHAR FROM lake.%s t`,
		quoteIdent(table))).Scan(&sum); err != nil {
		t.Logf("checksum unavailable: %v", err)
	}
	st.checksum = sum.String

	if err := db.QueryRow(
		`SELECT COUNT(*) FROM __ducklake_metadata_lake.ducklake_snapshot
		  WHERE CAST(snapshot_id AS BIGINT) =
		        (SELECT MAX(CAST(snapshot_id AS BIGINT)) FROM __ducklake_metadata_lake.ducklake_snapshot)`).
		Scan(&st.latestSnapshotRows); err != nil {
		t.Fatalf("snapshot invariant: %v", err)
	}

	rows, err := db.Query(
		`SELECT df.path, df.path_is_relative, t.path
		   FROM __ducklake_metadata_lake.ducklake_data_file df
		   JOIN __ducklake_metadata_lake.ducklake_table t ON t.table_id = df.table_id
		  WHERE df.end_snapshot IS NULL AND t.end_snapshot IS NULL AND t.table_name = ?`, table)
	if err != nil {
		t.Fatalf("list active files: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var p, tablePath string
		var isRel int64
		if err := rows.Scan(&p, &isRel, &tablePath); err != nil {
			t.Fatal(err)
		}
		st.activeFiles++
		if _, err := os.Stat(tableFileAbs(dataPath, tablePath, p, isRel)); err != nil {
			st.missingFiles++
			t.Logf("missing file: %s", p)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return st
}
