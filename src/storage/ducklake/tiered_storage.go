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
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/sipcapture/homer-core/src/storage/ducklake/mover"
	logger "github.com/sipcapture/homer-core/src/utils/logging"
)

// VolumeType represents the type of storage volume
type VolumeType string

const (
	VolumeTypeLocal VolumeType = "local"
	VolumeTypeS3    VolumeType = "s3"
	VolumeTypeAzure VolumeType = "azure"
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
	S3URLStyle  string

	// Azure Blob Storage configuration
	AzureAccountName      string
	AzureAccountKey       string
	AzureConnectionString string

	// OverrideDataPath passes OVERRIDE_DATA_PATH TRUE to DuckLake ATTACH; see config.VolumeConfig.
	OverrideDataPath bool

	// CatalogPath is the resolved SQLite catalog file path (set during attach).
	CatalogPath string
}

// TieredStorageConfig holds configuration for tiered storage
type TieredStorageConfig struct {
	Enable             bool
	Volumes            []Volume
	TTLMoveIntervalSec int
	MoveFactor         float64
	ConcurrentMoves    int
	MoveOnStartup      bool
	MoveEngine         string

	// Base config for catalog
	CatalogType CatalogType
	CatalogPath string

	// CatalogLocker serializes hot-catalog writes with the writer flush path.
	CatalogLocker CatalogLocker
}

// TieredStorageManager manages multiple storage volumes with automatic tiering
type TieredStorageManager struct {
	config        TieredStorageConfig
	db            *sql.DB
	volumes       []*Volume
	writers       map[string]*MultiTableWriter // lakeName -> writer
	catalogLocker CatalogLocker
	mu            sync.RWMutex
	stopChan      chan struct{}
	wg            sync.WaitGroup
	stopOnce      sync.Once

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
		catalogLocker: config.CatalogLocker,
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
	// One connection so S3 secrets/SET from attachVolume stay on every Exec
	// (pool growth would otherwise yield connections without credentials).
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	// Load DuckLake extension
	if _, err := db.Exec("LOAD ducklake;"); err != nil {
		return fmt.Errorf("failed to load ducklake extension: %w", err)
	}

	// Load SQLite extension for the DuckLake catalog file
	if _, err := db.Exec("LOAD sqlite;"); err != nil {
		return fmt.Errorf("failed to load sqlite extension: %w", err)
	}

	// Load the AWS extension so S3 secrets can use PROVIDER credential_chain
	// (resolves IAM-role / instance-profile / env credentials via the AWS SDK).
	// Must be pre-installed (run homer-core --install-extensions, or bundled
	// offline). Best-effort: static-key or local-only setups do not need it,
	// so a load failure must not block startup.
	if _, err := db.Exec("LOAD aws;"); err != nil {
		logger.Warn("TieredStorageManager: failed to load aws extension (credential_chain unavailable; run --install-extensions)", "error", err)
	}

	// Load the Azure extension so az:// volumes can attach. Best-effort, same
	// contract as LOAD aws above: local/S3-only setups do not need it.
	EnsureAzureCACertPath()
	if _, err := db.Exec("LOAD azure;"); err != nil {
		logger.Warn("TieredStorageManager: failed to load azure extension (Azure volumes unavailable; run --install-extensions)", "error", err)
	}

	// Attach each volume as a separate DuckLake
	for _, vol := range tsm.volumes {
		if err := tsm.attachVolume(vol); err != nil {
			return fmt.Errorf("failed to attach volume %s: %w", vol.Name, err)
		}
	}

	// Enable WAL on the hot catalog to reduce SQLite lock contention with the writer.
	if tsm.primaryVolume != nil && tsm.primaryVolume.CatalogPath != "" {
		if err := EnableSQLiteWALMode(tsm.primaryVolume.CatalogPath); err != nil {
			logger.Warn("TieredStorageManager: failed to enable WAL on hot catalog",
				"path", tsm.primaryVolume.CatalogPath,
				"error", err)
		}
	}

	logger.Info("TieredStorageManager started",
		"volumes", len(tsm.volumes),
		"primary_volume", tsm.primaryVolume.Name)

	return nil
}

func s3SecretName(volumeName string) string {
	return fmt.Sprintf("s3_secret_%s", volumeName)
}

func volumeS3EndpointHost(endpoint string) string {
	endpoint = strings.TrimSpace(endpoint)
	endpoint = strings.TrimPrefix(endpoint, "http://")
	endpoint = strings.TrimPrefix(endpoint, "https://")
	return endpoint
}

// usesS3CredentialChain is the native-AWS branch of buildS3SecretSQL: empty
// static key and no custom endpoint, so DuckDB resolves IMDS / IRSA / env.
func usesS3CredentialChain(accessKey, endpoint string) bool {
	return strings.TrimSpace(accessKey) == "" && strings.TrimSpace(endpoint) == ""
}

func s3SecretSQLForVolume(vol *Volume, replace bool) string {
	endpoint := volumeS3EndpointHost(vol.S3Endpoint)
	region := strings.TrimSpace(vol.S3Region)
	if region == "" && endpoint != "" {
		region = "us-east-1"
	}
	sql := buildS3SecretSQL(s3SecretName(vol.Name), vol.S3AccessKey, vol.S3SecretKey, region, endpoint, vol.S3UseSSL, vol.S3URLStyle)
	if replace {
		sql = strings.Replace(sql, "CREATE SECRET", "CREATE OR REPLACE SECRET", 1)
	}
	return sql
}

func (tsm *TieredStorageManager) createVolumeS3Secret(vol *Volume, replace bool) error {
	secretName := s3SecretName(vol.Name)
	if !replace {
		dropSecret := fmt.Sprintf("DROP SECRET IF EXISTS %s;", secretName)
		if _, err := tsm.db.Exec(dropSecret); err != nil {
			logger.Warn("TieredStorageManager: Failed to drop existing secret", "secret", secretName, "error", err)
		}
	}

	endpoint := volumeS3EndpointHost(vol.S3Endpoint)
	createSecret := s3SecretSQLForVolume(vol, replace)
	if replace {
		logger.Debug("TieredStorageManager: Refreshing credential_chain S3 secret",
			"volume", vol.Name)
	} else {
		logger.Info("TieredStorageManager: Creating S3 secret",
			"volume", vol.Name,
			"endpoint", endpoint,
			"use_ssl", vol.S3UseSSL)
	}

	if _, err := tsm.db.Exec(createSecret); err != nil {
		return fmt.Errorf("failed to create S3 secret for volume %s: %w", vol.Name, err)
	}
	return nil
}

func azureSecretName(volumeName string) string {
	return fmt.Sprintf("azure_secret_%s", volumeName)
}

// usesAzureCredentialChain is the ambient-identity branch of
// buildAzureSecretSQL: no static account key and no connection string, so
// DuckDB resolves credentials through the Azure SDK default chain (env,
// workload identity, managed identity, Azure CLI).
func usesAzureCredentialChain(accountKey, connectionString string) bool {
	return strings.TrimSpace(accountKey) == "" && strings.TrimSpace(connectionString) == ""
}

func azureSecretSQLForVolume(vol *Volume, replace bool) string {
	sql := buildAzureSecretSQL(azureSecretName(vol.Name), vol.AzureAccountName, vol.AzureAccountKey, vol.AzureConnectionString)
	if replace {
		sql = strings.Replace(sql, "CREATE SECRET", "CREATE OR REPLACE SECRET", 1)
	}
	return sql
}

func (tsm *TieredStorageManager) createVolumeAzureSecret(vol *Volume, replace bool) error {
	secretName := azureSecretName(vol.Name)
	if !replace {
		dropSecret := fmt.Sprintf("DROP SECRET IF EXISTS %s;", secretName)
		if _, err := tsm.db.Exec(dropSecret); err != nil {
			logger.Warn("TieredStorageManager: Failed to drop existing secret", "secret", secretName, "error", err)
		}
	}

	createSecret := azureSecretSQLForVolume(vol, replace)
	if replace {
		logger.Debug("TieredStorageManager: Refreshing credential_chain Azure secret",
			"volume", vol.Name)
	} else {
		logger.Info("TieredStorageManager: Creating Azure secret",
			"volume", vol.Name,
			"account_name", vol.AzureAccountName)
	}

	if _, err := tsm.db.Exec(createSecret); err != nil {
		return fmt.Errorf("failed to create Azure secret for volume %s: %w", vol.Name, err)
	}
	return nil
}

// refreshCredentialChainSecret re-resolves a role-based S3 secret so DuckDB
// does not keep the session token captured at CREATE SECRET / process start.
// REFRESH auto is the DuckDB-side counterpart, but S3 ExpiredToken is HTTP 400
// and may not trigger that retry; native register and volume maintenance
// therefore recreate the secret explicitly. No-op for static keys / MinIO.
func (tsm *TieredStorageManager) refreshCredentialChainSecret(vol *Volume) error {
	if tsm == nil || tsm.db == nil || vol == nil {
		return nil
	}
	switch vol.Type {
	case VolumeTypeS3:
		if !usesS3CredentialChain(vol.S3AccessKey, volumeS3EndpointHost(vol.S3Endpoint)) {
			return nil
		}
		return tsm.createVolumeS3Secret(vol, true)
	case VolumeTypeAzure:
		if !usesAzureCredentialChain(vol.AzureAccountKey, vol.AzureConnectionString) {
			return nil
		}
		return tsm.createVolumeAzureSecret(vol, true)
	default:
		return nil
	}
}

// RefreshCredentialChainSecrets re-creates DuckDB S3 secrets that use
// PROVIDER credential_chain. Call at the start of a tiering cycle so COUNT,
// expire, native register, and volume maintenance do not use a token resolved
// at process start (sipcapture/homer#980).
func (tsm *TieredStorageManager) RefreshCredentialChainSecrets() {
	tsm.refreshCredentialChainSecrets()
}

func (tsm *TieredStorageManager) refreshCredentialChainSecrets() {
	if tsm == nil {
		return
	}
	for _, vol := range tsm.volumes {
		if err := tsm.refreshCredentialChainSecret(vol); err != nil {
			logger.Warn("TieredStorageManager: failed to refresh credential_chain S3 secret",
				"volume", vol.Name, "error", err)
		}
	}
}

// attachVolume attaches a volume as a DuckLake database
func (tsm *TieredStorageManager) attachVolume(vol *Volume) error {
	// Configure S3 / Azure secret if needed
	switch vol.Type {
	case VolumeTypeS3:
		if err := tsm.createVolumeS3Secret(vol, false); err != nil {
			return err
		}
	case VolumeTypeAzure:
		if err := tsm.createVolumeAzureSecret(vol, false); err != nil {
			return err
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
	overrideOpt := ""
	if vol.OverrideDataPath {
		overrideOpt = ", OVERRIDE_DATA_PATH TRUE"
	}
	attachSQL := fmt.Sprintf(
		"ATTACH 'ducklake:sqlite:%s' AS %s (DATA_PATH '%s', AUTOMATIC_MIGRATION TRUE%s);",
		catalogPath, vol.LakeName, vol.Path, overrideOpt,
	)

	if _, err := tsm.db.Exec(attachSQL); err != nil {
		return fmt.Errorf("failed to attach DuckLake for volume %s: %w", vol.Name, err)
	}

	vol.CatalogPath = catalogPath

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

// MovePartition moves a partition (date) from source to destination volume.
// DuckDB does not support transactions across multiple attached databases, so
// INSERT and DELETE run as separate operations with idempotency on the destination.
//
// The default path is DuckDB INSERT…SELECT. Set move_engine=native to copy
// parquet files outside DuckDB so a large S3 copy does not hold the writer
// catalog lock (sipcapture/homer#969). Native falls back to INSERT…SELECT.
func (tsm *TieredStorageManager) MovePartition(tableName string, date string, srcVol, dstVol *Volume) error {
	srcTable := fmt.Sprintf("%s.main.%s", srcVol.LakeName, tableName)
	dstTable := fmt.Sprintf("%s.main.%s", dstVol.LakeName, tableName)

	logger.Info("TieredStorageManager: Moving partition",
		"table", tableName,
		"date", date,
		"from", srcVol.Name,
		"to", dstVol.Name,
		"engine", tsm.moveEngine())

	// DuckDB COUNT / INSERT / add_data_files on the cold lake use the session
	// secret, not the AWS SDK copier. Re-resolve role credentials first.
	if err := tsm.refreshCredentialChainSecret(dstVol); err != nil {
		logger.Warn("TieredStorageManager: failed to refresh destination S3 secret",
			"volume", dstVol.Name, "error", err)
	}

	hotLocker := tsm.hotCatalogLocker(srcVol)

	srcCount, err := tsm.partitionRowCount(srcTable, date)
	if err != nil {
		return fmt.Errorf("failed to count source partition rows: %w", err)
	}

	dstCount, err := tsm.partitionRowCount(dstTable, date)
	if err != nil {
		return fmt.Errorf("failed to count destination partition rows: %w", err)
	}

	if srcCount == 0 {
		if dstCount > 0 {
			logger.Info("TieredStorageManager: Partition already on destination",
				"table", tableName,
				"date", date,
				"rows", dstCount)
			return nil
		}
		return nil
	}

	if dstCount > 0 {
		if dstCount != srcCount {
			logger.Warn("TieredStorageManager: Destination row count differs from source; delete-only retry",
				"table", tableName,
				"date", date,
				"source_rows", srcCount,
				"destination_rows", dstCount)
		} else {
			logger.Info("TieredStorageManager: Destination already has partition; retrying source delete only",
				"table", tableName,
				"date", date,
				"rows", dstCount)
		}
		return tsm.deleteSourcePartition(srcTable, date, tableName, srcCount, dstCount, hotLocker)
	}

	if tsm.useNativeMove(srcVol, dstVol) {
		err := tsm.movePartitionNative(tableName, date, srcVol, dstVol)
		if err == nil {
			return nil
		}
		if !errors.Is(err, mover.ErrFallback) {
			return err
		}
		logger.Info("TieredStorageManager: native file move unavailable, using duckdb INSERT",
			"table", tableName,
			"date", date,
			"reason", err)
	}

	return tsm.movePartitionDuckDB(srcTable, dstTable, tableName, date, srcCount, hotLocker)
}

func (tsm *TieredStorageManager) MoveEngine() string {
	if tsm == nil {
		return "duckdb"
	}
	if strings.EqualFold(strings.TrimSpace(tsm.config.MoveEngine), "native") {
		return "native"
	}
	return "duckdb"
}

func (tsm *TieredStorageManager) moveEngine() string {
	return tsm.MoveEngine()
}

func (tsm *TieredStorageManager) useNativeMove(srcVol, dstVol *Volume) bool {
	if tsm.moveEngine() != "native" {
		return false
	}
	if srcVol == nil || dstVol == nil {
		return false
	}
	if IsRemoteLakeDataPath(srcVol.Path) {
		return false
	}
	return true
}

func (tsm *TieredStorageManager) nativeMoveOptions(tableName, date string, srcVol, dstVol *Volume) mover.Options {
	opts := mover.Options{
		DB:          tsm.db,
		SrcLake:     srcVol.LakeName,
		DstLake:     dstVol.LakeName,
		SrcDataPath: srcVol.Path,
		DstDataPath: dstVol.Path,
		TableName:   tableName,
		Partition:   date,
	}
	if locker := tsm.hotCatalogLocker(srcVol); locker != nil {
		opts.Lock = locker.CatalogLock
		opts.Unlock = locker.CatalogUnlock
	}
	switch {
	case dstVol.Type == VolumeTypeS3 || isS3Path(dstVol.Path):
		opts.S3 = &mover.S3Config{
			Region:    dstVol.S3Region,
			AccessKey: dstVol.S3AccessKey,
			SecretKey: dstVol.S3SecretKey,
			Endpoint:  dstVol.S3Endpoint,
			URLStyle:  dstVol.S3URLStyle,
			UseSSL:    dstVol.S3UseSSL,
		}
		// PUT uses the AWS SDK (auto-refresh). Register is DuckDB read_blob /
		// add_data_files against the secret captured at start — recreate it
		// after a long copy so ExpiredToken cannot land on register.
		dst := dstVol
		opts.BeforeRegister = func() error {
			return tsm.refreshCredentialChainSecret(dst)
		}
	case dstVol.Type == VolumeTypeAzure || isAzurePath(dstVol.Path):
		opts.Azure = &mover.AzureConfig{
			AccountName:      dstVol.AzureAccountName,
			AccountKey:       dstVol.AzureAccountKey,
			ConnectionString: dstVol.AzureConnectionString,
		}
		// Same rationale as the S3 branch above: the Azure SDK credential
		// chain can outlive a Managed Identity token over a long copy;
		// recreate the DuckDB secret before register so it isn't stale.
		dst := dstVol
		opts.BeforeRegister = func() error {
			return tsm.refreshCredentialChainSecret(dst)
		}
	}
	return opts
}

func (tsm *TieredStorageManager) movePartitionNative(tableName, date string, srcVol, dstVol *Volume) error {
	opts := tsm.nativeMoveOptions(tableName, date, srcVol, dstVol)

	res, err := mover.Move(context.Background(), opts)
	if err != nil {
		return err
	}
	logger.Info("TieredStorageManager: Partition moved",
		"table", tableName,
		"date", date,
		"rows", res.RowsMoved,
		"files", res.FilesCopied,
		"bytes", res.BytesCopied,
		"engine", "native")
	return nil
}

func (tsm *TieredStorageManager) movePartitionDuckDB(srcTable, dstTable, tableName, date string, srcCount int64, hotLocker CatalogLocker) error {
	insertSQL := fmt.Sprintf(
		"INSERT INTO %s SELECT * FROM %s WHERE date = ?",
		dstTable, srcTable,
	)
	if _, err := execWithRetry(
		tsm.db,
		tieringMaxRetries,
		tieringBaseBackoff,
		hotLocker,
		insertSQL,
		date,
	); err != nil {
		return fmt.Errorf("failed to insert into destination: %w", err)
	}
	return tsm.deleteSourcePartition(srcTable, date, tableName, srcCount, 0, hotLocker)
}

func (tsm *TieredStorageManager) deleteSourcePartition(srcTable, date, tableName string, srcCount, dstCount int64, hotLocker CatalogLocker) error {
	deleteSQL := fmt.Sprintf("DELETE FROM %s WHERE date = ?", srcTable)
	if _, err := execWithRetry(
		tsm.db,
		tieringMaxRetries,
		tieringBaseBackoff,
		hotLocker,
		deleteSQL,
		date,
	); err != nil {
		rowsCopied := srcCount
		if dstCount > 0 {
			rowsCopied = dstCount
		}
		logger.Warn("TieredStorageManager: Partition copied, source delete pending",
			"table", tableName,
			"date", date,
			"rows", rowsCopied,
			"error", err)
		return fmt.Errorf("source delete failed after copy: %w", err)
	}

	logger.Info("TieredStorageManager: Partition moved",
		"table", tableName,
		"date", date,
		"rows", srcCount)

	return nil
}

// DeletePartition permanently removes a date partition from the given volume.
// Used to enforce max_data_age_days on the final storage-policy volume (no next tier).
func (tsm *TieredStorageManager) DeletePartition(tableName string, date string, vol *Volume) error {
	tableFQN := fmt.Sprintf("%s.main.%s", vol.LakeName, tableName)

	count, err := tsm.partitionRowCount(tableFQN, date)
	if err != nil {
		return fmt.Errorf("failed to count partition rows: %w", err)
	}
	if count == 0 {
		logger.Debug("TieredStorageManager: Partition already empty, skip expire",
			"table", tableName,
			"date", date,
			"volume", vol.Name)
		return nil
	}

	logger.Info("TieredStorageManager: Expiring partition",
		"table", tableName,
		"date", date,
		"volume", vol.Name,
		"rows", count)

	deleteSQL := fmt.Sprintf("DELETE FROM %s WHERE date = ?", tableFQN)
	if _, err := execWithRetry(
		tsm.db,
		tieringMaxRetries,
		tieringBaseBackoff,
		tsm.hotCatalogLocker(vol),
		deleteSQL,
		date,
	); err != nil {
		return fmt.Errorf("failed to delete partition: %w", err)
	}

	logger.Info("TieredStorageManager: Partition expired",
		"table", tableName,
		"date", date,
		"volume", vol.Name,
		"rows", count)
	return nil
}

// volumeMaintenanceSQL returns the DuckLake maintenance calls that physically
// reclaim storage on a volume's lake: expire snapshots referencing deleted
// files, then delete the superseded/orphaned parquet objects (local or S3).
func volumeMaintenanceSQL(lakeName string, snapshotOlderThanSec int) []string {
	if snapshotOlderThanSec <= 0 {
		snapshotOlderThanSec = 3600
	}
	return []string{
		fmt.Sprintf(
			"CALL ducklake_expire_snapshots('%s', older_than => CAST(NOW() - INTERVAL '%d seconds' AS TIMESTAMPTZ))",
			lakeName, snapshotOlderThanSec,
		),
		fmt.Sprintf("CALL ducklake_cleanup_old_files('%s', cleanup_all => true)", lakeName),
		fmt.Sprintf("CALL ducklake_delete_orphaned_files('%s', cleanup_all => true)", lakeName),
	}
}

// RunVolumeMaintenance expires old snapshots and deletes obsolete parquet
// files on the given volume's lake. Without this, DELETEs performed by
// tiering (move source delete, final-volume TTL expire) only mark files
// obsolete in the DuckLake catalog while the objects stay on disk/S3
// (issue #882 follow-up: cold physical space was never reclaimed because
// the writer CompactionService only maintains the writer lake).
func (tsm *TieredStorageManager) RunVolumeMaintenance(vol *Volume, snapshotOlderThanSec int) error {
	if err := tsm.refreshCredentialChainSecret(vol); err != nil {
		logger.Warn("TieredStorageManager: failed to refresh S3 secret before maintenance",
			"volume", vol.Name, "error", err)
	}

	locker := tsm.hotCatalogLocker(vol)

	logger.Info("TieredStorageManager: Running volume maintenance",
		"volume", vol.Name,
		"lake", vol.LakeName,
		"snapshot_older_than_sec", snapshotOlderThanSec)

	var firstErr error
	for _, stmt := range volumeMaintenanceSQL(vol.LakeName, snapshotOlderThanSec) {
		if _, err := execWithRetry(
			tsm.db,
			tieringMaxRetries,
			tieringBaseBackoff,
			locker,
			stmt,
		); err != nil {
			logger.Warn("TieredStorageManager: Volume maintenance step failed",
				"volume", vol.Name,
				"lake", vol.LakeName,
				"sql", stmt,
				"error", err)
			if firstErr == nil {
				firstErr = err
			}
		}
	}

	if firstErr == nil {
		logger.Info("TieredStorageManager: Volume maintenance completed",
			"volume", vol.Name,
			"lake", vol.LakeName)
	}
	return firstErr
}

func (tsm *TieredStorageManager) hotCatalogLocker(srcVol *Volume) CatalogLocker {
	if tsm.primaryVolume != nil && srcVol.LakeName == tsm.primaryVolume.LakeName {
		return tsm.catalogLocker
	}
	return nil
}

func (tsm *TieredStorageManager) partitionRowCount(tableFQN, date string) (int64, error) {
	query := fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE date = ?", tableFQN)
	var count int64
	if err := tsm.db.QueryRow(query, date).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

// GetPartitionsOlderThan returns distinct partition dates that should be
// tiered off the volume: those with date on or before cutoffDate (inclusive).
// Callers pass cutoffDate = calendar(today) minus MaxDataAgeDays, so for
// max_data_age_days=1 on 2026-05-12 the cutoff is 2026-05-11 and the partition
// date=2026-05-11 is included (yesterday's data can move to cold).
func (tsm *TieredStorageManager) GetPartitionsOlderThan(vol *Volume, tableName string, cutoffDate string) ([]string, error) {
	tableFQN := fmt.Sprintf("%s.main.%s", vol.LakeName, tableName)

	query := fmt.Sprintf(`
		SELECT DISTINCT date
		FROM %s
		WHERE date <= ?
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

// buildS3SecretSQL returns the CREATE SECRET SQL for an S3 volume.
// When accessKey is empty and endpoint is empty (native AWS S3), the secret
// uses PROVIDER credential_chain so DuckDB resolves credentials through the
// AWS SDK default chain (env, container/Pod Identity, instance profile).
// REFRESH auto re-runs that chain when the cached session token expires
// (EC2 instance-profile tokens last ~6h; IRSA / Pod Identity are shorter);
// without it the secret is resolved once at CREATE SECRET and every later
// cold-volume operation fails with ExpiredToken until homer-core restarts.
// Static keys and custom endpoints (MinIO / R2) use explicit KEY_ID/SECRET.
func buildS3SecretSQL(secretName, accessKey, secretKey, region, endpoint string, useSSL bool, urlStyle string) string {
	switch {
	case usesS3CredentialChain(accessKey, endpoint):
		// Default region for native AWS S3 so the secret signs correctly even
		// when no s3_region is set (the SDK can also resolve it from the
		// environment / instance metadata, but an explicit value is safer).
		if strings.TrimSpace(region) == "" {
			region = "us-east-1"
		}
		return fmt.Sprintf(`
			CREATE SECRET %s (
				TYPE S3,
				PROVIDER credential_chain,
				REFRESH auto,
				REGION '%s'
			);
		`, secretName, region)
	case endpoint != "":
		return fmt.Sprintf(`
			CREATE SECRET %s (
				TYPE S3,
				KEY_ID '%s',
				SECRET '%s',
				REGION '%s',
				ENDPOINT '%s',
				URL_STYLE '%s',
				USE_SSL %t
			);
		`, secretName, accessKey, secretKey, region, endpoint, strings.ReplaceAll(NormalizeS3URLStyle(urlStyle), "'", "''"), useSSL)
	default:
		return fmt.Sprintf(`
			CREATE SECRET %s (
				TYPE S3,
				KEY_ID '%s',
				SECRET '%s',
				REGION '%s'
			);
		`, secretName, accessKey, secretKey, region)
	}
}

// buildAzureSecretSQL returns the CREATE SECRET SQL for an Azure Blob volume.
//
// Verified against DuckDB's azure extension directly (v1.5.5, the version
// this repo bundles): the PROVIDER config secret has no ACCOUNT_KEY
// parameter — a bare account name + key can only be authenticated via a full
// CONNECTION_STRING, so when the caller supplies AccountKey (not a raw
// ConnectionString) this function assembles one. REFRESH auto is not a valid
// parameter for azure secrets of any provider (unlike S3) — credential_chain
// secrets are kept fresh the same way Homer already refreshes S3
// credential_chain secrets: by dropping and re-CREATE-ing them before each
// tiering cycle (see refreshCredentialChainSecret), not by a SQL-level
// REFRESH clause.
func buildAzureSecretSQL(secretName, accountName, accountKey, connectionString string) string {
	connStr := strings.TrimSpace(connectionString)
	if connStr == "" && strings.TrimSpace(accountKey) != "" {
		connStr = fmt.Sprintf(
			"DefaultEndpointsProtocol=https;AccountName=%s;AccountKey=%s;EndpointSuffix=core.windows.net",
			accountName, accountKey,
		)
	}
	switch {
	case connStr != "":
		return fmt.Sprintf(`
			CREATE SECRET %s (
				TYPE azure,
				CONNECTION_STRING '%s'
			);
		`, secretName, strings.ReplaceAll(connStr, "'", "''"))
	default:
		// No key, no connection string: ambient identity via the Azure SDK's
		// default credential chain. env/cli let a developer override locally;
		// managed_identity is what resolves in production on an Azure VM.
		return fmt.Sprintf(`
			CREATE SECRET %s (
				TYPE azure,
				PROVIDER credential_chain,
				CHAIN 'env;managed_identity;cli',
				ACCOUNT_NAME '%s'
			);
		`, secretName, strings.ReplaceAll(accountName, "'", "''"))
	}
}

// fileExists checks if a file exists at the given path
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
