package compactor

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// retiredFileRef is a fully-superseded data file safe to physically remove.
type retiredFileRef struct {
	dataFileID int64
	absPath    string
	dir        string
}

// reap removes data files that no retained snapshot can still see and prunes
// the snapshots/metadata that have aged out of the retention window.
//
// A file with end_snapshot = E becomes invisible to every retained snapshot
// once E <= minRetained, where minRetained is the smallest snapshot still kept
// for time travel (snapshots younger than window, plus always the latest). At
// that point the physical parquet and its catalog rows can be dropped.
//
// Ordering: catalog rows are deleted first and committed, then the physical
// files are unlinked. A crash in between leaves orphan parquet (no catalog
// reference), never a catalog row pointing at a missing file.
func (c *Catalog) reap(dataPath string, window time.Duration) (filesDeleted int, snapshotsPruned int, err error) {
	var latest int64
	if err := c.db.QueryRow(`SELECT MAX(snapshot_id) FROM ducklake_snapshot`).Scan(&latest); err != nil {
		return 0, 0, fmt.Errorf("read latest snapshot: %w", err)
	}

	minRetained, err := c.minRetainedSnapshot(latest, window)
	if err != nil {
		return 0, 0, err
	}

	tablePaths, err := c.tablePaths()
	if err != nil {
		return 0, 0, err
	}

	rows, err := c.db.Query(
		`SELECT data_file_id, table_id, path, path_is_relative
		   FROM ducklake_data_file
		  WHERE end_snapshot IS NOT NULL AND end_snapshot <= ?`,
		minRetained,
	)
	if err != nil {
		return 0, 0, fmt.Errorf("list superseded files: %w", err)
	}
	var refs []retiredFileRef
	for rows.Next() {
		var id, tableID, isRel int64
		var path string
		if err := rows.Scan(&id, &tableID, &path, &isRel); err != nil {
			rows.Close()
			return 0, 0, err
		}
		abs := path
		if isRel != 0 {
			abs = filepath.Join(dataPath, "main", tablePaths[tableID], path)
		}
		refs = append(refs, retiredFileRef{dataFileID: id, absPath: abs, dir: filepath.Dir(abs)})
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, 0, err
	}
	rows.Close()

	tx, err := c.db.Begin()
	if err != nil {
		return 0, 0, fmt.Errorf("begin reap tx: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	for _, r := range refs {
		for _, stmt := range []string{
			`DELETE FROM ducklake_file_column_stats WHERE data_file_id = ?`,
			`DELETE FROM ducklake_file_partition_value WHERE data_file_id = ?`,
			`DELETE FROM ducklake_files_scheduled_for_deletion WHERE data_file_id = ?`,
			`DELETE FROM ducklake_data_file WHERE data_file_id = ?`,
		} {
			if _, err := tx.Exec(stmt, r.dataFileID); err != nil {
				return 0, 0, fmt.Errorf("prune data_file %d: %w", r.dataFileID, err)
			}
		}
	}

	pruneRes, err := tx.Exec(`DELETE FROM ducklake_snapshot WHERE snapshot_id < ?`, minRetained)
	if err != nil {
		return 0, 0, fmt.Errorf("prune snapshots: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM ducklake_snapshot_changes WHERE snapshot_id < ?`, minRetained); err != nil {
		return 0, 0, fmt.Errorf("prune snapshot_changes: %w", err)
	}
	if n, e := pruneRes.RowsAffected(); e == nil {
		snapshotsPruned = int(n)
	}

	if err := tx.Commit(); err != nil {
		return 0, 0, fmt.Errorf("commit reap tx: %w", err)
	}
	committed = true

	dirs := map[string]struct{}{}
	for _, r := range refs {
		if err := os.Remove(r.absPath); err == nil || os.IsNotExist(err) {
			filesDeleted++
			dirs[r.dir] = struct{}{}
		}
	}
	for d := range dirs {
		// Remove now-empty date= directories (ignore non-empty/err).
		_ = os.Remove(d)
	}
	return filesDeleted, snapshotsPruned, nil
}

// minRetainedSnapshot returns the smallest snapshot_id that must be kept for
// time travel: the oldest snapshot still within the retention window, or the
// latest snapshot when every snapshot has aged out.
func (c *Catalog) minRetainedSnapshot(latest int64, window time.Duration) (int64, error) {
	cutoff := time.Now().Add(-window)
	rows, err := c.db.Query(`SELECT snapshot_id, snapshot_time FROM ducklake_snapshot`)
	if err != nil {
		return 0, fmt.Errorf("scan snapshots: %w", err)
	}
	defer rows.Close()
	minRetained := latest
	for rows.Next() {
		var id int64
		var ts string
		if err := rows.Scan(&id, &ts); err != nil {
			return 0, err
		}
		t, perr := time.Parse(duckLakeTimeLayout, ts)
		if perr != nil {
			// Unparseable timestamp: keep the snapshot conservatively.
			if id < minRetained {
				minRetained = id
			}
			continue
		}
		if t.After(cutoff) && id < minRetained {
			minRetained = id
		}
	}
	return minRetained, rows.Err()
}

// tablePaths maps table_id to the catalog table path (e.g. "hep_proto_1_call/").
func (c *Catalog) tablePaths() (map[int64]string, error) {
	rows, err := c.db.Query(`SELECT table_id, path FROM ducklake_table WHERE end_snapshot IS NULL`)
	if err != nil {
		return nil, fmt.Errorf("read table paths: %w", err)
	}
	defer rows.Close()
	out := map[int64]string{}
	for rows.Next() {
		var id int64
		var path string
		if err := rows.Scan(&id, &path); err != nil {
			return nil, err
		}
		out[id] = path
	}
	return out, rows.Err()
}
