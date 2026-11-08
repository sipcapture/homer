// Copyright (C) 2026 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package node

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/sipcapture/homer-core/src/config"
	"github.com/sipcapture/homer-core/src/storage/ducklake"
	logger "github.com/sipcapture/homer-core/src/utils/logging"
)

// openNodeDuckDB opens the node's in-memory DuckDB, attaches the lake and,
// unless node.duckdb_sandbox is false, turns off DuckDB external access.
//
// Raw SQL from the coordinator, FlightSQL and /query runs on this instance.
// The SQL validator is a denylist, and each new DuckDB reader or path trick
// has reopened file access (GHSA-c5mq-r27x-v29v). With the sandbox on,
// DuckDB itself refuses any file, URL, ATTACH or extension outside the lake.
//
// A sandbox that breaks reads of the lake is worse than none, so the lake is
// read once after it is applied. On a sandbox denial the node reopens DuckDB
// without it and logs an error; search keeps working.
func openNodeDuckDB(cfg *config.NodeConfig) (*sql.DB, []VolumeInfo, error) {
	db, volumes, err := openNodeDuckDBOnce(cfg)
	if err != nil || !cfg.DuckDBSandboxEnabled() {
		return db, volumes, err
	}

	sandboxErr := ducklake.ApplyQuerySandbox(db, nodeQuerySandbox(db, cfg))
	if sandboxErr == nil {
		sandboxErr = probeSandboxedLake(db, volumes)
	}
	if sandboxErr == nil {
		logger.Info("Node: DuckDB sandbox on (external access disabled outside the lake)")
		return db, volumes, nil
	}

	logger.Error("Node: DuckDB sandbox could not be applied; running without it. Set node.duckdb_sandbox=false to silence, and report this so the allow list can be fixed",
		"error", sandboxErr)
	_ = db.Close()
	return openNodeDuckDBOnce(cfg)
}

func openNodeDuckDBOnce(cfg *config.NodeConfig) (*sql.DB, []VolumeInfo, error) {
	db, err := sql.Open("duckdb", "")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open DuckDB: %w", err)
	}
	// Use single connection to ensure ATTACH catalogs are always visible
	db.SetMaxOpenConns(1)

	volumes, err := configureDuckLake(db, cfg)
	if err != nil {
		db.Close()
		return nil, nil, fmt.Errorf("failed to configure DuckLake: %w", err)
	}
	return db, volumes, nil
}

// nodeQuerySandbox lists what the node DuckDB still needs: every volume's
// data path (tier moves can leave absolute paths into another volume), the
// data path stored in each catalog, the catalog files and the spill directory.
func nodeQuerySandbox(db *sql.DB, cfg *config.NodeConfig) ducklake.QuerySandbox {
	var s ducklake.QuerySandbox

	add := func(ct, path, data string) {
		if data != "" {
			s.Directories = append(s.Directories, data)
		}
		typ, err := ducklake.ResolveCatalogType(ducklake.CatalogType(ct), path)
		if err == nil {
			s.Paths = append(s.Paths, ducklake.SandboxCatalogPaths(typ, path)...)
		}
	}
	for _, vol := range cfg.DuckLake.Volumes {
		add(vol.CatalogType, vol.CatalogPath, vol.Path)
	}
	add(config.NodeCatalogType(cfg), config.NodeCatalogPath(cfg), "")

	// The catalog may store a different DATA_PATH than the config, and
	// DuckLake reads from the stored one unless override_data_path is set.
	rows, err := db.Query(`SELECT database_name FROM duckdb_databases() WHERE database_name LIKE '\_\_ducklake\_metadata\_%' ESCAPE '\'`)
	if err == nil {
		var metas []string
		for rows.Next() {
			var name string
			if rows.Scan(&name) == nil {
				metas = append(metas, name)
			}
		}
		rows.Close()
		for _, m := range metas {
			var data string
			q := fmt.Sprintf(`SELECT value FROM "%s".ducklake_metadata WHERE key = 'data_path'`, strings.ReplaceAll(m, `"`, `""`))
			if db.QueryRow(q).Scan(&data) == nil {
				s.Directories = append(s.Directories, data)
			}
		}
	}

	var temp string
	if db.QueryRow("SELECT current_setting('temp_directory')::VARCHAR").Scan(&temp) == nil {
		s.Directories = append(s.Directories, temp)
	}
	return s
}

// probeSandboxedLake reads each attached lake's metadata and one row of one
// table. Only a sandbox denial counts: an empty lake or a missing table is
// not the sandbox's fault.
func probeSandboxedLake(db *sql.DB, volumes []VolumeInfo) error {
	for _, vol := range volumes {
		var table string
		err := db.QueryRow(`SELECT table_name FROM information_schema.tables
			WHERE table_catalog = ? AND table_schema = 'main' AND table_type = 'BASE TABLE'
			ORDER BY table_name LIMIT 1`, vol.LakeName).Scan(&table)
		if err == sql.ErrNoRows {
			continue
		}
		if err != nil {
			if ducklake.IsSandboxDenial(err) {
				return fmt.Errorf("volume %s catalog: %w", vol.Name, err)
			}
			continue
		}
		q := fmt.Sprintf(`SELECT 1 FROM "%s"."main"."%s" LIMIT 1`,
			strings.ReplaceAll(vol.LakeName, `"`, `""`), strings.ReplaceAll(table, `"`, `""`))
		rows, err := db.Query(q)
		if err == nil {
			for rows.Next() {
			}
			err = rows.Err()
			rows.Close()
		}
		if ducklake.IsSandboxDenial(err) {
			return fmt.Errorf("volume %s data: %w", vol.Name, err)
		}
	}
	return nil
}
