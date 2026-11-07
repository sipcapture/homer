package writer

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/sipcapture/homer-core/src/storage/ducklake"
	"github.com/sipcapture/homer-core/src/utils/logging"
)

func TestPostgresCompactionScheduleSkipsSequentialReplica(t *testing.T) {
	logging.InitLoggerSimple("info", true, false)
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
	// Let DuckLake create its PostgreSQL catalog and real Parquet files.
	dataPath := filepath.Join(t.TempDir(), "parquet")
	db, err := sql.Open("duckdb", "")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(2)
	if _, err := db.Exec("LOAD ducklake; LOAD postgres"); err != nil {
		t.Fatal(err)
	}
	attach, err := ducklake.CatalogAttachSQL(ducklake.CatalogPostgres, dsn, "lake", dataPath, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(attach); err != nil {
		t.Fatal(err)
	}
	for _, stmt := range []string{
		"CALL lake.set_option('data_inlining_row_limit', 0)",
		"CREATE TABLE lake.measurement (date DATE, timestamp TIMESTAMP, id BIGINT)",
		"ALTER TABLE lake.measurement SET PARTITIONED BY (date)",
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 6; i++ {
		if _, err := db.Exec(fmt.Sprintf("INSERT INTO lake.measurement VALUES (CURRENT_DATE, CURRENT_TIMESTAMP, %d)", i)); err != nil {
			t.Fatal(err)
		}
	}
	newReplica := func(alias string) *CompactionService {
		c := NewCompactionService(db, alias, dataPath, dsn, CompactionConfig{
			Stateless: true, CatalogType: ducklake.CatalogPostgres, CheckIntervalSec: 1800, Engine: EngineDuckDB,
		}, nil, nil, nil)
		t.Cleanup(c.cancel)
		return c
	}
	first := newReplica("lake")
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
	t.Run("vacuum and tiering contend with writer merge", func(t *testing.T) { checkCompactionMaintenanceContention(t, first, dsn, dataPath) })
	release()
	first.runCompaction()
	if first.totalCompactions != 1 {
		t.Fatal("real compaction cycle did not complete")
	}
	files, err := first.getTableFileCount("measurement")
	if err != nil || files >= 6 {
		t.Fatalf("compaction did not merge files: files=%d err=%v", files, err)
	}
	var count, distinct int
	if err := db.QueryRow("SELECT count(*), count(DISTINCT id) FROM lake.measurement").Scan(&count, &distinct); err != nil || count != 6 || distinct != 6 {
		t.Fatalf("compaction lost or duplicated rows: %d/%d %v", count, distinct, err)
	}
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
	// Exercise a complete cycle with the schedule still unreadable. Fresh files
	// must merge even though recording completion cannot succeed.
	for i := 6; i < 12; i++ {
		if _, err := db.Exec(fmt.Sprintf("INSERT INTO lake.measurement VALUES (CURRENT_DATE, CURRENT_TIMESTAMP, %d)", i)); err != nil {
			t.Fatal(err)
		}
	}
	releaseOther()
	fallback := newReplica("lake")
	fallback.runCompaction()
	if files, err := fallback.getTableFileCount("measurement"); err != nil || files != 1 {
		t.Fatalf("schedule error stopped real compaction: files=%d err=%v", files, err)
	}
	if err := db.QueryRow("SELECT count(*), count(DISTINCT id) FROM lake.measurement").Scan(&count, &distinct); err != nil || count != 12 || distinct != 12 {
		t.Fatalf("fallback lost or duplicated rows: %d/%d %v", count, distinct, err)
	}
	_, err = probe.Exec(ctx, "ALTER TABLE public.homer_maintenance_schedule RENAME COLUMN broken TO completed_at")
	_ = probe.Close(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	releaseOther()
	// A valid shared marker must skip the second full compaction cycle.
	second.runCompaction()
	if second.totalCompactions != 0 || second.cycleRunning.Load() || second.maintenanceLease.Load() != nil {
		t.Fatal("skipped cycle ran or leaked cycle/lease state")
	}

	releaseLegacyOwner, acquired := second.tryMaintenanceLock()
	if !acquired {
		t.Fatal("cannot hold maintenance lock for legacy-mode check")
	}
	defer releaseLegacyOwner()
	legacy := newReplica("lake")
	legacy.config.Stateless = false
	probe, err = pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer probe.Close(context.Background())
	var before, after time.Time
	if err := probe.QueryRow(ctx, "SELECT completed_at FROM public.homer_maintenance_schedule WHERE task='homer.maintenance.compaction.completed_at'").Scan(&before); err != nil {
		t.Fatal(err)
	}
	// second still owns the PostgreSQL maintenance lock; legacy mode must run.
	legacy.runCompaction()
	if legacy.totalCompactions != 1 || legacy.maintenanceLease.Load() != nil {
		t.Fatal("non-stateless PostgreSQL compaction was coordinated")
	}
	if err := probe.QueryRow(ctx, "SELECT completed_at FROM public.homer_maintenance_schedule WHERE task='homer.maintenance.compaction.completed_at'").Scan(&after); err != nil {
		t.Fatal(err)
	}
	if !before.Equal(after) {
		t.Fatal("non-stateless compaction changed the shared schedule")
	}
}

func TestCompactionEligibilityPollInterval(t *testing.T) {
	for _, tc := range []struct {
		name      string
		stateless bool
		typ       ducklake.CatalogType
		path      string
		seconds   int
		want      time.Duration
	}{
		{"postgres", true, ducklake.CatalogPostgres, "", 1800, time.Minute},
		{"dsn", true, "", "postgres://localhost/lake", 1800, time.Minute},
		{"short", true, ducklake.CatalogPostgres, "", 10, 10 * time.Second},
		{"default", true, ducklake.CatalogPostgres, "", 0, time.Minute},
		{"sqlite", false, ducklake.CatalogSQLite, "catalog.sqlite", 1800, 30 * time.Minute},
		{"postgres-off", false, ducklake.CatalogPostgres, "", 1800, 30 * time.Minute},
		{"dsn-off", false, "", "postgres://localhost/lake", 1800, 30 * time.Minute},
		{"default-off", false, ducklake.CatalogPostgres, "", 0, time.Hour},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := &CompactionService{catalogPath: tc.path, config: CompactionConfig{Stateless: tc.stateless, CatalogType: tc.typ, CheckIntervalSec: tc.seconds}}
			if got := c.compactionPollInterval(); got != tc.want {
				t.Fatalf("poll interval=%v want=%v", got, tc.want)
			}
		})
	}
}

func TestCompactionFirstDelay(t *testing.T) {
	for _, tc := range []struct {
		typ       ducklake.CatalogType
		stateless bool
		seconds   int
		want      time.Duration
	}{
		{ducklake.CatalogPostgres, true, 1800, time.Minute},
		{ducklake.CatalogPostgres, true, 10, 10 * time.Second},
		{ducklake.CatalogSQLite, false, 1800, 5 * time.Minute},
		{ducklake.CatalogPostgres, false, 1800, 5 * time.Minute},
	} {
		c := &CompactionService{config: CompactionConfig{Stateless: tc.stateless, CatalogType: tc.typ, CheckIntervalSec: tc.seconds}}
		if got := c.compactionFirstDelay(); got != tc.want {
			t.Fatalf("delay=%v want=%v", got, tc.want)
		}
	}
}
