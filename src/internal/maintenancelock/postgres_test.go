package maintenancelock

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

func testCatalogDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("HOMER_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set HOMER_TEST_POSTGRES_DSN for PostgreSQL integration")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	// Go runs packages concurrently. Give this test its own database so the
	// writer integration test cannot contend for this test's lock.
	admin, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	name := fmt.Sprintf("homer_maintenance_lock_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		_ = admin.Close(ctx)
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, _ = admin.Exec(ctx, "DROP DATABASE "+name+" WITH (FORCE)")
		_ = admin.Close(ctx)
	})
	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		u, err := url.Parse(dsn)
		if err != nil {
			t.Fatal("invalid PostgreSQL test URL")
		}
		u.Path = "/" + name
		query := u.Query()
		query.Del("dbname")
		u.RawQuery = query.Encode()
		dsn = u.String()
	} else {
		dsn += " dbname=" + name
	}
	return dsn
}

func TestPostgresMaintenanceLockAcrossReplicas(t *testing.T) {
	dsn := testCatalogDSN(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	release, acquired, err := Try(ctx, dsn)
	if err != nil || !acquired {
		t.Fatalf("first owner: acquired=%v err=%v", acquired, err)
	}
	defer release()
	otherRelease, acquired, err := Try(ctx, dsn)
	if otherRelease != nil {
		defer otherRelease()
	}
	if err != nil || acquired {
		t.Fatalf("another replica bypassed lock: acquired=%v err=%v", acquired, err)
	}
	// Ordinary catalog clients and coordinator bootstrap remain independent.
	client, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close(context.Background())
	var bootstrap bool
	if err := client.QueryRow(ctx, "SELECT pg_try_advisory_lock(724193507)").Scan(&bootstrap); err != nil || !bootstrap {
		t.Fatalf("bootstrap lock interfered: acquired=%v err=%v", bootstrap, err)
	}
	release() // Closing a physical session releases ownership without an unlock.
	nextRelease, acquired, err := Try(ctx, dsn)
	if err != nil || !acquired {
		t.Fatalf("next owner: acquired=%v err=%v", acquired, err)
	}
	nextRelease()
}

func TestPostgresMaintenanceLockConnectionFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	release, acquired, err := Try(ctx, "postgres://user:private-password@127.0.0.1:1/catalog")
	if release != nil || acquired || err == nil {
		t.Fatalf("failed connection allowed maintenance: acquired=%v err=%v", acquired, err)
	}
	if strings.Contains(err.Error(), "private-password") {
		t.Fatal("error exposed credentials")
	}
}

func TestPostgresMaintenanceLockLossCancelsSQL(t *testing.T) {
	dsn := testCatalogDSN(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	lease, acquired, err := Acquire(ctx, dsn)
	if err != nil || !acquired {
		t.Fatalf("acquire: %v %v", acquired, err)
	}
	defer lease.Release()
	lease.mu.Lock()
	pid := lease.conns[0].PgConn().PID()
	lease.mu.Unlock()
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	db := stdlib.OpenDB(*cfg)
	defer db.Close()
	guarded := DB{Raw: db, Lease: lease}
	admin, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close(context.Background())
	done := make(chan error, 1)
	go func() { _, err := guarded.Exec("SELECT pg_sleep(30)"); done <- err }()
	// Confirm SQL is actually running before killing the owning lock session.
	for {
		var running bool
		if err := admin.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND query='SELECT pg_sleep(30)' AND state='active')").Scan(&running); err != nil {
			t.Fatal(err)
		}
		if running {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("maintenance SQL never started")
		case <-time.After(10 * time.Millisecond):
		}
	}
	if _, err := admin.Exec(ctx, "SELECT pg_terminate_backend($1)", pid); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("in-flight SQL completed after losing lock")
		}
	case <-ctx.Done():
		t.Fatal("in-flight SQL was not canceled")
	}
	if _, err := guarded.Exec("CREATE TABLE forbidden_after_lock_loss(id int)"); err == nil {
		t.Fatal("follow-up SQL ran without ownership")
	}
	var exists bool
	if err := admin.QueryRow(ctx, "SELECT to_regclass('public.forbidden_after_lock_loss') IS NOT NULL").Scan(&exists); err != nil || exists {
		t.Fatalf("follow-up mutation occurred: exists=%v err=%v", exists, err)
	}
	next, acquired, err := Acquire(ctx, dsn)
	if err != nil || !acquired {
		t.Fatalf("takeover: %v %v", acquired, err)
	}
	next.Release()
}

func TestPostgresMaintenanceMultiCatalogRollback(t *testing.T) {
	a, b := testCatalogDSN(t), testCatalogDSN(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	// Hold the last catalog in acquisition order so an earlier lock must roll back.
	paths := []string{a, b}
	sort.Strings(paths)
	owner, acquired, err := Acquire(ctx, paths[1])
	if err != nil || !acquired {
		t.Fatalf("owner: %v %v", acquired, err)
	}
	defer owner.Release()
	group, acquired, err := Acquire(ctx, paths[1], paths[0])
	if group != nil {
		group.Release()
	}
	if err != nil || acquired {
		t.Fatalf("contending group: %v %v", acquired, err)
	}
	first, acquired, err := Acquire(ctx, paths[0])
	if err != nil || !acquired {
		t.Fatalf("partial lock leaked: %v %v", acquired, err)
	}
	first.Release()
	owner.Release()
	group, acquired, err = Acquire(ctx, paths[1], paths[0], paths[0])
	if err != nil || !acquired {
		t.Fatalf("group with duplicate catalog: %v %v", acquired, err)
	}
	group.Release()
}
