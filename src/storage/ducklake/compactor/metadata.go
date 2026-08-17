package compactor

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// metadataSchema is the DuckDB schema through which an ATTACHed DuckLake
// exposes its catalog tables, e.g. "__ducklake_metadata_homer_lake".
func metadataSchema(lakeName string) string {
	return quoteIdent("__ducklake_metadata_" + lakeName)
}

// quoteIdent renders a SQL identifier, doubling embedded quotes.
func quoteIdent(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}

// tableRef is the fully qualified name of a lake table, e.g.
// "homer_lake"."main"."hep_proto_1_call".
func tableRef(lakeName, tableName string) string {
	return quoteIdent(lakeName) + "." + quoteIdent("main") + "." + quoteIdent(tableName)
}

// safeTypeName matches the DuckLake column_type strings we are willing to
// interpolate into a CAST, e.g. "date", "varchar", "decimal(18,3)". Anything
// else is rejected rather than risk building malformed or unsafe SQL.
var safeTypeName = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9 _]*(\([0-9]+(,[0-9]+)?\))?$`)

// partitionKey describes the single partition column the compactor supports.
// Only an identity transform is usable: the compactor retires a partition with
// an equality predicate on this column, which is exact only when the stored
// partition_value maps 1:1 to a column value.
type partitionKey struct {
	columnName string
	columnType string
	transform  string
}

// tableMeta is the metadata needed to plan and register one table's compaction.
type tableMeta struct {
	tableID   int64
	tablePath string
	partition partitionKey
}

// sourceFile is one active parquet file of a table within a single partition.
type sourceFile struct {
	dataFileID    int64
	path          string // as stored in ducklake_data_file (usually relative)
	pathIsRel     int64
	recordCount   int64
	fileSizeBytes int64
	rowIDStart    sql.NullInt64
	partitionVal  string
	// ageSec is how long ago the snapshot that added this file was taken, used to
	// tell a partition ingest is still writing from one that has settled.
	ageSec sql.NullInt64
}

// readTableMeta resolves the active table id, its data path and its partition
// key. ok is false when the table does not exist.
func readTableMeta(ctx context.Context, db *sql.DB, lakeName, tableName string) (tableMeta, bool, error) {
	md := metadataSchema(lakeName)
	var m tableMeta
	err := db.QueryRowContext(ctx, fmt.Sprintf(
		`SELECT table_id, path FROM %s.ducklake_table
		  WHERE table_name = ? AND end_snapshot IS NULL`, md), tableName).
		Scan(&m.tableID, &m.tablePath)
	if err == sql.ErrNoRows {
		return m, false, nil
	}
	if err != nil {
		return m, false, fmt.Errorf("read table %q: %w", tableName, err)
	}

	pk, err := readPartitionKey(ctx, db, lakeName, m.tableID)
	if err != nil {
		return m, false, err
	}
	m.partition = pk
	return m, true, nil
}

// readPartitionKey returns the table's sole partition column. A table with no
// partitioning, or with more than one partition key, yields an empty
// columnName: the compactor cannot express a safe retirement predicate for it
// and skips the table.
func readPartitionKey(ctx context.Context, db *sql.DB, lakeName string, tableID int64) (partitionKey, error) {
	md := metadataSchema(lakeName)
	rows, err := db.QueryContext(ctx, fmt.Sprintf(
		`SELECT c.column_name, c.column_type, pc.transform
		   FROM %[1]s.ducklake_partition_info pi
		   JOIN %[1]s.ducklake_partition_column pc
		     ON pc.partition_id = pi.partition_id AND pc.table_id = pi.table_id
		   JOIN %[1]s.ducklake_column c
		     ON c.table_id = pi.table_id AND c.column_id = pc.column_id
		    AND c.end_snapshot IS NULL
		  WHERE pi.table_id = ? AND pi.end_snapshot IS NULL
		  ORDER BY pc.partition_key_index`, md), tableID)
	if err != nil {
		return partitionKey{}, fmt.Errorf("read partition key: %w", err)
	}
	defer rows.Close()

	var keys []partitionKey
	for rows.Next() {
		var k partitionKey
		if err := rows.Scan(&k.columnName, &k.columnType, &k.transform); err != nil {
			return partitionKey{}, err
		}
		keys = append(keys, k)
	}
	if err := rows.Err(); err != nil {
		return partitionKey{}, err
	}
	if len(keys) != 1 {
		return partitionKey{}, nil
	}
	return keys[0], nil
}

// usable reports whether a partition key supports the compactor's swap: it must
// exist, use the identity transform (so partition_value equals a column value),
// and have a type safe to CAST to.
func (p partitionKey) usable() bool {
	return p.columnName != "" &&
		strings.EqualFold(p.transform, "identity") &&
		safeTypeName.MatchString(p.columnType)
}

// hasDeleteFiles reports whether the table has active row-level delete files.
// The compactor rewrites whole partitions and cannot preserve row-level deletes,
// so it skips such tables.
func hasDeleteFiles(ctx context.Context, db *sql.DB, lakeName string, tableID int64) (bool, error) {
	var n int64
	err := db.QueryRowContext(ctx, fmt.Sprintf(
		`SELECT COUNT(*) FROM %s.ducklake_delete_file
		  WHERE table_id = ? AND end_snapshot IS NULL`, metadataSchema(lakeName)), tableID).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("count delete files: %w", err)
	}
	return n > 0, nil
}

// activeFilesByPartition returns the table's active parquet files grouped by
// partition value, each group ordered by row_id_start so concatenation
// preserves the catalog's logical ordering. Files without a partition value are
// grouped under "" and are never compacted (no safe retirement predicate).
func activeFilesByPartition(ctx context.Context, db *sql.DB, lakeName string, tableID int64) (map[string][]sourceFile, error) {
	md := metadataSchema(lakeName)
	// The age of a file comes from the snapshot that added it, not from its mtime:
	// storage_policy moves parquet between volumes, which rewrites mtime and would
	// make old data look freshly written.
	rows, err := db.QueryContext(ctx, fmt.Sprintf(
		`SELECT df.data_file_id, df.path, df.path_is_relative,
		        df.record_count, df.file_size_bytes, df.row_id_start,
		        COALESCE(pv.partition_value, ''),
		        -- Computed in SQL against now(), which is exactly how DuckLake
		        -- itself compares snapshot_time (expire_snapshots older_than =>
		        -- NOW()). Reading the raw value into Go instead would need us to
		        -- guess whether the catalog stores local or UTC wall clock.
		        date_diff('second', TRY_CAST(s.snapshot_time AS TIMESTAMP), now()::TIMESTAMP)
		   FROM %[1]s.ducklake_data_file df
		   LEFT JOIN %[1]s.ducklake_file_partition_value pv
		          ON pv.data_file_id = df.data_file_id
		         AND pv.table_id = df.table_id
		         AND pv.partition_key_index = 0
		   LEFT JOIN %[1]s.ducklake_snapshot s
		          ON s.snapshot_id = df.begin_snapshot
		  WHERE df.table_id = ? AND df.end_snapshot IS NULL`, md), tableID)
	if err != nil {
		return nil, fmt.Errorf("read active files: %w", err)
	}
	defer rows.Close()

	groups := map[string][]sourceFile{}
	for rows.Next() {
		var f sourceFile
		if err := rows.Scan(
			&f.dataFileID, &f.path, &f.pathIsRel,
			&f.recordCount, &f.fileSizeBytes, &f.rowIDStart, &f.partitionVal,
			&f.ageSec,
		); err != nil {
			return nil, err
		}
		groups[f.partitionVal] = append(groups[f.partitionVal], f)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, files := range groups {
		sort.SliceStable(files, func(i, j int) bool {
			return files[i].rowIDStart.Int64 < files[j].rowIDStart.Int64
		})
	}
	return groups, nil
}

// hasInlinedData reports whether the lake still holds catalog-resident (inlined)
// rows. Inlined rows are invisible to the parquet merge but would be removed by
// the retirement DELETE, so the compactor refuses to run until they are flushed.
func hasInlinedData(ctx context.Context, db *sql.DB, lakeName string) (bool, error) {
	var n int64
	err := db.QueryRowContext(ctx, fmt.Sprintf(
		`SELECT COUNT(*) FROM %s.ducklake_inlined_data_tables`, metadataSchema(lakeName))).Scan(&n)
	if err != nil {
		// Older catalogs may not have the table at all, which means no inlining.
		if strings.Contains(strings.ToLower(err.Error()), "does not exist") {
			return false, nil
		}
		return false, fmt.Errorf("count inlined data tables: %w", err)
	}
	return n > 0, nil
}
