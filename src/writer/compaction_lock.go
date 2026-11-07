package writer

import (
	"time"

	"github.com/sipcapture/homer-core/src/internal/maintenancelock"
	"github.com/sipcapture/homer-core/src/storage/ducklake"
	logger "github.com/sipcapture/homer-core/src/utils/logging"
)

// Stateless PostgreSQL polls eligibility separately from the full compaction
// interval so another replica can retry interrupted/partial work promptly.
func (c *CompactionService) compactionPollInterval() time.Duration {
	interval := time.Duration(c.config.CheckIntervalSec) * time.Second
	if interval <= 0 {
		interval = time.Hour
	}
	if c.config.Stateless && (c.config.CatalogType == ducklake.CatalogPostgres || ducklake.IsPostgresCatalogPath(c.catalogPath)) && interval > time.Minute {
		return time.Minute
	}
	return interval
}

func (c *CompactionService) compactionDue() bool {
	if !c.config.Stateless {
		return true
	}
	lease := c.maintenanceLease.Load()
	if lease == nil {
		return true
	}
	interval := time.Duration(c.config.CheckIntervalSec) * time.Second
	if interval <= 0 {
		interval = time.Hour
	}
	due, err := lease.Due("compaction", interval)
	if err != nil {
		logger.Warn("CompactionService: shared schedule unavailable; running cycle under advisory lock", "error", err)
		return true
	}
	if !due {
		logger.Info("CompactionService: catalog already compacted within interval; skipping cycle", "interval", interval.String())
	}
	return due
}

func (c *CompactionService) recordCompactionCompletion() bool {
	if !c.config.Stateless {
		return true
	}
	if c.cycleIncomplete.Load() {
		logger.Warn("CompactionService: cycle partially failed; shared completion not advanced")
		return false
	}
	if lease := c.maintenanceLease.Load(); lease != nil {
		if err := lease.Complete("compaction"); err != nil {
			logger.Warn("CompactionService: failed to record shared compaction schedule", "error", err)
			return false
		}
	}
	return true
}

func (c *CompactionService) tryMaintenanceLock() (func(), bool) {
	if !c.config.Stateless || (c.config.CatalogType != ducklake.CatalogPostgres && !ducklake.IsPostgresCatalogPath(c.catalogPath)) {
		return func() {}, true
	}
	lease, acquired, err := ducklake.AcquireMaintenance(c.context(), c.config.Stateless, c.config.CatalogType, c.catalogPath)
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

// compactionFirstDelay also bounds startup retries when another replica owns
// the lock. Stateless PostgreSQL begins its minute polling immediately.
func (c *CompactionService) compactionFirstDelay() time.Duration {
	interval := c.compactionPollInterval()
	if interval < 5*time.Minute {
		return interval
	}
	return 5 * time.Minute
}
