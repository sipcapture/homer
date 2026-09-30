// Copyright (C) 2026 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package ducklake

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Row checks for a live catalog. They run on a short-lived DuckDB instance that
// reads the SQLite file through the same sqlite extension as the writer (so the
// same copy of SQLite, see ErrCatalogAttachedInProcess) with every column read
// as VARCHAR: DuckLake's own reads abort on the first value of the wrong type,
// which is exactly what these checks have to get past.

const scanCatalogAlias = "homer_catalog_scan"

// malformedScheduledDeletionPredicate matches deletion-queue rows DuckLake
// cannot parse. One such row makes cleanup_old_files and
// delete_orphaned_files fail for the whole queue, every cycle
// (sipcapture/homer#1048).
const malformedScheduledDeletionPredicate = `
	NOT regexp_full_match(coalesce(data_file_id, ''), '[0-9]+')
	OR coalesce(path, '') = ''
	OR regexp_full_match(path, '[0-9]+')
	OR (path_is_relative IS NOT NULL AND lower(path_is_relative) NOT IN ('0', '1', 'true', 'false'))
	OR TRY_CAST(schedule_start AS TIMESTAMPTZ) IS NULL`

// openCatalogScanDB attaches catalogPath to a new single-connection DuckDB with
// sqlite_all_varchar on. reference, when set, must have loaded the sqlite
// extension from the same file.
func openCatalogScanDB(catalogPath string, reference *sql.DB) (*sql.DB, error) {
	db, err := sql.Open("duckdb", "")
	if err != nil {
		return nil, fmt.Errorf("open catalog scan DuckDB: %w", err)
	}
	db.SetMaxOpenConns(1)
	fail := func(err error) (*sql.DB, error) {
		_ = db.Close()
		return nil, err
	}
	if _, err := db.Exec("LOAD sqlite;"); err != nil {
		return fail(fmt.Errorf("catalog scan: load sqlite extension: %w", err))
	}
	if reference != nil {
		if err := sameSQLiteExtension(reference, db); err != nil {
			return fail(err)
		}
	}
	if _, err := db.Exec("SET sqlite_all_varchar = true;"); err != nil {
		return fail(fmt.Errorf("catalog scan: %w", err))
	}
	attach := fmt.Sprintf("ATTACH %s AS %s (TYPE sqlite);", sqliteQuoteLiteral(catalogPath), scanCatalogAlias)
	if _, err := db.Exec(attach); err != nil {
		return fail(fmt.Errorf("catalog scan: attach %s: %w", catalogPath, err))
	}
	return db, nil
}

func scanTableExists(db *sql.DB, table string) (bool, error) {
	var n int
	err := db.QueryRow(`SELECT count(*) FROM duckdb_tables() WHERE database_name = ? AND table_name = ?`,
		scanCatalogAlias, table).Scan(&n)
	return n > 0, err
}

// ScheduledDeletionQuarantine reports what QuarantineMalformedScheduledDeletions removed.
type ScheduledDeletionQuarantine struct {
	Removed int
	// File holds the removed rows as JSON lines, next to the catalog.
	File string
}

// QuarantineMalformedScheduledDeletions removes rows DuckLake cannot parse from
// ducklake_files_scheduled_for_deletion so file cleanup can run again. The rows
// are saved to `<catalog>.quarantine-<timestamp>.jsonl` first; the parquet they
// pointed at is picked up later by delete_orphaned_files.
//
// The caller must hold the catalog lock.
func QuarantineMalformedScheduledDeletions(catalogPath string, reference *sql.DB) (ScheduledDeletionQuarantine, error) {
	var res ScheduledDeletionQuarantine
	db, err := openCatalogScanDB(catalogPath, reference)
	if err != nil {
		return res, err
	}
	defer db.Close()

	const table = "ducklake_files_scheduled_for_deletion"
	if ok, err := scanTableExists(db, table); err != nil || !ok {
		return res, err
	}

	rows, err := db.Query(fmt.Sprintf(`SELECT rowid, data_file_id, path, path_is_relative, schedule_start
		FROM %s.%s WHERE %s`, scanCatalogAlias, table, malformedScheduledDeletionPredicate))
	if err != nil {
		return res, fmt.Errorf("scan %s: %w", table, err)
	}
	type badRow struct {
		RowID          int64   `json:"rowid"`
		DataFileID     *string `json:"data_file_id"`
		Path           *string `json:"path"`
		PathIsRelative *string `json:"path_is_relative"`
		ScheduleStart  *string `json:"schedule_start"`
	}
	var bad []badRow
	for rows.Next() {
		var r badRow
		var id, p, rel, start sql.NullString
		if err := rows.Scan(&r.RowID, &id, &p, &rel, &start); err != nil {
			rows.Close()
			return res, fmt.Errorf("scan %s: %w", table, err)
		}
		r.DataFileID, r.Path, r.PathIsRelative, r.ScheduleStart = nullable(id), nullable(p), nullable(rel), nullable(start)
		bad = append(bad, r)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return res, fmt.Errorf("scan %s: %w", table, err)
	}
	rows.Close()
	if len(bad) == 0 {
		return res, nil
	}

	res.File = catalogPath + ".quarantine-" + time.Now().UTC().Format("20060102T150405Z") + ".jsonl"
	f, err := os.OpenFile(res.File, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return res, fmt.Errorf("write quarantine file: %w", err)
	}
	enc := json.NewEncoder(f)
	ids := make([]string, 0, len(bad))
	for _, r := range bad {
		if err := enc.Encode(r); err != nil {
			_ = f.Close()
			return res, fmt.Errorf("write quarantine file: %w", err)
		}
		ids = append(ids, fmt.Sprint(r.RowID))
	}
	if err := f.Close(); err != nil {
		return res, fmt.Errorf("write quarantine file: %w", err)
	}

	result, err := db.Exec(fmt.Sprintf(`DELETE FROM %s.%s WHERE rowid IN (%s)`,
		scanCatalogAlias, table, strings.Join(ids, ", ")))
	if err != nil {
		return res, fmt.Errorf("delete malformed %s rows: %w", table, err)
	}
	n, _ := result.RowsAffected()
	res.Removed = int(n)
	return res, nil
}

func nullable(s sql.NullString) *string {
	if !s.Valid {
		return nil
	}
	return &s.String
}

// CountCorruptDataFileTableIDsLive counts ducklake_data_file rows whose table_id
// is not an integer (sipcapture/homer#900, #1048), on a catalog that is
// attached in this process.
func CountCorruptDataFileTableIDsLive(catalogPath string, reference *sql.DB) (int, error) {
	db, err := openCatalogScanDB(catalogPath, reference)
	if err != nil {
		return 0, err
	}
	defer db.Close()
	if ok, err := scanTableExists(db, "ducklake_data_file"); err != nil || !ok {
		return 0, err
	}
	var n int
	err = db.QueryRow(fmt.Sprintf(`SELECT count(*) FROM %s.ducklake_data_file
		WHERE NOT regexp_full_match(coalesce(table_id, ''), '[0-9]+')`, scanCatalogAlias)).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("scan ducklake_data_file: %w", err)
	}
	return n, nil
}

// ErrSQLiteCLIUnavailable means the sqlite3 command-line tool is not installed.
var ErrSQLiteCLIUnavailable = errors.New("sqlite3 CLI not found")

// QuickCheckCatalog runs PRAGMA quick_check on catalogPath in a sqlite3 child
// process, which is safe while DuckDB in this process has the file attached.
// It returns "" for a healthy catalog, otherwise SQLite's report.
func QuickCheckCatalog(catalogPath string) (string, error) {
	if _, err := exec.LookPath("sqlite3"); err != nil {
		return "", ErrSQLiteCLIUnavailable
	}
	out, err := exec.Command("sqlite3", "-readonly", catalogPath, "PRAGMA busy_timeout=30000; PRAGMA quick_check;").CombinedOutput()
	report := strings.TrimSpace(string(out))
	if err != nil {
		lower := strings.ToLower(report)
		if report == "" || strings.Contains(lower, "locked") || strings.Contains(lower, "busy") {
			return "", fmt.Errorf("sqlite3 quick_check: %w: %s", err, report)
		}
		return report, nil
	}
	lines := strings.Split(report, "\n")
	if last := strings.TrimSpace(lines[len(lines)-1]); last == "ok" {
		return "", nil
	}
	return report, nil
}
