package maintenancelock

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// Existing DuckLake metadata must remain separate from Homer scheduling.
func scheduleCatalog(t *testing.T) string {
	t.Helper()
	dsn := testCatalogDSN(t)
	conn, err := pgx.Connect(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(context.Background())
	_, err = conn.Exec(context.Background(), `CREATE TABLE public.ducklake_metadata (
		key VARCHAR NOT NULL, value VARCHAR NOT NULL, scope VARCHAR, scope_id BIGINT);
		INSERT INTO public.ducklake_metadata VALUES ('version', '1.1', NULL, NULL)`)
	if err != nil {
		t.Fatal(err)
	}
	return dsn
}

func TestPostgresMaintenanceScheduleAcrossReplicas(t *testing.T) {
	dsn := scheduleCatalog(t)
	acquire := func(dsn string) *Lease {
		t.Helper()
		lease, acquired, err := Acquire(context.Background(), dsn)
		if err != nil || !acquired {
			t.Fatalf("acquire: %v %v", acquired, err)
		}
		t.Cleanup(lease.Release)
		return lease
	}
	check := func(lease *Lease, task string, want bool) {
		t.Helper()
		if due, err := lease.Due(task, time.Hour); err != nil || due != want {
			t.Fatalf("%s due=%v want=%v err=%v", task, due, want, err)
		}
	}
	first := acquire(dsn)
	check(first, "compaction", true)
	first.Release() // Interrupted before completion: another replica may retry.
	second := acquire(dsn)
	check(second, "compaction", true)
	if err := second.Complete("compaction"); err != nil {
		t.Fatal(err)
	}
	second.Release()
	third := acquire(dsn) // A new physical session models a replica restart.
	check(third, "compaction", false)
	check(third, "other-task", true)
	// Make the interval expire using database time, without timing-sensitive sleeps.
	third.mu.Lock()
	_, err := third.conns[0].Exec(context.Background(), `UPDATE public.homer_maintenance_schedule
		SET completed_at = clock_timestamp() - interval '2 hours' WHERE task = $1`, scheduleKey("compaction"))
	third.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	check(third, "compaction", true)
	if err := third.Complete("compaction"); err != nil {
		t.Fatal(err)
	}
	check(third, "compaction", false)
	// Independent catalog databases must not suppress each other's cycles.
	independent := acquire(scheduleCatalog(t))
	check(independent, "compaction", true)
	third.Release()
	if err := third.Complete("compaction"); err == nil {
		t.Fatal("released lease updated the schedule")
	}
	if _, err := third.Due("compaction", time.Hour); err == nil {
		t.Fatal("released lease read the schedule")
	}
}

func TestPostgresMaintenanceScheduleWithoutDuckLakeMetadata(t *testing.T) {
	dsn := testCatalogDSN(t)
	lease, acquired, err := Acquire(context.Background(), dsn)
	if err != nil || !acquired {
		t.Fatalf("acquire: %v %v", acquired, err)
	}
	defer lease.Release()
	if due, err := lease.Due("compaction", time.Hour); err != nil || !due {
		t.Fatalf("new schedule: due=%v err=%v", due, err)
	}
	if err := lease.Complete("compaction"); err != nil {
		t.Fatal(err)
	}
	if due, err := lease.Due("compaction", time.Hour); err != nil || due {
		t.Fatalf("completed schedule: due=%v err=%v", due, err)
	}
}
