// Copyright (C) 2026 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package node

import (
	"database/sql"
	"path/filepath"
	"strings"

	"github.com/sipcapture/homer-core/src/config"
)

// In the all-in-one process the node used to run every read on the writer's
// DuckDB, which cannot be sandboxed: flush, compaction and maintenance need
// full file access there. With node.duckdb_sandbox on, reads run on the node's
// own sandboxed DuckDB instead, the same way a standalone node reads the lake.
// The writer's DuckDB is kept only for /exec (parameterized PCAP import).
//
// The cost is search_buffer: the memory buffers live in the writer's DuckDB,
// so unflushed rows show up in search after the next flush.

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
