// Copyright (C) 2026 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package ducklake

import (
	"database/sql"
	"fmt"
	"sync"
	"time"

	logger "github.com/sipcapture/homer-core/src/utils/logging"
)

// objectStoreOrphanScanInterval bounds how often delete_orphaned_files lists
// and probes an object-store volume. Orphans only come from interrupted
// writes, while the sweep costs one request per object (sipcapture/homer#1037).
const objectStoreOrphanScanInterval = 24 * time.Hour

// maintenanceDB is the lazily opened DuckDB instance used for object-store
// file cleanup. It attaches the same SQLite catalogs as the tiering instance.
type maintenanceDB struct {
	mu             sync.Mutex
	db             *sql.DB
	attached       map[string]bool
	lastOrphanScan map[string]time.Time
}

// usesDedicatedFileCleanupDB reports whether cleanup_old_files and
// delete_orphaned_files for vol run on the maintenance instance instead of
// the query-serving tiering connection.
func usesDedicatedFileCleanupDB(vol *Volume) bool {
	return vol != nil && vol.Type == VolumeTypeS3
}

// orphanScanDue reports whether delete_orphaned_files should run for vol at now.
// Local volumes are cheap to list and keep running every cycle.
func (m *maintenanceDB) orphanScanDue(vol *Volume, now time.Time) bool {
	if vol.Type == VolumeTypeLocal {
		return true
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	last, ok := m.lastOrphanScan[vol.LakeName]
	return !ok || now.Sub(last) >= objectStoreOrphanScanInterval
}

func (m *maintenanceDB) markOrphanScan(vol *Volume, now time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.lastOrphanScan == nil {
		m.lastOrphanScan = make(map[string]time.Time)
	}
	m.lastOrphanScan[vol.LakeName] = now
}

func (m *maintenanceDB) close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.db != nil {
		if err := m.db.Close(); err != nil {
			logger.Warn("TieredStorageManager: failed to close maintenance DuckDB", "error", err)
		}
		m.db = nil
		m.attached = nil
	}
}

// fileCleanupDB returns the maintenance instance with vol attached, opening
// it on first use. Credential-chain S3 secrets are refreshed on every call.
func (tsm *TieredStorageManager) fileCleanupDB(vol *Volume) (*sql.DB, error) {
	m := &tsm.maint
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.db == nil {
		db, err := tsm.openMaintenanceDB()
		if err != nil {
			return nil, err
		}
		m.db = db
		m.attached = make(map[string]bool)
	}

	if m.attached[vol.LakeName] {
		if vol.Type == VolumeTypeS3 && usesS3CredentialChain(vol.S3AccessKey, volumeS3EndpointHost(vol.S3Endpoint)) {
			if err := createVolumeS3SecretOn(m.db, vol, true); err != nil {
				return nil, err
			}
		}
		return m.db, nil
	}

	if vol.Type == VolumeTypeS3 {
		if err := createVolumeS3SecretOn(m.db, vol, false); err != nil {
			return nil, err
		}
	}
	if vol.CatalogPath == "" {
		return nil, fmt.Errorf("volume %s has no resolved catalog path", vol.Name)
	}
	if _, err := m.db.Exec(volumeAttachSQL(vol, vol.CatalogPath)); err != nil {
		return nil, fmt.Errorf("attach volume %s on maintenance DuckDB: %w", vol.Name, err)
	}
	m.attached[vol.LakeName] = true
	logger.Info("TieredStorageManager: Volume attached for file cleanup",
		"volume", vol.Name,
		"lake", vol.LakeName)
	return m.db, nil
}

func (tsm *TieredStorageManager) openMaintenanceDB() (*sql.DB, error) {
	db, err := sql.Open("duckdb", "")
	if err != nil {
		return nil, fmt.Errorf("open maintenance DuckDB: %w", err)
	}
	// Same single-connection contract as the tiering instance: secrets and
	// ATTACH must be visible to every Exec.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	ApplyHomerDuckDBDefaults(db, tsm.config.TuningThreads, tsm.config.TuningMemoryLimit,
		tsm.config.TuningTempDirectory, tsm.config.CatalogPath, "tiering-maintenance")

	for _, ext := range []string{"ducklake", "sqlite"} {
		if _, err := db.Exec("LOAD " + ext + ";"); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("maintenance DuckDB: load %s extension: %w", ext, err)
		}
	}
	if _, err := db.Exec("LOAD aws;"); err != nil {
		logger.Warn("TieredStorageManager: maintenance DuckDB failed to load aws extension (credential_chain unavailable)", "error", err)
	}
	return db, nil
}

// scheduledFileCount returns how many data files the lake still lists as
// scheduled for deletion.
func scheduledFileCount(db *sql.DB, lakeName string) (int64, error) {
	q := fmt.Sprintf("SELECT COUNT(*) FROM %s.ducklake_files_scheduled_for_deletion", ducklakeMetadataIdent(lakeName))
	var n int64
	if err := db.QueryRow(q).Scan(&n); err != nil {
		return 0, fmt.Errorf("count files scheduled for deletion: %w", err)
	}
	return n, nil
}

// runFileCleanup deletes superseded files and, when due, orphaned files for vol on db.
func (tsm *TieredStorageManager) runFileCleanup(db *sql.DB, vol *Volume, locker CatalogLocker, cleanupSQL, orphanSQL string, record func(string, error)) {
	before, countErr := scheduledFileCount(db, vol.LakeName)
	_, err := execWithRetry(db, tieringMaxRetries, tieringBaseBackoff, locker, cleanupSQL)
	record(cleanupSQL, err)
	if err == nil && countErr == nil && before > 0 {
		if after, err := scheduledFileCount(db, vol.LakeName); err == nil && after >= before {
			logger.Warn("TieredStorageManager: cleanup_old_files did not reduce files scheduled for deletion",
				"volume", vol.Name,
				"lake", vol.LakeName,
				"scheduled_files", after,
				"hint", "check that the volume credentials may delete objects (e.g. s3:DeleteObject)")
		}
	}

	now := time.Now()
	if !tsm.maint.orphanScanDue(vol, now) {
		logger.Debug("TieredStorageManager: Skipping orphaned file scan until interval elapses",
			"volume", vol.Name,
			"interval", objectStoreOrphanScanInterval.String())
		return
	}
	// Marked even on failure so a denied or broken sweep is not retried every cycle.
	tsm.maint.markOrphanScan(vol, now)
	_, err = execWithRetry(db, tieringMaxRetries, tieringBaseBackoff, locker, orphanSQL)
	record(orphanSQL, err)
}
