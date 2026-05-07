// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package ducklake

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync/atomic"
)

// ShardedWriter wraps N independent MultiTableWriter instances, each with its
// own DuckDB engine and catalog. Writes are distributed round-robin across
// shards so that DuckDB INSERT throughput scales linearly.
//
// For reads, all shards are queried and results merged via UNION ALL.
type ShardedWriter struct {
	shards []*MultiTableWriter
	count  int
	next   atomic.Uint64
	config Config
}

// NewShardedWriter creates N MultiTableWriter shards. Each shard gets its own
// catalog file (e.g. homer_catalog_0.sqlite, homer_catalog_1.sqlite) and lake name
// (homer_lake_0, homer_lake_1) but shares the same data directory.
func NewShardedWriter(config Config) (*ShardedWriter, error) {
	n := config.ShardCount
	if n <= 0 {
		n = 1
	}

	sw := &ShardedWriter{
		shards: make([]*MultiTableWriter, n),
		count:  n,
		config: config,
	}

	for i := 0; i < n; i++ {
		shardCfg := config
		if n > 1 {
			shardCfg.CatalogPath = shardCatalogPath(config.CatalogPath, i)
			shardCfg.LakeName = fmt.Sprintf("%s_%d", config.LakeName, i)
			shardCfg.DataPath = config.DataPath
		}
		// ShardCount=1 inside each shard to avoid recursion
		shardCfg.ShardCount = 1

		mtw, err := NewMultiTableWriter(shardCfg)
		if err != nil {
			// Clean up already-created shards
			for j := 0; j < i; j++ {
				sw.shards[j].Stop()
			}
			return nil, fmt.Errorf("failed to create shard %d: %w", i, err)
		}
		sw.shards[i] = mtw
	}

	return sw, nil
}

// shardCatalogPath generates per-shard catalog path:
// "homer_catalog.sqlite" → "homer_catalog_0.sqlite"
func shardCatalogPath(base string, idx int) string {
	ext := filepath.Ext(base)
	name := strings.TrimSuffix(base, ext)
	return fmt.Sprintf("%s_%d%s", name, idx, ext)
}

// Start starts all shards.
func (sw *ShardedWriter) Start() {
	for _, s := range sw.shards {
		s.Start()
	}
}

// Stop stops all shards and flushes remaining data.
func (sw *ShardedWriter) Stop() error {
	var firstErr error
	for _, s := range sw.shards {
		if err := s.Stop(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// WriteRecord distributes writes round-robin across shards.
func (sw *ShardedWriter) WriteRecord(key TableKey, values []interface{}) error {
	idx := sw.next.Add(1) - 1
	shard := sw.shards[idx%uint64(sw.count)]
	return shard.WriteRecord(key, values)
}

// Shards returns all underlying MultiTableWriter instances (for reader access).
func (sw *ShardedWriter) Shards() []*MultiTableWriter {
	return sw.shards
}

// ShardCount returns the number of shards.
func (sw *ShardedWriter) ShardCount() int {
	return sw.count
}

// Primary returns the first shard (used for schema queries, table creation, etc.)
func (sw *ShardedWriter) Primary() *MultiTableWriter {
	return sw.shards[0]
}

// SearchBufferEnabled returns whether read queries should include memory tables.
func (sw *ShardedWriter) SearchBufferEnabled() bool {
	return sw.config.SearchBuffer
}

// ListTableKeys returns all TableKeys from the primary shard.
func (sw *ShardedWriter) ListTableKeys() []TableKey {
	return sw.shards[0].ListTableKeys()
}

// ListProtoTypes returns unique proto_types from the primary shard.
func (sw *ShardedWriter) ListProtoTypes() []uint32 {
	return sw.shards[0].ListProtoTypes()
}

// CatalogLock acquires catalog lock on all shards (for compaction).
func (sw *ShardedWriter) CatalogLock() {
	for _, s := range sw.shards {
		s.CatalogLock()
	}
}

// CatalogUnlock releases catalog lock on all shards.
func (sw *ShardedWriter) CatalogUnlock() {
	for i := len(sw.shards) - 1; i >= 0; i-- {
		sw.shards[i].CatalogUnlock()
	}
}
