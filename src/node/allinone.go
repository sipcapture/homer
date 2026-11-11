// Copyright (C) 2026 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package node

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/sipcapture/homer-core/src/config"
)

// In the all-in-one process the node used to run every read on the writer's
// DuckDB, which cannot be sandboxed: flush, compaction and maintenance need
// full file access there. With node.duckdb_sandbox on, reads run on the node's
// own sandboxed DuckDB instead, the same way a standalone node reads the lake.
// The writer's DuckDB is kept for /exec (parameterized PCAP import) and for
// search_buffer: before a search that reads the memory buffers, the node
// copies them out of the writer's DuckDB with a fixed query of its own, and
// the search runs on the copies. User SQL never runs on the writer's DuckDB.

// IsolatedReadsPlan reports whether an all-in-one node can read the lake on
// its own sandboxed DuckDB, and why not when it cannot.
func IsolatedReadsPlan(nodeCfg *config.NodeConfig, storageCfg *config.StorageConfig) (bool, string) {
	if nodeCfg == nil || storageCfg == nil {
		return false, "no node or storage config"
	}
	if !nodeCfg.DuckDBSandboxEnabled() {
		return false, "node.duckdb_sandbox is false"
	}
	if storageCfg.DuckLake.StoragePolicy.Enable {
		return false, "storage.ducklake.storage_policy (hot/cold tiers) is enabled"
	}
	if nodeCfg.DuckLake.LakeName != storageCfg.DuckLake.LakeName {
		return false, "node.ducklake.lake_name differs from storage.ducklake.lake_name"
	}
	want := catalogKey(storageCfg.DuckLake.CatalogType, storageCfg.DuckLake.CatalogPath)
	if want == "" {
		return false, "storage.ducklake.catalog_path is empty"
	}
	for _, vol := range nodeCfg.DuckLake.Volumes {
		if vol.Name == "default" && catalogKey(vol.CatalogType, vol.CatalogPath) == want {
			return true, ""
		}
	}
	return false, "node.ducklake.volumes has no \"default\" volume on the writer's catalog"
}

// catalogKey normalizes a catalog location so the node and writer configs
// compare equal when they point at the same catalog.
func catalogKey(ct, path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	typ, err := config.ResolveCatalogType(ct, path)
	if err != nil {
		return ""
	}
	if typ == "postgres" {
		return "postgres:" + config.NormalizePostgresDSN(path)
	}
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	return typ + ":" + filepath.Clean(path)
}

// SetWriterDB gives the node the writer's DuckDB for /exec only. Reads keep
// using the node's own DuckDB. Use it instead of SetSharedDB when
// IsolatedReadsPlan allows it.
func (n *Node) SetWriterDB(db *sql.DB) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.writerDB = db
}

// execDB returns the DuckDB that /exec inserts into: the writer's when the
// node runs next to it, else the node's own.
func (n *Node) execDB() *sql.DB {
	n.mu.RLock()
	defer n.mu.RUnlock()
	if n.sharedDB != nil {
		return n.sharedDB
	}
	if n.writerDB != nil {
		return n.writerDB
	}
	return n.db
}

// SandboxActive reports whether the node's own DuckDB currently runs with
// external access disabled. It is false after a sandbox fallback.
func (n *Node) SandboxActive() bool {
	n.mu.RLock()
	db := n.db
	n.mu.RUnlock()
	return duckDBSandboxed(db)
}

func duckDBSandboxed(db *sql.DB) bool {
	if db == nil {
		return false
	}
	var v string
	if err := db.QueryRow("SELECT current_setting('enable_external_access')::VARCHAR").Scan(&v); err != nil {
		return false
	}
	return v == "false"
}

// stagedBufferCatalog is where copies of the writer buffers live in the node
// DuckDB: its own in-memory catalog, not the attached lake.
const stagedBufferCatalog = "memory.main."

// memBufferTableRe matches the writer buffer tables (see ducklake.NewTableWriter).
var memBufferTableRe = regexp.MustCompile(`^mem_hep_proto_[a-z0-9_]+_[ab]$`)

var sqlOrWordRegexp = regexp.MustCompile(`(?i)\bOR\b`)

// bufferCopyEnabled reports whether searches read copies of the writer
// buffers: all-in-one with sandboxed reads and search_buffer on.
func (n *Node) bufferCopyEnabled() bool {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.sharedDB == nil && n.writerDB != nil && n.searchBufferEnabled()
}

// stageBufferTables copies the writer buffer tables a search reads into the
// node DuckDB, through a parquet file in the node's spill directory (the
// sandbox allows it). Only table names matching memBufferTableRe and a time
// filter built from parsed integers reach the writer's DuckDB. Callers hold
// n.bufferMu until the search is done, then call cleanup.
func (n *Node) stageBufferTables(ctx context.Context, nodeDB *sql.DB, tables []string, query string) (func(), error) {
	n.mu.RLock()
	writer := n.writerDB
	n.mu.RUnlock()
	if writer == nil || nodeDB == nil {
		return nil, fmt.Errorf("writer or node DuckDB not set")
	}
	if len(tables) == 0 {
		return nil, fmt.Errorf("no buffer tables in the plan")
	}
	dir, err := bufferStageDir(nodeDB)
	if err != nil {
		return nil, err
	}
	where := bufferTimeFilter(query)

	var staged []string
	cleanup := func() {
		for _, t := range staged {
			_, _ = nodeDB.ExecContext(context.Background(), "DROP TABLE IF EXISTS "+stagedBufferCatalog+t)
		}
	}
	for _, t := range tables {
		if !memBufferTableRe.MatchString(t) {
			cleanup()
			return nil, fmt.Errorf("unexpected buffer table %q", t)
		}
		// Use the name as the writer's catalog spells it, so no text taken
		// from the search reaches the statements below.
		var name string
		if err := writer.QueryRowContext(ctx,
			`SELECT table_name FROM duckdb_tables() WHERE database_name = current_database() AND schema_name = 'main' AND table_name = ?`,
			t).Scan(&name); err != nil {
			cleanup()
			return nil, fmt.Errorf("buffer table %q: %w", t, err)
		}
		t = name
		f, err := os.CreateTemp(dir, "homer-buffer-*.parquet")
		if err != nil {
			cleanup()
			return nil, fmt.Errorf("buffer copy file: %w", err)
		}
		file := f.Name()
		_ = f.Close()
		lit := strings.ReplaceAll(file, "'", "''")
		_, err = writer.ExecContext(ctx, "COPY (SELECT * FROM "+t+where+") TO '"+lit+"' (FORMAT parquet)")
		if err == nil {
			_, err = nodeDB.ExecContext(ctx, "CREATE OR REPLACE TABLE "+stagedBufferCatalog+t+" AS SELECT * FROM read_parquet('"+lit+"')")
		}
		_ = os.Remove(file)
		if err != nil {
			cleanup()
			return nil, fmt.Errorf("copy %s: %w", t, err)
		}
		staged = append(staged, t)
	}
	return cleanup, nil
}

// bufferStageDir returns the node DuckDB's spill directory, which the sandbox
// keeps readable, as an absolute path.
func bufferStageDir(db *sql.DB) (string, error) {
	var dir string
	if err := db.QueryRow("SELECT current_setting('temp_directory')::VARCHAR").Scan(&dir); err != nil {
		return "", fmt.Errorf("read temp_directory: %w", err)
	}
	if strings.TrimSpace(dir) == "" {
		return "", fmt.Errorf("node DuckDB has no temp_directory")
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(abs, 0o750); err != nil {
		return "", fmt.Errorf("create %s: %w", abs, err)
	}
	return abs, nil
}

// bufferTimeFilter narrows the buffer copy to the search's time range. It is
// used only when the query has both bounds and no OR, so the filter can only
// drop rows the search would drop too.
func bufferTimeFilter(query string) string {
	if sqlOrWordRegexp.MatchString(query) {
		return ""
	}
	fromMs, toMs, ok := memorySplitBoundsMs(query)
	if !ok {
		return ""
	}
	return fmt.Sprintf(" WHERE timestamp >= (to_timestamp(%d / 1000.0) AT TIME ZONE 'UTC') AND timestamp <= (to_timestamp(%d / 1000.0) AT TIME ZONE 'UTC')", fromMs, toMs)
}
