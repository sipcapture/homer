package writer

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/sipcapture/homer-core/src/storage/ducklake"
)

func TestPostgresCompactionScheduleSkipsSequentialReplica(t *testing.T) {
	dsn := os.Getenv("HOMER_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set HOMER_TEST_POSTGRES_DSN for PostgreSQL integration")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close(context.Background())
	name := fmt.Sprintf("homer_compaction_schedule_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = admin.Exec(ctx, "DROP DATABASE "+name+" WITH (FORCE)") }()
	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		u, err := url.Parse(dsn)
		if err != nil {
			t.Fatal(err)
		}
		u.Path = "/" + name
		query := u.Query()
		query.Del("dbname")
		u.RawQuery = query.Encode()
		dsn = u.String()
	} else {
		dsn += " dbname=" + name
	}
	// DuckLake normally provisions this table; emulate its existing metadata.
	catalog, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	_, err = catalog.Exec(ctx, `CREATE TABLE public.ducklake_metadata (
		key VARCHAR NOT NULL, value VARCHAR NOT NULL, scope VARCHAR, scope_id BIGINT)`)
	_ = catalog.Close(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	newReplica := func(alias string) *CompactionService {
		c := NewCompactionService(nil, alias, "", dsn, CompactionConfig{
			CatalogType: ducklake.CatalogPostgres, CheckIntervalSec: 1800,
		}, nil, nil, nil)
		t.Cleanup(c.cancel)
		return c
	}
	first := newReplica("first")
	release, acquired := first.tryMaintenanceLock()
	if !acquired {
		t.Fatal("first replica cannot acquire lock")
	}
	defer release()
	if !first.compactionDue() {
		t.Fatal("first cycle should be due")
	}
	first.recordMaintenanceResult("cleanup_old_files", errors.New("object store unavailable"))
	if first.recordCompactionCompletion() {
		t.Fatal("partial failure recorded full completion")
	}
	if !first.compactionDue() {
		t.Fatal("partial cycle must remain due")
	}
	first.cycleIncomplete.Store(false) // A subsequent cycle completes all steps.
	if !first.recordCompactionCompletion() {
		t.Fatal("successful cycle did not record completion")
	}
	release()
	second := newReplica("different_alias")
	releaseOther, acquired := second.tryMaintenanceLock()
	if !acquired {
		t.Fatal("second replica cannot acquire released lock")
	}
	defer releaseOther()
	if second.compactionDue() {
		t.Fatal("sequential replica ignored shared completion")
	}
	// A schedule read error must not disable the owner of the advisory lock.
	probe, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	_, err = probe.Exec(ctx, "ALTER TABLE public.homer_maintenance_schedule RENAME COLUMN completed_at TO broken")
	if err != nil {
		t.Fatal(err)
	}
	if !second.compactionDue() {
		t.Fatal("schedule read failure disabled compaction")
	}
	_, err = probe.Exec(ctx, "ALTER TABLE public.homer_maintenance_schedule RENAME COLUMN broken TO completed_at")
	_ = probe.Close(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	releaseOther()
	// No DuckDB is available: the schedule must skip before catalog operations.
	second.runCompaction()
	if second.totalCompactions != 0 || second.cycleRunning.Load() || second.maintenanceLease.Load() != nil {
		t.Fatal("skipped cycle ran or leaked cycle/lease state")
	}
}

func TestCompactionEligibilityPollInterval(t *testing.T) {
	for _, tc := range []struct {
		name    string
		typ     ducklake.CatalogType
		path    string
		seconds int
		want    time.Duration
	}{
		{"postgres", ducklake.CatalogPostgres, "", 1800, time.Minute},
		{"dsn", "", "postgres://localhost/lake", 1800, time.Minute},
		{"short", ducklake.CatalogPostgres, "", 10, 10 * time.Second},
		{"default", ducklake.CatalogPostgres, "", 0, time.Minute},
		{"sqlite", ducklake.CatalogSQLite, "catalog.sqlite", 1800, 30 * time.Minute},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := &CompactionService{catalogPath: tc.path, config: CompactionConfig{CatalogType: tc.typ, CheckIntervalSec: tc.seconds}}
			if got := c.compactionPollInterval(); got != tc.want {
				t.Fatalf("poll interval=%v want=%v", got, tc.want)
			}
		})
	}
}
