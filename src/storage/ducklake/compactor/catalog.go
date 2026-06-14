// Package compactor implements a DuckDB-free compactor for DuckLake catalogs.
//
// Instead of calling ducklake_merge_adjacent_files (which loads a whole
// partition into memory and OOMs on wide SIP parquet), this compactor:
//
//  1. concatenates a partition's parquet row groups into new files of at most
//     a target size, copying one row group at a time (peak memory ≈ one row
//     group), and
//  2. registers the new files / retires the old ones by writing the DuckLake
//     SQLite catalog directly through a pure-Go sqlite driver
//     (modernc.org/sqlite), matching the snapshot protocol exactly.
//
// It must run while the writer holds the DuckLake CatalogLock so it never
// races a DuckDB catalog mutation. The catalog file is opened with WAL +
// busy_timeout so concurrent DuckDB readers (the node) are tolerated.
package compactor

import (
	"database/sql"
	"fmt"
	"sort"

	// Pure-Go SQLite driver (no cgo). Registers driver name "sqlite".
	_ "modernc.org/sqlite"
)

// Catalog is a thin read/write wrapper over a DuckLake SQLite catalog file.
type Catalog struct {
	db *sql.DB
}

// snapshotState captures the bookkeeping carried by the latest snapshot.
type snapshotState struct {
	snapshotID    int64
	schemaVersion int64
	nextCatalogID int64
	nextFileID    int64
}

// tableStats mirrors a ducklake_table_stats row.
type tableStats struct {
	recordCount   int64
	nextRowID     int64
	fileSizeBytes int64
}

// columnDef is one active column of a table (id + DuckLake type string).
type columnDef struct {
	columnID int64
	name     string
	colType  string
}

// sourceFile is one active parquet file of a table within a single partition.
type sourceFile struct {
	dataFileID    int64
	path          string // path as stored in ducklake_data_file (usually relative)
	pathIsRel     int64
	fileFormat    string
	recordCount   int64
	fileSizeBytes int64
	footerSize    int64
	fileOrder     sql.NullInt64
	partitionID   sql.NullInt64
	mappingID     sql.NullInt64
	partitionVal  string // partition_value (the date= value)
}

// colStat mirrors a ducklake_file_column_stats row for one column of one file.
type colStat struct {
	columnID        int64
	columnSizeBytes int64
	valueCount      int64
	nullCount       int64
	minValue        sql.NullString
	maxValue        sql.NullString
	containsNaN     sql.NullInt64
}

// OpenCatalog opens the DuckLake SQLite catalog for read/write. WAL mode and a
// generous busy_timeout let it coexist with DuckDB readers/writers that hold
// short locks; the caller is still expected to serialize via CatalogLock.
func OpenCatalog(path string) (*Catalog, error) {
	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(30000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(0)", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open catalog %q: %w", path, err)
	}
	// One physical connection keeps the WAL/busy_timeout pragmas effective and
	// avoids surprising cross-connection lock contention for our short txns.
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping catalog %q: %w", path, err)
	}
	return &Catalog{db: db}, nil
}

// Close releases the underlying database handle.
func (c *Catalog) Close() error {
	if c == nil || c.db == nil {
		return nil
	}
	return c.db.Close()
}

// tableID returns the active table_id for a table name.
func (c *Catalog) tableID(name string) (int64, bool, error) {
	var id int64
	err := c.db.QueryRow(
		`SELECT table_id FROM ducklake_table WHERE table_name = ? AND end_snapshot IS NULL`,
		name,
	).Scan(&id)
	if err == sql.ErrNoRows {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("lookup table %q: %w", name, err)
	}
	return id, true, nil
}

// hasDeleteFiles reports whether the table has any delete files. Native
// compaction reassigns row_id_start, which is only safe when no delete file
// references the old row ids, so callers must skip compaction when true.
func (c *Catalog) hasDeleteFiles(tableID int64) (bool, error) {
	var n int64
	err := c.db.QueryRow(
		`SELECT COUNT(*) FROM ducklake_delete_file WHERE table_id = ? AND end_snapshot IS NULL`,
		tableID,
	).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("count delete files: %w", err)
	}
	return n > 0, nil
}

// columns returns the active columns of a table ordered by column_order.
func (c *Catalog) columns(tableID int64) ([]columnDef, error) {
	rows, err := c.db.Query(
		`SELECT column_id, column_name, column_type
		   FROM ducklake_column
		  WHERE table_id = ? AND end_snapshot IS NULL AND parent_column IS NULL
		  ORDER BY column_order`,
		tableID,
	)
	if err != nil {
		return nil, fmt.Errorf("read columns: %w", err)
	}
	defer rows.Close()
	var out []columnDef
	for rows.Next() {
		var cd columnDef
		if err := rows.Scan(&cd.columnID, &cd.name, &cd.colType); err != nil {
			return nil, err
		}
		out = append(out, cd)
	}
	return out, rows.Err()
}

// latestSnapshot returns the bookkeeping fields of the highest snapshot_id.
func (c *Catalog) latestSnapshot() (snapshotState, error) {
	var s snapshotState
	err := c.db.QueryRow(
		`SELECT snapshot_id, schema_version, next_catalog_id, next_file_id
		   FROM ducklake_snapshot
		  ORDER BY snapshot_id DESC
		  LIMIT 1`,
	).Scan(&s.snapshotID, &s.schemaVersion, &s.nextCatalogID, &s.nextFileID)
	if err != nil {
		return s, fmt.Errorf("read latest snapshot: %w", err)
	}
	return s, nil
}

// stats returns the ducklake_table_stats row for a table.
func (c *Catalog) stats(tableID int64) (tableStats, error) {
	var t tableStats
	err := c.db.QueryRow(
		`SELECT record_count, next_row_id, file_size_bytes
		   FROM ducklake_table_stats WHERE table_id = ?`,
		tableID,
	).Scan(&t.recordCount, &t.nextRowID, &t.fileSizeBytes)
	if err != nil {
		return t, fmt.Errorf("read table stats: %w", err)
	}
	return t, nil
}

// activeFilesByPartition returns active parquet files grouped by partition
// value (the date= value). Files within each group are ordered by row_id_start
// so concatenation preserves the catalog's logical ordering.
func (c *Catalog) activeFilesByPartition(tableID int64) (map[string][]sourceFile, error) {
	rows, err := c.db.Query(
		`SELECT df.data_file_id, df.path, df.path_is_relative, df.file_format,
		        df.record_count, df.file_size_bytes, df.footer_size,
		        df.file_order, df.partition_id, df.mapping_id, df.row_id_start,
		        COALESCE(pv.partition_value, '')
		   FROM ducklake_data_file df
		   LEFT JOIN ducklake_file_partition_value pv
		          ON pv.data_file_id = df.data_file_id
		         AND pv.table_id = df.table_id
		         AND pv.partition_key_index = 0
		  WHERE df.table_id = ? AND df.end_snapshot IS NULL`,
		tableID,
	)
	if err != nil {
		return nil, fmt.Errorf("read active files: %w", err)
	}
	defer rows.Close()

	type withOrder struct {
		f         sourceFile
		rowIDSort sql.NullInt64
	}
	groups := map[string][]withOrder{}
	for rows.Next() {
		var f sourceFile
		var rowIDStart sql.NullInt64
		if err := rows.Scan(
			&f.dataFileID, &f.path, &f.pathIsRel, &f.fileFormat,
			&f.recordCount, &f.fileSizeBytes, &f.footerSize,
			&f.fileOrder, &f.partitionID, &f.mappingID, &rowIDStart,
			&f.partitionVal,
		); err != nil {
			return nil, err
		}
		groups[f.partitionVal] = append(groups[f.partitionVal], withOrder{f: f, rowIDSort: rowIDStart})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := make(map[string][]sourceFile, len(groups))
	for k, v := range groups {
		sort.SliceStable(v, func(i, j int) bool {
			return v[i].rowIDSort.Int64 < v[j].rowIDSort.Int64
		})
		files := make([]sourceFile, len(v))
		for i := range v {
			files[i] = v[i].f
		}
		out[k] = files
	}
	return out, nil
}

// columnStats returns the per-column stats rows for one data file.
func (c *Catalog) columnStats(dataFileID, tableID int64) ([]colStat, error) {
	rows, err := c.db.Query(
		`SELECT column_id, column_size_bytes, value_count, null_count,
		        min_value, max_value, contains_nan
		   FROM ducklake_file_column_stats
		  WHERE data_file_id = ? AND table_id = ?`,
		dataFileID, tableID,
	)
	if err != nil {
		return nil, fmt.Errorf("read column stats: %w", err)
	}
	defer rows.Close()
	var out []colStat
	for rows.Next() {
		var s colStat
		if err := rows.Scan(
			&s.columnID, &s.columnSizeBytes, &s.valueCount, &s.nullCount,
			&s.minValue, &s.maxValue, &s.containsNaN,
		); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}
