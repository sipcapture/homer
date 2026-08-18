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

// catalogBackupSuffix marks the rotating copies BackupCatalog creates.
const catalogBackupSuffix = ".bak-"

// catalogPreRestoreSuffix marks the live catalog moved aside by RestoreCatalog.
const catalogPreRestoreSuffix = ".pre-restore-"

// DefaultCatalogBackupKeep is how many rotating VACUUM INTO copies to retain.
// Used by native compaction and by `homer catalog backup` when --keep is omitted.
const DefaultCatalogBackupKeep = 3

// CatalogBackup describes one catalog snapshot next to the live catalog file.
type CatalogBackup struct {
	Path    string
	Size    int64
	ModTime time.Time
}

// BackupCatalog writes a consistent snapshot of the DuckLake SQLite catalog with
// VACUUM INTO and keeps only the newest keep copies. VACUUM INTO runs inside a
// read transaction, so it is safe while DuckDB has the catalog attached and does
// not block writers on a WAL database.
//
// keep <= 0 disables pruning. Otherwise all but the newest keep `.bak-*` copies
// next to the catalog are deleted.
//
// It is a cheap insurance policy for maintenance that rewrites catalog rows: the
// catalog is metadata only (kilobytes to a few megabytes), while the parquet it
// describes is not copied.
func BackupCatalog(catalogPath string, keep int) (string, error) {
	return backupCatalog(catalogPath, "", keep)
}

// BackupCatalogTo writes a consistent snapshot to dest (VACUUM INTO). dest is
// not rotated with the `.bak-*` copies.
func BackupCatalogTo(catalogPath, dest string) (string, error) {
	dest = strings.TrimSpace(dest)
	if dest == "" {
		return "", fmt.Errorf("no backup destination")
	}
	return backupCatalog(catalogPath, dest, 0)
}

func backupCatalog(catalogPath, dest string, keep int) (string, error) {
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

	prune := dest == ""
	if dest == "" {
		dest = catalogPath + catalogBackupSuffix + time.Now().UTC().Format("20060102T150405Z")
	} else {
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return "", fmt.Errorf("create backup directory: %w", err)
		}
		if abs, err := filepath.Abs(dest); err == nil {
			dest = abs
		}
	}
	// VACUUM INTO refuses to overwrite, so a stale file from the same second
	// would fail the backup.
	_ = os.Remove(dest)
	if _, err := db.Exec(fmt.Sprintf("VACUUM INTO %s", sqliteQuoteLiteral(dest))); err != nil {
		return "", fmt.Errorf("vacuum into %s: %w", dest, err)
	}

	if prune {
		pruneCatalogBackups(catalogPath, keep)
	}
	return dest, nil
}

// sqliteQuoteLiteral renders a SQL string literal, doubling embedded quotes.
func sqliteQuoteLiteral(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

// pruneCatalogBackups deletes all but the newest keep backups of a catalog.
// Names embed a sortable UTC timestamp, so lexical order is chronological.
// keep <= 0 leaves every `.bak-*` copy in place.
func pruneCatalogBackups(catalogPath string, keep int) {
	if keep < 1 {
		return
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

// ListCatalogBackups returns rotating `.bak-*` copies and `.pre-restore-*`
// aside files next to catalogPath, newest first.
func ListCatalogBackups(catalogPath string) ([]CatalogBackup, error) {
	if strings.TrimSpace(catalogPath) == "" {
		return nil, fmt.Errorf("no catalog path")
	}
	dir := filepath.Dir(catalogPath)
	if dir == "" {
		dir = "."
	}
	base := filepath.Base(catalogPath)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []CatalogBackup
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasPrefix(name, base+catalogBackupSuffix) &&
			!strings.HasPrefix(name, base+catalogPreRestoreSuffix) {
			continue
		}
		// Sidecars of the aside file (-wal/-shm) are not restore sources.
		if strings.HasSuffix(name, "-wal") || strings.HasSuffix(name, "-shm") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, CatalogBackup{
			Path:    filepath.Join(dir, name),
			Size:    info.Size(),
			ModTime: info.ModTime(),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].ModTime.Equal(out[j].ModTime) {
			return out[i].ModTime.After(out[j].ModTime)
		}
		return out[i].Path > out[j].Path
	})
	return out, nil
}

// RestoreCatalog replaces the live SQLite catalog with backupPath (VACUUM INTO).
// The writer must be stopped: this takes the exclusive catalog lock and refuses
// if another homer writer holds it. The previous catalog (plus -wal/-shm) is
// renamed to `<catalog>.pre-restore-<timestamp>` so the restore is reversible.
//
// backupPath may be absolute, relative to the current directory, or a basename
// next to the live catalog. An empty backupPath selects the newest listed copy.
//
// Parquet data is not copied; restore only rewinds catalog metadata.
func RestoreCatalog(catalogPath, backupPath string) (previous string, err error) {
	if strings.TrimSpace(catalogPath) == "" {
		return "", fmt.Errorf("no catalog path")
	}
	backupPath, err = resolveCatalogBackupPath(catalogPath, backupPath)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(backupPath); err != nil {
		return "", fmt.Errorf("stat backup: %w", err)
	}
	if samePath(catalogPath, backupPath) {
		return "", fmt.Errorf("backup path %q is the live catalog; refuse to restore a file onto itself", backupPath)
	}
	if err := validateDuckLakeCatalogFile(backupPath); err != nil {
		return "", err
	}

	lock, err := acquireCatalogWriterLock(catalogPath)
	if err != nil {
		return "", fmt.Errorf("restore: %w", err)
	}
	defer releaseCatalogWriterLock(lock)

	if err := assertCatalogIdle(catalogPath); err != nil {
		return "", err
	}

	renamed, err := asideCatalogFiles(catalogPath, catalogPreRestoreSuffix)
	if err != nil {
		return "", fmt.Errorf("move live catalog aside: %w", err)
	}
	for _, pair := range renamed {
		if pair[0] == catalogPath {
			previous = pair[1]
			break
		}
	}

	if err := vacuumCatalogInto(backupPath, catalogPath); err != nil {
		undoCatalogRenames(renamed)
		return "", fmt.Errorf("restore catalog (previous catalog put back): %w", err)
	}
	return previous, nil
}

func resolveCatalogBackupPath(catalogPath, from string) (string, error) {
	from = strings.TrimSpace(from)
	if from == "" {
		list, err := ListCatalogBackups(catalogPath)
		if err != nil {
			return "", err
		}
		prefix := filepath.Base(catalogPath) + catalogBackupSuffix
		for _, b := range list {
			if strings.HasPrefix(filepath.Base(b.Path), prefix) {
				return b.Path, nil
			}
		}
		return "", fmt.Errorf("no catalog backups next to %s; run `homer catalog backup` first or pass --from", catalogPath)
	}
	if filepath.IsAbs(from) {
		return from, nil
	}
	if _, err := os.Stat(from); err == nil {
		if abs, err := filepath.Abs(from); err == nil {
			return abs, nil
		}
		return from, nil
	}
	candidate := filepath.Join(filepath.Dir(catalogPath), from)
	if _, err := os.Stat(candidate); err == nil {
		return candidate, nil
	}
	return "", fmt.Errorf("backup not found: %s", from)
}

func validateDuckLakeCatalogFile(path string) error {
	dsn := fmt.Sprintf("file:%s?mode=ro", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return fmt.Errorf("open backup: %w", err)
	}
	db.SetMaxOpenConns(1)
	defer db.Close()

	var n int
	q := `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name IN ('ducklake_snapshot','ducklake_table','ducklake_data_file')`
	if err := db.QueryRow(q).Scan(&n); err != nil {
		return fmt.Errorf("read backup %s: %w", path, err)
	}
	if n == 0 {
		return fmt.Errorf("%s does not look like a DuckLake catalog (missing ducklake_* tables)", path)
	}
	return nil
}

func assertCatalogIdle(catalogPath string) error {
	if _, err := os.Stat(catalogPath); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("stat catalog: %w", err)
	}
	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(2000)", catalogPath)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return fmt.Errorf("open catalog: %w", err)
	}
	db.SetMaxOpenConns(1)
	defer db.Close()
	if _, err := db.Exec("BEGIN EXCLUSIVE"); err != nil {
		return fmt.Errorf("catalog appears in use (stop homer-core first): %w", err)
	}
	_, _ = db.Exec("ROLLBACK")
	return nil
}

func vacuumCatalogInto(src, dest string) error {
	dsn := fmt.Sprintf("file:%s?mode=ro", src)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return fmt.Errorf("open backup: %w", err)
	}
	db.SetMaxOpenConns(1)
	defer db.Close()
	_ = os.Remove(dest)
	if _, err := db.Exec(fmt.Sprintf("VACUUM INTO %s", sqliteQuoteLiteral(dest))); err != nil {
		return fmt.Errorf("vacuum into %s: %w", dest, err)
	}
	return nil
}

// asideCatalogFiles renames the catalog and its -wal/-shm sidecars to
// `<path><suffix><timestamp>`. Returns src→dst pairs that were moved.
func asideCatalogFiles(catalogPath, suffix string) ([][2]string, error) {
	tag := suffix + time.Now().UTC().Format("20060102T150405Z")
	var renamed [][2]string
	for _, ext := range []string{"", "-wal", "-shm"} {
		src := catalogPath + ext
		if _, err := os.Stat(src); err != nil {
			continue
		}
		dst := catalogPath + tag + ext
		if err := os.Rename(src, dst); err != nil {
			undoCatalogRenames(renamed)
			return nil, fmt.Errorf("rename %s -> %s: %w", src, dst, err)
		}
		renamed = append(renamed, [2]string{src, dst})
	}
	return renamed, nil
}

func undoCatalogRenames(renamed [][2]string) {
	for i := len(renamed) - 1; i >= 0; i-- {
		_ = os.Rename(renamed[i][1], renamed[i][0])
	}
}

func samePath(a, b string) bool {
	absA, errA := filepath.Abs(a)
	absB, errB := filepath.Abs(b)
	if errA != nil || errB != nil {
		return filepath.Clean(a) == filepath.Clean(b)
	}
	return absA == absB
}
