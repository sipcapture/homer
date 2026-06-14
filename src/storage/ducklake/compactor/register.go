package compactor

import (
	"database/sql"
	"fmt"
	"time"
)

// duckLakeTimeLayout matches the timestamp text DuckLake writes into
// ducklake_snapshot.snapshot_time / schedule_start, e.g. "2026-06-14 00:44:08.483832+02".
const duckLakeTimeLayout = "2006-01-02 15:04:05.000000-07"

func nowDuckLakeTime() string {
	return time.Now().Format(duckLakeTimeLayout)
}

// newFile is a compacted output file to register in the catalog.
type newFile struct {
	relPath        string // path relative to the per-table directory
	fileFormat     string
	recordCount    int64
	fileSizeBytes  int64
	footerSize     int64
	partitionID    sql.NullInt64
	mappingID      sql.NullInt64
	fileOrder      sql.NullInt64
	partitionValue string
	stats          []aggregatedColStat
	// batchFiles holds the source files merged into this output; used at commit
	// time to read their column stats under the lock. Not persisted directly.
	batchFiles []sourceFile
}

// retiredFile is an existing data file superseded by compaction.
type retiredFile struct {
	dataFileID int64
	path       string
	pathIsRel  int64
}

// commitInput is one atomic compaction snapshot for a single table.
type commitInput struct {
	tableID      int64
	newFiles     []newFile
	retired      []retiredFile
	netSizeDelta int64 // sum(new file sizes) - sum(retired file sizes)
}

// commit writes one DuckLake snapshot that registers newFiles and retires the
// old ones, in a single SQLite transaction. New files are allocated fresh
// data_file_ids (from the snapshot's next_file_id) and fresh row_id_start
// values (from ducklake_table_stats.next_row_id); fresh row ids guarantee no
// overlap with any surviving file and are safe because the table is verified
// append-only (no delete files). Returns the new snapshot_id.
func (c *Catalog) commit(in commitInput) (int64, error) {
	tx, err := c.db.Begin()
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	var s snapshotState
	if err := tx.QueryRow(
		`SELECT snapshot_id, schema_version, next_catalog_id, next_file_id
		   FROM ducklake_snapshot ORDER BY snapshot_id DESC LIMIT 1`,
	).Scan(&s.snapshotID, &s.schemaVersion, &s.nextCatalogID, &s.nextFileID); err != nil {
		return 0, fmt.Errorf("read latest snapshot in tx: %w", err)
	}

	var ts tableStats
	if err := tx.QueryRow(
		`SELECT record_count, next_row_id, file_size_bytes
		   FROM ducklake_table_stats WHERE table_id = ?`, in.tableID,
	).Scan(&ts.recordCount, &ts.nextRowID, &ts.fileSizeBytes); err != nil {
		return 0, fmt.Errorf("read table stats in tx: %w", err)
	}

	newSnap := s.snapshotID + 1
	now := nowDuckLakeTime()

	nextFileID := s.nextFileID
	nextRowID := ts.nextRowID
	var addedRows int64

	for _, nf := range in.newFiles {
		dataFileID := nextFileID
		nextFileID++
		rowIDStart := nextRowID
		nextRowID += nf.recordCount
		addedRows += nf.recordCount

		format := nf.fileFormat
		if format == "" {
			format = "parquet"
		}
		if _, err := tx.Exec(
			`INSERT INTO ducklake_data_file
			   (data_file_id, table_id, begin_snapshot, end_snapshot, file_order,
			    path, path_is_relative, file_format, record_count, file_size_bytes,
			    footer_size, row_id_start, partition_id, encryption_key, mapping_id, partial_max)
			 VALUES (?, ?, ?, NULL, ?, ?, 1, ?, ?, ?, ?, ?, ?, NULL, ?, NULL)`,
			dataFileID, in.tableID, newSnap, nf.fileOrder,
			nf.relPath, format, nf.recordCount, nf.fileSizeBytes,
			nf.footerSize, rowIDStart, nf.partitionID, nf.mappingID,
		); err != nil {
			return 0, fmt.Errorf("insert data_file: %w", err)
		}

		for _, st := range nf.stats {
			var minV, maxV any
			if st.minValue != nil {
				minV = *st.minValue
			}
			if st.maxValue != nil {
				maxV = *st.maxValue
			}
			var nan int64
			if st.containsNaN {
				nan = 1
			}
			if _, err := tx.Exec(
				`INSERT INTO ducklake_file_column_stats
				   (data_file_id, table_id, column_id, column_size_bytes, value_count,
				    null_count, min_value, max_value, contains_nan, extra_stats)
				 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, NULL)`,
				dataFileID, in.tableID, st.columnID, st.columnSizeBytes,
				st.valueCount, st.nullCount, minV, maxV, nan,
			); err != nil {
				return 0, fmt.Errorf("insert file_column_stats: %w", err)
			}
		}

		if nf.partitionValue != "" {
			if _, err := tx.Exec(
				`INSERT INTO ducklake_file_partition_value
				   (data_file_id, table_id, partition_key_index, partition_value)
				 VALUES (?, ?, 0, ?)`,
				dataFileID, in.tableID, nf.partitionValue,
			); err != nil {
				return 0, fmt.Errorf("insert file_partition_value: %w", err)
			}
		}
	}

	for _, rf := range in.retired {
		if _, err := tx.Exec(
			`UPDATE ducklake_data_file SET end_snapshot = ?
			  WHERE data_file_id = ? AND table_id = ? AND end_snapshot IS NULL`,
			newSnap, rf.dataFileID, in.tableID,
		); err != nil {
			return 0, fmt.Errorf("retire data_file %d: %w", rf.dataFileID, err)
		}
		if _, err := tx.Exec(
			`INSERT INTO ducklake_files_scheduled_for_deletion
			   (data_file_id, path, path_is_relative, schedule_start)
			 VALUES (?, ?, ?, ?)`,
			rf.dataFileID, rf.path, rf.pathIsRel, now,
		); err != nil {
			return 0, fmt.Errorf("schedule deletion %d: %w", rf.dataFileID, err)
		}
	}

	if _, err := tx.Exec(
		`INSERT INTO ducklake_snapshot
		   (snapshot_id, snapshot_time, schema_version, next_catalog_id, next_file_id)
		 VALUES (?, ?, ?, ?, ?)`,
		newSnap, now, s.schemaVersion, s.nextCatalogID, nextFileID,
	); err != nil {
		return 0, fmt.Errorf("insert snapshot: %w", err)
	}

	if _, err := tx.Exec(
		`INSERT INTO ducklake_snapshot_changes
		   (snapshot_id, changes_made, author, commit_message, commit_extra_info)
		 VALUES (?, ?, '', '', '')`,
		newSnap, fmt.Sprintf("compacted_table:%d", in.tableID),
	); err != nil {
		return 0, fmt.Errorf("insert snapshot_changes: %w", err)
	}

	if _, err := tx.Exec(
		`UPDATE ducklake_table_stats
		    SET next_row_id = next_row_id + ?, file_size_bytes = file_size_bytes + ?
		  WHERE table_id = ?`,
		addedRows, in.netSizeDelta, in.tableID,
	); err != nil {
		return 0, fmt.Errorf("update table_stats: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit tx: %w", err)
	}
	committed = true
	return newSnap, nil
}
