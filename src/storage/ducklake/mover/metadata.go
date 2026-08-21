package mover

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strings"
)

func metadataSchema(lakeName string) string {
	return quoteIdent("__ducklake_metadata_" + lakeName)
}

func quoteIdent(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}

func quoteLiteral(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

func tableRef(lakeName, tableName string) string {
	return quoteIdent(lakeName) + "." + quoteIdent("main") + "." + quoteIdent(tableName)
}

var safeTypeName = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9 _]*(\([0-9]+(,[0-9]+)?\))?$`)

type partitionKey struct {
	columnName string
	columnType string
	transform  string
}

func (p partitionKey) usable() bool {
	return p.columnName != "" &&
		strings.EqualFold(p.transform, "identity") &&
		safeTypeName.MatchString(p.columnType)
}

type tableMeta struct {
	tableID   int64
	tablePath string
	partition partitionKey
}

type sourceFile struct {
	path          string
	pathIsRel     int64
	recordCount   int64
	fileSizeBytes int64
	hasDeletes    bool
}

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

func hasInlinedData(ctx context.Context, db *sql.DB, lakeName string) (bool, error) {
	var n int64
	err := db.QueryRowContext(ctx, fmt.Sprintf(
		`SELECT COUNT(*) FROM %s.ducklake_inlined_data_tables`, metadataSchema(lakeName))).Scan(&n)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "does not exist") {
			return false, nil
		}
		return false, fmt.Errorf("count inlined data tables: %w", err)
	}
	return n > 0, nil
}

func partitionFiles(ctx context.Context, db *sql.DB, lakeName string, tableID int64, partVal string) ([]sourceFile, error) {
	md := metadataSchema(lakeName)
	rows, err := db.QueryContext(ctx, fmt.Sprintf(
		`SELECT df.path, df.path_is_relative, df.record_count, df.file_size_bytes,
		        EXISTS (SELECT 1 FROM %[1]s.ducklake_delete_file dfx
		                 WHERE dfx.data_file_id = df.data_file_id
		                   AND dfx.table_id = df.table_id
		                   AND dfx.end_snapshot IS NULL),
		        COALESCE(pv.partition_value, '')
		   FROM %[1]s.ducklake_data_file df
		   LEFT JOIN %[1]s.ducklake_file_partition_value pv
		          ON pv.data_file_id = df.data_file_id
		         AND pv.table_id = df.table_id
		         AND pv.partition_key_index = 0
		  WHERE df.table_id = ? AND df.end_snapshot IS NULL`, md), tableID)
	if err != nil {
		return nil, fmt.Errorf("read active files: %w", err)
	}
	defer rows.Close()

	var out []sourceFile
	for rows.Next() {
		var f sourceFile
		var pv string
		if err := rows.Scan(&f.path, &f.pathIsRel, &f.recordCount, &f.fileSizeBytes, &f.hasDeletes, &pv); err != nil {
			return nil, err
		}
		if !partitionValueMatches(pv, partVal) {
			continue
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

func partitionValueMatches(stored, want string) bool {
	if stored == want {
		return true
	}
	// DuckLake may store a DATE as "2026-07-18" or "2026-07-18 00:00:00".
	if len(stored) >= 10 && len(want) >= 10 && stored[:10] == want[:10] {
		return true
	}
	return false
}

func filesHaveDeletes(files []sourceFile) bool {
	for _, f := range files {
		if f.hasDeletes {
			return true
		}
	}
	return false
}

func sumRecords(files []sourceFile) int64 {
	var n int64
	for _, f := range files {
		n += f.recordCount
	}
	return n
}

func addDataFilesSupported(ctx context.Context, db *sql.DB) (bool, string) {
	if db == nil {
		return false, "no database handle"
	}
	var n int64
	err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM duckdb_functions() WHERE function_name = 'ducklake_add_data_files'`).Scan(&n)
	if err != nil {
		return false, fmt.Sprintf("cannot probe duckdb_functions(): %v", err)
	}
	if n == 0 {
		return false, "ducklake_add_data_files is not available in the loaded ducklake extension"
	}
	return true, ""
}
