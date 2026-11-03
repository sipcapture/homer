//go:build !vet

package ducklake

import (
	"context"
	"testing"
)

func TestPostgresTieringMaintenanceContention(t *testing.T) {
	dsn, _ := postgresLakeFixture(t)
	owner, acquired, err := AcquireMaintenance(context.Background(), CatalogPostgres, dsn)
	if err != nil || !acquired {
		t.Fatalf("owner: %v %v", acquired, err)
	}
	defer owner.Release()
	// No DuckDB handle: contention must return before any catalog work or copy.
	tsm := &TieredStorageManager{config: TieredStorageConfig{CatalogType: CatalogPostgres}}
	src := &Volume{Name: "hot", LakeName: "hot_alias", CatalogPath: dsn}
	dst := &Volume{Name: "cold", LakeName: "cold_alias", CatalogPath: dsn}
	for name, op := range map[string]func() error{
		"move":    func() error { return tsm.MovePartition("test", "2026-11-03", src, dst) },
		"expire":  func() error { return tsm.DeletePartition("test", "2026-11-03", src) },
		"cleanup": func() error { return tsm.RunVolumeMaintenance(src, 3600) },
	} {
		t.Run(name, func(t *testing.T) {
			if err := op(); err == nil {
				t.Fatal("maintenance bypassed another process's lock")
			}
		})
	}
}
