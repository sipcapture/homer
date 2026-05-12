// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

// Package ducklake provides HEP storage using DuckLake format.
// DuckLake is a lakehouse format that combines Parquet files with a SQL catalog
// for features like time travel, snapshots, and ACID transactions.
//
// DuckLake catalog — sqlite. Parquet data may live on local disk or S3 via data_path.
package ducklake

import (
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "github.com/duckdb/duckdb-go/v2"
	logger "github.com/sipcapture/homer-core/src/utils/logging"
)

// CatalogType — DuckLake catalog (sqlite).
type CatalogType string

const (
	// CatalogSQLite — DuckLake catalog backed by a local .sqlite file.
	CatalogSQLite CatalogType = "sqlite"
)

// Config holds DuckLake configuration
type Config struct {
	// CatalogType — sqlite (empty = sqlite).
	CatalogType CatalogType

	// CatalogPath — .sqlite file for the DuckLake catalog
	CatalogPath string

	// DataPath: path for Parquet data files
	// Can be local path or S3 URL (s3://bucket/path/)
	DataPath string

	// LakeName: name of the DuckLake database (default: "homer_lake")
	LakeName string

	// TableName: name of the HEP table (default: "hep_messages")
	TableName string

	// BatchSize: number of records to buffer before writing
	BatchSize int

	// FlushInterval: max time between flushes
	FlushInterval time.Duration

	// SearchBuffer: when true, read queries include in-memory buffer tables
	// via UNION ALL so un-flushed data is visible immediately.
	SearchBuffer bool

	// ShardCount: number of parallel DuckDB writer shards (default 1).
	// Each shard has its own DuckDB instance + catalog for parallel writes.
	ShardCount int

	// FlushQueue enables the single-writer flush queue pattern.
	// When true, all DuckLake INSERT operations are serialized through a single
	// goroutine, eliminating SQLite "database is locked" errors entirely.
	// Default: auto-enabled for SQLite catalogs.
	FlushQueue *bool

	// DataInliningRowLimit overrides the DuckLake data inlining threshold (DuckLake v1.0).
	// Small writes of ≤N rows are stored in the catalog database instead of creating Parquet files.
	// -1 = use DuckLake built-in default (10 rows); 0 = disable inlining; >0 = custom limit.
	DataInliningRowLimit int

	// S3 configuration (if DataPath is S3)
	S3Region          string
	S3AccessKeyID     string
	S3SecretAccessKey string
	S3Endpoint        string
	S3UseSSL          bool

	// DuckDB engine tuning. Empty / zero values mean "leave DuckDB's
	// own default" — these knobs are opt-in. See ApplyDuckDBTuning
	// for the SQL we actually run.
	TuningThreads       int
	TuningMemoryLimit   string
	TuningTempDirectory string
}

// isS3Path checks if path is an S3 URL
func isS3Path(path string) bool {
	return strings.HasPrefix(path, "s3://") || strings.HasPrefix(path, "s3a://")
}

// IsRemoteLakeDataPath reports whether lake parquet roots live on object storage
// (s3:// or s3a://) rather than the local filesystem.
func IsRemoteLakeDataPath(p string) bool {
	return isS3Path(p)
}

// JoinLakeDataPath appends path elements to a lake data root. For s3:// and s3a://
// bases it uses URL-style '/' joining only — do not use filepath.Join, which on
// Unix collapses "s3://" to "s3:/" and breaks object URLs.
func JoinLakeDataPath(base string, elems ...string) string {
	if isS3Path(base) {
		out := strings.TrimRight(base, "/")
		for _, e := range elems {
			e = strings.Trim(e, "/")
			if e != "" {
				out += "/" + e
			}
		}
		return out
	}
	return filepath.Join(append([]string{base}, elems...)...)
}

// NormalizeSQLiteCatalog returns sqlite for empty or "sqlite"; otherwise an error.
func NormalizeSQLiteCatalog(ct CatalogType) (CatalogType, error) {
	s := strings.TrimSpace(string(ct))
	if s == "" {
		return CatalogSQLite, nil
	}
	if strings.EqualFold(s, "sqlite") {
		return CatalogSQLite, nil
	}
	return "", fmt.Errorf("ducklake catalog_type %q: DuckLake catalog — sqlite", s)
}

// DefaultConfig returns default DuckLake configuration
func DefaultConfig() Config {
	return Config{
		CatalogType:          CatalogSQLite,
		CatalogPath:          "homer_catalog.sqlite",
		DataPath:             "/var/lib/homer/parquet",
		LakeName:             "homer_lake",
		TableName:            "hep_messages",
		BatchSize:            10000,
		FlushInterval:        30 * time.Second,
		DataInliningRowLimit: -1, // use DuckLake built-in default
	}
}

// HEPRecord represents a HEP packet for storage
type HEPRecord struct {
	UUID      string
	Timestamp time.Time
	SessionID string
	Caller    string
	Callee    string
	SrcIP     string
	DstIP     string
	SrcPort   uint32
	DstPort   uint32
	Event     string
	ProtoType uint32
	Protocol  uint32
	NodeID    string
	CID       string
	Payload   string
	DataExtra string // JSON
}

// centralFlushJob carries a table writer and slot index for the centralized flush queue.
type centralFlushJob struct {
	tw      *TableWriter
	slotIdx int
}

// MultiTableWriter manages multiple tables based on proto_type and sub_type
type MultiTableWriter struct {
	config        Config
	db            *sql.DB
	tables        map[TableKey]*TableWriter
	schemas       map[TableKey]*TableSchema
	mu            sync.RWMutex
	catalogMu     sync.Mutex // serializes catalog-modifying operations (flush, compaction)
	stopChan      chan struct{}
	wg            sync.WaitGroup
	stopOnce      sync.Once
	flushInterval time.Duration
	lakeName      string
	searchBuffer  bool // include memory tables in read queries

	// Centralized single-writer flush queue. When non-nil, all DuckLake INSERT
	// operations go through a single goroutine, eliminating catalog contention.
	flushQueueCh chan centralFlushJob
	flushQueueWg sync.WaitGroup
}

// Writer is a deprecated single-table writer kept only for type compatibility
// with legacy code in api.go, hep_adapter.go, and timetravel.go.
// All production write paths use MultiTableWriter + TableWriter instead.
// TODO: remove Writer and all legacy references (api.go, hep_adapter.go, timetravel.go)
type Writer struct {
	db       *sql.DB
	tableFQN string
}

// Write is a no-op stub for the deprecated Writer. Use MultiTableWriter instead.
func (w *Writer) Write(record HEPRecord) error {
	return fmt.Errorf("deprecated: Writer.Write is no longer supported, use MultiTableWriter")
}

// GetStats is a no-op stub for the deprecated Writer. Use MultiTableWriter instead.
func (w *Writer) GetStats() (map[string]interface{}, error) {
	return nil, fmt.Errorf("deprecated: Writer.GetStats is no longer supported, use MultiTableWriter")
}

// Snapshot represents a DuckLake snapshot
type Snapshot struct {
	ID        int64
	CreatedAt time.Time
	RowCount  int64
}


// NewMultiTableWriter creates a new multi-table DuckLake writer
func NewMultiTableWriter(config Config) (*MultiTableWriter, error) {
	ct, err := NormalizeSQLiteCatalog(config.CatalogType)
	if err != nil {
		return nil, err
	}
	config.CatalogType = ct

	if config.LakeName == "" {
		config.LakeName = "homer_lake"
	}
	if config.FlushInterval <= 0 {
		config.FlushInterval = 30 * time.Second
	}

	// Create data directory if it doesn't exist (for local paths, not S3)
	if config.DataPath != "" && !isS3Path(config.DataPath) {
		if err := os.MkdirAll(config.DataPath, 0755); err != nil {
			return nil, fmt.Errorf("failed to create data directory %s: %w", config.DataPath, err)
		}
		logger.Info("DuckLake data directory ensured", "path", config.DataPath)
	}

	// Create catalog directory if it doesn't exist (SQLite catalog file)
	if config.CatalogPath != "" && config.CatalogType == CatalogSQLite {
		catalogDir := filepath.Dir(config.CatalogPath)
		if catalogDir != "" && catalogDir != "." {
			if err := os.MkdirAll(catalogDir, 0755); err != nil {
				return nil, fmt.Errorf("failed to create catalog directory %s: %w", catalogDir, err)
			}
			logger.Info("DuckLake catalog directory ensured", "path", catalogDir)
		}
	}

	mtw := &MultiTableWriter{
		config:        config,
		tables:        make(map[TableKey]*TableWriter),
		schemas:       GetTableSchemas(),
		stopChan:      make(chan struct{}),
		flushInterval: config.FlushInterval,
		lakeName:      config.LakeName,
		searchBuffer:  config.SearchBuffer,
	}

	// Enable single-writer flush queue: explicit config or auto for SQLite (always)
	useFlushQueue := config.FlushQueue != nil && *config.FlushQueue
	if config.FlushQueue == nil && config.CatalogType == CatalogSQLite {
		useFlushQueue = true
	}
	if useFlushQueue {
		mtw.flushQueueCh = make(chan centralFlushJob, 64)
		logger.Info("DuckLake: single-writer flush queue enabled (eliminates SQLite catalog contention)")
	}

	if err := mtw.connect(); err != nil {
		return nil, err
	}

	// Pre-create all known tables at startup so that other modules (node)
	// that ATTACH the same catalog see them immediately.
	for key := range mtw.schemas {
		if _, err := mtw.getOrCreateTable(key); err != nil {
			logger.Warn(fmt.Sprintf("DuckLake: pre-create table %s failed: %v", key.String(), err))
		}
	}

	return mtw, nil
}

// connect establishes connection to DuckDB and attaches DuckLake
func (mtw *MultiTableWriter) connect() error {
	// Open in-memory DuckDB (DuckLake uses it as query engine)
	db, err := sql.Open("duckdb", "")
	if err != nil {
		return fmt.Errorf("failed to open DuckDB: %w", err)
	}
	// Single connection: SET s3_* and CREATE SECRET from connect() apply per
	// sql.DB connection; with MaxOpenConns>1 the pool can hand out a fresh
	// connection without those settings → intermittent S3 404 / NoSuchBucket on
	// flush (same pattern as node Flight DuckDB: SetMaxOpenConns(1)).
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	mtw.db = db

	// Apply DuckDB engine tuning (memory_limit, threads, temp_directory)
	// before LOAD / ATTACH so the limits are in effect for the catalog
	// bring-up too. Empty / zero values are no-ops, so this is safe to
	// call unconditionally.
	ApplyDuckDBTuning(db, mtw.config.TuningThreads, mtw.config.TuningMemoryLimit, mtw.config.TuningTempDirectory, "writer")

	// Load DuckLake extension (must be pre-installed via --install-extensions)
	if _, err := db.Exec("LOAD ducklake;"); err != nil {
		return fmt.Errorf("failed to load ducklake extension (run homer-core --install-extensions first): %w", err)
	}

	// Load SQLite extension for the DuckLake catalog file
	if _, err := db.Exec("LOAD sqlite;"); err != nil {
		return fmt.Errorf("failed to load sqlite extension (run homer-core --install-extensions first): %w", err)
	}
	// Enable WAL mode for SQLite catalog to allow concurrent reads/writes
	if err := EnableSQLiteWALMode(mtw.config.CatalogPath); err != nil {
		logger.Warn(fmt.Sprintf("Failed to enable WAL mode for SQLite catalog (may cause lock errors): %v", err))
	}

	if err := ApplyDuckDBS3ClientSettings(db,
		mtw.config.S3Region,
		mtw.config.S3AccessKeyID,
		mtw.config.S3SecretAccessKey,
		mtw.config.S3Endpoint,
		mtw.config.S3UseSSL,
	); err != nil {
		return fmt.Errorf("failed to configure S3: %w", err)
	}
	if isS3Path(mtw.config.DataPath) {
		if err := EnsureWriterS3Secret(db,
			mtw.config.S3Region,
			mtw.config.S3AccessKeyID,
			mtw.config.S3SecretAccessKey,
			mtw.config.S3Endpoint,
			mtw.config.S3UseSSL,
		); err != nil {
			return fmt.Errorf("failed to configure S3 secret for DuckLake: %w", err)
		}
	}

	// Build attach statement (SQLite catalog only)
	attachSQL := fmt.Sprintf(
		"ATTACH 'ducklake:sqlite:%s' AS %s (DATA_PATH '%s', AUTOMATIC_MIGRATION TRUE);",
		mtw.config.CatalogPath, mtw.config.LakeName, mtw.config.DataPath,
	)

	if _, err := db.Exec(attachSQL); err != nil {
		return fmt.Errorf("failed to attach ducklake: %w", err)
	}

	logger.Info("DuckLake (multi-table) attached", "catalog", string(mtw.config.CatalogType), "data_path", mtw.config.DataPath)

	// Apply data inlining row limit when explicitly configured (DuckLake v1.0).
	// -1 (default) leaves DuckLake's own default (10 rows) unchanged.
	// 0 disables inlining; any positive value overrides the threshold.
	if mtw.config.DataInliningRowLimit >= 0 {
		inlineSQL := fmt.Sprintf(
			"CALL %s.set_option('DATA_INLINING_ROW_LIMIT', %d);",
			mtw.config.LakeName, mtw.config.DataInliningRowLimit,
		)
		if _, err := db.Exec(inlineSQL); err != nil {
			logger.Warn("failed to set DATA_INLINING_ROW_LIMIT", "err", err)
		} else {
			logger.Info("DuckLake: data inlining row limit set", "limit", mtw.config.DataInliningRowLimit)
		}
	}

	return nil
}

// getOrCreateTable gets existing or creates new table for TableKey
func (mtw *MultiTableWriter) getOrCreateTable(key TableKey) (*TableWriter, error) {
	// Fast path: check if table exists with read lock
	mtw.mu.RLock()
	if tw, ok := mtw.tables[key]; ok {
		mtw.mu.RUnlock()
		return tw, nil
	}
	mtw.mu.RUnlock()

	// Slow path: create table with write lock
	mtw.mu.Lock()
	defer mtw.mu.Unlock()

	// Double-check after acquiring write lock
	if tw, ok := mtw.tables[key]; ok {
		return tw, nil
	}

	// Get schema for this table key
	schema, ok := mtw.schemas[key]
	if !ok {
		schema = GetDefaultSchema(key)
		mtw.schemas[key] = schema
	}

	// Create table writer
	tw, err := NewTableWriter(mtw.db, mtw.lakeName, schema, mtw.config.BatchSize, &mtw.catalogMu)
	if err != nil {
		return nil, fmt.Errorf("failed to create table for %s: %w", key.String(), err)
	}

	mtw.tables[key] = tw
	logger.Info("Created new table", "table", tw.TableFQN())

	return tw, nil
}

// WriteRecord writes a record to the appropriate table based on TableKey
func (mtw *MultiTableWriter) WriteRecord(key TableKey, values []interface{}) error {
	tw, err := mtw.getOrCreateTable(key)
	if err != nil {
		return err
	}
	return tw.Write(values)
}

// Start starts the background flush goroutine
func (mtw *MultiTableWriter) Start() {
	if mtw.flushQueueCh != nil {
		go mtw.flushQueueWorker()
	}
	mtw.wg.Add(1)
	go mtw.flushLoop()
	logger.Info("DuckLake multi-table writer started")
}

// flushQueueWorker is the single goroutine that drains the centralized flush
// queue. Jobs run sequentially; flushSlotDirect passes catalogMu so each
// DuckLake INSERT is serialized with compaction (CatalogLock) without holding
// the mutex across backoff sleeps — holding it for the full retry loop would
// starve compaction and amplify S3/catalog races.
func (mtw *MultiTableWriter) flushQueueWorker() {
	for job := range mtw.flushQueueCh {
		job.tw.flushSlotDirect(job.slotIdx, &mtw.catalogMu)
		mtw.flushQueueWg.Done()
	}
}

// flushLoop periodically flushes all buffered records
func (mtw *MultiTableWriter) flushLoop() {
	defer mtw.wg.Done()

	ticker := time.NewTicker(mtw.flushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-mtw.stopChan:
			mtw.flushAll()
			return
		case <-ticker.C:
			mtw.flushAll()
		}
	}
}

// flushAll triggers a double-buffer swap for every table.
// When the centralized flush queue is active, buffers are swapped and jobs are
// enqueued to the single-writer goroutine (no catalog contention).
// Otherwise, each TableWriter's dedicated flush goroutine handles the INSERT.
func (mtw *MultiTableWriter) flushAll() {
	mtw.mu.RLock()
	tables := make([]*TableWriter, 0, len(mtw.tables))
	for _, tw := range mtw.tables {
		tables = append(tables, tw)
	}
	mtw.mu.RUnlock()

	if mtw.flushQueueCh != nil {
		// Centralized flush queue: swap buffers, then enqueue INSERT jobs.
		for _, tw := range tables {
			if err := tw.flushBatch(); err != nil {
				logger.Warn(fmt.Sprintf("Failed to drain batch before swap for %s: %v", tw.tableFQN, err))
			}
			old := int(tw.activeIdx.Load())
			next := 1 - old
			tw.activeIdx.Store(int32(next))
			mtw.flushQueueWg.Add(1)
			mtw.flushQueueCh <- centralFlushJob{tw: tw, slotIdx: old}
		}
		return
	}

	// Legacy per-table flush workers
	for _, tw := range tables {
		tw.SwapAndFlush()
	}
}

// Stop stops the writer, flushes remaining data, and waits for all flush
// goroutines to finish.
func (mtw *MultiTableWriter) Stop() error {
	var closeErr error
	mtw.stopOnce.Do(func() {
		close(mtw.stopChan)
		mtw.wg.Wait() // waits for flushLoop to exit (which calls flushAll one last time)

		// Wait for all centralized flush jobs to complete, then close the channel
		if mtw.flushQueueCh != nil {
			mtw.flushQueueWg.Wait()
			close(mtw.flushQueueCh)
		}

		// Close each table writer (drains batches, closes flush channel,
		// waits for flush goroutine).
		mtw.mu.RLock()
		for _, tw := range mtw.tables {
			if err := tw.Close(); err != nil {
				logger.Warn(fmt.Sprintf("failed to close table writer %s: %v", tw.TableFQN(), err))
			}
		}
		mtw.mu.RUnlock()

		if mtw.db != nil {
			closeErr = mtw.db.Close()
		}
	})
	return closeErr
}

// GetStats returns aggregated statistics from all tables
func (mtw *MultiTableWriter) GetStats() (map[string]interface{}, error) {
	stats := make(map[string]interface{})
	stats["catalog_type"] = string(mtw.config.CatalogType)
	stats["data_path"] = mtw.config.DataPath

	mtw.mu.RLock()
	defer mtw.mu.RUnlock()

	var totalRows int64
	var minTs, maxTs int64
	var totalBuffer int
	tableStats := make([]map[string]interface{}, 0, len(mtw.tables))

	for key, tw := range mtw.tables {
		ts, err := tw.GetStats()
		if err != nil {
			continue
		}

		ts["proto_type"] = key.ProtoType
		ts["sub_type"] = key.SubType
		ts["table_key"] = key.String()
		tableStats = append(tableStats, ts)

		if rc, ok := ts["row_count"].(int64); ok {
			totalRows += rc
		}
		if bs, ok := ts["buffer_size"].(int64); ok {
			totalBuffer += int(bs)
		}
		if mt, ok := ts["min_timestamp"].(int64); ok {
			if minTs == 0 || mt < minTs {
				minTs = mt
			}
		}
		if mt, ok := ts["max_timestamp"].(int64); ok {
			if mt > maxTs {
				maxTs = mt
			}
		}
	}

	stats["total_row_count"] = totalRows
	stats["total_buffer_size"] = totalBuffer
	stats["table_count"] = len(mtw.tables)
	stats["tables"] = tableStats

	if minTs > 0 {
		stats["min_timestamp"] = minTs
		stats["oldest_data"] = time.Unix(0, minTs).Format(time.RFC3339)
	}
	if maxTs > 0 {
		stats["max_timestamp"] = maxTs
		stats["newest_data"] = time.Unix(0, maxTs).Format(time.RFC3339)
	}

	return stats, nil
}

// CatalogLock acquires the catalog mutex to serialize catalog-modifying operations.
// This must be held during flush and compaction to prevent "database is locked" errors.
func (mtw *MultiTableWriter) CatalogLock() {
	mtw.catalogMu.Lock()
}

// CatalogUnlock releases the catalog mutex.
func (mtw *MultiTableWriter) CatalogUnlock() {
	mtw.catalogMu.Unlock()
}

// GetDB returns the database connection
func (mtw *MultiTableWriter) GetDB() *sql.DB {
	return mtw.db
}

// GetLakeName returns the lake name
func (mtw *MultiTableWriter) GetLakeName() string {
	return mtw.lakeName
}

// GetTableFQN returns the fully qualified table name for a TableKey
func (mtw *MultiTableWriter) GetTableFQN(key TableKey) string {
	mtw.mu.RLock()
	defer mtw.mu.RUnlock()

	if tw, ok := mtw.tables[key]; ok {
		return tw.TableFQN()
	}

	// Return expected name even if table doesn't exist yet
	schema, ok := mtw.schemas[key]
	if !ok {
		schema = GetDefaultSchema(key)
	}
	return fmt.Sprintf("%s.hep_proto_%s", mtw.lakeName, schema.TableSuffix)
}

// GetAllTableFQNs returns all table names
func (mtw *MultiTableWriter) GetAllTableFQNs() []string {
	mtw.mu.RLock()
	defer mtw.mu.RUnlock()

	tables := make([]string, 0, len(mtw.tables))
	for _, tw := range mtw.tables {
		tables = append(tables, tw.TableFQN())
	}
	return tables
}

// ListTableKeys returns all TableKeys with tables
func (mtw *MultiTableWriter) ListTableKeys() []TableKey {
	mtw.mu.RLock()
	defer mtw.mu.RUnlock()

	keys := make([]TableKey, 0, len(mtw.tables))
	for key := range mtw.tables {
		keys = append(keys, key)
	}
	return keys
}

// ListProtoTypes returns unique proto_types with tables
func (mtw *MultiTableWriter) ListProtoTypes() []uint32 {
	mtw.mu.RLock()
	defer mtw.mu.RUnlock()

	seen := make(map[uint32]bool)
	for key := range mtw.tables {
		seen[key.ProtoType] = true
	}

	types := make([]uint32, 0, len(seen))
	for pt := range seen {
		types = append(types, pt)
	}
	return types
}

// GetTable returns the TableWriter for a specific TableKey (for internal use)
func (mtw *MultiTableWriter) GetTable(key TableKey) *TableWriter {
	mtw.mu.RLock()
	defer mtw.mu.RUnlock()
	return mtw.tables[key]
}

// SearchBufferEnabled returns whether read queries should include memory tables.
func (mtw *MultiTableWriter) SearchBufferEnabled() bool {
	return mtw.searchBuffer
}

// EnableSQLiteWALMode enables WAL (Write-Ahead Logging) mode for SQLite catalog
// This allows concurrent reads and writes, reducing "database is locked" errors
func EnableSQLiteWALMode(catalogPath string) error {
	// Check if file exists
	if _, err := os.Stat(catalogPath); os.IsNotExist(err) {
		// File doesn't exist yet, will be created with WAL mode by SQLite
		return nil
	}

	// Try to enable WAL mode using sqlite3 CLI (if available)
	cmd := exec.Command("sqlite3", catalogPath, "PRAGMA journal_mode=WAL;")
	output, err := cmd.CombinedOutput()
	if err != nil {
		// sqlite3 CLI not available or failed - this is non-fatal
		return fmt.Errorf("sqlite3 CLI unavailable: %w", err)
	}

	// Check if WAL mode was enabled
	outputStr := strings.TrimSpace(string(output))
	if strings.Contains(strings.ToLower(outputStr), "wal") {
		logger.Info("SQLite WAL mode enabled for catalog", "path", catalogPath)
		return nil
	}

	return fmt.Errorf("failed to enable WAL mode, got: %s", outputStr)
}
