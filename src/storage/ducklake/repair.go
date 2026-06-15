package ducklake

import (
	"database/sql"
	"fmt"
	"os"

	_ "modernc.org/sqlite" // pure-Go SQLite driver for out-of-band catalog repair
)

// RepairResult summarizes what a startup catalog repair changed.
type RepairResult struct {
	DuplicateSnapshots int // ducklake_snapshot rows removed
	DuplicateTables    int // ducklake_table rows removed
}

// Changed reports whether the repair removed anything.
func (r RepairResult) Changed() bool {
	return r.DuplicateSnapshots > 0 || r.DuplicateTables > 0
}

// RepairCatalogSnapshots removes duplicate metadata rows from a DuckLake SQLite
// catalog that produce the fatal "Corrupt DuckLake - multiple snapshots
// returned from database" error.
//
// Root cause: when two writers (or a writer + an out-of-band native compactor)
// committed against the same SQLite catalog without coordination, they could
// insert two rows sharing the same snapshot_id (or table_id). DuckLake's
// get-by-id reads then return >1 row and abort the whole query path.
//
// The fix is surgical and lossless for data: for each duplicated key we keep
// the most recently written row (highest rowid) and delete the rest. The kept
// row still references the same Parquet data files, so no data is lost — only
// the conflicting duplicate metadata rows are dropped.
//
// MUST be called while no other process has the catalog open (the same
// pre-ATTACH window GCOrphanInlineTables uses). Best-effort: a missing file or
// missing tables is a no-op; any operation error is returned so the caller can
// log it and continue (the worst case is the original corruption error, which
// the operator can still fix with `homer system --rebuild-catalog`).
func RepairCatalogSnapshots(catalogPath string) (RepairResult, error) {
	var res RepairResult
	if catalogPath == "" {
		return res, nil
	}
	if _, err := os.Stat(catalogPath); os.IsNotExist(err) {
		return res, nil // fresh catalog, nothing to repair
	}

	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(30000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(0)", catalogPath)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return res, fmt.Errorf("open catalog for repair: %w", err)
	}
	db.SetMaxOpenConns(1)
	defer db.Close()
	if err := db.Ping(); err != nil {
		return res, fmt.Errorf("ping catalog for repair: %w", err)
	}

	// ducklake_snapshot: collapse duplicate snapshot_id rows, keeping the last.
	if exists, _ := sqliteTableExists(db, "ducklake_snapshot"); exists {
		n, err := dedupeByKey(db, "ducklake_snapshot", "snapshot_id")
		if err != nil {
			return res, err
		}
		res.DuplicateSnapshots = n
	}

	// ducklake_table: duplicate table_id rows are the secondary corruption two
	// concurrent writers create (duplicate table registration). Same fix.
	if exists, _ := sqliteTableExists(db, "ducklake_table"); exists {
		n, err := dedupeByKey(db, "ducklake_table", "table_id")
		if err != nil {
			return res, err
		}
		res.DuplicateTables = n
	}

	return res, nil
}

// sqliteTableExists reports whether a table is present in the SQLite schema.
func sqliteTableExists(db *sql.DB, name string) (bool, error) {
	var x int
	err := db.QueryRow(
		"SELECT 1 FROM sqlite_master WHERE type='table' AND name=? LIMIT 1", name).Scan(&x)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return err == nil, err
}

// dedupeByKey deletes rows of table sharing the same keyCol value, keeping the
// one with the highest rowid (the most recently written). Returns the number of
// rows removed. The DELETE runs in a single implicit transaction.
func dedupeByKey(db *sql.DB, table, keyCol string) (int, error) {
	// Count duplicates first so we can skip the (locking) DELETE on a healthy
	// catalog — the common case on every startup.
	var dupRows int
	countSQL := fmt.Sprintf(
		`SELECT COALESCE(SUM(c-1),0) FROM (SELECT COUNT(*) c FROM "%s" GROUP BY "%s" HAVING c>1)`,
		table, keyCol)
	if err := db.QueryRow(countSQL).Scan(&dupRows); err != nil {
		return 0, fmt.Errorf("count duplicate %s: %w", keyCol, err)
	}
	if dupRows == 0 {
		return 0, nil
	}

	delSQL := fmt.Sprintf(
		`DELETE FROM "%s" WHERE rowid NOT IN (SELECT MAX(rowid) FROM "%s" GROUP BY "%s")`,
		table, table, keyCol)
	resExec, err := db.Exec(delSQL)
	if err != nil {
		return 0, fmt.Errorf("dedupe %s by %s: %w", table, keyCol, err)
	}
	n, _ := resExec.RowsAffected()
	return int(n), nil
}
