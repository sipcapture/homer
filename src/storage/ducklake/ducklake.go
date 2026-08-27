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
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	duckdb "github.com/duckdb/duckdb-go/v2"
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
	S3URLStyle        string

	// Azure Blob Storage configuration (if DataPath is az:// or azure://)
	AzureAccountName      string
	AzureAccountKey       string
	AzureConnectionString string

	// DuckDB engine tuning. Empty / zero values mean "leave DuckDB's
	// own default" — these knobs are opt-in. See ApplyDuckDBTuning
	// for the SQL we actually run.
	TuningThreads       int
	TuningMemoryLimit   string
	TuningTempDirectory string

	// ExclusiveLock, when true, makes the writer take an exclusive OS file lock
	// (flock) on the SQLite catalog before attaching. A second writer process on
	// the same catalog then refuses to start instead of corrupting it (two
	// concurrent DuckLake writers on SQLite duplicate snapshot/table ids — see
	// "Corrupt DuckLake - multiple snapshots returned"). Set only on writer
	// paths; readers and CLI maintenance leave it false.
	ExclusiveLock bool

	// AutoRepairCatalog, when true, runs RepairCatalogSnapshots on startup
	// (before ATTACH) to remove duplicate snapshot/table metadata rows that
	// cause "Corrupt DuckLake - multiple snapshots returned from database".
	// Lossless for data; only conflicting duplicate metadata rows are dropped.
	AutoRepairCatalog bool
}

// isS3Path checks if path is an S3 URL
func isS3Path(path string) bool {
	return strings.HasPrefix(path, "s3://") || strings.HasPrefix(path, "s3a://")
}

// IsS3Path is isS3Path exported for callers outside package ducklake that
// need to branch S3-specific setup separately from Azure (e.g. cli_cmd.go's
// openDuckLakeReadOnly, so an S3-only data_path does not also attempt to
// LOAD the azure extension).
func IsS3Path(path string) bool { return isS3Path(path) }

// isAzurePath checks if path is an Azure Blob Storage URL
func isAzurePath(path string) bool {
	return strings.HasPrefix(path, "az://") || strings.HasPrefix(path, "azure://")
}

// IsAzurePath is isAzurePath exported; see IsS3Path.
func IsAzurePath(path string) bool { return isAzurePath(path) }

// IsRemoteLakeDataPath reports whether lake parquet roots live on object storage
// (s3://, s3a://, az://, azure://) rather than the local filesystem.
func IsRemoteLakeDataPath(p string) bool {
	return isS3Path(p) || isAzurePath(p)
}

// JoinLakeDataPath appends path elements to a lake data root. For s3://, s3a://,
// az://, and azure:// bases it uses URL-style '/' joining only — do not use
// filepath.Join, which on Unix collapses "s3://" to "s3:/" and breaks object URLs.
func JoinLakeDataPath(base string, elems ...string) string {
	if isS3Path(base) || isAzurePath(base) {
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

	// writerLock holds the exclusive flock on the SQLite catalog (when
	// config.ExclusiveLock is set). Kept open for the writer's lifetime; closing
	// it (on Stop) releases the lock. The OS also releases it on process exit.
	writerLock *os.File

	// Centralized single-writer flush queue. When non-nil, all DuckLake INSERT
	// operations go through a single goroutine, eliminating catalog contention.
	flushQueueCh chan centralFlushJob
	flushQueueWg sync.WaitGroup

	// lastAzureSecretRefresh tracks when the az:// credential_chain secret
	// was last recreated (see ensureAzureSecretFresh). Only touched by the
	// single flushLoop goroutine, so no lock needed.
	lastAzureSecretRefresh time.Time
}

// azureCredentialChainRefreshInterval bounds how often flushLoop recreates a
// credential_chain Azure secret. Independent of compaction (which only runs
// when compaction.enable is true and has its own, often longer, cadence) —
// this is the one path that always runs for a live single-volume writer, so
// it is what keeps a Managed Identity token (~1h IMDS lifetime) from going
// stale on a deployment that has compaction disabled. Well under an hour to
// leave margin.
const azureCredentialChainRefreshInterval = 20 * time.Minute

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

	// Create data directory if it doesn't exist (for local paths only, not
	// object storage: os.MkdirAll on a URL collapses "az://"/"s3://" to
	// "az:/"/"s3:/" on Unix and fails, or worse writes a bogus local dir).
	if config.DataPath != "" && !IsRemoteLakeDataPath(config.DataPath) {
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

// writerPoolConns is the connection pool size for the writer's DuckDB.
// All pool connections are sessions on ONE DuckDB instance (one Connector ==
// one duckdb database), so the attached DuckLake catalog, mem_* buffer tables
// and secrets are shared. More than one connection lets node search queries
// (handleQuery runs on this DB via SetSharedDB) execute concurrently with
// flush inserts and long compaction calls — with a single connection every
// SELECT queues behind maintenance and intermittently hits the coordinator
// query timeout (sipcapture/homer#785). Catalog-modifying operations stay
// serialized via catalogMu, so write-write races are unchanged.
const writerPoolConns = 4

// connect establishes connection to DuckDB and attaches DuckLake
func (mtw *MultiTableWriter) connect() error {
	// Guard against a second writer attaching the same SQLite catalog (two
	// concurrent DuckLake writers corrupt it — duplicate snapshot/table ids).
	if mtw.config.ExclusiveLock && mtw.config.CatalogPath != "" &&
		(mtw.config.CatalogType == CatalogSQLite || mtw.config.CatalogType == "") {
		lock, err := acquireCatalogWriterLock(mtw.config.CatalogPath)
		if err != nil {
			return err
		}
		mtw.writerLock = lock
		logger.Info(fmt.Sprintf("DuckLake writer holds exclusive catalog lock: %s.lock", mtw.config.CatalogPath))
	}

	// SET s3_* is session-scoped: replay it on every new pooled connection.
	// Without this, the pool can hand out a fresh connection without those
	// settings → intermittent S3 404 / NoSuchBucket on flush.
	s3Stmts := S3ClientSettingsSQL(
		mtw.config.S3Region,
		mtw.config.S3AccessKeyID,
		mtw.config.S3SecretAccessKey,
		mtw.config.S3Endpoint,
		mtw.config.S3UseSSL,
		mtw.config.S3URLStyle,
	)
	connector, err := duckdb.NewConnector("", func(execer driver.ExecerContext) error {
		for _, stmt := range s3Stmts {
			if _, err := execer.ExecContext(context.Background(), stmt, nil); err != nil {
				return fmt.Errorf("duckdb conn init %s: %w", s3SettingName(stmt), err)
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to open DuckDB: %w", err)
	}
	db := sql.OpenDB(connector)
	db.SetMaxOpenConns(writerPoolConns)
	db.SetMaxIdleConns(writerPoolConns)
	mtw.db = db

	// Apply DuckDB engine tuning (memory_limit, threads, temp_directory)
	// before LOAD / ATTACH so the limits are in effect for the catalog
	// bring-up too. When the operator hasn't set threads or memory_limit,
	// apply sensible defaults so DuckDB doesn't oversubscribe the host
	// (by default DuckDB claims all cores and ~80% of RAM).
	threads := mtw.config.TuningThreads
	memLimit := mtw.config.TuningMemoryLimit
	if threads == 0 {
		threads = AutoThreads()
		logger.Info("DuckDB writer: auto-limiting threads (operator did not set tuning.threads)",
			"threads", threads, "host_cpus", runtime.NumCPU())
	}
	if strings.TrimSpace(memLimit) == "" {
		memLimit = "2GB"
		logger.Info("DuckDB writer: auto-limiting memory (operator did not set tuning.memory_limit)",
			"memory_limit", memLimit)
	}
	// In-memory DuckDB has disk spilling DISABLED unless temp_directory is
	// set. With the writer pool, search + flush + compaction run concurrently
	// against the same memory_limit, so without a spill path long-range
	// searches abort with Out of Memory instead of spilling to disk.
	tempDir := mtw.config.TuningTempDirectory
	if strings.TrimSpace(tempDir) == "" {
		tempDir = DefaultSpillDirectory(mtw.config.CatalogPath)
		if tempDir != "" {
			logger.Info("DuckDB writer: defaulting spill directory (operator did not set tuning.temp_directory)",
				"temp_directory", tempDir)
		}
	}
	ApplyDuckDBTuning(db, threads, memLimit, tempDir, "writer")
	ApplyDuckDBMemorySafety(db, "writer")

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
	// Drain the leaked inline-table backlog before DuckLake opens the catalog
	// (upstream duckdb/ducklake#1065). Must run before ATTACH so we have
	// exclusive access to the sqlite file. Non-fatal.
	if n, err := GCOrphanInlineTables(mtw.config.CatalogPath); err != nil {
		logger.Warn("DuckLake inline GC failed (non-fatal)", "err", err)
	} else if n > 0 {
		logger.Info("DuckLake inline GC: dropped empty ducklake_inlined_data_* tables (upstream #1065)", "dropped", n)
	}

	// Autofix the "Corrupt DuckLake - multiple snapshots returned" corruption
	// before ATTACH, while we still have exclusive access to the sqlite file.
	// Lossless: only duplicate snapshot/table metadata rows are collapsed.
	if mtw.config.AutoRepairCatalog &&
		(mtw.config.CatalogType == CatalogSQLite || mtw.config.CatalogType == "") {
		res, err := RepairCatalogSnapshots(mtw.config.CatalogPath)
		switch {
		case err != nil && res.Malformed:
			logger.Error("DuckLake catalog file is physically malformed; row-level "+
				"auto-repair cannot recover it. Salvage with "+
				"`sqlite3 CATALOG .dump | sqlite3 CATALOG.fixed` then swap the file, "+
				"or run `homer system --rebuild-catalog` to rebuild from parquet.",
				"err", err, "catalog", mtw.config.CatalogPath)
		case err != nil:
			logger.Warn("DuckLake catalog auto-repair failed (non-fatal); "+
				"run `homer system --rebuild-catalog` if queries still error", "err", err)
		case res.Changed():
			logger.Warn("DuckLake catalog auto-repair: removed duplicate metadata rows "+
				"(fixed 'Corrupt DuckLake - multiple snapshots returned')",
				"duplicate_snapshots", res.DuplicateSnapshots,
				"duplicate_snapshot_changes", res.DuplicateSnapshotChanges,
				"duplicate_tables", res.DuplicateTables,
				"latest_snapshot_rows", res.LatestSnapshotRows,
				"catalog", mtw.config.CatalogPath)
		default:
			logger.Debug("DuckLake catalog auto-repair: catalog healthy, nothing to fix",
				"latest_snapshot_rows", res.LatestSnapshotRows,
				"catalog", mtw.config.CatalogPath)
		}
		// Even after a (no-op or successful) repair the latest-snapshot probe may
		// still return >1 row — e.g. corruption the lightweight dedup can't reach.
		// Surface this loudly so the operator runs the heavyweight rebuild instead
		// of chasing silent 500s.
		if err == nil && !res.Healthy() {
			logger.Error("DuckLake catalog still reports multiple latest snapshots after "+
				"auto-repair; queries will fail with 'Corrupt DuckLake - multiple snapshots "+
				"returned'. Run `homer system --rebuild-catalog` to rebuild from parquet.",
				"latest_snapshot_rows", res.LatestSnapshotRows,
				"catalog", mtw.config.CatalogPath)
		}
		// Duplicate table registrations (same table_name, different table_id) are
		// the deeper corruption from two concurrent writers (sipcapture/homer#809).
		// We don't auto-delete them (it can orphan data files), so warn explicitly.
		if err == nil && res.DuplicateTableNames > 0 {
			logger.Warn("DuckLake catalog has duplicate table registrations (same name, "+
				"different ids) from a concurrent-writer event; per-table queries may fail. "+
				"Run `homer system --rebuild-catalog` to rebuild the catalog from parquet.",
				"duplicate_table_names", res.DuplicateTableNames,
				"catalog", mtw.config.CatalogPath)
		}
		// Non-INTEGER table_id values in ducklake_data_file (sipcapture/homer#900)
		// abort DuckLake file-list resolution with a Mismatch Type Error. Auto-
		// repair cannot rewrite those rows safely — rebuild from parquet.
		if err == nil && res.CorruptDataFileTableIDs > 0 {
			logger.Error("DuckLake catalog has corrupt ducklake_data_file.table_id values "+
				"(expected INTEGER, found non-integer storage class such as a parquet path). "+
				"Lake queries will fail with 'Mismatch Type Error: Failed to get data file list'. "+
				"Stop the writer and run `homer system --rebuild-catalog`.",
				"corrupt_table_id_rows", res.CorruptDataFileTableIDs,
				"catalog", mtw.config.CatalogPath)
		}
	}

	// SET s3_* is applied per pooled connection by the connector init above.
	if isS3Path(mtw.config.DataPath) {
		if err := EnsureWriterS3Secret(db,
			mtw.config.S3Region,
			mtw.config.S3AccessKeyID,
			mtw.config.S3SecretAccessKey,
			mtw.config.S3Endpoint,
			mtw.config.S3UseSSL,
			mtw.config.S3URLStyle,
		); err != nil {
			return fmt.Errorf("failed to configure S3 secret for DuckLake: %w", err)
		}
	}

	if isAzurePath(mtw.config.DataPath) {
		// Best-effort load, same contract as tiered storage's LOAD aws: a
		// missing azure extension must not block startup for local/S3-only
		// deployments, but this path only runs when DataPath is az://.
		EnsureAzureCACertPath()
		if _, err := db.Exec("LOAD azure;"); err != nil {
			logger.Warn("DuckLake writer: failed to load azure extension (run --install-extensions)", "error", err)
		}
		if err := EnsureWriterAzureSecret(db,
			mtw.config.AzureAccountName,
			mtw.config.AzureAccountKey,
			mtw.config.AzureConnectionString,
		); err != nil {
			return fmt.Errorf("failed to configure Azure secret for DuckLake: %w", err)
		}
	}

	// Build attach statement (SQLite catalog only)
	attachSQL := mtw.buildAttachSQL()

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

// buildAttachSQL returns the ATTACH statement for this writer's DuckLake catalog.
func (mtw *MultiTableWriter) buildAttachSQL() string {
	return fmt.Sprintf(
		"ATTACH 'ducklake:sqlite:%s' AS %s (DATA_PATH '%s', AUTOMATIC_MIGRATION TRUE);",
		mtw.config.CatalogPath, mtw.config.LakeName, mtw.config.DataPath,
	)
}

// refreshCatalogCache drops DuckLake's in-memory snapshot/stats cache by
// detaching and re-attaching the catalog, so the next flush re-reads the latest
// snapshot_id / next_file_id from the SQLite catalog.
//
// This is required after the native compactor commits a snapshot out-of-band
// (through a separate SQLite connection): without it, the DuckDB writer keeps
// allocating ids from its stale cached counter and collides with the
// compactor's snapshot ("Corrupt DuckLake - multiple snapshots returned").
//
// The caller MUST hold the catalog lock (CatalogLock) so no flush runs during
// the DETACH/ATTACH. Best-effort: on failure the catalog is left as-is and the
// error is returned so the caller can log it.
func (mtw *MultiTableWriter) refreshCatalogCache() error {
	if mtw.db == nil {
		return nil
	}
	if _, err := mtw.db.Exec(fmt.Sprintf("DETACH %s;", mtw.config.LakeName)); err != nil {
		return fmt.Errorf("detach %s: %w", mtw.config.LakeName, err)
	}
	if _, err := mtw.db.Exec(mtw.buildAttachSQL()); err != nil {
		return fmt.Errorf("re-attach %s: %w", mtw.config.LakeName, err)
	}
	if mtw.config.DataInliningRowLimit >= 0 {
		_, _ = mtw.db.Exec(fmt.Sprintf(
			"CALL %s.set_option('DATA_INLINING_ROW_LIMIT', %d);",
			mtw.config.LakeName, mtw.config.DataInliningRowLimit))
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
			mtw.ensureAzureSecretFresh()
			mtw.flushAll()
		}
	}
}

// ensureAzureSecretFresh recreates the single-volume writer's az:// secret
// once azureCredentialChainRefreshInterval has elapsed, when it is a
// PROVIDER credential_chain secret (Managed Identity / ambient identity —
// the case that actually goes stale; static keys are left alone). This is
// what keeps flush working on a deployment with compaction disabled — see
// CompactionService.ensureAzureClientSettings for the compaction-side half
// of this same fix.
func (mtw *MultiTableWriter) ensureAzureSecretFresh() {
	if mtw == nil || mtw.db == nil || !isAzurePath(mtw.config.DataPath) {
		return
	}
	if !UsesAzureCredentialChain(mtw.config.AzureAccountKey, mtw.config.AzureConnectionString) {
		return
	}
	if time.Since(mtw.lastAzureSecretRefresh) < azureCredentialChainRefreshInterval {
		return
	}
	if err := EnsureWriterAzureSecret(mtw.db, mtw.config.AzureAccountName, mtw.config.AzureAccountKey, mtw.config.AzureConnectionString); err != nil {
		logger.Warn("DuckLake writer: failed to refresh Azure credential_chain secret", "error", err)
		return
	}
	mtw.lastAzureSecretRefresh = time.Now()
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

		// Release the exclusive catalog lock last, after the DB is closed.
		if mtw.writerLock != nil {
			releaseCatalogWriterLock(mtw.writerLock)
			mtw.writerLock = nil
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

// GetBufferStats returns only the total in-memory buffer size across all tables.
// Unlike GetStats, this does NOT run expensive lake-wide COUNT/MIN/MAX queries,
// making it safe to call frequently without blocking the single DuckDB connection.
func (mtw *MultiTableWriter) GetBufferStats() (bufferRows int, tableCount int) {
	mtw.mu.RLock()
	defer mtw.mu.RUnlock()

	for _, tw := range mtw.tables {
		bufferRows += int(tw.GetBufferStats())
	}
	return bufferRows, len(mtw.tables)
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

// duckLakeTableExists reports whether a table is already present in the
// attached DuckLake catalog. Used to avoid re-issuing schema-bumping DDL
// (SET PARTITIONED BY / SET SORTED BY) on every startup. Best-effort: any
// query error is treated as "does not exist" so the caller falls back to the
// safe (configure-the-table) path.
func duckLakeTableExists(db *sql.DB, lakeName, tableName string) bool {
	row := db.QueryRow(
		"SELECT 1 FROM information_schema.tables WHERE table_catalog = ? AND table_name = ? LIMIT 1",
		lakeName, tableName)
	var x int
	return row.Scan(&x) == nil
}

// runSQLiteCLI feeds a SQL script to the sqlite3 CLI on stdin and returns its
// stdout. Used for catalog maintenance that must run with exclusive access,
// before DuckDB ATTACHes the catalog.
func runSQLiteCLI(catalogPath, sql string) (string, error) {
	cmd := exec.Command("sqlite3", "-batch", "-noheader", catalogPath)
	cmd.Stdin = strings.NewReader(sql)
	out, err := cmd.Output()
	return string(out), err
}

func sqliteLines(catalogPath, sql string) ([]string, error) {
	out, err := runSQLiteCLI(catalogPath, sql)
	if err != nil {
		return nil, err
	}
	var lines []string
	for _, l := range strings.Split(out, "\n") {
		l = strings.TrimRight(l, "\r")
		if strings.TrimSpace(l) != "" {
			lines = append(lines, l)
		}
	}
	return lines, nil
}

// GCOrphanInlineTables drops empty `ducklake_inlined_data_*` physical tables
// left behind in the catalog. DuckLake never DROPs them: schema_version bumps
// on every DDL (incl. SET PARTITIONED BY / SET SORTED BY), flush only DELETEs
// the rows, and expire/cleanup remove registry rows but not the tables
// (upstream bug duckdb/ducklake#1065). They accumulate per
// (table_id, schema_version) and, although tiny on disk, the DuckLake
// extension rebuilds an in-memory stats map over ALL of them on every catalog
// refresh (DuckLakeCatalog::ConstructStatsMap) — which grows RSS to multiple GB.
//
// Must run BEFORE ATTACH, while the sqlite file is not yet opened by DuckDB, so
// we have exclusive access (same window EnableSQLiteWALMode uses). Only EMPTY
// tables are dropped: their rows were already flushed to Parquet, so this is
// lossless. Best-effort; returns the number of tables dropped.
func GCOrphanInlineTables(catalogPath string) (int, error) {
	if _, err := os.Stat(catalogPath); os.IsNotExist(err) {
		return 0, nil // fresh catalog, nothing to GC
	}

	names, err := sqliteLines(catalogPath,
		`SELECT name FROM sqlite_master WHERE type='table' `+
			`AND name LIKE 'ducklake_inlined_data\_%' ESCAPE '\' `+
			`AND name <> 'ducklake_inlined_data_tables';`)
	if err != nil || len(names) == 0 {
		return 0, err
	}

	// Identify the empty ones in a single query (one row per empty table).
	var q strings.Builder
	for i, n := range names {
		if i > 0 {
			q.WriteString("\nUNION ALL ")
		}
		fmt.Fprintf(&q, `SELECT '%s' WHERE (SELECT count(*) FROM "%s")=0`, n, n)
	}
	empties, err := sqliteLines(catalogPath, q.String())
	if err != nil || len(empties) == 0 {
		return 0, err
	}

	var script strings.Builder
	script.WriteString("BEGIN;\n")
	for _, t := range empties {
		fmt.Fprintf(&script, "DROP TABLE IF EXISTS \"%s\";\n", t)
		fmt.Fprintf(&script, "DELETE FROM ducklake_inlined_data_tables WHERE table_name='%s';\n", t)
	}
	script.WriteString("COMMIT;\n")

	if _, err := runSQLiteCLI(catalogPath, script.String()); err != nil {
		return 0, err
	}
	return len(empties), nil
}
