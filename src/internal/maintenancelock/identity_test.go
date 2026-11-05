package maintenancelock

import (
	"context"
	"github.com/jackc/pgx/v5"
	"testing"
)

func TestCatalogIdentityAliases(t *testing.T) {
	var want string
	for _, dsn := range []string{"postgres://localhost/lake", "postgres://127.0.0.1:5432/lake", "postgres://[::1]/lake"} {
		cfg, err := pgx.ParseConfig(dsn)
		if err != nil {
			t.Fatal(err)
		}
		got, err := catalogIdentity(context.Background(), cfg)
		if err != nil {
			t.Fatal(err)
		}
		if want == "" {
			want = got
		}
		if got != want {
			t.Fatalf("%s: %s != %s", dsn, got, want)
		}
	}
	cfg, _ := pgx.ParseConfig("postgres://127.0.0.1/lake")
	cfg.Port = 0
	got, err := catalogIdentity(context.Background(), cfg)
	if err != nil || got != want {
		t.Fatalf("default port: %s %v", got, err)
	}
}
