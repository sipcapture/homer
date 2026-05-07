// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

// Package ducklake provides tiered storage support for DuckLake.
// Tiered storage allows automatic movement of old data from hot (local) to cold (S3) storage.
package ducklake

import (
	"database/sql"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	logger "github.com/sipcapture/homer-core/src/utils/logging"
)

// VolumeType represents the type of storage volume
type VolumeType string

const (
	VolumeTypeLocal VolumeType = "local"
	VolumeTypeS3    VolumeType = "s3"
)

// Volume represents a storage volume in the tiered storage system
type Volume struct {
	Name           string
	Type           VolumeType
	Path           string
	Priority       int
	MaxDataAgeDays int
	MaxSizeGB      int
	LakeName       string // DuckLake name for this volume (e.g., "homer_lake_hot")

	// S3 configuration
	S3Region    string
	S3AccessKey string
	S3SecretKey string
	S3Endpoint  string
	S3UseSSL    bool
}

// TieredStorageConfig holds configuration for tiered storage
type TieredStorageConfig struct {
	Enable             bool
	Volumes            []Volume
	TTLMoveIntervalSec int
	MoveFactor         float64
	ConcurrentMoves    int
	MoveOnStartup      bool

	// Base config for catalog
	CatalogType CatalogType
	CatalogPath string
}

// TieredStorageManager manages multiple storage volumes with automatic tiering
type TieredStorageManager struct {
	config   TieredStorageConfig
	db       *sql.DB
	volumes  []*Volume
	writers  map[string]*MultiTableWriter // lakeName -> writer
	mu       sync.RWMutex
	stopChan chan struct{}
	wg       sync.WaitGroup
	stopOnce sync.Once

	// Primary volume for writes (lowest priority number)
	primaryVolume *Volume
}

// NewTieredStorageManager creates a new tiered storage manager
func NewTieredStorageManager(config TieredStorageConfig) (*TieredStorageManager, error) {
	if len(config.Volumes) == 0 {
		return nil, fmt.Errorf("at least one volume is required")
	}

	ct, err := NormalizeSQLiteCatalog(config.CatalogType)
	if err != nil {
		return nil, err
	}
	config.CatalogType = ct

	// Sort volumes by priority (lower = higher priority)
	sortedVolumes := make([]*Volume, len(config.Volumes))
	for i := range config.Volumes {
		sortedVolumes[i] = &config.Volumes[i]
	}
	sort.Slice(sortedVolumes, func(i, j int) bool {
		return sortedVolumes[i].Priority < sortedVolumes[j].Priority
	})

	tsm := &TieredStorageManager{
		config:        config,
		volumes:       sortedVolumes,
		writers:       make(map[string]*MultiTableWriter),
		stopChan:      make(chan struct{}),
		primaryVolume: sortedVolumes[0],
	}

	return tsm, nil
}

// Start initializes all volumes and starts the tiering service
func (tsm *TieredStorageManager) Start() error {
	// Open shared DuckDB connection
	db, err := sql.Open("duckdb", "")
	if err != nil {
		return fmt.Errorf("failed to open DuckDB: %w", err)
	}
	tsm.db = db

	// Load DuckLake extension
	if _, err := db.Exec("LOAD ducklake;"); err != nil {
		return fmt.Errorf("failed to load ducklake extension: %w", err)
	}

	// Load SQLite extension for the DuckLake catalog file
	if _, err := db.Exec("LOAD sqlite;"); err != nil {
		return fmt.Errorf("failed to load sqlite extension: %w", err)
	}

	// Attach each volume as a separate DuckLake
	for _, vol := range tsm.volumes {
		if err := tsm.attachVolume(vol); err != nil {
			return fmt.Errorf("failed to attach volume %s: %w", vol.Name, err)
		}
	}

	logger.Info("TieredStorageManager started",
		"volumes", len(tsm.volumes),
		"primary_volume", tsm.primaryVolume.Name)

	return nil
}

// attachVolume attaches a volume as a DuckLake database
func (tsm *TieredStorageManager) attachVolume(vol *Volume) error {
	// Configure S3 if needed
	if vol.Type == VolumeTypeS3 {
		// For S3-compatible storage (RustFS, MinIO, R2), create a secret
		// This is required for DuckLake to use custom S3 endpoints
		secretName := fmt.Sprintf("s3_secret_%s", vol.Name)

		// Drop existing secret if any
		dropSecret := fmt.Sprintf("DROP SECRET IF EXISTS %s;", secretName)
		if _, err := tsm.db.Exec(dropSecret); err != nil {
			logger.Warn("TieredStorageManager: Failed to drop existing secret", "secret", secretName, "error", err)
		}

		// Build endpoint URL
		endpoint := vol.S3Endpoint
		if endpoint != "" {
			endpoint = strings.TrimPrefix(endpoint, "http://")
			endpoint = strings.TrimPrefix(endpoint, "https://")
		}

		// Create secret with S3 credentials
		var createSecret string
		if endpoint != "" {
			createSecret = fmt.Sprintf(`
				CREATE SECRET %s (
					TYPE S3,
					KEY_ID '%s',
					SECRET '%s',
					REGION '%s',
					ENDPOINT '%s',
					URL_STYLE 'path',
					USE_SSL %t
				);
			`, secretName, vol.S3AccessKey, vol.S3SecretKey, vol.S3Region, endpoint, vol.S3UseSSL)
		} else {
			createSecret = fmt.Sprintf(`
				CREATE SECRET %s (
					TYPE S3,
					KEY_ID '%s',
					SECRET '%s',
					REGION '%s'
				);
			`, secretName, vol.S3AccessKey, vol.S3SecretKey, vol.S3Region)
		}

		logger.Info("TieredStorageManager: Creating S3 secret",
			"volume", vol.Name,
			"endpoint", endpoint,
			"use_ssl", vol.S3UseSSL)

		if _, err := tsm.db.Exec(createSecret); err != nil {
			return fmt.Errorf("failed to create S3 secret for volume %s: %w", vol.Name, err)
		}
	}

	// Build catalog path for this volume
	catalogPath := tsm.config.CatalogPath

	// For hot volume (priority 0), check if legacy catalog exists for migration
	if vol.Priority == 0 && len(tsm.volumes) > 1 {
		legacyPath := catalogPath // e.g., /data/homer/homer_catalog.sqlite
		if fileExists(legacyPath) {
			// Use legacy catalog for hot volume (migration mode)
			logger.Info("TieredStorageManager: Using legacy catalog for hot volume (migration mode)",
				"path", legacyPath)
			// catalogPath stays as is (legacy path)
		} else {
			// Create new catalog with _hot suffix
			catalogPath = strings.TrimSuffix(catalogPath, ".sqlite") + "_" + vol.Name + ".sqlite"
		}
	} else if len(tsm.volumes) > 1 {
		// Cold volumes always get suffix
		catalogPath = strings.TrimSuffix(catalogPath, ".sqlite") + "_" + vol.Name + ".sqlite"
	}

	// Build attach statement (SQLite catalog only)
	attachSQL := fmt.Sprintf(
		"ATTACH 'ducklake:sqlite:%s' AS %s (DATA_PATH '%s', AUTOMATIC_MIGRATION TRUE);",
		catalogPath, vol.LakeName, vol.Path,
	)

	if _, err := tsm.db.Exec(attachSQL); err != nil {
		return fmt.Errorf("failed to attach DuckLake for volume %s: %w", vol.Name, err)
	}

	logger.Info("TieredStorageManager: Volume attached",
		"volume", vol.Name,
		"type", vol.Type,
		"lake", vol.LakeName,
		"path", vol.Path)

	return nil
}

// GetDB returns the shared database connection
func (tsm *TieredStorageManager) GetDB() *sql.DB {
	return tsm.db
}

// GetPrimaryVolume returns the primary (hot) volume for writes
func (tsm *TieredStorageManager) GetPrimaryVolume() *Volume {
	return tsm.primaryVolume
}

// GetPrimaryLakeName returns the lake name of the primary volume
func (tsm *TieredStorageManager) GetPrimaryLakeName() string {
	return tsm.primaryVolume.LakeName
}

// GetAllLakeNames returns all lake names for querying
func (tsm *TieredStorageManager) GetAllLakeNames() []string {
	tsm.mu.RLock()
	defer tsm.mu.RUnlock()

	names := make([]string, len(tsm.volumes))
	for i, vol := range tsm.volumes {
		names[i] = vol.LakeName
	}
	return names
}

// GetVolumes returns all configured volumes
func (tsm *TieredStorageManager) GetVolumes() []*Volume {
	tsm.mu.RLock()
	defer tsm.mu.RUnlock()
	return tsm.volumes
}

// QueryAllVolumes executes a query across all volumes using UNION ALL
func (tsm *TieredStorageManager) QueryAllVolumes(tableName, whereClause string, limit int) ([]map[string]interface{}, error) {
	tsm.mu.RLock()
	volumes := tsm.volumes
	tsm.mu.RUnlock()

	var allResults []map[string]interface{}

	for _, vol := range volumes {
		results, err := func() ([]map[string]interface{}, error) {
			tableFQN := fmt.Sprintf("%s.main.%s", vol.LakeName, tableName)

			query := fmt.Sprintf("SELECT * FROM %s", tableFQN)
			if whereClause != "" {
				query += " WHERE " + whereClause
			}
			query += " ORDER BY timestamp DESC"
			if limit > 0 {
				query += fmt.Sprintf(" LIMIT %d", limit)
			}

			rows, err := tsm.db.Query(query)
			if err != nil {
				// Table might not exist in this volume, skip
				logger.Debug("TieredStorageManager: Query skipped for volume",
					"volume", vol.Name,
					"table", tableName,
					"error", err)
				return nil, err
			}
			defer rows.Close()

			return scanRowsToMaps(rows)
		}()
		if err != nil {
			continue
		}

		allResults = append(allResults, results...)
	}

	// Sort by timestamp DESC and apply limit
	sortResultsByTimestamp(allResults)
	if limit > 0 && len(allResults) > limit {
		allResults = allResults[:limit]
	}

	return allResults, nil
}

// MovePartition moves a partition (date) from source to destination volume
// Note: DuckDB doesn't support transactions across multiple attached databases,
// so we execute INSERT and DELETE as separate operations.
func (tsm *TieredStorageManager) MovePartition(tableName string, date string, srcVol, dstVol *Volume) error {
	srcTable := fmt.Sprintf("%s.main.%s", srcVol.LakeName, tableName)
	dstTable := fmt.Sprintf("%s.main.%s", dstVol.LakeName, tableName)

	logger.Info("TieredStorageManager: Moving partition",
		"table", tableName,
		"date", date,
		"from", srcVol.Name,
		"to", dstVol.Name)

	// Step 1: Insert into destination (cold storage) with retry for SQLite locks
	insertSQL := fmt.Sprintf(
		"INSERT INTO %s SELECT * FROM %s WHERE date = ?",
		dstTable, srcTable,
	)

	var result sql.Result
	var err error
	maxRetries := 5
	for attempt := 0; attempt < maxRetries; attempt++ {
		result, err = tsm.db.Exec(insertSQL, date)
		if err == nil {
			break
		}
		// Retry on database lock errors
		if strings.Contains(err.Error(), "database is locked") ||
			strings.Contains(err.Error(), "Failed to commit") {
			backoff := time.Duration(100*(1<<attempt)) * time.Millisecond // 100ms, 200ms, 400ms, 800ms, 1600ms
			logger.Warn("TieredStorageManager: Database locked, retrying",
				"table", tableName,
				"attempt", attempt+1,
				"backoff", backoff)
			time.Sleep(backoff)
			continue
		}
		return fmt.Errorf("failed to insert into destination: %w", err)
	}
	if err != nil {
		return fmt.Errorf("failed to insert into destination after %d retries: %w", maxRetries, err)
	}

	rowsInserted, _ := result.RowsAffected()

	// Step 2: Delete from source (hot storage) - only if insert succeeded
	deleteSQL := fmt.Sprintf(
		"DELETE FROM %s WHERE date = ?",
		srcTable,
	)
	if _, err := tsm.db.Exec(deleteSQL, date); err != nil {
		// Log warning but don't fail - data is now in both places (safe)
		logger.Warn("TieredStorageManager: Failed to delete from source after successful insert",
			"table", tableName,
			"date", date,
			"error", err)
		// Don't return error - data was copied successfully
	}

	logger.Info("TieredStorageManager: Partition moved",
		"table", tableName,
		"date", date,
		"rows", rowsInserted)

	return nil
}

// GetPartitionsOlderThan returns partitions older than the given date
func (tsm *TieredStorageManager) GetPartitionsOlderThan(vol *Volume, tableName string, cutoffDate string) ([]string, error) {
	tableFQN := fmt.Sprintf("%s.main.%s", vol.LakeName, tableName)

	query := fmt.Sprintf(`
		SELECT DISTINCT date
		FROM %s
		WHERE date < ?
		ORDER BY date
	`, tableFQN)

	rows, err := tsm.db.Query(query, cutoffDate)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var partitions []string
	for rows.Next() {
		var date string
		if err := rows.Scan(&date); err != nil {
			continue
		}
		partitions = append(partitions, date)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return partitions, nil
}

// GetTableNames returns all table names in a volume
func (tsm *TieredStorageManager) GetTableNames(vol *Volume) ([]string, error) {
	query := `
		SELECT table_name
		FROM information_schema.tables
		WHERE table_catalog = ?
		  AND table_schema = 'main'
		  AND table_name LIKE 'hep_proto_%'
	`

	rows, err := tsm.db.Query(query, vol.LakeName)
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
		tables = append(tables, tableName)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return tables, nil
}

// GetVolumeSize returns the total size of data in a volume (in bytes)
func (tsm *TieredStorageManager) GetVolumeSize(vol *Volume) (int64, error) {
	tables, err := tsm.GetTableNames(vol)
	if err != nil {
		return 0, err
	}

	var totalSize int64
	for _, tableName := range tables {
		tableFQN := fmt.Sprintf("%s.main.%s", vol.LakeName, tableName)

		// Get approximate size using row count * estimated row size
		query := fmt.Sprintf("SELECT COUNT(*) FROM %s", tableFQN)
		var rowCount int64
		if err := tsm.db.QueryRow(query).Scan(&rowCount); err != nil {
			continue
		}

		// Estimate ~500 bytes per row (typical HEP message)
		totalSize += rowCount * 500
	}

	return totalSize, nil
}

// GetOldestPartitions returns the N oldest partitions from a table
func (tsm *TieredStorageManager) GetOldestPartitions(vol *Volume, tableName string, limit int) ([]string, error) {
	tableFQN := fmt.Sprintf("%s.main.%s", vol.LakeName, tableName)

	query := fmt.Sprintf(`
		SELECT DISTINCT date 
		FROM %s 
		ORDER BY date ASC
		LIMIT %d
	`, tableFQN, limit)

	rows, err := tsm.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var partitions []string
	for rows.Next() {
		var date string
		if err := rows.Scan(&date); err != nil {
			continue
		}
		partitions = append(partitions, date)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return partitions, nil
}

// Stop stops the tiered storage manager
func (tsm *TieredStorageManager) Stop() error {
	var closeErr error
	tsm.stopOnce.Do(func() {
		close(tsm.stopChan)
		tsm.wg.Wait()
		if tsm.db != nil {
			closeErr = tsm.db.Close()
		}
	})
	return closeErr
}

// scanRowsToMaps converts sql.Rows to []map[string]interface{}
func scanRowsToMaps(rows *sql.Rows) ([]map[string]interface{}, error) {
	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	var results []map[string]interface{}
	for rows.Next() {
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			continue
		}

		row := make(map[string]interface{})
		for i, col := range columns {
			row[col] = values[i]
		}
		results = append(results, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return results, nil
}

// sortResultsByTimestamp sorts results by timestamp DESC (for merging multi-volume queries)
func sortResultsByTimestamp(results []map[string]interface{}) {
	if len(results) <= 1 {
		return
	}
	sort.Slice(results, func(i, j int) bool {
		ti, _ := results[i]["timestamp"].(time.Time)
		tj, _ := results[j]["timestamp"].(time.Time)
		return ti.After(tj) // DESC
	})
}

// fileExists checks if a file exists at the given path
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
