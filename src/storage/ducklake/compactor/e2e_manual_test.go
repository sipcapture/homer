package compactor

import (
	"context"
	"os"
	"testing"
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
	res, err := CompactTable(context.Background(), Options{
		CatalogPath:         cat,
		DataPath:            data,
		TargetFileSizeBytes: 512 << 20,
		SnapshotRetention:   0, // force reaping of superseded files
	}, table)
	if err != nil {
		t.Fatalf("CompactTable: %v", err)
	}
	t.Logf("result: %+v", res)
}
