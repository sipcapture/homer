package writer

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sipcapture/homer-core/src/config"
	"github.com/sipcapture/homer-core/src/storage/ducklake"
	"github.com/sipcapture/homer-core/src/storage/ducklake/compactor"
	logger "github.com/sipcapture/homer-core/src/utils/logging"
)

// reBrokenParquet matches DuckDB errors about unreadable parquet files.
// Two forms are handled:
//
//	IO Error:    Cannot open file "/path/to/file.parquet": No such file or directory
//	Input Error: Invalid footer length provided for file '/path/to/file.parquet'
var reBrokenParquet = regexp.MustCompile(
	`Cannot open file "([^"]+\.parquet)"|Invalid footer.*for file '([^']+\.parquet)'`)

// catalogPathToAbs converts a DuckLake catalog-relative path to an absolute filesystem path.
//
// DuckLake stores file paths in ducklake_data_file.path relative to the per-table
// root directory: {DataPath}/main/{tableName}/.  Older catalog entries (written by
// a previous DuckLake version) include the "main/{table}/" prefix themselves.
func catalogPathToAbs(dataPath, tableName, catalogPath string) string {
	if filepath.IsAbs(catalogPath) {
		return catalogPath
	}
	if dataPath == "" {
		return catalogPath
	}
	// Paths that already start with "main/" carry their own table subdirectory.
	if strings.HasPrefix(catalogPath, "main/") {
		if ducklake.IsRemoteLakeDataPath(dataPath) {
			return ducklake.JoinLakeDataPath(dataPath, catalogPath)
		}
		return filepath.Join(dataPath, catalogPath)
	}
	// Current DuckLake format: path is relative to {DataPath}/main/{tableName}/
	if ducklake.IsRemoteLakeDataPath(dataPath) {
		return ducklake.JoinLakeDataPath(dataPath, "main", tableName, catalogPath)
	}
	return filepath.Join(dataPath, "main", tableName, catalogPath)
}

// parquetMagic is the 4-byte magic number at the start and end of every valid Parquet file.
var parquetMagic = []byte("PAR1")

// isGhostOrCorrupt returns true if the given parquet file is missing, too small,
// or does not have valid Parquet magic bytes ("PAR1") at the start and end.
func isGhostOrCorrupt(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return os.IsNotExist(err)
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return true
	}
	// Minimum valid Parquet: 4 (magic) + 4 (footer len) + 4 (magic) = 12 bytes
	if info.Size() < 12 {
		return true
	}

	// Check leading magic "PAR1"
	head := make([]byte, 4)
	if _, err := f.Read(head); err != nil || string(head) != string(parquetMagic) {
		return true
	}

	// Check trailing magic "PAR1"
	tail := make([]byte, 4)
	if _, err := f.ReadAt(tail, info.Size()-4); err != nil || string(tail) != string(parquetMagic) {
		return true
	}

	return false
}

// extractBrokenParquetPath parses the absolute parquet path from a DuckDB error
// about a file that is missing or has an invalid format (corrupt footer).
func extractBrokenParquetPath(err error) string {
	if err == nil {
		return ""
	}
	m := reBrokenParquet.FindStringSubmatch(err.Error())
	if len(m) < 2 {
		return ""
	}
	// Group 1 = "Cannot open file" match, Group 2 = "Invalid footer" match.
	if m[1] != "" {
		return m[1]
	}
	if len(m) >= 3 {
		return m[2]
	}
	return ""
}

// CompactionConfig holds compaction settings
type CompactionConfig struct {
	Enable           bool `json:"enable"`
	CheckIntervalSec int  `json:"check_interval_sec"` // How often to run compaction (default: 3600 = 1 hour)
	RetentionDays    int  `json:"retention_days"`     // Delete data older than N days (0 = disabled)
	// RetentionDaysByTable overrides RetentionDays per bare table name
	// (e.g. "hep_proto_1_registration"). Override 0 disables TTL for that table.
	RetentionDaysByTable map[string]int `json:"retention_days_by_table"`
	// RetentionUnit selects how RetentionDays / RetentionDaysByTable are
	// interpreted: "days" (default) or "hours". See config.RetentionUnit*.
	RetentionUnit string `json:"retention_unit"`
	// SnapshotExpireIntervalSec controls how long to keep DuckLake snapshots (seconds).
	SnapshotExpireIntervalSec int `json:"snapshot_expire_interval_sec"`
	// MinAgeSec leaves a partition alone until nothing has been written to it for
	// this long. Enforced by the native engine only.
	MinAgeSec int `json:"min_age_sec"`
	// MinFileSizeBytes: minimum file size for merge (smaller files will be merged). 0 = no limit.
	MinFileSizeBytes int64 `json:"min_file_size_bytes"`
	// MaxFileSizeBytes: maximum size of merged file. 0 = no limit (DuckLake default).
	MaxFileSizeBytes int64 `json:"max_file_size_bytes"`
	// MaxCompactedFiles: maximum number of compaction operations (output files)
	// per table per cycle. 0 = writer default (32). This is not an input-file
	// cap; peak memory is bounded by MaxFileSizeBytes and merge threads=1.
	MaxCompactedFiles int `json:"max_compacted_files"`
	// Engine selects the compaction backend: "duckdb" (default) or "native".
	Engine string `json:"engine"`
	// TargetFileSizeBytes caps each merged file for the native engine. 0 = 512MB.
	TargetFileSizeBytes int64 `json:"target_file_size_bytes"`
}

// Compaction engine identifiers.
const (
	EngineDuckDB   = "duckdb"
	EngineNativeGo = "native"
)

// Defaults for the DuckDB merge path (sipcapture/homer#945).
//
// max_compacted_files is the number of compaction *operations* (output
// files) per CALL, not the number of input files. Each operation may
// rewrite many parquet files up to max_file_size. Memory is therefore
// bounded by max_file_size + threads=1 (one group at a time), not by
// this count. 32 is a drain-vs-lock compromise: 8 falls behind a 30s
// flush (~120 files/h) on busy call tables; 100 held the catalog lock
// long enough for ingest buffers to blow RSS. Leftovers go to the next
// cycle. Operators may raise this; we do not clamp an explicit value.
const (
	defaultDuckDBMaxCompactedFiles = 32
	defaultDuckDBMaxFileSizeBytes  = 64 << 20 // 64 MiB
)

// CatalogLocker provides Lock/Unlock for serializing catalog-modifying operations.
// Implemented by the DuckLake Manager to coordinate flush and compaction.
type CatalogLocker interface {
	CatalogLock()
	CatalogUnlock()
}

// nativeCatalogBackupsKept is how many VACUUM INTO copies of the catalog the
// native engine retains before each cycle.
const nativeCatalogBackupsKept = 3

// CompactionS3Client holds DuckDB httpfs S3 settings for maintenance calls that
// read s3:// objects (ducklake_cleanup_old_files / ducklake_delete_orphaned_files).
// Some DuckLake versions appear to evaluate read_blob without inheriting prior
// session SET s3_* on the shared *sql.DB; re-applying avoids spurious 403/404.
// Nil or empty AccessKeyID means skip (ApplyDuckDBS3ClientSettings is a no-op).
type CompactionS3Client struct {
	Region, AccessKeyID, SecretAccessKey, Endpoint string
	UseSSL                                         bool
	URLStyle                                       string
}

// CompactionService handles periodic compaction and retention
type CompactionService struct {
	db            *sql.DB
	lakeName      string
	dataPath      string // root dir for parquet files; catalog paths are relative to this
	catalogPath   string // DuckLake SQLite catalog file (used by the native engine)
	config        CompactionConfig
	tables        []string
	catalogLocker CatalogLocker // serializes catalog access with writer flush
	s3Client      *CompactionS3Client

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	mu     sync.RWMutex

	// nativeDisabled latches when a native cycle leaves the catalog failing its
	// invariants, so the process stops using the native engine until restart.
	nativeDisabled atomic.Bool

	// nativeUnsupported remembers tables DuckLake will not accept merged files
	// for, keyed by table name with the reason as value. Retrying them would
	// rewrite the same partitions every cycle only to discard the result.
	nativeUnsupported unsupportedTables

	// Stats
	lastCompactionTime time.Time
	totalCompactions   int64
	totalRowsDeleted   int64
}

// NewCompactionService creates a new compaction service.
// catalogLocker serializes catalog access with writer flush to prevent "database is locked".
// dataPath is the root directory for Parquet files; catalog paths are relative to it.
// s3Client may be nil when data_path is local or credentials are not used.
func NewCompactionService(db *sql.DB, lakeName, dataPath, catalogPath string, config CompactionConfig, catalogLocker CatalogLocker, s3Client *CompactionS3Client) *CompactionService {
	ctx, cancel := context.WithCancel(context.Background())

	return &CompactionService{
		db:            db,
		lakeName:      lakeName,
		dataPath:      dataPath,
		catalogPath:   catalogPath,
		config:        config,
		catalogLocker: catalogLocker,
		s3Client:      s3Client,
		ctx:           ctx,
		cancel:        cancel,
	}
}

// ensureS3ClientSettings reapplies DuckDB SET s3_* before procedures that list s3://.
func (c *CompactionService) ensureS3ClientSettings() {
	if c == nil || c.db == nil || c.s3Client == nil {
		return
	}
	s := c.s3Client
	if err := ducklake.ApplyDuckDBS3ClientSettings(c.db, s.Region, s.AccessKeyID, s.SecretAccessKey, s.Endpoint, s.UseSSL, s.URLStyle); err != nil {
		logger.Warn("CompactionService: ApplyDuckDBS3ClientSettings failed", "error", err)
	}
}

func (c *CompactionService) warnMaintenanceS3Failure(op string, err error) {
	logger.Warn("CompactionService: "+op+" failed", "error", err)
	if err == nil {
		return
	}
	if strings.Contains(err.Error(), "NoSuchBucket") && ducklake.IsRemoteLakeDataPath(c.dataPath) {
		logger.Warn("CompactionService: NoSuchBucket — bucket missing or wrong name for data_path; create it on storage.ducklake.s3.endpoint or fix data_path",
			"data_path", c.dataPath)
	}
}

// Start begins the compaction service
func (c *CompactionService) Start() error {
	if !c.config.Enable {
		// Compaction (merge/expire/cleanup) is off, but inlined data must
		// still be drained. Disabling data inlining only stops NEW inlining;
		// rows inlined earlier (e.g. a catalog created before inlining was
		// disabled) stay in the catalog and resident in the DuckLake
		// extension's memory until flushed. Without this, an upgraded node
		// with compaction off would never release its legacy inline backlog.
		logger.Info("CompactionService: compaction disabled; starting inline-flush-only maintenance",
			"interval", fmt.Sprintf("%ds", c.config.CheckIntervalSec))
		c.wg.Add(1)
		go c.inlineFlushOnlyLoop()
		return nil
	}

	// Discover tables
	if err := c.discoverTables(); err != nil {
		return fmt.Errorf("failed to discover tables: %w", err)
	}

	logger.Info("CompactionService starting",
		"interval", fmt.Sprintf("%ds", c.config.CheckIntervalSec),
		"min_age_sec", c.config.MinAgeSec,
		"retention_days", c.config.RetentionDays,
		"retention_days_by_table", len(c.config.RetentionDaysByTable),
		"tables", len(c.tables))

	// Always run compaction on startup after a short delay.
	// Before the first compaction, scan for ghost catalog entries — files
	// registered in DuckLake metadata but missing on disk (typical after a
	// force-kill during a flush). The catalog entries are deleted so that
	// the normal merge/expire/cleanup cycle can proceed cleanly.
	go func() {
		time.Sleep(5 * time.Second) // Wait for system to stabilize
		logger.Info("CompactionService: Running initial compaction on startup")
		c.withCatalogLock(c.recoverGhostFiles)
		c.runCompaction()
	}()

	// Start periodic compaction
	c.wg.Add(1)
	go c.compactionLoop()

	return nil
}

// Stop stops the compaction service
func (c *CompactionService) Stop() {
	c.cancel()
	c.wg.Wait()
	logger.Info("CompactionService stopped")
}

// compactionLoop runs periodic compaction every CheckIntervalSec.
// First scheduled run is after 5 min (or interval if shorter) so compaction runs
// sooner when there are many files.
func (c *CompactionService) compactionLoop() {
	defer c.wg.Done()
	defer func() {
		if r := recover(); r != nil {
			logger.Error("CompactionService: Periodic loop panic", "panic", r)
		}
	}()

	interval := time.Duration(c.config.CheckIntervalSec) * time.Second
	firstDelay := 5 * time.Minute
	if interval < firstDelay {
		firstDelay = interval
	}

	logger.Info("CompactionService: Periodic loop started",
		"interval_sec", c.config.CheckIntervalSec,
		"first_run_in", firstDelay.String())

	// First scheduled run after firstDelay (sooner than full interval)
	timer := time.NewTimer(firstDelay)
	select {
	case <-c.ctx.Done():
		timer.Stop()
		return
	case <-timer.C:
		logger.Info("CompactionService: First scheduled compaction")
		c.runCompaction()
	}

	// Then every interval
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			logger.Info("CompactionService: Interval elapsed, running scheduled compaction")
			c.runCompaction()
		}
	}
}

// withCatalogLock runs fn while holding the catalog lock.
func (c *CompactionService) withCatalogLock(fn func()) {
	if c.catalogLocker != nil {
		c.catalogLocker.CatalogLock()
		defer c.catalogLocker.CatalogUnlock()
	}
	fn()
}

// flushInlinedData drains DuckLake inlined rows into Parquet and drops the
// backing ducklake_inlined_data_* tables. Disabling data inlining only stops
// NEW inlining; rows inlined earlier stay in the catalog and resident in the
// DuckLake extension's memory until flushed. Cheap no-op once nothing is
// inlined. Shared by the full compaction cycle (step 0) and the flush-only
// loop used when compaction is disabled.
func (c *CompactionService) flushInlinedData() {
	c.withCatalogLock(func() {
		c.ensureS3ClientSettings()
		logger.Info("CompactionService: Flush inlined data", "lake", c.lakeName)
		flushSQL := fmt.Sprintf("CALL ducklake_flush_inlined_data('%s')", c.lakeName)
		if _, err := c.execWithRetry(flushSQL); err != nil {
			logger.Warn("CompactionService: flush_inlined_data failed", "error", err)
		}
	})
}

// inlineFlushOnlyLoop periodically drains inlined data when full compaction is
// disabled, so an upgraded node with compaction off still releases its legacy
// inline backlog from the DuckLake extension's memory. Paced by
// CheckIntervalSec; the flush is a no-op once nothing is inlined.
func (c *CompactionService) inlineFlushOnlyLoop() {
	defer c.wg.Done()
	defer func() {
		if r := recover(); r != nil {
			logger.Error("CompactionService: inline-flush loop panic", "panic", r)
		}
	}()

	interval := time.Duration(c.config.CheckIntervalSec) * time.Second
	if interval <= 0 {
		interval = time.Hour
	}

	// First run shortly after startup so a legacy backlog drains promptly
	// instead of waiting a full interval.
	firstDelay := 1 * time.Minute
	if interval < firstDelay {
		firstDelay = interval
	}
	timer := time.NewTimer(firstDelay)
	select {
	case <-c.ctx.Done():
		timer.Stop()
		return
	case <-timer.C:
		c.flushInlinedData()
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			c.flushInlinedData()
		}
	}
}

// runCompaction performs a single compaction cycle.
// Instead of holding the catalog lock for the entire cycle (which can take
// minutes and block flush + queries), the lock is acquired and released
// per-table for retention and merge, then per-call for expire/cleanup.
func (c *CompactionService) runCompaction() {
	start := time.Now()
	logger.Info("CompactionService: Starting compaction cycle")

	// Refresh table list (read-only, no lock needed)
	if err := c.discoverTables(); err != nil {
		logger.Error("CompactionService: Failed to discover tables", "error", err)
	}

	var totalRowsDeleted int64

	// Run retention per table when global TTL or any per-table override is set.
	// Lock per table so flush is not blocked for the whole cycle.
	if c.retentionEnabled() {
		for _, table := range c.tables {
			value := c.retentionValueForTable(table)
			if value <= 0 {
				continue
			}
			c.withCatalogLock(func() {
				deleted, err := c.runRetention(table, value)
				if err != nil {
					logger.Error("CompactionService: Retention failed", "table", table, "retention_value", value, "retention_unit", c.config.RetentionUnit, "error", err)
				} else if deleted > 0 {
					logger.Info("CompactionService: Retention completed", "table", table, "retention_value", value, "retention_unit", c.config.RetentionUnit, "rows_deleted", deleted)
					totalRowsDeleted += deleted
				}
			})
		}
	}

	// Run merge per table — lock per table
	if err := c.runMerge(c.tables); err != nil {
		logger.Error("CompactionService: Merge failed", "error", err)
	}

	c.mu.Lock()
	c.lastCompactionTime = time.Now()
	c.totalCompactions++
	c.totalRowsDeleted += totalRowsDeleted
	c.mu.Unlock()

	elapsed := time.Since(start)
	logger.Info("CompactionService: Compaction cycle completed",
		"duration", elapsed.String(),
		"tables", len(c.tables),
		"rows_deleted", totalRowsDeleted)
}

// discoverTables finds all HEP tables in the DuckLake catalog
func (c *CompactionService) discoverTables() error {
	query := `
		SELECT table_name
		FROM information_schema.tables
		WHERE table_catalog = ?
		  AND table_schema = 'main'
		  AND table_name LIKE 'hep_proto_%'
	`

	rows, err := c.queryWithRetry(query, c.lakeName)
	if err != nil {
		return err
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var tableName string
		if err := rows.Scan(&tableName); err != nil {
			continue
		}
		tables = append(tables, fmt.Sprintf("%s.main.%s", c.lakeName, tableName))
	}
	if err := rows.Err(); err != nil {
		return err
	}

	c.mu.Lock()
	c.tables = tables
	c.mu.Unlock()

	return nil
}

// retentionEnabled reports whether any retention TTL is configured (global or
// a positive per-table override).
func (c *CompactionService) retentionEnabled() bool {
	if c.config.RetentionDays > 0 {
		return true
	}
	for _, days := range c.config.RetentionDaysByTable {
		if days > 0 {
			return true
		}
	}
	return false
}

// retentionValueForTable returns the raw TTL value for a catalog table, in
// whatever unit c.config.RetentionUnit specifies. Bare table names in
// RetentionDaysByTable win over the global RetentionDays. An explicit
// override of 0 disables TTL for that table.
func (c *CompactionService) retentionValueForTable(table string) int {
	name := tableNameFromFQN(table)
	if name == "" {
		name = table
	}
	if days, ok := c.config.RetentionDaysByTable[name]; ok {
		return days
	}
	return c.config.RetentionDays
}

// runRetention deletes data older than retentionValue (interpreted per
// c.config.RetentionUnit) for one table. Missing parquet files are silently
// skipped — they will be cleaned up by ducklake_delete_orphaned_files in the
// maintenance phase.
func (c *CompactionService) runRetention(table string, retentionValue int) (int64, error) {
	if retentionValue <= 0 {
		return 0, nil
	}
	cutoff := config.RetentionCutoff(time.Now(), retentionValue, c.config.RetentionUnit)
	cutoffStr := cutoff.UTC().Format("2006-01-02 15:04:05")

	query := fmt.Sprintf(`DELETE FROM %s WHERE timestamp < TIMESTAMP '%s'`,
		table, cutoffStr)

	result, err := c.execWithRetry(query)
	if err != nil {
		if strings.Contains(err.Error(), "No such file or directory") {
			logger.Warn("CompactionService: Retention skipped (missing parquet files, will be cleaned up)", "table", table)
			return 0, nil
		}
		return 0, err
	}

	rowsAffected, _ := result.RowsAffected()
	return rowsAffected, nil
}

// runMerge runs DuckLake maintenance functions (merge, expire, cleanup).
// IMPORTANT: Merge must happen BEFORE expire, because merge cannot process files
// associated with expired snapshots. Order: merge -> expire -> cleanup.
// The catalog lock is acquired per-table and per-maintenance-call to avoid
// blocking flush and queries for the entire duration of the cycle.
func (c *CompactionService) runMerge(tables []string) error {
	if len(tables) == 0 {
		logger.Info("CompactionService: No tables to compact", "lake", c.lakeName)
		return nil
	}

	// Log snapshot count before merge
	snapshotCount, err := c.getSnapshotCount()
	if err == nil {
		logger.Info("CompactionService: Active snapshots before merge", "count", snapshotCount)
	}

	// 0. Flush inlined data to Parquet FIRST, so the subsequent merge/expire
	// act on freshly written Parquet rather than catalog-resident rows. See
	// flushInlinedData for why this matters even when inlining is disabled.
	c.flushInlinedData()

	// Native engine: replace the DuckDB merge with a memory-bounded Go compactor
	// (see storage/ducklake/compactor). It needs local storage and a DuckLake
	// build that can register pre-merged files; otherwise we transparently fall
	// back to the DuckDB path so compaction still runs.
	if c.useNativeEngine() {
		return c.runNativeMerge(tables)
	}
	if c.config.Engine == EngineNativeGo {
		logger.Info("CompactionService: native engine unavailable, using duckdb",
			"lake", c.lakeName,
			"remote_data_path", ducklake.IsRemoteLakeDataPath(c.dataPath),
			"disabled_after_failure", c.nativeDisabled.Load())
	}

	maxFiles := c.effectiveMaxCompactedFiles()
	maxSize := c.effectiveMaxFileSizeBytes()

	// 1. Merge adjacent small files FIRST — lock per table.
	// The CALL runs on a dedicated pool connection with threads=1 so
	// date-partitions are compacted serially instead of in parallel
	// (each group fully decompresses + re-sorts SIP payloads).
	for _, table := range tables {
		tableName := tableNameFromFQN(table)
		if tableName == "" {
			logger.Warn("CompactionService: Skipping table with invalid name", "table", table)
			continue
		}

		c.withCatalogLock(func() {
			defer func() {
				if r := recover(); r != nil {
					logger.Error("CompactionService: merge panic", "table", tableName, "panic", r)
				}
			}()

			fileCount, err := c.getTableFileCount(tableName)
			if err == nil {
				logger.Info("CompactionService: Files before merge", "table", tableName, "count", fileCount)
				if fileCount < 2 {
					return
				}
			}

			logger.Info("CompactionService: Merge adjacent files",
				"lake", c.lakeName,
				"table", tableName,
				"max_compacted_files", maxFiles,
				"max_file_size_bytes", maxSize,
				"threads", 1)
			processed, created, err := c.mergeAdjacentFiles(tableName)
			if err != nil {
				if brokenPath := extractBrokenParquetPath(err); brokenPath != "" {
					logger.Warn("CompactionService: merge hit broken/missing parquet, removing catalog entry",
						"table", tableName, "path", brokenPath)
					if c.removeBrokenCatalogEntry(tableName, brokenPath) {
						_ = os.Remove(brokenPath)
						err = nil
					}
				}
			}
			if err != nil {
				logger.Warn("CompactionService: merge_adjacent_files failed", "table", tableName, "error", err)
				if strings.Contains(err.Error(), "Out of Memory") {
					c.logMemoryBreakdown()
				}
			} else {
				logger.Info("CompactionService: merge_adjacent_files completed",
					"table", tableName,
					"files_processed", processed,
					"files_created", created)

				fileCountAfter, err := c.getTableFileCount(tableName)
				if err == nil {
					logger.Info("CompactionService: Files after merge", "table", tableName, "count", fileCountAfter)
				}
			}
		})
	}

	c.runMaintenanceCalls()

	logger.Info("CompactionService: Maintenance completed", "lake", c.lakeName)
	return nil
}

// runMaintenanceCalls retires aged-out snapshots and removes the data files they
// were keeping alive. Shared by both engines: the native compactor retires files
// as ordinary DuckLake snapshots, so it needs exactly the same follow-up.
// Each CALL takes the catalog lock on its own rather than holding it for the
// whole sequence, so flush and queries are not blocked for the entire cycle.
func (c *CompactionService) runMaintenanceCalls() {
	// 1. Expire old snapshots (AFTER merge) — lock for this call only
	c.withCatalogLock(func() {
		expireSeconds := c.config.SnapshotExpireIntervalSec
		if expireSeconds <= 0 {
			expireSeconds = 3600
		}
		logger.Info("CompactionService: Expire snapshots",
			"lake", c.lakeName,
			"older_than_seconds", expireSeconds)
		expireSQL := fmt.Sprintf(
			"CALL ducklake_expire_snapshots('%s', older_than => CAST(NOW() - INTERVAL '%d seconds' AS TIMESTAMPTZ))",
			c.lakeName,
			expireSeconds,
		)
		if _, err := c.execWithRetry(expireSQL); err != nil {
			logger.Warn("CompactionService: expire_snapshots failed", "error", err)
		}
	})

	// 2. Cleanup old files — lock for this call only
	c.withCatalogLock(func() {
		c.ensureS3ClientSettings()
		logger.Info("CompactionService: Cleanup old files", "lake", c.lakeName)
		cleanupSQL := fmt.Sprintf("CALL ducklake_cleanup_old_files('%s', cleanup_all => true)", c.lakeName)
		if _, err := c.execWithRetry(cleanupSQL); err != nil {
			c.warnMaintenanceS3Failure("cleanup_old_files", err)
		}
	})

	// 3. Delete orphaned files — lock for this call only
	c.withCatalogLock(func() {
		c.ensureS3ClientSettings()
		logger.Info("CompactionService: Delete orphaned files", "lake", c.lakeName)
		orphanSQL := fmt.Sprintf("CALL ducklake_delete_orphaned_files('%s', cleanup_all => true)", c.lakeName)
		if _, err := c.execWithRetry(orphanSQL); err != nil {
			c.warnMaintenanceS3Failure("delete_orphaned_files", err)
		}
	})

	// 4. Remove empty directories (no catalog lock needed — filesystem only)
	if c.dataPath != "" && !ducklake.IsRemoteLakeDataPath(c.dataPath) {
		removed := cleanupEmptyDirs(filepath.Join(c.dataPath, "main"))
		if removed > 0 {
			logger.Info("CompactionService: Removed empty directories", "lake", c.lakeName, "count", removed)
		}
	}
}

// useNativeEngine reports whether the native compactor should run. The native
// engine is OPT-IN (the default empty engine and "duckdb" both use the DuckLake
// merge).
//
// It is safe to run alongside the live DuckDB writer because it registers merged
// files through DuckLake's own ducklake_add_data_files inside a short
// transaction: DuckLake allocates every snapshot and file id, so the writer's
// cached ids can never go stale. That is the difference from the version that
// corrupted catalogs, which wrote snapshots into SQLite itself and then tried to
// refresh the writer's cache with a DETACH that fails under load.
//
// The gate is therefore about capability, not about cache refreshing: without
// ducklake_add_data_files there is no way to hand a pre-written parquet file to
// DuckLake, and we stay on the DuckDB merge.
func (c *CompactionService) useNativeEngine() bool {
	if c.config.Engine != EngineNativeGo {
		// empty/default and "duckdb" → DuckLake merge.
		return false
	}
	if c.nativeDisabled.Load() {
		return false
	}
	if ducklake.IsRemoteLakeDataPath(c.dataPath) {
		return false
	}
	ctx := c.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	if ok, reason := compactor.SwapSupported(ctx, c.db); !ok {
		logger.Warn("CompactionService: native engine selected but this DuckLake build cannot "+
			"register pre-merged files; falling back to the DuckDB merge",
			"lake", c.lakeName, "reason", reason)
		return false
	}
	// DuckLake derives an added file's partition values from its whole path, so a
	// "key=value" directory anywhere above the table would be read as a column
	// the table does not have.
	if comp := compactor.HiveLikePathComponent(c.dataPath); comp != "" {
		logger.Warn("CompactionService: native engine cannot be used because data_path contains a "+
			"hive-style directory that DuckLake would read as a partition column; "+
			"falling back to the DuckDB merge",
			"lake", c.lakeName, "data_path", c.dataPath, "component", comp)
		return false
	}
	return true
}

// runNativeMerge compacts every table with the native compactor, then runs the
// same expire/cleanup maintenance as the DuckDB path.
//
// The compactor concatenates parquet row groups itself (peak memory ≈ one row
// group) and swaps each partition in through DuckLake's own API, so superseded
// files are retired as ordinary DuckLake snapshots. Removing them is therefore
// the job of the standard expire_snapshots / cleanup_old_files /
// delete_orphaned_files calls, which honour the configured time-travel window —
// the earlier native engine deleted parquet itself and could pull files out from
// under a node that still referenced them.
func (c *CompactionService) runNativeMerge(tables []string) error {
	if ducklake.IsRemoteLakeDataPath(c.dataPath) {
		logger.Warn("CompactionService: native engine requires a local data_path; skipping merge",
			"lake", c.lakeName, "data_path", c.dataPath)
		return nil
	}

	// Cheap insurance before metadata-rewriting maintenance: the catalog is
	// metadata only, so a copy costs little and makes a bad cycle recoverable.
	if path := strings.TrimSpace(c.catalogPath); path != "" {
		c.withCatalogLock(func() {
			dest, err := ducklake.BackupCatalog(path, nativeCatalogBackupsKept)
			if err != nil {
				logger.Warn("CompactionService: catalog backup before native merge failed",
					"lake", c.lakeName, "error", err)
				return
			}
			logger.Info("CompactionService: catalog backed up before native merge", "path", dest)
		})
	}

	// Lock/Unlock are passed into the compactor so it holds the CatalogLock only
	// for the short per-partition swap transaction — never during the slow
	// parquet merge — keeping flush/ingest responsive.
	var lockFn, unlockFn func()
	if c.catalogLocker != nil {
		lockFn = c.catalogLocker.CatalogLock
		unlockFn = c.catalogLocker.CatalogUnlock
	}

	for _, table := range tables {
		tableName := tableNameFromFQN(table)
		if tableName == "" {
			logger.Warn("CompactionService: Skipping table with invalid name", "table", table)
			continue
		}
		if reason, unsupported := c.nativeUnsupported.Load(tableName); unsupported {
			logger.Debug("CompactionService: native merge not possible for this table",
				"table", tableName, "reason", reason)
			continue
		}
		logger.Info("CompactionService: native merge starting", "table", tableName)
		res, err := compactor.CompactTable(c.ctx, compactor.Options{
			DB:                  c.db,
			LakeName:            c.lakeName,
			DataPath:            c.dataPath,
			TargetFileSizeBytes: c.config.TargetFileSizeBytes,
			MinAge:              time.Duration(c.config.MinAgeSec) * time.Second,
			Lock:                lockFn,
			Unlock:              unlockFn,
		}, tableName)
		if err != nil {
			logger.Warn("CompactionService: native merge failed", "table", tableName, "error", err)
			continue
		}
		if res.Skipped {
			if res.Unsupported {
				// Caused by the table's schema, so every later cycle would repeat
				// the same merge just to discard it. Remember and stop trying.
				c.nativeUnsupported.Store(tableName, res.SkipReason)
				logger.Warn("CompactionService: table cannot be compacted natively, "+
					"not retrying until restart",
					"table", tableName, "reason", res.SkipReason)
				continue
			}
			logger.Info("CompactionService: native merge skipped", "table", tableName, "reason", res.SkipReason)
			continue
		}
		logger.Info("CompactionService: native merge completed",
			"table", tableName,
			"files_before", res.FilesBefore,
			"files_merged", res.FilesMerged,
			"files_created", res.FilesCreated,
			"partitions", res.PartitionsCompacted,
			"partitions_deferred", res.PartitionsDeferred)
	}

	// A native cycle must never leave the catalog in the state that produced
	// "multiple snapshots returned from database". If it did, stop using the
	// native engine until the process restarts and an operator has looked.
	if err := c.checkCatalogInvariants(); err != nil {
		c.nativeDisabled.Store(true)
		logger.Error("CompactionService: catalog invariants violated after native merge; "+
			"native engine disabled until restart, falling back to the DuckDB merge",
			"lake", c.lakeName, "error", err)
	}

	c.runMaintenanceCalls()

	logger.Info("CompactionService: Native maintenance completed", "lake", c.lakeName)
	return nil
}

// unsupportedTables records, per table, why the native engine cannot be used for
// it. Compaction cycles run from a single goroutine, but the mutex keeps it safe
// if that ever changes and makes the intent explicit.
type unsupportedTables struct {
	mu     sync.Mutex
	byName map[string]string
}

func (u *unsupportedTables) Store(table, reason string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.byName == nil {
		u.byName = make(map[string]string)
	}
	u.byName[table] = reason
}

func (u *unsupportedTables) Load(table string) (string, bool) {
	u.mu.Lock()
	defer u.mu.Unlock()
	reason, ok := u.byName[table]
	return reason, ok
}

// checkCatalogInvariants verifies the catalog is still queryable after a native
// cycle.
func (c *CompactionService) checkCatalogInvariants() error {
	return verifySnapshotInvariants(c.db, fmt.Sprintf("__ducklake_metadata_%s", c.lakeName))
}

// verifySnapshotInvariants checks the two properties whose violation makes
// DuckLake abort every query. DuckLake resolves the current snapshot with
// "WHERE snapshot_id = (SELECT MAX(snapshot_id) ...)" and fails when that matches
// more than one row.
//
// Current catalog schemas declare snapshot_id as a primary key, so an exact
// duplicate is already rejected by SQLite. This stays as a cheap net for the
// variants that slip past it — most notably an id written with a different
// storage class, which the unique index treats as a distinct value while DuckLake
// reads the column as BIGINT and sees two rows (see RepairCatalogSnapshots).
func verifySnapshotInvariants(db *sql.DB, metadataSchema string) error {
	var latestRows int64
	if err := db.QueryRow(fmt.Sprintf(
		`SELECT COUNT(*) FROM %[1]s.ducklake_snapshot
		  WHERE CAST(snapshot_id AS BIGINT) =
		        (SELECT MAX(CAST(snapshot_id AS BIGINT)) FROM %[1]s.ducklake_snapshot)`,
		metadataSchema)).Scan(&latestRows); err != nil {
		return fmt.Errorf("read latest snapshot rows: %w", err)
	}
	if latestRows != 1 {
		return fmt.Errorf("latest snapshot matches %d rows, want exactly 1", latestRows)
	}

	var dupes int64
	if err := db.QueryRow(fmt.Sprintf(
		`SELECT COUNT(*) FROM (
		    SELECT CAST(snapshot_id AS BIGINT) AS sid FROM %s.ducklake_snapshot
		     GROUP BY sid HAVING COUNT(*) > 1)`,
		metadataSchema)).Scan(&dupes); err != nil {
		return fmt.Errorf("count duplicate snapshots: %w", err)
	}
	if dupes != 0 {
		return fmt.Errorf("%d duplicated snapshot_id values", dupes)
	}
	return nil
}

// cleanupEmptyDirs removes empty leaf directories under root, walking bottom-up.
// It stops at root itself — only descendants are considered.
// Returns the number of directories removed.
func cleanupEmptyDirs(root string) int {
	removed := 0

	// filepath.WalkDir visits in lexical order (top-down).
	// We collect all directory paths first, then process deepest first.
	var dirs []string
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || !d.IsDir() || path == root {
			return nil
		}
		dirs = append(dirs, path)
		return nil
	})

	// Process in reverse order (deepest first) so a parent is only
	// considered after all its children have been removed.
	for i := len(dirs) - 1; i >= 0; i-- {
		dir := dirs[i]
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		if len(entries) == 0 {
			if err := os.Remove(dir); err == nil {
				removed++
			}
		}
	}

	return removed
}

// GetStats returns compaction statistics
func (c *CompactionService) GetStats() map[string]interface{} {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return map[string]interface{}{
		"enabled":                 c.config.Enable,
		"check_interval_sec":      c.config.CheckIntervalSec,
		"retention_days":          c.config.RetentionDays,
		"retention_days_by_table": c.config.RetentionDaysByTable,
		"tables":                  len(c.tables),
		"last_compaction_time":    c.lastCompactionTime,
		"total_compactions":       c.totalCompactions,
		"total_rows_deleted":      c.totalRowsDeleted,
	}
}

func (c *CompactionService) effectiveMaxCompactedFiles() int {
	if c.config.MaxCompactedFiles > 0 {
		return c.config.MaxCompactedFiles
	}
	return defaultDuckDBMaxCompactedFiles
}

func (c *CompactionService) effectiveMaxFileSizeBytes() int64 {
	if c.config.MaxFileSizeBytes > 0 {
		return c.config.MaxFileSizeBytes
	}
	return defaultDuckDBMaxFileSizeBytes
}

// buildMergeSQL constructs the merge SQL with optional parameters
func (c *CompactionService) buildMergeSQL(tableName string) string {
	params := []string{"schema => 'main'"}

	if c.config.MinFileSizeBytes > 0 {
		params = append(params, fmt.Sprintf("min_file_size => %d", c.config.MinFileSizeBytes))
	}
	params = append(params, fmt.Sprintf("max_file_size => %d", c.effectiveMaxFileSizeBytes()))
	params = append(params, fmt.Sprintf("max_compacted_files => %d", c.effectiveMaxCompactedFiles()))

	return fmt.Sprintf(
		"CALL ducklake_merge_adjacent_files('%s', '%s', %s)",
		c.lakeName,
		tableName,
		strings.Join(params, ", "),
	)
}

// mergeAdjacentFiles runs ducklake_merge_adjacent_files on a dedicated
// connection with threads=1. Query (not Exec) is required to read the
// files_processed/files_created result set; Exec always reports 0.
func (c *CompactionService) mergeAdjacentFiles(tableName string) (filesProcessed, filesCreated int64, err error) {
	if c.db == nil {
		return 0, 0, fmt.Errorf("no database")
	}
	ctx := c.ctx
	if ctx == nil {
		ctx = context.Background()
	}

	const attempts = 4
	backoff := 500 * time.Millisecond
	var lastErr error
	mergeSQL := c.buildMergeSQL(tableName)

	for attempt := 1; attempt <= attempts; attempt++ {
		filesProcessed, filesCreated, lastErr = c.mergeAdjacentFilesOnce(ctx, mergeSQL)
		if lastErr == nil {
			return filesProcessed, filesCreated, nil
		}
		if !isDatabaseLocked(lastErr) {
			return 0, 0, lastErr
		}
		if attempt < attempts {
			logger.Warn("CompactionService: database locked, retrying",
				"attempt", attempt,
				"next_wait", backoff.String(),
				"error", lastErr)
			time.Sleep(backoff)
			backoff *= 2
		}
	}
	return 0, 0, lastErr
}

func (c *CompactionService) mergeAdjacentFilesOnce(ctx context.Context, mergeSQL string) (filesProcessed, filesCreated int64, err error) {
	conn, err := c.db.Conn(ctx)
	if err != nil {
		return 0, 0, err
	}
	defer conn.Close()

	// Session-scoped: only this connection, not the rest of the writer pool.
	// Serializes per-partition merge groups so two date= partitions of
	// hep_proto_1_call are not decompressed at once.
	if _, err := conn.ExecContext(ctx, "SET threads = 1"); err != nil {
		logger.Warn("CompactionService: SET threads=1 for merge failed", "error", err)
	}

	rows, err := conn.QueryContext(ctx, mergeSQL)
	if err != nil {
		return 0, 0, err
	}
	defer rows.Close()

	for rows.Next() {
		var schemaName, tableName string
		var processed, created int64
		if scanErr := rows.Scan(&schemaName, &tableName, &processed, &created); scanErr != nil {
			// Older DuckLake builds may return a different shape; the merge
			// itself succeeded, so do not fail the cycle.
			logger.Warn("CompactionService: merge result scan failed", "error", scanErr)
			continue
		}
		filesProcessed += processed
		filesCreated += created
	}
	if err := rows.Err(); err != nil {
		return filesProcessed, filesCreated, err
	}
	return filesProcessed, filesCreated, nil
}

// logMemoryBreakdown dumps DuckDB's buffer-manager accounting after an
// Out of Memory failure so operators can see WHO holds the memory_limit
// budget (in-memory buffer tables, hash joins, parquet readers, ...) and
// whether temp spilling is actually engaged (temporary_storage_bytes > 0
// proves temp_directory is set and used; all-zero means spilling is off,
// i.e. the instance runs without a temp_directory).
func (c *CompactionService) logMemoryBreakdown() {
	rows, err := c.db.Query(`
		SELECT tag,
		       memory_usage_bytes,
		       temporary_storage_bytes
		FROM duckdb_memory()
		ORDER BY memory_usage_bytes DESC`)
	if err != nil {
		logger.Warn("CompactionService: duckdb_memory() failed", "error", err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var tag string
		var memBytes, tempBytes int64
		if err := rows.Scan(&tag, &memBytes, &tempBytes); err != nil {
			continue
		}
		if memBytes == 0 && tempBytes == 0 {
			continue
		}
		logger.Warn("CompactionService: duckdb memory breakdown",
			"tag", tag,
			"memory_mb", memBytes/(1<<20),
			"spilled_to_disk_mb", tempBytes/(1<<20))
	}
	if err := rows.Err(); err != nil {
		logger.Warn("CompactionService: duckdb_memory() iteration failed", "error", err)
	}
}

func (c *CompactionService) execWithRetry(query string, args ...any) (sql.Result, error) {
	const attempts = 4
	backoff := 500 * time.Millisecond // Start with 500ms to reduce lock conflicts with flush
	var lastErr error

	for attempt := 1; attempt <= attempts; attempt++ {
		var result sql.Result
		result, lastErr = c.db.Exec(query, args...)
		if lastErr == nil {
			return result, nil
		}
		if !isDatabaseLocked(lastErr) {
			return nil, lastErr
		}
		if attempt < attempts {
			logger.Warn("CompactionService: database locked, retrying",
				"attempt", attempt,
				"next_wait", backoff.String(),
				"error", lastErr)
			time.Sleep(backoff)
			backoff *= 2
		}
	}
	return nil, lastErr
}

func (c *CompactionService) queryWithRetry(query string, args ...any) (*sql.Rows, error) {
	const attempts = 4
	backoff := 500 * time.Millisecond // Start with 500ms to reduce lock conflicts with flush
	var lastErr error

	for attempt := 1; attempt <= attempts; attempt++ {
		var rows *sql.Rows
		rows, lastErr = c.db.Query(query, args...)
		if lastErr == nil {
			return rows, nil
		}
		if !isDatabaseLocked(lastErr) {
			return nil, lastErr
		}
		if attempt < attempts {
			logger.Warn("CompactionService: database locked, retrying",
				"attempt", attempt,
				"next_wait", backoff.String(),
				"error", lastErr)
			time.Sleep(backoff)
			backoff *= 2
		}
	}
	return nil, lastErr
}

func isDatabaseLocked(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "database is locked")
}

func tableNameFromFQN(table string) string {
	parts := strings.Split(table, ".")
	if len(parts) == 0 {
		return ""
	}
	return parts[len(parts)-1]
}

func (c *CompactionService) getSnapshotCount() (int64, error) {
	query := fmt.Sprintf("SELECT COUNT(*) FROM %s.ducklake_snapshot", fmt.Sprintf("__ducklake_metadata_%s", c.lakeName))
	rows, err := c.queryWithRetry(query)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	var count int64
	if rows.Next() {
		if err := rows.Scan(&count); err != nil {
			return 0, err
		}
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	return count, nil
}

// recoverGhostFiles detects and repairs catalog entries that reference parquet
// files missing from disk. This is a common post-crash artifact: DuckLake
// committed a snapshot to the catalog but the process was killed before the
// physical parquet file was fully written. merge_adjacent_files fails with an
// IO error when it encounters such an entry.
//
// All active entries (end_snapshot IS NULL) are scanned regardless of age —
// a ghost can originate from any past interrupted transaction, not only the
// last one. The scan touches only the catalog (SQLite metadata), not the
// Parquet files themselves, so it is fast even with thousands of entries.
func (c *CompactionService) recoverGhostFiles() {
	if ducklake.IsRemoteLakeDataPath(c.dataPath) {
		// filepath.Join corrupts s3:// to s3:/; os.Open does not read object storage.
		// Ghost detection is local-parquet only; DuckLake maintenance handles remote blobs.
		logger.Info("CompactionService: skip local ghost scan (remote data_path)",
			"data_path", c.dataPath)
		return
	}

	metadataSchema := fmt.Sprintf("__ducklake_metadata_%s", c.lakeName)

	// Scan ALL active catalog entries (end_snapshot IS NULL).
	// We do not limit by snapshot window because ghosts may come from any
	// past interrupted transaction — not just the most recent one.
	query := fmt.Sprintf(`
		SELECT t.table_name, f.path
		FROM %s.ducklake_data_file f
		JOIN %s.ducklake_table t ON t.table_id = f.table_id
		WHERE f.end_snapshot IS NULL
	`, metadataSchema, metadataSchema)

	rows, err := c.queryWithRetry(query)
	if err != nil {
		logger.Warn("CompactionService: catalog ghost-file scan failed", "error", err)
		return
	}
	defer rows.Close()

	type entry struct{ table, path string }
	var ghosts []entry
	for rows.Next() {
		var table, catalogPath string
		if err := rows.Scan(&table, &catalogPath); err != nil {
			continue
		}
		absPath := catalogPathToAbs(c.dataPath, table, catalogPath)
		if isGhostOrCorrupt(absPath) {
			ghosts = append(ghosts, entry{table, absPath})
		}
	}

	if len(ghosts) == 0 {
		return
	}

	logger.Warn("CompactionService: found ghost/corrupt catalog entries",
		"count", len(ghosts))

	start := time.Now()
	removed, failed := 0, 0
	const logEvery = 100

	for i, g := range ghosts {
		if c.removeBrokenCatalogEntry(g.table, g.path) {
			_ = os.Remove(g.path)
			removed++
		} else {
			failed++
		}
		if (i+1)%logEvery == 0 {
			logger.Info("CompactionService: ghost recovery progress",
				"done", i+1, "total", len(ghosts), "removed", removed, "failed", failed)
		}
	}

	logger.Info("CompactionService: ghost recovery complete",
		"total", len(ghosts), "removed", removed, "failed", failed,
		"elapsed", time.Since(start).Round(time.Millisecond))
}

// removeBrokenCatalogEntry deletes the ducklake_data_file row that references
// the given absolute path. The path stored in the catalog may be relative to
// {DataPath}/main/{tableName}/ so we try matching both the raw path and the
// relative suffix after stripping the dataPath prefix.
func (c *CompactionService) removeBrokenCatalogEntry(tableName, absPath string) bool {
	metadataSchema := fmt.Sprintf("__ducklake_metadata_%s", c.lakeName)

	// Build the relative path as stored in the catalog.
	relPath := absPath
	var prefix string
	if ducklake.IsRemoteLakeDataPath(c.dataPath) {
		prefix = ducklake.JoinLakeDataPath(c.dataPath, "main", tableName) + "/"
	} else {
		prefix = filepath.Join(c.dataPath, "main", tableName) + "/"
	}
	if strings.HasPrefix(absPath, prefix) {
		relPath = strings.TrimPrefix(absPath, prefix)
	}

	deleteSQL := fmt.Sprintf(`
		DELETE FROM %s.ducklake_data_file
		WHERE path = '%s'
		   OR path = '%s'
	`, metadataSchema, relPath, absPath)

	result, err := c.execWithRetry(deleteSQL)
	if err != nil {
		logger.Warn("CompactionService: failed to remove broken catalog entry",
			"table", tableName, "path", absPath, "error", err)
		return false
	}
	rows, _ := result.RowsAffected()
	if rows > 0 {
		logger.Info("CompactionService: removed broken catalog entry",
			"table", tableName, "path", absPath, "rows_deleted", rows)
	}
	return rows > 0
}

func (c *CompactionService) getTableFileCount(tableName string) (int64, error) {
	metadataSchema := fmt.Sprintf("__ducklake_metadata_%s", c.lakeName)
	query := fmt.Sprintf(`
		SELECT COUNT(*)
		FROM %s.ducklake_data_file f
		JOIN %s.ducklake_table t ON t.table_id = f.table_id
		WHERE t.table_name = ?
		  AND f.end_snapshot IS NULL
	`, metadataSchema, metadataSchema)

	rows, err := c.queryWithRetry(query, tableName)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	var count int64
	if rows.Next() {
		if err := rows.Scan(&count); err != nil {
			return 0, err
		}
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	return count, nil
}
