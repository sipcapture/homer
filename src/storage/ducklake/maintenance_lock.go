package ducklake

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/sipcapture/homer-core/src/config"
	"github.com/sipcapture/homer-core/src/internal/maintenancelock"
)

// AcquireMaintenance locks participating PostgreSQL catalogs only in stateless
// mode. Other modes retain their existing coordination. Local aliases are not
// part of the lock identity.
func AcquireMaintenance(ctx context.Context, stateless bool, ct CatalogType, paths ...string) (*maintenancelock.Lease, bool, error) {
	return acquireMaintenanceUntil(ctx, time.Time{}, stateless, ct, paths...)
}

func acquireMaintenanceUntil(ctx context.Context, deadline time.Time, stateless bool, ct CatalogType, paths ...string) (*maintenancelock.Lease, bool, error) {
	if !stateless {
		return maintenancelock.Uncoordinated(ctx), true, nil
	}
	dsns := make([]string, 0, len(paths))
	for _, path := range paths {
		if ct == CatalogPostgres || IsPostgresCatalogPath(path) {
			if strings.TrimSpace(path) == "" {
				return nil, false, fmt.Errorf("maintenance requires a resolved PostgreSQL catalog path")
			}
			dsns = append(dsns, config.NormalizePostgresDSN(path))
		}
	}
	return maintenancelock.AcquireUntil(ctx, deadline, dsns...)
}

// PrimaryCatalogPath resolves the primary shard before opening a manager, so
// command-line maintenance can acquire ownership before initialization writes.
func PrimaryCatalogPath(cfg Config) (string, error) {
	ct, err := ResolveCatalogType(cfg.CatalogType, cfg.CatalogPath)
	if err != nil {
		return "", err
	}
	if cfg.ShardCount <= 1 {
		return cfg.CatalogPath, nil
	}
	if ct == CatalogPostgres {
		return suffixedPostgresCatalog(cfg.CatalogPath, "0")
	}
	return shardCatalogPath(cfg.CatalogPath, 0), nil
}
