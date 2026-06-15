package ducklake

import (
	"database/sql"
	"fmt"
	"os"
	"strings"

	_ "modernc.org/sqlite" // pure-Go SQLite driver for out-of-band catalog repair
)

// RepairResult summarizes what a startup catalog repair changed (or detected).
type RepairResult struct {
	DuplicateSnapshots       int // ducklake_snapshot rows removed
	DuplicateSnapshotChanges int // ducklake_snapshot_changes rows removed
	DuplicateTables          int // ducklake_table rows removed (by table_id)
	// LatestSnapshotRows is the number of rows matching the latest snapshot_id
	// AFTER repair, evaluated the way DuckLake itself evaluates it. >1 means the
	// fatal "multiple snapshots returned" condition is still present and the
	// lightweight repair was not enough (operator must --rebuild-catalog).
	LatestSnapshotRows int

	// --- Detection-only signals (NOT auto-fixed) -------------------------------
	// These are the broader corruption shapes seen when two writers raced on the
	// same catalog (sipcapture/homer#809). We deliberately do not auto-delete
	// them: dropping a duplicate table/column/schema row can orphan the
	// data-file rows that reference its table_id and silently lose data. They are
	// surfaced so the operator runs the authoritative `--rebuild-catalog`.

	// DuplicateTableNames counts ducklake_table rows that share a table_name but
	// were registered with different table_ids (two writers each created the
	// table). Cleaning these correctly requires knowing which table_id the
	// data files reference, so we only report it.
	DuplicateTableNames int
	// Malformed is set when an operation failed because the SQLite file is
	// physically corrupt ("database disk image is malformed"). Only a
	// .dump/reimport salvage or a full rebuild recovers from this.
	Malformed bool
}

// Changed reports whether the repair removed anything.
func (r RepairResult) Changed() bool {
	return r.DuplicateSnapshots > 0 || r.DuplicateSnapshotChanges > 0 || r.DuplicateTables > 0
}

// NeedsRebuild reports whether the lightweight repair was insufficient and the
// operator should run `homer system --rebuild-catalog`.
func (r RepairResult) NeedsRebuild() bool {
	return r.Malformed || r.DuplicateTableNames > 0 || !r.Healthy()
}

// isMalformedErr reports whether a SQLite error indicates physical file
// corruption that row-level repair cannot fix.
func isMalformedErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "malformed") || strings.Contains(msg, "disk image") ||
		strings.Contains(msg, "database is corrupt") || strings.Contains(msg, "not a database")
}

// Healthy reports whether, after repair, the catalog presents exactly one
// latest snapshot (the precondition DuckLake's GetSnapshot requires). A value
// of 0 means the check could not run (e.g. fresh/missing catalog) and is
// treated as healthy.
func (r RepairResult) Healthy() bool {
	return r.LatestSnapshotRows <= 1
}

// RepairCatalogSnapshots removes duplicate metadata rows from a DuckLake SQLite
// catalog that produce the fatal "Corrupt DuckLake - multiple snapshots
// returned from database" error.
//
// Root cause: when two writers (or a writer + an out-of-band native compactor)
// committed against the same SQLite catalog without coordination, they could
// insert two rows sharing the same snapshot_id (or table_id). DuckLake's
// get-by-id reads then return >1 row and abort the whole query path. DuckLake's
// own latest-snapshot probe is:
//
//	SELECT ... FROM ducklake_snapshot
//	WHERE snapshot_id = (SELECT MAX(snapshot_id) FROM ducklake_snapshot);
//
// so anything that makes that return >1 row is fatal.
//
// The fix is surgical and lossless for data: for each duplicated key we keep
// the most recently written row (highest rowid) and delete the rest. The kept
// row still references the same Parquet data files, so no data is lost — only
// the conflicting duplicate metadata rows are dropped.
//
// Two robustness details matter here, because the previous implementation could
// silently find "0 duplicates" while DuckDB still aborted:
//
//  1. We checkpoint the WAL (TRUNCATE) before AND after so a leftover, never
//     checkpointed WAL from an OOM-killed writer is folded into the main DB and
//     the cleaned state is what DuckDB reads when it ATTACHes.
//  2. We group by CAST(key AS INTEGER), not the raw column. SQLite groups by
//     storage class, so an id stored once as INTEGER 5 and once as TEXT '5'
//     looks like two distinct keys to a plain GROUP BY (no duplicate found),
//     yet DuckLake reads the column as BIGINT and sees two rows with the same
//     id — exactly the "multiple snapshots" abort. Casting makes our view match
//     DuckLake's.
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

	// Fold a leftover WAL (e.g. from an OOM-killed previous writer) into the
	// main DB so the rows we inspect are the same rows DuckDB will read.
	checkpointWAL(db)

	// ducklake_snapshot: collapse duplicate snapshot_id rows, keeping the last.
	if exists, _ := sqliteTableExists(db, "ducklake_snapshot"); exists {
		n, err := dedupeByKey(db, "ducklake_snapshot", "snapshot_id")
		if err != nil {
			res.Malformed = isMalformedErr(err)
			return res, err
		}
		res.DuplicateSnapshots = n
	}

	// ducklake_snapshot_changes shares the snapshot_id PK and is read alongside
	// the snapshot during commit/cleanup; collapse its duplicates too.
	if exists, _ := sqliteTableExists(db, "ducklake_snapshot_changes"); exists {
		n, err := dedupeByKey(db, "ducklake_snapshot_changes", "snapshot_id")
		if err != nil {
			res.Malformed = isMalformedErr(err)
			return res, err
		}
		res.DuplicateSnapshotChanges = n
	}

	// ducklake_table: duplicate table_id rows are the secondary corruption two
	// concurrent writers create (duplicate table registration). Same fix.
	if exists, _ := sqliteTableExists(db, "ducklake_table"); exists {
		n, err := dedupeByKey(db, "ducklake_table", "table_id")
		if err != nil {
			res.Malformed = isMalformedErr(err)
			return res, err
		}
		res.DuplicateTables = n

		// Detect (do NOT auto-fix) the harder case from sipcapture/homer#809:
		// two writers each registered the same table_name under a different
		// table_id. Removing one orphans the data files referencing its id, so
		// this requires `--rebuild-catalog`, not a blind dedup.
		res.DuplicateTableNames = countDuplicateTableNames(db)
	}

	// Push the cleaned state back into the main DB file so the about-to-run
	// DuckLake ATTACH reads it without depending on our WAL frames.
	checkpointWAL(db)

	// Verify the way DuckLake does: how many rows match the latest snapshot_id?
	// If still >1 the lightweight repair could not resolve it and the caller
	// should surface the --rebuild-catalog hint.
	if exists, _ := sqliteTableExists(db, "ducklake_snapshot"); exists {
		res.LatestSnapshotRows = countLatestSnapshotRows(db)
	}

	return res, nil
}

// checkpointWAL best-effort folds the WAL back into the main database file.
func checkpointWAL(db *sql.DB) {
	// TRUNCATE checkpoints and then truncates the WAL to zero bytes.
	_, _ = db.Exec("PRAGMA wal_checkpoint(TRUNCATE)")
}

// countDuplicateTableNames returns how many extra (active) ducklake_table rows
// share a table_name with another row — the duplicate-registration corruption
// from sipcapture/homer#809. Only currently-live rows (end_snapshot IS NULL)
// are considered so historically renamed/dropped tables don't count. Returns 0
// on any error (best-effort detection).
func countDuplicateTableNames(db *sql.DB) int {
	// end_snapshot may not exist on very old catalogs; fall back to all rows.
	filter := ""
	if columnExists(db, "ducklake_table", "end_snapshot") {
		filter = "WHERE end_snapshot IS NULL"
	}
	q := fmt.Sprintf(
		`SELECT COALESCE(SUM(c-1),0) FROM (SELECT COUNT(*) c FROM ducklake_table %s GROUP BY table_name HAVING c>1)`,
		filter)
	var n int
	if err := db.QueryRow(q).Scan(&n); err != nil {
		return 0
	}
	return n
}

// columnExists reports whether a column is present on a SQLite table.
func columnExists(db *sql.DB, table, column string) bool {
	rows, err := db.Query(fmt.Sprintf("PRAGMA table_info(%q)", table))
	if err != nil {
		return false
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return false
		}
		if name == column {
			return true
		}
	}
	return false
}

// countLatestSnapshotRows reproduces DuckLake's latest-snapshot probe under the
// same integer interpretation DuckLake uses. Returns the number of rows that
// share the maximum snapshot_id (the value DuckLake's ParseSnapshot iterates).
// Returns -1 if the probe itself errors.
func countLatestSnapshotRows(db *sql.DB) int {
	const q = `SELECT COUNT(*) FROM ducklake_snapshot
WHERE CAST(snapshot_id AS INTEGER) = (SELECT MAX(CAST(snapshot_id AS INTEGER)) FROM ducklake_snapshot)`
	var n int
	if err := db.QueryRow(q).Scan(&n); err != nil {
		return -1
	}
	return n
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
//
// The key is normalized with CAST(... AS INTEGER) so that ids differing only by
// SQLite storage class (INTEGER 5 vs TEXT '5') are treated as the same key —
// matching how DuckLake reads these BIGINT id columns.
func dedupeByKey(db *sql.DB, table, keyCol string) (int, error) {
	keyExpr := fmt.Sprintf(`CAST("%s" AS INTEGER)`, keyCol)

	// Count duplicates first so we can skip the (locking) DELETE on a healthy
	// catalog — the common case on every startup.
	var dupRows int
	countSQL := fmt.Sprintf(
		`SELECT COALESCE(SUM(c-1),0) FROM (SELECT COUNT(*) c FROM "%s" GROUP BY %s HAVING c>1)`,
		table, keyExpr)
	if err := db.QueryRow(countSQL).Scan(&dupRows); err != nil {
		return 0, fmt.Errorf("count duplicate %s: %w", keyCol, err)
	}
	if dupRows == 0 {
		return 0, nil
	}

	delSQL := fmt.Sprintf(
		`DELETE FROM "%s" WHERE rowid NOT IN (SELECT MAX(rowid) FROM "%s" GROUP BY %s)`,
		table, table, keyExpr)
	resExec, err := db.Exec(delSQL)
	if err != nil {
		return 0, fmt.Errorf("dedupe %s by %s: %w", table, keyCol, err)
	}
	n, _ := resExec.RowsAffected()
	return int(n), nil
}
