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
	"time"

	"github.com/sipcapture/homer-core/src/storage/ducklake"
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
	// SnapshotExpireIntervalSec controls how long to keep DuckLake snapshots (seconds).
	SnapshotExpireIntervalSec int `json:"snapshot_expire_interval_sec"`
	// MinAgeSec controls how old data must be before compaction runs.
	MinAgeSec int `json:"min_age_sec"`
	// MinFileSizeBytes: minimum file size for merge (smaller files will be merged). 0 = no limit.
	MinFileSizeBytes int64 `json:"min_file_size_bytes"`
	// MaxFileSizeBytes: maximum size of merged file. 0 = no limit (DuckLake default).
	MaxFileSizeBytes int64 `json:"max_file_size_bytes"`
	// MaxCompactedFiles: maximum number of files to merge per table per cycle. 0 = no limit.
	MaxCompactedFiles int `json:"max_compacted_files"`
}

// CatalogLocker provides Lock/Unlock for serializing catalog-modifying operations.
// Implemented by the DuckLake Manager to coordinate flush and compaction.
type CatalogLocker interface {
	CatalogLock()
	CatalogUnlock()
}

// CompactionS3Client holds DuckDB httpfs S3 settings for maintenance calls that
// read s3:// objects (ducklake_cleanup_old_files / ducklake_delete_orphaned_files).
// Some DuckLake versions appear to evaluate read_blob without inheriting prior
// session SET s3_* on the shared *sql.DB; re-applying avoids spurious 403/404.
// Nil or empty AccessKeyID means skip (ApplyDuckDBS3ClientSettings is a no-op).
type CompactionS3Client struct {
	Region, AccessKeyID, SecretAccessKey, Endpoint string
	UseSSL                                           bool
}

// CompactionService handles periodic compaction and retention
type CompactionService struct {
	db            *sql.DB
	lakeName      string
	dataPath      string       // root dir for parquet files; catalog paths are relative to this
	config        CompactionConfig
	tables        []string
	catalogLocker CatalogLocker // serializes catalog access with writer flush
	s3Client      *CompactionS3Client

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	mu     sync.RWMutex

	// Stats
	lastCompactionTime time.Time
	totalCompactions   int64
	totalRowsDeleted   int64
}

// NewCompactionService creates a new compaction service.
// catalogLocker serializes catalog access with writer flush to prevent "database is locked".
// dataPath is the root directory for Parquet files; catalog paths are relative to it.
// s3Client may be nil when data_path is local or credentials are not used.
func NewCompactionService(db *sql.DB, lakeName, dataPath string, config CompactionConfig, catalogLocker CatalogLocker, s3Client *CompactionS3Client) *CompactionService {
	ctx, cancel := context.WithCancel(context.Background())

	return &CompactionService{
		db:            db,
		lakeName:      lakeName,
		dataPath:      dataPath,
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
	if err := ducklake.ApplyDuckDBS3ClientSettings(c.db, s.Region, s.AccessKeyID, s.SecretAccessKey, s.Endpoint, s.UseSSL); err != nil {
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
		logger.Info("CompactionService disabled")
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

	// Run retention per table if configured — lock per table
	if c.config.RetentionDays > 0 {
		for _, table := range c.tables {
			c.withCatalogLock(func() {
				deleted, err := c.runRetention(table)
				if err != nil {
					logger.Error("CompactionService: Retention failed", "table", table, "error", err)
				} else if deleted > 0 {
					logger.Info("CompactionService: Retention completed", "table", table, "rows_deleted", deleted)
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

// runRetention deletes data older than retention period.
// Missing parquet files are silently skipped — they will be cleaned up
// by ducklake_delete_orphaned_files in the maintenance phase.
func (c *CompactionService) runRetention(table string) (int64, error) {
	cutoff := time.Now().AddDate(0, 0, -c.config.RetentionDays)
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

	// 0. Flush inlined data to Parquet FIRST. DuckLake inlines small writes
	// (DATA_INLINING_ROW_LIMIT) directly into the catalog DB; with inlining
	// left enabled and no periodic flush, those rows accumulate inside the
	// catalog forever — an 800 MB catalog backing only a few dozen Parquet
	// files is the classic symptom, and DuckLake mirrors the catalog in
	// memory (multi-GB RSS). Flushing first also lets the subsequent merge /
	// expire act on freshly written Parquet instead of catalog-resident rows.
	// Harmless no-op when inlining is disabled (the recommended default).
	c.withCatalogLock(func() {
		logger.Info("CompactionService: Flush inlined data", "lake", c.lakeName)
		flushSQL := fmt.Sprintf("CALL ducklake_flush_inlined_data('%s')", c.lakeName)
		if _, err := c.execWithRetry(flushSQL); err != nil {
			logger.Warn("CompactionService: flush_inlined_data failed", "error", err)
		}
	})

	// 1. Merge adjacent small files FIRST — lock per table
	for _, table := range tables {
		tableName := tableNameFromFQN(table)
		if tableName == "" {
			logger.Warn("CompactionService: Skipping table with invalid name", "table", table)
			continue
		}

		c.withCatalogLock(func() {
			fileCount, err := c.getTableFileCount(tableName)
			if err == nil {
				logger.Info("CompactionService: Files before merge", "table", tableName, "count", fileCount)
			}

			mergeSQL := c.buildMergeSQL(tableName)
			logger.Info("CompactionService: Merge adjacent files", "lake", c.lakeName, "table", tableName)
			result, err := c.execWithRetry(mergeSQL)
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
			} else if result != nil {
				rowsAffected, _ := result.RowsAffected()
				logger.Info("CompactionService: merge_adjacent_files completed", "table", tableName, "files_merged", rowsAffected)

				fileCountAfter, err := c.getTableFileCount(tableName)
				if err == nil {
					logger.Info("CompactionService: Files after merge", "table", tableName, "count", fileCountAfter)
				}
			}
		})
	}

	// 2. Expire old snapshots (AFTER merge) — lock for this call only
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

	// 3. Cleanup old files — lock for this call only
	c.withCatalogLock(func() {
		c.ensureS3ClientSettings()
		logger.Info("CompactionService: Cleanup old files", "lake", c.lakeName)
		cleanupSQL := fmt.Sprintf("CALL ducklake_cleanup_old_files('%s', cleanup_all => true)", c.lakeName)
		if _, err := c.execWithRetry(cleanupSQL); err != nil {
			c.warnMaintenanceS3Failure("cleanup_old_files", err)
		}
	})

	// 4. Delete orphaned files — lock for this call only
	c.withCatalogLock(func() {
		c.ensureS3ClientSettings()
		logger.Info("CompactionService: Delete orphaned files", "lake", c.lakeName)
		orphanSQL := fmt.Sprintf("CALL ducklake_delete_orphaned_files('%s', cleanup_all => true)", c.lakeName)
		if _, err := c.execWithRetry(orphanSQL); err != nil {
			c.warnMaintenanceS3Failure("delete_orphaned_files", err)
		}
	})

	// 5. Remove empty directories (no catalog lock needed — filesystem only)
	if c.dataPath != "" && !ducklake.IsRemoteLakeDataPath(c.dataPath) {
		removed := cleanupEmptyDirs(filepath.Join(c.dataPath, "main"))
		if removed > 0 {
			logger.Info("CompactionService: Removed empty directories", "lake", c.lakeName, "count", removed)
		}
	}

	logger.Info("CompactionService: Maintenance completed", "lake", c.lakeName)
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
		"enabled":              c.config.Enable,
		"check_interval_sec":   c.config.CheckIntervalSec,
		"retention_days":       c.config.RetentionDays,
		"tables":               len(c.tables),
		"last_compaction_time": c.lastCompactionTime,
		"total_compactions":    c.totalCompactions,
		"total_rows_deleted":   c.totalRowsDeleted,
	}
}

// buildMergeSQL constructs the merge SQL with optional parameters
func (c *CompactionService) buildMergeSQL(tableName string) string {
	// Build optional parameters
	var params []string
	params = append(params, "schema => 'main'")

	if c.config.MinFileSizeBytes > 0 {
		params = append(params, fmt.Sprintf("min_file_size => %d", c.config.MinFileSizeBytes))
	}
	if c.config.MaxFileSizeBytes > 0 {
		params = append(params, fmt.Sprintf("max_file_size => %d", c.config.MaxFileSizeBytes))
	}
	if c.config.MaxCompactedFiles > 0 {
		params = append(params, fmt.Sprintf("max_compacted_files => %d", c.config.MaxCompactedFiles))
	}

	return fmt.Sprintf(
		"CALL ducklake_merge_adjacent_files('%s', '%s', %s)",
		c.lakeName,
		tableName,
		strings.Join(params, ", "),
	)
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
