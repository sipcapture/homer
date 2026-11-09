//go:build !vet

package ducklake

import (
	"context"
	"database/sql"
	"github.com/sipcapture/homer-core/src/internal/maintenancelock"
	"testing"
	"time"
)

func TestPostgresTieringMaintenanceContention(t *testing.T) {
	dsn, _ := postgresLakeFixture(t)
	owner, acquired, err := AcquireMaintenance(context.Background(), true, CatalogPostgres, dsn)
	if err != nil || !acquired {
		t.Fatalf("owner: %v %v", acquired, err)
	}
	defer owner.Release()
	// No DuckDB handle: contention must return before any catalog work or copy.
	tsm := &TieredStorageManager{config: TieredStorageConfig{Stateless: true, CatalogType: CatalogPostgres}}
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

func TestPostgresMaintenanceOptIn(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	lease, acquired, err := AcquireMaintenance(ctx, false, CatalogPostgres, "not a PostgreSQL DSN")
	if err != nil || !acquired || lease == nil {
		t.Fatalf("disabled coordination attempted a connection: %v", err)
	}
	defer lease.Release()
	cancel()
	if lease.Context().Err() == nil || lease.Check() == nil {
		t.Fatal("disabled coordination lost caller cancellation")
	}
	manager := &TieredStorageManager{config: TieredStorageConfig{CatalogType: CatalogPostgres}}
	unmanaged, err := manager.acquireMaintenance("not a PostgreSQL DSN")
	if err != nil {
		t.Fatalf("non-stateless tiering tried to connect: %v", err)
	}
	unmanaged.Release()
	manager.config.Stateless = true
	if owned, err := manager.acquireMaintenance("not a PostgreSQL DSN"); err == nil {
		owned.Release()
		t.Fatal("stateless tiering bypassed coordination")
	}
}

func TestPostgresTieringWaitsForMaintenance(t *testing.T) {
	dsn, _ := postgresLakeFixture(t)
	owner, ok, err := AcquireMaintenance(context.Background(), true, CatalogPostgres, dsn)
	if err != nil || !ok {
		t.Fatal("owner", err)
	}
	defer owner.Release()
	manager := &TieredStorageManager{config: TieredStorageConfig{Stateless: true, CatalogType: CatalogPostgres}}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if lease, err := manager.acquireMaintenanceWithin(ctx, dsn); err == nil {
		lease.Release()
		t.Fatal("wait exceeded contention bound")
	}
	done := make(chan *maintenancelock.Lease, 1)
	errors := make(chan error, 1)
	go func() {
		lease, err := manager.acquireMaintenance(dsn)
		if err != nil {
			errors <- err
			return
		}
		done <- lease
	}()
	select {
	case lease := <-done:
		lease.Release()
		t.Fatal("waiter bypassed owner")
	case err := <-errors:
		t.Fatal(err)
	case <-time.After(150 * time.Millisecond):
	}
	owner.Release()
	select {
	case lease := <-done:
		defer lease.Release()
		if err := lease.Check(); err != nil {
			t.Fatal("lease canceled after acquisition", err)
		}
	case err := <-errors:
		t.Fatal(err)
	case <-time.After(3 * time.Second):
		t.Fatal("waiter did not acquire released lock")
	}
}

func TestPostgresTieringDeleteRetriesInSameCycle(t *testing.T) {
	dsn, path := postgresLakeFixture(t)
	db, err := sql.Open("duckdb", "")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := loadLakeExtensions(db, CatalogPostgres); err != nil {
		t.Fatal(err)
	}
	attach, err := CatalogAttachSQL(CatalogPostgres, dsn, "lake", path, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{attach, "CREATE TABLE lake.test (date DATE, id BIGINT)", "INSERT INTO lake.test VALUES ('2026-11-03',1)"} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	owner, ok, err := AcquireMaintenance(context.Background(), true, CatalogPostgres, dsn)
	if err != nil || !ok {
		t.Fatal("owner", err)
	}
	defer owner.Release()
	manager := &TieredStorageManager{db: db, config: TieredStorageConfig{Stateless: true, CatalogType: CatalogPostgres}}
	vol := &Volume{LakeName: "lake", CatalogPath: dsn}
	done := make(chan error, 1)
	go func() { done <- manager.DeletePartition("test", "2026-11-03", vol) }()
	select {
	case err := <-done:
		t.Fatalf("delete returned while writer owns lease: %v", err)
	case <-time.After(150 * time.Millisecond):
	}
	owner.Release()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("delete did not finish after lease release")
	}
	var rows int
	if err := db.QueryRow("SELECT count(*) FROM lake.test").Scan(&rows); err != nil || rows != 0 {
		t.Fatalf("partition not deleted: rows=%d err=%v", rows, err)
	}
}
