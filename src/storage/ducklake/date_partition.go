// Copyright (C) 2026 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package ducklake

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	logger "github.com/sipcapture/homer-core/src/utils/logging"
)

// DateColumn is the daily partition column shared by HEP, OTLP and Line
// Protocol tables.
const DateColumn = "date"

func quoteLakeIdent(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}

func lakeMetadataSchema(lakeName string) string {
	return quoteLakeIdent("__ducklake_metadata_" + lakeName)
}

// IsDuckLakeCatalog reports whether lakeName is an attached DuckLake catalog.
func IsDuckLakeCatalog(ctx context.Context, db *sql.DB, lakeName string) bool {
	var key string
	err := db.QueryRowContext(ctx,
		"SELECT key FROM "+lakeMetadataSchema(lakeName)+".ducklake_metadata LIMIT 1").Scan(&key)
	return err == nil
}

// partitionColumns lists the current partition key columns of a table.
// ok is false when the catalog is not DuckLake.
func partitionColumns(ctx context.Context, db *sql.DB, lakeName, schema, table string) ([]string, bool) {
	md := lakeMetadataSchema(lakeName)
	rows, err := db.QueryContext(ctx, `
		SELECT c.column_name
		FROM `+md+`.ducklake_partition_info pi
		JOIN `+md+`.ducklake_partition_column pc
		  ON pc.partition_id = pi.partition_id AND pc.table_id = pi.table_id
		JOIN `+md+`.ducklake_column c
		  ON c.table_id = pi.table_id AND c.column_id = pc.column_id AND c.end_snapshot IS NULL
		JOIN `+md+`.ducklake_table t
		  ON t.table_id = pi.table_id AND t.end_snapshot IS NULL
		JOIN `+md+`.ducklake_schema s
		  ON s.schema_id = t.schema_id AND s.end_snapshot IS NULL
		WHERE pi.end_snapshot IS NULL AND t.table_name = ? AND s.schema_name = ?
		ORDER BY pc.partition_key_index`, table, schema)
	if err != nil {
		return nil, false
	}
	defer rows.Close()
	var cols []string
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			return nil, false
		}
		cols = append(cols, c)
	}
	return cols, rows.Err() == nil
}

// EnsureDatePartition gives a DuckLake table daily partitioning: a DATE column
// `date` derived from timeCol, PARTITIONED BY (date) and SORTED BY timeCol.
// It returns true when writers must fill `date` from timeCol.
//
// A table without partitioning gets the column and the partitioning once;
// older rows keep date NULL. A table that is already partitioned keeps its
// keys. Tables whose `date` is not a DATE, tables without a TIMESTAMP timeCol
// and non-DuckLake catalogs are left alone. The DDL bumps the DuckLake schema
// version (duckdb/ducklake#1065), so it only runs while the table is
// unpartitioned.
func EnsureDatePartition(ctx context.Context, db *sql.DB, lakeName, schema, table, timeCol string) (bool, error) {
	if schema == "" {
		schema = "main"
	}
	types := map[string]string{}
	rows, err := db.QueryContext(ctx, `
		SELECT column_name, data_type FROM information_schema.columns
		WHERE table_catalog = ? AND table_schema = ? AND table_name = ?`,
		lakeName, schema, table)
	if err != nil {
		return false, err
	}
	for rows.Next() {
		var name, typ string
		if err := rows.Scan(&name, &typ); err != nil {
			rows.Close()
			return false, err
		}
		types[name] = strings.ToUpper(typ)
	}
	rows.Close()

	if !strings.HasPrefix(types[timeCol], "TIMESTAMP") {
		return false, nil
	}
	fqn := quoteLakeIdent(lakeName) + "." + quoteLakeIdent(schema) + "." + quoteLakeIdent(table)
	dateType, hasDate := types[DateColumn]
	if hasDate && dateType != "DATE" {
		logger.Info("ducklake: table not partitioned by date, its date column is not DATE",
			"table", fqn, "type", dateType)
		return false, nil
	}

	keys, ok := partitionColumns(ctx, db, lakeName, schema, table)
	if !ok {
		return false, nil
	}
	if len(keys) > 0 {
		return hasDate, nil
	}

	if !hasDate {
		if _, err := db.ExecContext(ctx, fmt.Sprintf("ALTER TABLE %s ADD COLUMN IF NOT EXISTS %s DATE",
			fqn, quoteLakeIdent(DateColumn))); err != nil {
			return false, fmt.Errorf("add date column to %s: %w", fqn, err)
		}
	}
	if _, err := db.ExecContext(ctx, fmt.Sprintf("ALTER TABLE %s SET PARTITIONED BY (%s)",
		fqn, quoteLakeIdent(DateColumn))); err != nil {
		return true, fmt.Errorf("partition %s by date: %w", fqn, err)
	}
	if _, err := db.ExecContext(ctx, fmt.Sprintf("ALTER TABLE %s SET SORTED BY (%s ASC)",
		fqn, quoteLakeIdent(timeCol))); err != nil {
		logger.Warn("ducklake: failed to set sort order", "table", fqn, "error", err)
	}
	logger.Info("ducklake: table partitioned by date", "table", fqn, "time_column", timeCol)
	return true, nil
}

// DatePartitionedTable is a main-schema table partitioned by the identity of
// its DATE `date` column, with the TIMESTAMP column retention deletes by.
type DatePartitionedTable struct {
	Name       string
	TimeColumn string
}

// DatePartitionedTables lists main-schema tables partitioned by `date` alone
// that have a `timestamp` or `ts` TIMESTAMP column. Line Protocol tables are
// found this way, since their names are the measurement names.
func DatePartitionedTables(ctx context.Context, db *sql.DB, lakeName string) ([]DatePartitionedTable, error) {
	md := lakeMetadataSchema(lakeName)
	rows, err := db.QueryContext(ctx, `
		WITH keys AS (
			SELECT t.table_name, any_value(c.column_name) AS col, any_value(pc.transform) AS transform,
			       count(*) AS n
			FROM `+md+`.ducklake_partition_info pi
			JOIN `+md+`.ducklake_partition_column pc
			  ON pc.partition_id = pi.partition_id AND pc.table_id = pi.table_id
			JOIN `+md+`.ducklake_column c
			  ON c.table_id = pi.table_id AND c.column_id = pc.column_id AND c.end_snapshot IS NULL
			JOIN `+md+`.ducklake_table t
			  ON t.table_id = pi.table_id AND t.end_snapshot IS NULL
			JOIN `+md+`.ducklake_schema s
			  ON s.schema_id = t.schema_id AND s.end_snapshot IS NULL
			WHERE pi.end_snapshot IS NULL AND s.schema_name = 'main'
			GROUP BY t.table_name
		)
		SELECT k.table_name,
		       min(CASE i.column_name WHEN 'timestamp' THEN 0 ELSE 1 END) AS pick
		FROM keys k
		JOIN information_schema.columns d
		  ON d.table_catalog = ? AND d.table_schema = 'main' AND d.table_name = k.table_name
		 AND d.column_name = 'date' AND upper(d.data_type) = 'DATE'
		JOIN information_schema.columns i
		  ON i.table_catalog = ? AND i.table_schema = 'main' AND i.table_name = k.table_name
		 AND i.column_name IN ('timestamp', 'ts') AND upper(i.data_type) LIKE 'TIMESTAMP%'
		WHERE k.n = 1 AND k.col = 'date' AND k.transform = 'identity'
		GROUP BY k.table_name
		ORDER BY k.table_name`, lakeName, lakeName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []DatePartitionedTable
	for rows.Next() {
		var name string
		var pick int
		if err := rows.Scan(&name, &pick); err != nil {
			return nil, err
		}
		col := "timestamp"
		if pick == 1 {
			col = "ts"
		}
		out = append(out, DatePartitionedTable{Name: name, TimeColumn: col})
	}
	return out, rows.Err()
}
