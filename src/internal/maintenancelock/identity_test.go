package maintenancelock

import (
	"context"
	"errors"
	"net"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestCatalogAddressSetDeduplication(t *testing.T) {
	lookup := func(_ context.Context, host string) ([]string, error) {
		switch host {
		case "localhost":
			return []string{"127.0.0.1", "::1"}, nil
		case "a.test":
			return []string{"192.0.2.1", "192.0.2.2"}, nil
		case "b.test":
			return []string{"192.0.2.2", "192.0.2.3"}, nil
		default:
			return nil, errors.New("DNS unavailable")
		}
	}
	for _, tc := range []struct {
		name string
		dsns []string
		want int
	}{
		{"loopback", []string{"postgres://localhost/lake", "postgres://127.0.0.1:5432/lake", "postgres://[::1]/lake"}, 1},
		{"non-first address", []string{"postgres://a.test/lake", "postgres://192.0.2.2/lake"}, 1},
		{"transitive overlap", []string{"postgres://192.0.2.1/lake", "postgres://192.0.2.3/lake", "postgres://a.test/lake", "postgres://b.test/lake"}, 1},
		{"separate database and port", []string{"postgres://a.test/lake", "postgres://192.0.2.2/other", "postgres://192.0.2.2:5433/lake"}, 3},
		{"resolver failure keeps host", []string{"postgres://unresolved.test/lake", "postgres://unresolved.test:5432/lake"}, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var configs []*pgx.ConnConfig
			for _, dsn := range tc.dsns {
				cfg, err := pgx.ParseConfig(dsn)
				if err != nil {
					t.Fatal(err)
				}
				configs = append(configs, cfg)
			}
			got := deduplicateCatalogConfigs(context.Background(), configs, lookup)
			if len(got) != tc.want {
				t.Fatalf("catalogs=%d want=%d", len(got), tc.want)
			}
		})
	}
	cfg, _ := pgx.ParseConfig("postgres://127.0.0.1/lake")
	cfg.Port = 0
	keys := catalogAddressKeys(context.Background(), cfg, lookup)
	if !keys["127.0.0.1:5432/lake"] {
		t.Fatalf("default port not normalized: %v", keys)
	}
}

func TestCatalogResolverFailureReachesConnect(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	called := false
	lookup := func(context.Context, string) ([]string, error) {
		called = true
		return nil, errors.New("DNS unavailable")
	}
	_, _, err := acquireWithLookup(ctx, lookup, "postgres://unresolved.test/lake")
	if !called || err == nil || !strings.Contains(err.Error(), "connect to PostgreSQL") {
		t.Fatalf("resolver failure preempted connection attempt: lookup=%v err=%v", called, err)
	}
}

func TestPostgresMaintenanceOverlappingDNSAddresses(t *testing.T) {
	dsn := testCatalogDSN(t)
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	makeDSN := func(host string) string {
		u := url.URL{Scheme: "postgresql", Host: net.JoinHostPort(host, strconv.Itoa(int(cfg.Port))),
			User: url.UserPassword(cfg.User, cfg.Password), Path: "/" + cfg.Database, RawQuery: "sslmode=disable"}
		return u.String()
	}
	lookup := func(_ context.Context, host string) ([]string, error) {
		if host == "zz-multiple.test" {
			return []string{"192.0.2.1", cfg.Host}, nil
		}
		return nil, errors.New("resolver unavailable")
	}
	// The fake hostname sorts after the real host. Its second address aliases
	// the actual database; taking both locks would contend with this process.
	lease, acquired, err := acquireWithLookup(context.Background(), lookup, makeDSN(cfg.Host), makeDSN("zz-multiple.test"))
	if err != nil || !acquired {
		t.Fatalf("alias caused self-contention: %v %v", acquired, err)
	}
	defer lease.Release()
	if len(lease.conns) != 1 {
		t.Fatalf("opened %d sessions for one database", len(lease.conns))
	}
}
