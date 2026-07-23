// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

// Package writer provides the TieringService for automatic data movement
// between storage volumes based on age policies.
package writer

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/sipcapture/homer-core/src/storage/ducklake"
	logger "github.com/sipcapture/homer-core/src/utils/logging"
)

// TieringConfig configures the tiering service
type TieringConfig struct {
	Enable           bool
	CheckIntervalSec int
	ConcurrentMoves  int
	MoveOnStartup    bool
	MoveFactor       float64 // Move data when volume fill ratio exceeds this value (0.0-1.0)
	// SnapshotExpireSec is the snapshot retention window used when running
	// DuckLake maintenance on tiered volumes after moves/expires (mirrors
	// compaction.snapshot_expire_interval_sec; 0 = default 3600).
	SnapshotExpireSec int
}

// TieringService manages automatic data tiering between storage volumes
type TieringService struct {
	config        TieringConfig
	tieredStorage *ducklake.TieredStorageManager
	stopChan      chan struct{}
	wg            sync.WaitGroup
	mu            sync.Mutex
	isRunning     bool
	stopOnce      sync.Once
}

// NewTieringService creates a new tiering service
func NewTieringService(config TieringConfig, tieredStorage *ducklake.TieredStorageManager) *TieringService {
	return &TieringService{
		config:        config,
		tieredStorage: tieredStorage,
		stopChan:      make(chan struct{}),
	}
}

// Start starts the tiering service
func (ts *TieringService) Start() error {
	if ts.tieredStorage == nil {
		return fmt.Errorf("tiered storage manager is not configured")
	}

	volumes := ts.tieredStorage.GetVolumes()
	if len(volumes) < 2 {
		logger.Info("TieringService: Not enough volumes for tiering (need at least 2)")
		return nil
	}

	interval := time.Duration(ts.config.CheckIntervalSec) * time.Second
	if interval == 0 {
		interval = time.Hour
	}

	logger.Info("TieringService: Starting",
		"interval", interval.String(),
		"concurrent_moves", ts.config.ConcurrentMoves,
		"volumes", len(volumes))

	// Run initial tiering if configured
	if ts.config.MoveOnStartup {
		go func() {
			time.Sleep(10 * time.Second) // Wait for system to stabilize
			ts.runTieringCycle()
		}()
	}

	// Start background tiering loop
	ts.wg.Add(1)
	go ts.tieringLoop(interval)

	return nil
}

// tieringLoop runs the periodic tiering check
func (ts *TieringService) tieringLoop(interval time.Duration) {
	defer ts.wg.Done()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ts.stopChan:
			logger.Info("TieringService: Stopping tiering loop")
			return
		case <-ticker.C:
			ts.runTieringCycle()
		}
	}
}

// runTieringCycle executes one tiering cycle
func (ts *TieringService) runTieringCycle() {
	ts.mu.Lock()
	if ts.isRunning {
		ts.mu.Unlock()
		logger.Debug("TieringService: Cycle already running, skipping")
		return
	}
	ts.isRunning = true
	ts.mu.Unlock()

	defer func() {
		ts.mu.Lock()
		ts.isRunning = false
		ts.mu.Unlock()
	}()

	startTime := time.Now()
	logger.Info("TieringService: Starting tiering cycle")

	volumes := ts.tieredStorage.GetVolumes()
	var totalMoved int64
	var totalExpired int64

	// Process each volume pair (hot -> next volume based on priority)
	for i := 0; i < len(volumes)-1; i++ {
		srcVol := volumes[i]
		dstVol := volumes[i+1]

		var moved int64
		var err error

		// Check if volume exceeds move_factor threshold (size-based move)
		if srcVol.MaxSizeGB > 0 && ts.config.MoveFactor > 0 && ts.config.MoveFactor < 1.0 {
			needsSizeMove, currentSizeGB := ts.checkVolumeSizeThreshold(srcVol)
			if needsSizeMove {
				logger.Info("TieringService: Volume exceeds move_factor threshold",
					"volume", srcVol.Name,
					"current_size_gb", currentSizeGB,
					"max_size_gb", srcVol.MaxSizeGB,
					"move_factor", ts.config.MoveFactor,
					"threshold_gb", float64(srcVol.MaxSizeGB)*ts.config.MoveFactor)

				moved, err = ts.moveOldestPartitions(srcVol, dstVol, currentSizeGB)
				if err != nil {
					logger.Error("TieringService: Failed to move partitions by size",
						"source", srcVol.Name,
						"dest", dstVol.Name,
						"error", err)
				}
				totalMoved += moved
			}
		}

		// TTL-based move (age)
		// max_data_age_days: 0 = disabled (no age-based tiering)
		// max_data_age_days: N = move data older than N days
		if srcVol.MaxDataAgeDays > 0 {
			moved, err = ts.moveOldPartitions(srcVol, dstVol)
			if err != nil {
				logger.Error("TieringService: Failed to move partitions by age",
					"source", srcVol.Name,
					"dest", dstVol.Name,
					"error", err)
				continue
			}
			totalMoved += moved
		}
	}

	// Final volume: max_data_age_days expires partitions (no next tier to move to).
	if last := finalVolumeForExpire(volumes); last != nil {
		expired, err := ts.expireOldPartitions(last)
		if err != nil {
			logger.Error("TieringService: Failed to expire partitions on final volume",
				"source", last.Name,
				"error", err)
		} else {
			totalExpired += expired
		}
	}

	// Tiering DELETEs (move source delete, final-volume TTL expire) only mark
	// parquet files as deleted in the DuckLake catalog — the objects stay on
	// disk/S3 until snapshots are expired and cleanup runs on that lake. The
	// writer CompactionService only maintains the writer lake, so run
	// per-volume maintenance every cycle: this also drains obsolete backlog
	// from earlier cycles even when nothing moved this time (#882).
	for _, vol := range volumes {
		if err := ts.tieredStorage.RunVolumeMaintenance(vol, ts.config.SnapshotExpireSec); err != nil {
			logger.Error("TieringService: Volume maintenance failed",
				"volume", vol.Name,
				"error", err)
		}
	}

	// Cleanup empty partition directories after maintenance removed files
	for _, vol := range volumes {
		if vol.Type == ducklake.VolumeTypeLocal {
			ts.cleanupEmptyPartitions(vol.Path)
		}
	}

	duration := time.Since(startTime)
	logger.Info("TieringService: Tiering cycle completed",
		"duration", duration.String(),
		"partitions_moved", totalMoved,
		"partitions_expired", totalExpired)
}

// checkVolumeSizeThreshold checks if volume size exceeds move_factor threshold
func (ts *TieringService) checkVolumeSizeThreshold(vol *ducklake.Volume) (bool, float64) {
	sizeBytes, err := ts.tieredStorage.GetVolumeSize(vol)
	if err != nil {
		logger.Warn("TieringService: Failed to get volume size",
			"volume", vol.Name,
			"error", err)
		return false, 0
	}

	currentSizeGB := float64(sizeBytes) / (1024 * 1024 * 1024)
	thresholdGB := float64(vol.MaxSizeGB) * ts.config.MoveFactor

	return currentSizeGB > thresholdGB, currentSizeGB
}

// moveOldestPartitions moves oldest partitions until volume is below threshold
func (ts *TieringService) moveOldestPartitions(srcVol, dstVol *ducklake.Volume, currentSizeGB float64) (int64, error) {
	targetSizeGB := float64(srcVol.MaxSizeGB) * ts.config.MoveFactor * 0.9 // Target 90% of threshold
	bytesToMove := int64((currentSizeGB - targetSizeGB) * 1024 * 1024 * 1024)

	if bytesToMove <= 0 {
		return 0, nil
	}

	logger.Info("TieringService: Moving oldest partitions by size",
		"source", srcVol.Name,
		"dest", dstVol.Name,
		"bytes_to_move", bytesToMove)

	// Get all tables
	tables, err := ts.tieredStorage.GetTableNames(srcVol)
	if err != nil {
		return 0, fmt.Errorf("failed to get table names: %w", err)
	}

	var totalMoved int64
	var movedBytes int64
	semaphore := make(chan struct{}, ts.config.ConcurrentMoves)
	var wg sync.WaitGroup
	var mu sync.Mutex

	for _, tableName := range tables {
		if movedBytes >= bytesToMove {
			break
		}

		// Get oldest partitions first
		partitions, err := ts.tieredStorage.GetOldestPartitions(srcVol, tableName, 10)
		if err != nil {
			continue
		}

		for _, partition := range partitions {
			if movedBytes >= bytesToMove {
				break
			}

			wg.Add(1)
			semaphore <- struct{}{}

			go func(table, date string) {
				defer wg.Done()
				defer func() { <-semaphore }()

				if err := ts.ensureTableExists(srcVol, dstVol, table); err != nil {
					logger.Error("TieringService: Failed to ensure table exists",
						"table", table,
						"error", err)
					return
				}

				if err := ts.tieredStorage.MovePartition(table, date, srcVol, dstVol); err != nil {
					logger.Error("TieringService: Failed to move partition",
						"table", table,
						"date", date,
						"error", err)
					return
				}

				mu.Lock()
				totalMoved++
				movedBytes += 100 * 1024 * 1024 // Estimate 100MB per partition
				mu.Unlock()
			}(tableName, partition)
		}
	}

	wg.Wait()
	return totalMoved, nil
}

// moveOldPartitions moves partitions older than max_data_age_days from source to destination
func (ts *TieringService) moveOldPartitions(srcVol, dstVol *ducklake.Volume) (int64, error) {
	cutoffDate := time.Now().AddDate(0, 0, -srcVol.MaxDataAgeDays).Format("2006-01-02")

	logger.Info("TieringService: TTL partition scan",
		"source", srcVol.Name,
		"dest", dstVol.Name,
		"max_data_age_days", srcVol.MaxDataAgeDays,
		"partition_date_cutoff", cutoffDate,
		"rule", "partition date <= cutoff (inclusive); cutoff is calendar today minus max_data_age_days")

	// Get all tables in source volume
	tables, err := ts.tieredStorage.GetTableNames(srcVol)
	if err != nil {
		return 0, fmt.Errorf("failed to get table names: %w", err)
	}
	if len(tables) == 0 {
		logger.Info("TieringService: No hep_proto* tables on tiered source volume — nothing to move by age",
			"source", srcVol.Name,
			"lake", srcVol.LakeName,
			"hint", "Tiering runs on the TieredStorageManager DuckDB; if that catalog has no tables while ingest uses the writer DuckLake only, hot tier stays empty until writer and tiered paths share the same catalog data.")
		return 0, nil
	}

	var totalMoved int64
	semaphore := make(chan struct{}, ts.config.ConcurrentMoves)
	var wg sync.WaitGroup
	var mu sync.Mutex

	for _, tableName := range tables {
		// Get old partitions for this table
		partitions, err := ts.tieredStorage.GetPartitionsOlderThan(srcVol, tableName, cutoffDate)
		if err != nil {
			logger.Warn("TieringService: Failed to get partitions",
				"table", tableName,
				"error", err)
			continue
		}

		if len(partitions) == 0 {
			continue
		}

		logger.Info("TieringService: Found old partitions",
			"table", tableName,
			"count", len(partitions),
			"dates", partitions)

		for _, partition := range partitions {
			wg.Add(1)
			semaphore <- struct{}{} // Acquire semaphore

			go func(table, date string) {
				defer wg.Done()
				defer func() { <-semaphore }() // Release semaphore

				// Ensure destination table exists
				if err := ts.ensureTableExists(srcVol, dstVol, table); err != nil {
					logger.Error("TieringService: Failed to ensure table exists",
						"table", table,
						"error", err)
					return
				}

				// Move the partition
				if err := ts.tieredStorage.MovePartition(table, date, srcVol, dstVol); err != nil {
					logger.Error("TieringService: Failed to move partition",
						"table", table,
						"date", date,
						"error", err)
					return
				}

				mu.Lock()
				totalMoved++
				mu.Unlock()
			}(tableName, partition)
		}
	}

	wg.Wait()
	if totalMoved == 0 {
		logger.Info("TieringService: TTL pass completed with no moves",
			"source", srcVol.Name,
			"tables_checked", len(tables),
			"partition_date_cutoff", cutoffDate,
			"hint", "No partition dates on or before cutoff; increase max_data_age_days or wait for older calendar partitions.")
	}
	return totalMoved, nil
}

// finalVolumeForExpire returns the last storage-policy volume when it has a
// positive max_data_age_days (TTL expire). Intermediate volumes only move data;
// the final volume has no destination, so age TTL deletes partitions instead.
func finalVolumeForExpire(volumes []*ducklake.Volume) *ducklake.Volume {
	if len(volumes) == 0 {
		return nil
	}
	last := volumes[len(volumes)-1]
	if last == nil || last.MaxDataAgeDays <= 0 {
		return nil
	}
	return last
}

// expireOldPartitions deletes partitions older than max_data_age_days from the
// final storage-policy volume (there is no next tier to move into).
func (ts *TieringService) expireOldPartitions(vol *ducklake.Volume) (int64, error) {
	cutoffDate := time.Now().AddDate(0, 0, -vol.MaxDataAgeDays).Format("2006-01-02")

	logger.Info("TieringService: TTL partition expire scan",
		"source", vol.Name,
		"max_data_age_days", vol.MaxDataAgeDays,
		"partition_date_cutoff", cutoffDate,
		"rule", "partition date <= cutoff (inclusive); cutoff is calendar today minus max_data_age_days; final volume expires instead of moving")

	tables, err := ts.tieredStorage.GetTableNames(vol)
	if err != nil {
		return 0, fmt.Errorf("failed to get table names: %w", err)
	}
	if len(tables) == 0 {
		logger.Info("TieringService: No hep_proto* tables on final volume — nothing to expire by age",
			"source", vol.Name,
			"lake", vol.LakeName)
		return 0, nil
	}

	var totalExpired int64
	semaphore := make(chan struct{}, ts.config.ConcurrentMoves)
	var wg sync.WaitGroup
	var mu sync.Mutex

	for _, tableName := range tables {
		partitions, err := ts.tieredStorage.GetPartitionsOlderThan(vol, tableName, cutoffDate)
		if err != nil {
			logger.Warn("TieringService: Failed to get partitions for expire",
				"table", tableName,
				"error", err)
			continue
		}
		if len(partitions) == 0 {
			continue
		}

		logger.Info("TieringService: Found expired partitions",
			"table", tableName,
			"count", len(partitions),
			"dates", partitions)

		for _, partition := range partitions {
			wg.Add(1)
			semaphore <- struct{}{}

			go func(table, date string) {
				defer wg.Done()
				defer func() { <-semaphore }()

				if err := ts.tieredStorage.DeletePartition(table, date, vol); err != nil {
					logger.Error("TieringService: Failed to expire partition",
						"table", table,
						"date", date,
						"volume", vol.Name,
						"error", err)
					return
				}

				mu.Lock()
				totalExpired++
				mu.Unlock()
			}(tableName, partition)
		}
	}

	wg.Wait()
	if totalExpired == 0 {
		logger.Info("TieringService: TTL expire pass completed with no deletes",
			"source", vol.Name,
			"tables_checked", len(tables),
			"partition_date_cutoff", cutoffDate,
			"hint", "No partition dates on or before cutoff on the final volume.")
	}
	return totalExpired, nil
}

// ensureTableExists ensures the destination table exists with the same schema and partitioning as source
func (ts *TieringService) ensureTableExists(srcVol, dstVol *ducklake.Volume, tableName string) error {
	db := ts.tieredStorage.GetDB()

	// Check if table exists in destination
	checkQuery := fmt.Sprintf(`
		SELECT COUNT(*) FROM information_schema.tables 
		WHERE table_catalog = '%s' 
		  AND table_schema = 'main' 
		  AND table_name = '%s'
	`, dstVol.LakeName, tableName)

	var count int
	if err := db.QueryRow(checkQuery).Scan(&count); err != nil {
		return fmt.Errorf("failed to check table existence: %w", err)
	}

	if count > 0 {
		return nil // Table already exists
	}

	// Get column definitions from source table
	srcTable := fmt.Sprintf("%s.main.%s", srcVol.LakeName, tableName)
	dstTable := fmt.Sprintf("%s.main.%s", dstVol.LakeName, tableName)

	// Get columns from source table
	columnsQuery := fmt.Sprintf(`
		SELECT column_name, data_type 
		FROM information_schema.columns 
		WHERE table_catalog = '%s' 
		  AND table_schema = 'main' 
		  AND table_name = '%s'
		ORDER BY ordinal_position
	`, srcVol.LakeName, tableName)

	rows, err := db.Query(columnsQuery)
	if err != nil {
		return fmt.Errorf("failed to get source table columns: %w", err)
	}
	defer rows.Close()

	var columns []string
	hasDateColumn := false
	for rows.Next() {
		var colName, colType string
		if err := rows.Scan(&colName, &colType); err != nil {
			continue
		}
		columns = append(columns, fmt.Sprintf("%s %s", colName, colType))
		if colName == "date" {
			hasDateColumn = true
		}
	}

	if len(columns) == 0 {
		// Fallback to simple CREATE TABLE AS SELECT
		createQuery := fmt.Sprintf(`
			CREATE TABLE IF NOT EXISTS %s AS 
			SELECT * FROM %s WHERE 1=0
		`, dstTable, srcTable)
		if _, err := db.Exec(createQuery); err != nil {
			return fmt.Errorf("failed to create table in destination: %w", err)
		}
	} else {
		// Create table with explicit schema
		columnsDef := strings.Join(columns, ", ")
		createQuery := fmt.Sprintf(`
			CREATE TABLE IF NOT EXISTS %s (%s)
		`, dstTable, columnsDef)
		if _, err := db.Exec(createQuery); err != nil {
			return fmt.Errorf("failed to create table in destination: %w", err)
		}
	}

	// Set partitioning by date column if present (DuckLake syntax)
	if hasDateColumn {
		alterQuery := fmt.Sprintf(`ALTER TABLE %s SET PARTITIONED BY (date)`, dstTable)
		if _, err := db.Exec(alterQuery); err != nil {
			logger.Warn(fmt.Sprintf("TieringService: Failed to set partitioning (may already be set): %v", err))
		}
	}

	logger.Info("TieringService: Created table in destination",
		"table", tableName,
		"volume", dstVol.Name,
		"partitioned", hasDateColumn)

	return nil
}

// Stop stops the tiering service
func (ts *TieringService) Stop() error {
	ts.stopOnce.Do(func() {
		close(ts.stopChan)
		ts.wg.Wait()
	})
	logger.Info("TieringService: Stopped")
	return nil
}

// cleanupEmptyPartitions removes empty partition directories (date=YYYY-MM-DD/)
func (ts *TieringService) cleanupEmptyPartitions(basePath string) {
	// Walk through main/table_name/date=*/ directories
	mainPath := filepath.Join(basePath, "main")

	tables, err := os.ReadDir(mainPath)
	if err != nil {
		return
	}

	var cleaned int
	for _, table := range tables {
		if !table.IsDir() {
			continue
		}

		tablePath := filepath.Join(mainPath, table.Name())
		partitions, err := os.ReadDir(tablePath)
		if err != nil {
			continue
		}

		for _, partition := range partitions {
			if !partition.IsDir() || !strings.HasPrefix(partition.Name(), "date=") {
				continue
			}

			partitionPath := filepath.Join(tablePath, partition.Name())

			// Check if directory is empty
			entries, err := os.ReadDir(partitionPath)
			if err != nil {
				continue
			}

			if len(entries) == 0 {
				// Remove empty partition directory
				if err := os.Remove(partitionPath); err == nil {
					cleaned++
					logger.Debug("TieringService: Removed empty partition directory",
						"path", partitionPath)
				}
			}
		}
	}

	if cleaned > 0 {
		logger.Info("TieringService: Cleaned up empty partition directories",
			"count", cleaned)
	}
}
