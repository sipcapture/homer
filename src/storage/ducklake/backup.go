// Copyright (C) 2026 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package ducklake

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	// Pure-Go SQLite driver (no cgo). Registers driver name "sqlite".
	_ "modernc.org/sqlite"
)

// catalogBackupSuffix marks the copies BackupCatalog creates.
const catalogBackupSuffix = ".bak-"

// BackupCatalog writes a consistent snapshot of the DuckLake SQLite catalog with
// VACUUM INTO and keeps only the newest keep copies. VACUUM INTO runs inside a
// read transaction, so it is safe while DuckDB has the catalog attached and does
// not block writers on a WAL database.
//
// It is a cheap insurance policy for maintenance that rewrites catalog rows: the
// catalog is metadata only (kilobytes to a few megabytes), while the parquet it
// describes is not copied.
func BackupCatalog(catalogPath string, keep int) (string, error) {
	if strings.TrimSpace(catalogPath) == "" {
		return "", fmt.Errorf("no catalog path")
	}
	if _, err := os.Stat(catalogPath); err != nil {
		return "", fmt.Errorf("stat catalog: %w", err)
	}

	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(30000)", catalogPath)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return "", fmt.Errorf("open catalog for backup: %w", err)
	}
	db.SetMaxOpenConns(1)
	defer db.Close()

	dest := catalogPath + catalogBackupSuffix + time.Now().UTC().Format("20060102T150405Z")
	// VACUUM INTO refuses to overwrite, so a stale file from the same second
	// would fail the backup.
	_ = os.Remove(dest)
	if _, err := db.Exec(fmt.Sprintf("VACUUM INTO %s", sqliteQuoteLiteral(dest))); err != nil {
		return "", fmt.Errorf("vacuum into %s: %w", dest, err)
	}

	pruneCatalogBackups(catalogPath, keep)
	return dest, nil
}

// sqliteQuoteLiteral renders a SQL string literal, doubling embedded quotes.
func sqliteQuoteLiteral(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

// pruneCatalogBackups deletes all but the newest keep backups of a catalog.
// Names embed a sortable UTC timestamp, so lexical order is chronological.
func pruneCatalogBackups(catalogPath string, keep int) {
	if keep < 1 {
		keep = 1
	}
	dir := filepath.Dir(catalogPath)
	prefix := filepath.Base(catalogPath) + catalogBackupSuffix
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	var backups []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasPrefix(e.Name(), prefix) {
			backups = append(backups, e.Name())
		}
	}
	if len(backups) <= keep {
		return
	}
	sort.Strings(backups)
	for _, name := range backups[:len(backups)-keep] {
		_ = os.Remove(filepath.Join(dir, name))
	}
}
