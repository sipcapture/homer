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
}
