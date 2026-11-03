package writer

import (
	"github.com/sipcapture/homer-core/src/internal/maintenancelock"
	"github.com/sipcapture/homer-core/src/storage/ducklake"
	logger "github.com/sipcapture/homer-core/src/utils/logging"
)

func (c *CompactionService) tryMaintenanceLock() (func(), bool) {
	if c.config.CatalogType != ducklake.CatalogPostgres && !ducklake.IsPostgresCatalogPath(c.catalogPath) {
		return func() {}, true
	}
	lease, acquired, err := ducklake.AcquireMaintenance(c.context(), c.config.CatalogType, c.catalogPath)
	if err != nil {
		logger.Warn("CompactionService: PostgreSQL maintenance lock unavailable; skipping cycle", "error", err)
		return nil, false
	}
	if !acquired {
		logger.Info("CompactionService: another replica owns maintenance lock; skipping cycle")
		return nil, false
	}

	c.maintenanceLease.Store(lease)
	return func() {
		c.maintenanceLease.Store(nil)
		lease.Release()
	}, true
}

func (c *CompactionService) runInlineFlushCycle() {
	if !c.beginCycle() {
		return
	}
	defer c.endCycle()
	release, acquired := c.tryMaintenanceLock()
	if !acquired {
		return
	}
	defer release()
	c.flushInlinedData()
}

func (c *CompactionService) maintenanceSQL() maintenancelock.DB {
	return maintenancelock.DB{Raw: c.database(), Lease: c.maintenanceLease.Load()}
}
