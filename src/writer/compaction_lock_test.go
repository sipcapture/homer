package writer

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/sipcapture/homer-core/src/config"
	"github.com/sipcapture/homer-core/src/internal/maintenancelock"
)

func TestPostgresCompetingMaintenanceSkipsCatalogWork(t *testing.T) {
	dsn := os.Getenv("HOMER_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set HOMER_TEST_POSTGRES_DSN for PostgreSQL integration")
	}
	c := NewCompactionService(nil, "different_alias", "", dsn, CompactionConfig{Stateless: true}, nil, nil, nil)
	defer c.cancel()
	release, acquired, err := maintenancelock.Try(c.context(), config.NormalizePostgresDSN(dsn))
	if err != nil || !acquired {
		t.Fatalf("owner: acquired=%v err=%v", acquired, err)
	}
	defer release()
	// Nil DuckDB is deliberate: competing entry points must skip all catalog
	// operations, including recovery and inline flush, before touching it.
	c.runCompaction()
	c.runInlineFlushCycle()
	if c.totalCompactions != 0 || c.cycleRunning.Load() {
		t.Fatal("competing maintenance ran or leaked the local cycle flag")
	}
}

func TestLocalMaintenanceLock(t *testing.T) {
	c := &CompactionService{catalogPath: "/tmp/catalog.sqlite"}
	release, acquired := c.tryMaintenanceLock()
	if !acquired || release == nil {
		t.Fatal("local catalog unexpectedly requires PostgreSQL")
	}
	release()
}

func TestPostgresMaintenanceLossInterruptsDuckDB(t *testing.T) {
	dsn := os.Getenv("HOMER_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set HOMER_TEST_POSTGRES_DSN for PostgreSQL integration")
	}
	db, err := sql.Open("duckdb", "")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	c := NewCompactionService(db, "lake", "", dsn, CompactionConfig{Stateless: true}, nil, nil, nil)
	defer c.cancel()
	release, acquired := c.tryMaintenanceLock()
	if !acquired {
		t.Fatal("cannot acquire maintenance lock")
	}
	defer release()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, config.NormalizePostgresDSN(dsn))
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close(context.Background())
	var pid uint32
	if err := admin.QueryRow(ctx, `SELECT pid FROM pg_locks WHERE locktype='advisory' AND objid=$1::oid AND classid=0 AND granted AND database=(SELECT oid FROM pg_database WHERE datname=current_database())`, maintenancelock.Key).Scan(&pid); err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		close(started)
		_, err := c.execWithRetry("SELECT sum(i::DOUBLE) FROM range(1000000000000) t(i)")
		done <- err
	}()
	<-started
	select {
	case err := <-done:
		t.Fatalf("long DuckDB query exited before termination: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	if _, err := admin.Exec(ctx, "SELECT pg_terminate_backend($1)", pid); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("DuckDB query completed without cancellation")
		}
	case <-ctx.Done():
		t.Fatal("lock loss did not interrupt DuckDB")
	}
	if _, err := c.execWithRetry("CREATE TABLE forbidden_after_lock_loss(id INTEGER)"); err == nil {
		t.Fatal("follow-up write bypassed lost ownership")
	}
	// Lease cancellation must not close the shared DuckDB or stop the service.
	var one int
	if err := db.QueryRow("SELECT 1").Scan(&one); err != nil || one != 1 {
		t.Fatalf("shared DB unusable: %v", err)
	}
	release()
	next, acquired := c.tryMaintenanceLock()
	if !acquired {
		t.Fatal("next cycle cannot acquire ownership")
	}
	defer next()
	if _, err := c.execWithRetry("CREATE TABLE permitted_after_takeover(id INTEGER)"); err != nil {
		t.Fatalf("next cycle failed: %v", err)
	}
}

func TestPostgresMaintenanceRequiresStateless(t *testing.T) {
	c := &CompactionService{catalogPath: "not a valid PostgreSQL DSN", config: CompactionConfig{CatalogType: "postgres"}}
	release, acquired := c.tryMaintenanceLock()
	if !acquired || release == nil || c.maintenanceLease.Load() != nil {
		t.Fatal("non-stateless compaction attempted a PostgreSQL lease")
	}
	release()
	if !c.compactionDue() || !c.recordCompactionCompletion() {
		t.Fatal("non-stateless compaction used shared scheduling")
	}
	c.config.Stateless = true
	if _, acquired := c.tryMaintenanceLock(); acquired {
		t.Fatal("stateless compaction bypassed PostgreSQL acquisition")
	}
}
