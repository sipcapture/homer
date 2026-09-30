// Copyright (C) 2026 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package ducklake

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	logger "github.com/sipcapture/homer-core/src/utils/logging"
)

// maintenancePoolConns: the merge takes a dedicated connection (SET threads=1)
// while row counts and invariant checks run on another.
const maintenancePoolConns = 2

// DefaultMaintenanceMemoryLimit is the compaction instance's memory_limit when
// compaction.memory_limit is not set.
const DefaultMaintenanceMemoryLimit = "2GB"

// MaintenanceDB is a DuckDB instance dedicated to compaction and file cleanup,
// with the writer's DuckLake catalog attached. A DuckLake internal error in a
// merge invalidates the DuckDB instance it runs on; keeping compaction off the
// writer instance means that no longer takes ingest and search down with it
// (sipcapture/homer#1048).
type MaintenanceDB struct {
	DB      *sql.DB
	release func()
}

// Close closes the instance and clears its in-process attach mark.
func (m *MaintenanceDB) Close() error {
	if m == nil || m.DB == nil {
		return nil
	}
	err := m.DB.Close()
	if m.release != nil {
		m.release()
	}
	return err
}

// OpenMaintenanceDB opens a DuckDB instance for compaction with cfg's lake
// attached under the same name. No catalog lock file, repair or GC: the
// writer that owns the catalog already did that.
//
// reference is the writer's DuckDB. Both instances must reach the catalog
// through the same loaded sqlite extension, i.e. the same copy of SQLite: two
// copies in one process lose each other's POSIX locks and corrupt the file.
// OpenMaintenanceDB refuses when that cannot be confirmed.
func OpenMaintenanceDB(cfg Config, memoryLimit string, reference *sql.DB) (*MaintenanceDB, error) {
	if strings.TrimSpace(cfg.CatalogPath) == "" {
		return nil, errors.New("maintenance DuckDB: no catalog path")
	}
	if strings.TrimSpace(memoryLimit) == "" {
		memoryLimit = DefaultMaintenanceMemoryLimit
	}
	db, err := newLakeDuckDB(cfg, maintenancePoolConns)
	if err != nil {
		return nil, err
	}
	fail := func(err error) (*MaintenanceDB, error) {
		_ = db.Close()
		return nil, err
	}

	ApplyHomerDuckDBDefaults(db, cfg.TuningThreads, memoryLimit,
		maintenanceSpillDirectory(cfg), cfg.CatalogPath, "compaction")

	if err := loadLakeExtensions(db); err != nil {
		return fail(err)
	}
	if reference != nil {
		if err := sameSQLiteExtension(reference, db); err != nil {
			return fail(err)
		}
	}
	if err := configureLakeSecrets(db, cfg, "compaction"); err != nil {
		return fail(err)
	}
	if _, err := db.Exec(buildLakeAttachSQL(cfg)); err != nil {
		return fail(fmt.Errorf("maintenance DuckDB: attach ducklake: %w", err))
	}
	return &MaintenanceDB{DB: db, release: MarkCatalogAttached(cfg.CatalogPath)}, nil
}

// maintenanceSpillDirectory keeps the compaction instance's temp files apart
// from the writer's: two DuckDB instances must not share a temp_directory.
func maintenanceSpillDirectory(cfg Config) string {
	base := strings.TrimSpace(cfg.TuningTempDirectory)
	if base == "" {
		base = DefaultSpillDirectory(cfg.CatalogPath)
	}
	if base == "" {
		return ""
	}
	dir := filepath.Join(base, "compaction")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		logger.Warn("maintenance DuckDB: cannot create spill directory", "dir", dir, "error", err)
	}
	return dir
}

// sqliteExtensionPath returns where db loaded the sqlite extension from.
func sqliteExtensionPath(db *sql.DB) (string, error) {
	var path sql.NullString
	err := db.QueryRow(`SELECT install_path FROM duckdb_extensions()
		WHERE extension_name IN ('sqlite_scanner', 'sqlite') AND loaded
		LIMIT 1`).Scan(&path)
	if err != nil {
		return "", fmt.Errorf("locate loaded sqlite extension: %w", err)
	}
	return path.String, nil
}

func sameSQLiteExtension(a, b *sql.DB) error {
	pa, err := sqliteExtensionPath(a)
	if err != nil {
		return err
	}
	pb, err := sqliteExtensionPath(b)
	if err != nil {
		return err
	}
	if pa != pb {
		return fmt.Errorf("maintenance DuckDB loaded the sqlite extension from %q but the writer uses %q; "+
			"refusing to open the catalog with a second SQLite library", pb, pa)
	}
	return nil
}

// OpenMaintenanceDB opens a compaction instance for this writer's catalog.
func (mtw *MultiTableWriter) OpenMaintenanceDB(memoryLimit string) (*MaintenanceDB, error) {
	return OpenMaintenanceDB(mtw.config, memoryLimit, mtw.db)
}

// IsDuckDBFatalError reports whether err means DuckDB invalidated the instance
// that produced it; every later query on that instance fails.
func IsDuckDBFatalError(err error) bool {
	return err != nil && isDuckDBFatalError(err.Error())
}
