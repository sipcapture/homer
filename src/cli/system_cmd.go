package cli

import (
	"database/sql"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"

	_ "github.com/duckdb/duckdb-go/v2"
	"github.com/sipcapture/homer-core/src/config"
	"github.com/sipcapture/homer-core/src/storage/ducklake"
	logger "github.com/sipcapture/homer-core/src/utils/logging"
)

func tableNameFromFQN(table string) string {
	parts := strings.Split(table, ".")
	if len(parts) == 0 {
		return ""
	}
	return parts[len(parts)-1]
}

// reBrokenParquetCLI matches DuckDB errors about unreadable parquet files.
var reBrokenParquetCLI = regexp.MustCompile(
	`Cannot open file "([^"]+\.parquet)"|Invalid footer.*for file '([^']+\.parquet)'`)

// catalogPathToAbsCLI converts a DuckLake catalog-relative path to an absolute filesystem path.
func catalogPathToAbsCLI(dataPath, tableName, catalogPath string) string {
	if filepath.IsAbs(catalogPath) {
		return catalogPath
	}
	if dataPath == "" {
		return catalogPath
	}
	if strings.HasPrefix(catalogPath, "main/") {
		return filepath.Join(dataPath, catalogPath)
	}
	return filepath.Join(dataPath, "main", tableName, catalogPath)
}

// isGhostOrCorruptCLI returns true if the given parquet file is missing, too small,
// or does not have valid Parquet magic bytes ("PAR1") at the start and end.
func isGhostOrCorruptCLI(path string) bool {
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

	head := make([]byte, 4)
	if _, err := f.Read(head); err != nil || string(head) != "PAR1" {
		return true
	}

	tail := make([]byte, 4)
	if _, err := f.ReadAt(tail, info.Size()-4); err != nil || string(tail) != "PAR1" {
		return true
	}

	return false
}

// SystemFlags holds flags for the "homer system" subcommand.
type SystemFlags struct {
	ConfigPath                string
	CompactionForce           bool
	CompactionMerge           bool
	CompactionRepair          bool
	CompactionRecover         bool
	CompactionExpireSnapshots bool
	CompactionExpireOlderThan string
	CompactionRetentionDays   int
	CompactionMergeList       bool
	CompactionMergeListLimit  int
	InstallExtensions         bool
	Reload                    bool
	PidFile                   string
	DuckDBVersion             bool
	GenerateExampleConfig     bool
}

type systemFlagRefs struct {
	ConfigPath                *string
	CompactionForce           *bool
	CompactionMerge           *bool
	CompactionRepair          *bool
	CompactionRecover         *bool
	CompactionExpireSnapshots *bool
	CompactionExpireOlderThan *string
	CompactionRetentionDays   *int
	CompactionMergeList       *bool
	CompactionMergeListLimit  *int
	InstallExtensions         *bool
	Reload                    *bool
	PidFile                   *string
	DuckDBVersion             *bool
	GenerateExampleConfig     *bool
}

// RegisterSystemFlags creates a FlagSet for "homer system" subcommand.
func RegisterSystemFlags() (*flag.FlagSet, *systemFlagRefs) {
	fs := flag.NewFlagSet("system", flag.ExitOnError)
	refs := &systemFlagRefs{}

	refs.ConfigPath = fs.String("config-path", "", "path to config file or directory")
	refs.CompactionForce = fs.Bool("compaction-force", false, "run full compaction (merge, expire, cleanup, orphan) and exit")
	refs.CompactionMerge = fs.Bool("compaction-merge", false, "merge adjacent small files and exit")
	refs.CompactionRepair = fs.Bool("compaction-repair", false, "remove stale catalog entries for missing parquet files and exit")
	refs.CompactionRecover = fs.Bool("compaction-recover", false, "recover catalog by re-ingesting parquet files from disk into DuckLake tables and exit")
	refs.CompactionExpireSnapshots = fs.Bool("compaction-expire-snapshots", false, "expire DuckLake snapshots and exit")
	refs.CompactionExpireOlderThan = fs.String("compaction-expire-older-than", "1h", "age threshold for snapshot expiration (e.g., 30m, 2h, 3600s)")
	refs.CompactionRetentionDays = fs.Int("compaction-retention-days", 0, "delete data older than N days and exit")
	refs.CompactionMergeList = fs.Bool("compaction-merge-list", false, "list smallest DuckLake files before/after merge")
	refs.CompactionMergeListLimit = fs.Int("compaction-merge-list-limit", 50, "limit for compaction-merge-list output")
	refs.InstallExtensions = fs.Bool("install-extensions", false, "install DuckDB extensions (ducklake, sqlite) and exit")
	refs.Reload = fs.Bool("reload", false, "send SIGHUP to running homer-core process to reload config")
	refs.PidFile = fs.String("pid-file", "/var/run/homer-core.pid", "path to PID file (used with --reload)")
	refs.DuckDBVersion = fs.Bool("duckdb-version", false, "show DuckDB version and exit")
	refs.GenerateExampleConfig = fs.Bool("generate-example-config", false, "generate example JSON configuration and exit")

	return fs, refs
}

// ParseSystemFlags extracts SystemFlags from parsed flag refs.
func ParseSystemFlags(refs *systemFlagRefs) SystemFlags {
	return SystemFlags{
		ConfigPath:                *refs.ConfigPath,
		CompactionForce:           *refs.CompactionForce,
		CompactionMerge:           *refs.CompactionMerge,
		CompactionRepair:          *refs.CompactionRepair,
		CompactionRecover:         *refs.CompactionRecover,
		CompactionExpireSnapshots: *refs.CompactionExpireSnapshots,
		CompactionExpireOlderThan: *refs.CompactionExpireOlderThan,
		CompactionRetentionDays:   *refs.CompactionRetentionDays,
		CompactionMergeList:       *refs.CompactionMergeList,
		CompactionMergeListLimit:  *refs.CompactionMergeListLimit,
		InstallExtensions:         *refs.InstallExtensions,
		Reload:                    *refs.Reload,
		PidFile:                   *refs.PidFile,
		DuckDBVersion:             *refs.DuckDBVersion,
		GenerateExampleConfig:     *refs.GenerateExampleConfig,
	}
}

// RunSystemCmd dispatches system operations based on flags.
func RunSystemCmd(f SystemFlags) error {
	// Quick commands that don't need config
	if f.DuckDBVersion {
		fmt.Printf("DUCKDB_VERSION: %s\n", GetDuckDBVersion())
		return nil
	}

	if f.GenerateExampleConfig {
		if err := config.SaveExample("homer-core-example.json"); err != nil {
			return fmt.Errorf("failed to generate example config: %w", err)
		}
		fmt.Println("Example config saved to homer-core-example.json")
		return nil
	}

	if f.InstallExtensions {
		return installDuckDBExtensions()
	}

	if f.Reload {
		return handleReloadCommand(f.PidFile)
	}

	// Compaction commands -- need config
	if f.CompactionForce || f.CompactionMerge || f.CompactionRepair || f.CompactionRecover || f.CompactionExpireSnapshots || f.CompactionRetentionDays > 0 {
		return runCompaction(f)
	}

	return fmt.Errorf("no system operation specified; use --help for available flags")
}

// ---- compaction ------------------------------------------------------------

func runCompaction(f SystemFlags) error {
	cfg, err := config.Load(f.ConfigPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	logger.InitLoggerSimple("info", true, false)

	duckCfg := duckLakeConfigFromModular(cfg)
	manager, err := ducklake.NewManager(duckCfg)
	if err != nil {
		return fmt.Errorf("failed to init DuckLake: %w", err)
	}
	defer manager.Stop()

	db := manager.GetDB()
	lakeName := manager.GetLakeName()
	dataPath := duckCfg.DataPath

	// Retention
	if f.CompactionRetentionDays > 0 {
		tables, err := discoverDuckLakeTables(db, lakeName)
		if err != nil {
			return fmt.Errorf("failed to discover DuckLake tables: %w", err)
		}

		cutoff := time.Now().AddDate(0, 0, -f.CompactionRetentionDays).UnixNano()
		var totalRowsDeleted int64
		for _, table := range tables {
			query := fmt.Sprintf("DELETE FROM %s WHERE timestamp < %d", table, cutoff)
			result, err := db.Exec(query)
			if err != nil {
				logger.Error(fmt.Sprintf("Retention failed for %s: %v", table, err))
				continue
			}
			rowsAffected, _ := result.RowsAffected()
			if rowsAffected > 0 {
				logger.Info("Retention deleted rows", "count", rowsAffected, "table", table)
				totalRowsDeleted += rowsAffected
			}
		}
		logger.Info("Retention completed", "total_rows_deleted", totalRowsDeleted)
	}

	// Expire duration
	expireDuration := time.Hour
	if f.CompactionExpireOlderThan != "" {
		parsed, err := time.ParseDuration(f.CompactionExpireOlderThan)
		if err != nil {
			return fmt.Errorf("invalid --compaction-expire-older-than: %w", err)
		}
		if parsed > 0 {
			expireDuration = parsed
		}
	}
	expireSeconds := int64(expireDuration.Seconds())
	if expireSeconds < 1 {
		expireSeconds = 1
	}

	mergeListLimit := f.CompactionMergeListLimit

	if f.CompactionRepair {
		removed, err := repairCatalog(db, lakeName, dataPath)
		if err != nil {
			return fmt.Errorf("catalog repair failed: %w", err)
		}
		logger.Info("Catalog repair completed", "stale_entries_removed", removed)
		return nil
	}

	if f.CompactionRecover {
		recovered, err := recoverCatalog(db, lakeName, dataPath)
		if err != nil {
			return fmt.Errorf("catalog recovery failed: %w", err)
		}
		logger.Info("Catalog recovery completed", "tables_recovered", recovered)
		return nil
	}

	if f.CompactionForce {
		if removed, err := repairCatalog(db, lakeName, dataPath); err != nil {
			logger.Warn("Catalog repair step failed (continuing)", "error", err)
		} else if removed > 0 {
			logger.Info("Catalog repair: removed stale entries", "count", removed)
		}

		tables, err := discoverDuckLakeTables(db, lakeName)
		if err != nil {
			return fmt.Errorf("failed to discover tables: %w", err)
		}

		if f.CompactionMergeList {
			logDuckLakeSmallFiles(db, lakeName, "pre-merge", mergeListLimit)
		}

		var totalMerged int64
		for _, table := range tables {
			tableName := tableNameFromFQN(table)
			if tableName == "" {
				continue
			}
			mergeSQL := fmt.Sprintf("CALL ducklake_merge_adjacent_files('%s', '%s', schema => 'main')", lakeName, tableName)
			result, err := db.Exec(mergeSQL)
			if err != nil {
				if strings.Contains(err.Error(), "No such file or directory") {
					logger.Warn("Merge skipped (missing parquet files, catalog may have stale refs)", "table", tableName, "error", err)
					continue
				}
				return fmt.Errorf("merge adjacent files failed for %s: %w", tableName, err)
			}
			rows, _ := result.RowsAffected()
			if rows > 0 {
				logger.Info("Merge completed", "table", tableName, "files_merged", rows)
				totalMerged += rows
			}
		}
		logger.Info("Merge phase completed", "tables", len(tables), "total_files_merged", totalMerged)

		if f.CompactionMergeList {
			logDuckLakeSmallFiles(db, lakeName, "post-merge", mergeListLimit)
		}

		expireSQL := fmt.Sprintf(
			"CALL ducklake_expire_snapshots('%s', older_than => CAST(NOW() - INTERVAL '%d seconds' AS TIMESTAMPTZ))",
			lakeName, expireSeconds)
		if _, err := db.Exec(expireSQL); err != nil {
			return fmt.Errorf("expire snapshots failed: %w", err)
		}

		cleanupSQL := fmt.Sprintf("CALL ducklake_cleanup_old_files('%s', cleanup_all => true)", lakeName)
		if _, err := db.Exec(cleanupSQL); err != nil {
			logger.Warn(fmt.Sprintf("Cleanup old files failed: %v", err))
		}
		orphanSQL := fmt.Sprintf("CALL ducklake_delete_orphaned_files('%s', cleanup_all => true)", lakeName)
		if _, err := db.Exec(orphanSQL); err != nil {
			logger.Warn(fmt.Sprintf("Delete orphaned files failed: %v", err))
		}
		logger.Info("Compaction force completed", "expire_duration", expireDuration.String())
		return nil
	}

	if f.CompactionExpireSnapshots {
		expireSQL := fmt.Sprintf(
			"CALL ducklake_expire_snapshots('%s', older_than => CAST(NOW() - INTERVAL '%d seconds' AS TIMESTAMPTZ))",
			lakeName, expireSeconds)
		if _, err := db.Exec(expireSQL); err != nil {
			return fmt.Errorf("expire snapshots failed: %w", err)
		}
		logger.Info("Expired snapshots", "older_than", expireDuration.String())
	}

	if f.CompactionMerge {
		tables, err := discoverDuckLakeTables(db, lakeName)
		if err != nil {
			return fmt.Errorf("failed to discover tables: %w", err)
		}
		if f.CompactionMergeList {
			logDuckLakeSmallFiles(db, lakeName, "pre-merge", mergeListLimit)
		}
		var totalMerged int64
		for _, table := range tables {
			tableName := tableNameFromFQN(table)
			if tableName == "" {
				continue
			}
			mergeSQL := fmt.Sprintf("CALL ducklake_merge_adjacent_files('%s', '%s', schema => 'main')", lakeName, tableName)
			result, err := db.Exec(mergeSQL)
			if err != nil {
				if strings.Contains(err.Error(), "No such file or directory") {
					logger.Warn("Merge skipped (missing parquet files)", "table", tableName, "error", err)
					continue
				}
				return fmt.Errorf("merge adjacent files failed for %s: %w", tableName, err)
			}
			rows, _ := result.RowsAffected()
			totalMerged += rows
		}
		logger.Info("Merge completed", "files_merged", totalMerged)
		if f.CompactionMergeList {
			logDuckLakeSmallFiles(db, lakeName, "post-merge", mergeListLimit)
		}
	}

	return nil
}

func discoverDuckLakeTables(db *sql.DB, lakeName string) ([]string, error) {
	query := fmt.Sprintf(`
		SELECT table_name 
		FROM information_schema.tables 
		WHERE table_catalog = '%s' 
		  AND table_schema = 'main'
		  AND table_name LIKE 'hep_proto_%%'
	`, lakeName)

	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var tableName string
		if err := rows.Scan(&tableName); err != nil {
			continue
		}
		tables = append(tables, fmt.Sprintf("%s.main.%s", lakeName, tableName))
	}
	return tables, nil
}

// repairCatalog removes catalog entries for parquet files that no longer exist on disk
// or are corrupt (missing PAR1 magic bytes). Only active entries (end_snapshot IS NULL)
// are scanned — these are the ones that block merge operations.
//
// Returns the number of stale/corrupt entries removed.
func repairCatalog(db *sql.DB, lakeName, dataPath string) (int, error) {
	metadataSchema := fmt.Sprintf("__ducklake_metadata_%s", lakeName)

	// Scan only active entries to avoid touching already-expired history.
	rows, err := db.Query(fmt.Sprintf(`
		SELECT f.data_file_id, t.table_name, f.path
		FROM %s.ducklake_data_file f
		JOIN %s.ducklake_table t ON t.table_id = f.table_id
		WHERE f.end_snapshot IS NULL
	`, metadataSchema, metadataSchema))
	if err != nil {
		return 0, fmt.Errorf("failed to query catalog files: %w", err)
	}

	type fileEntry struct {
		id        int64
		tableName string
		path      string
		absPath   string
	}
	var stale []fileEntry
	for rows.Next() {
		var e fileEntry
		if err := rows.Scan(&e.id, &e.tableName, &e.path); err != nil {
			rows.Close()
			return 0, fmt.Errorf("failed to scan catalog row: %w", err)
		}
		e.absPath = catalogPathToAbsCLI(dataPath, e.tableName, e.path)
		if isGhostOrCorruptCLI(e.absPath) {
			stale = append(stale, e)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}

	if len(stale) == 0 {
		return 0, nil
	}

	for _, e := range stale {
		logger.Info("Removing stale/corrupt catalog entry", "data_file_id", e.id, "table", e.tableName, "path", e.path)
		if _, err := db.Exec(
			fmt.Sprintf(`DELETE FROM %s.ducklake_data_file WHERE data_file_id = ?`, metadataSchema),
			e.id,
		); err != nil {
			logger.Warn("Failed to remove stale entry", "data_file_id", e.id, "error", err)
			continue
		}
		_ = os.Remove(e.absPath)
	}

	return len(stale), nil
}

// recoverCatalog re-ingests parquet files from disk into DuckLake tables.
// Use this when ducklake_data_file entries are missing but the parquet files still exist.
// It works by reading each parquet file on disk and inserting its rows into the live DuckLake table.
// The original parquet files become orphans which can be cleaned with ducklake_delete_orphaned_files.
func recoverCatalog(db *sql.DB, lakeName, dataPath string) (int, error) {
	// Discover tables from the catalog (they should still exist in ducklake_table even if data files are gone).
	tables, err := discoverDuckLakeTables(db, lakeName)
	if err != nil {
		return 0, fmt.Errorf("failed to discover tables: %w", err)
	}
	if len(tables) == 0 {
		logger.Warn("No DuckLake tables found, nothing to recover")
		return 0, nil
	}

	recovered := 0
	for _, fqn := range tables {
		tableName := tableNameFromFQN(fqn)
		if tableName == "" {
			continue
		}
		tableDir := dataPath + "/main/" + tableName
		if _, err := os.Stat(tableDir); os.IsNotExist(err) {
			logger.Info("No parquet directory for table, skipping", "table", tableName)
			continue
		}

		globPattern := tableDir + "/date=*/**/*.parquet"
		insertSQL := fmt.Sprintf(
			`INSERT INTO %s SELECT * FROM read_parquet('%s', union_by_name=true, hive_partitioning=true)`,
			fqn, globPattern,
		)
		result, err := db.Exec(insertSQL)
		if err != nil {
			logger.Warn("Failed to recover table", "table", tableName, "error", err)
			continue
		}
		rows, _ := result.RowsAffected()
		logger.Info("Recovered table", "table", tableName, "rows_recovered", rows)
		recovered++
	}

	// Clean up orphaned original parquet files (now replaced by newly written ones).
	orphanSQL := fmt.Sprintf("CALL ducklake_delete_orphaned_files('%s', cleanup_all => true)", lakeName)
	if _, err := db.Exec(orphanSQL); err != nil {
		logger.Warn("Cleanup orphaned files after recovery failed (not critical)", "error", err)
	}

	return recovered, nil
}

func logDuckLakeSmallFiles(db *sql.DB, lakeName string, stage string, limit int) {
	if limit <= 0 {
		limit = 50
	}
	metadataSchema := fmt.Sprintf("__ducklake_metadata_%s", lakeName)
	query := fmt.Sprintf(`
		SELECT t.table_name, f.path, f.record_count
		FROM %s.ducklake_data_file f
		JOIN %s.ducklake_table t ON t.table_id = f.table_id
		ORDER BY f.record_count ASC, f.path ASC
		LIMIT %d
	`, metadataSchema, metadataSchema, limit)

	rows, err := db.Query(query)
	if err != nil {
		logger.Warn(fmt.Sprintf("Failed to list DuckLake files (%s): %v", stage, err))
		return
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var tableName, filePath string
		var rowCount int64
		if err := rows.Scan(&tableName, &filePath, &rowCount); err != nil {
			continue
		}
		logger.Info("DuckLake file", "stage", stage, "table", tableName, "rows", rowCount, "path", filePath)
		count++
	}

	if count == 0 {
		logger.Info("DuckLake file: no files found", "stage", stage)
	}
}

// ---- install extensions ----------------------------------------------------

func installDuckDBExtensions() error {
	fmt.Println("Installing DuckDB extensions...")

	db, err := sql.Open("duckdb", "")
	if err != nil {
		return fmt.Errorf("failed to open DuckDB: %w", err)
	}
	defer db.Close()

	extensions := []string{"ducklake", "sqlite"}

	for _, ext := range extensions {
		fmt.Printf("Installing %s extension...\n", ext)
		if _, err := db.Exec(fmt.Sprintf("INSTALL %s;", ext)); err != nil {
			return fmt.Errorf("failed to install %s: %w", ext, err)
		}
		fmt.Printf("Loading %s extension...\n", ext)
		if _, err := db.Exec(fmt.Sprintf("LOAD %s;", ext)); err != nil {
			return fmt.Errorf("failed to load %s: %w", ext, err)
		}
		fmt.Printf("%s extension installed successfully\n", ext)
	}

	fmt.Println("\nAll extensions installed successfully!")
	return nil
}

// ---- DuckDB version --------------------------------------------------------

// GetDuckDBVersion returns the DuckDB version string.
func GetDuckDBVersion() string {
	db, err := sql.Open("duckdb", "")
	if err != nil {
		return fmt.Sprintf("unknown (open error: %v)", err)
	}
	defer db.Close()

	var version string
	if err := db.QueryRow("SELECT version()").Scan(&version); err == nil {
		return version
	}

	return "unknown (query error: failed to resolve duckdb version)"
}

// ---- reload ----------------------------------------------------------------

func handleReloadCommand(pidFile string) error {
	data, err := os.ReadFile(pidFile)
	if err != nil {
		return fmt.Errorf("failed to read PID file %s: %w (make sure homer-core is running with --pid-file)", pidFile, err)
	}

	var pid int
	if _, err := fmt.Sscanf(strings.TrimSpace(string(data)), "%d", &pid); err != nil {
		return fmt.Errorf("invalid PID in file %s: %w", pidFile, err)
	}

	process, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("failed to find process %d: %w", pid, err)
	}

	if err := process.Signal(syscall.SIGHUP); err != nil {
		return fmt.Errorf("failed to send SIGHUP to process %d: %w", pid, err)
	}

	fmt.Printf("Sent SIGHUP to homer-core (PID %d) - reload triggered\n", pid)
	return nil
}
