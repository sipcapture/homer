// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package ducklake

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/sipcapture/homer-core/src/decoder"
	"github.com/sipcapture/homer-core/src/homerconfig"
	logger "github.com/sipcapture/homer-core/src/utils/logging"
)

// Manager manages DuckLake storage with multi-table support.
// When ShardCount > 1, writes are distributed across independent DuckDB shards
// for linear write throughput scaling.
type Manager struct {
	config  Config
	sharded *ShardedWriter
	adapter *ShardedAdapter
	api     *MultiTableAPI
}

// NewManager creates a new DuckLake manager with multi-table support
func NewManager(config Config) (*Manager, error) {
	sw, err := NewShardedWriter(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create DuckLake sharded writer: %w", err)
	}

	adapter := NewShardedAdapter(sw)

	return &Manager{
		config:  config,
		sharded: sw,
		adapter: adapter,
		api:     NewMultiTableAPI(sw.Primary()),
	}, nil
}

// NewManagerFromConfig creates a manager from application config
func NewManagerFromConfig() (*Manager, error) {
	settings := homerconfig.MainConfig.Setting.DUCKLAKE_SETTINGS

	if !settings.Enable {
		return nil, fmt.Errorf("DuckLake storage is disabled in config")
	}

	config := Config{
		CatalogType:   CatalogType(settings.CatalogType),
		CatalogPath:   settings.CatalogPath,
		DataPath:      settings.DataPath,
		LakeName:      settings.LakeName,
		TableName:     settings.TableName,
		BatchSize:     settings.BatchSize,
		FlushInterval: time.Duration(settings.FlushInterval) * time.Second,
		SearchBuffer:  settings.SearchBuffer,
		ShardCount:    settings.ShardCount,
		FlushQueue:    settings.FlushQueue,
	}

	// S3 config
	if settings.S3.AccessKeyID != "" {
		config.S3Region = settings.S3.Region
		config.S3AccessKeyID = settings.S3.AccessKeyID
		config.S3SecretAccessKey = settings.S3.SecretAccessKey
		config.S3Endpoint = settings.S3.Endpoint
		config.S3UseSSL = settings.S3.UseSSL
	}

	// Apply defaults
	if config.CatalogType == "" {
		config.CatalogType = CatalogSQLite
	}
	ct, err := NormalizeSQLiteCatalog(config.CatalogType)
	if err != nil {
		return nil, err
	}
	config.CatalogType = ct
	if config.CatalogPath == "" {
		config.CatalogPath = "homer_catalog.sqlite"
	}
	if config.DataPath == "" {
		config.DataPath = "/var/lib/homer/parquet"
	}
	if config.LakeName == "" {
		config.LakeName = "homer_lake"
	}
	if config.BatchSize <= 0 {
		config.BatchSize = 10000
	}
	if config.FlushInterval <= 0 {
		config.FlushInterval = 30 * time.Second
	}
	// Legacy writer/ingest path: guard against a second writer on the catalog.
	config.ExclusiveLock = true

	return NewManager(config)
}

// Start starts the DuckLake manager
func (m *Manager) Start() error {
	m.sharded.Start()
	logger.Info("DuckLake manager started", "catalog", string(m.config.CatalogType),
		"data", m.config.DataPath, "shards", m.sharded.ShardCount())
	return nil
}

// Stop stops the DuckLake manager
func (m *Manager) Stop() error {
	logger.Info("Stopping DuckLake manager...")
	return m.sharded.Stop()
}

// WriteHEP writes a HEP packet to the appropriate table based on proto_type
func (m *Manager) WriteHEP(hep *decoder.HEP) error {
	return m.adapter.WriteHEP(hep)
}

// GetAPI returns the API handler
func (m *Manager) GetAPI() *MultiTableAPI {
	return m.api
}

// GetWriter returns the primary shard's multi-table writer (for backward compat)
func (m *Manager) GetWriter() *MultiTableWriter {
	return m.sharded.Primary()
}

// GetShardedWriter returns the sharded writer
func (m *Manager) GetShardedWriter() *ShardedWriter {
	return m.sharded
}

// GetReader returns a sharded multi-table reader
func (m *Manager) GetReader() *ShardedMultiTableReader {
	return m.adapter.GetReader()
}

// GetStats returns storage statistics across all shards and tables
func (m *Manager) GetStats() (map[string]interface{}, error) {
	allStats := make(map[string]interface{})
	allStats["shard_count"] = m.sharded.ShardCount()
	allStats["catalog_type"] = string(m.config.CatalogType)
	allStats["data_path"] = m.config.DataPath

	var totalRows int64
	var totalBuffer int
	var tableCount int

	for i, shard := range m.sharded.Shards() {
		stats, err := shard.GetStats()
		if err != nil {
			continue
		}
		if rc, ok := stats["total_row_count"].(int64); ok {
			totalRows += rc
		}
		if bs, ok := stats["total_buffer_size"].(int); ok {
			totalBuffer += bs
		}
		if tc, ok := stats["table_count"].(int); ok {
			tableCount += tc
		}
		allStats[fmt.Sprintf("shard_%d", i)] = stats
	}

	allStats["total_row_count"] = totalRows
	allStats["total_buffer_size"] = totalBuffer
	allStats["table_count"] = tableCount

	return allStats, nil
}

// GetBufferStats returns only the in-memory buffer totals across all shards.
// This is much cheaper than GetStats because it skips expensive lake-wide
// COUNT(*)/MIN/MAX queries that scan Parquet files and block the single
// DuckDB connection. Use for periodic logging instead of GetStats.
func (m *Manager) GetBufferStats() (bufferRows int, tableCount int) {
	for _, shard := range m.sharded.Shards() {
		br, tc := shard.GetBufferStats()
		bufferRows += br
		tableCount += tc
	}
	return
}

// GetTimeRange returns the time range of stored data across all shards
func (m *Manager) GetTimeRange() (minTs, maxTs int64, err error) {
	return m.adapter.GetReader().GetTimeRange()
}

// GetTimeRangeForTableKey returns time range for specific TableKey
func (m *Manager) GetTimeRangeForTableKey(key TableKey) (minTs, maxTs int64, err error) {
	return m.adapter.GetReader().GetTimeRangeForTableKey(key)
}

// HasDataInRange checks if data exists in the given time range
func (m *Manager) HasDataInRange(queryMinTs, queryMaxTs int64) (bool, error) {
	return m.adapter.GetReader().HasDataInRange(queryMinTs, queryMaxTs)
}

// ListSnapshots returns available snapshots for a TableKey (from primary shard)
func (m *Manager) ListSnapshots(key TableKey, limit int) ([]Snapshot, error) {
	reader := NewMultiTableReader(m.sharded.Primary())
	return reader.ListSnapshots(key, limit)
}

// ListTableKeys returns all TableKeys with tables
func (m *Manager) ListTableKeys() []TableKey {
	return m.sharded.ListTableKeys()
}

// ListProtoTypes returns unique proto_types with tables
func (m *Manager) ListProtoTypes() []uint32 {
	return m.sharded.ListProtoTypes()
}

// Query queries a specific table by TableKey across all shards
func (m *Manager) Query(key TableKey, where string, limit int) ([]map[string]interface{}, error) {
	return m.adapter.GetReader().Query(key, where, limit)
}

// QueryAll queries all tables across all shards
func (m *Manager) QueryAll(where string, limit int) ([]map[string]interface{}, error) {
	return m.adapter.GetReader().QueryAll(where, limit)
}

// CatalogLock acquires the catalog mutex on all shards.
func (m *Manager) CatalogLock() {
	m.sharded.CatalogLock()
}

// CatalogUnlock releases the catalog mutex on all shards.
func (m *Manager) CatalogUnlock() {
	m.sharded.CatalogUnlock()
}

// GetDB returns the primary shard's DuckDB connection (for compaction/maintenance)
func (m *Manager) GetDB() *sql.DB {
	return m.sharded.Primary().GetDB()
}

// GetLakeName returns the DuckLake catalog name
func (m *Manager) GetLakeName() string {
	return m.config.LakeName
}

// GetShards returns all shard MultiTableWriters (for compaction across shards)
func (m *Manager) GetShards() []*MultiTableWriter {
	return m.sharded.Shards()
}
