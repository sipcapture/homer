// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

// Package ducklake provides tiered query support for DuckLake.
// TieredReader queries data across multiple storage volumes using UNION ALL.
package ducklake

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"

	logger "github.com/sipcapture/homer-core/src/utils/logging"
)

// TieredReader provides read operations across multiple storage volumes
type TieredReader struct {
	tieredStorage *TieredStorageManager
	db            *sql.DB
}

func sqlStringLiteral(value string) string {
	escaped := strings.ReplaceAll(value, "'", "''")
	return "'" + escaped + "'"
}

// NewTieredReader creates a reader for tiered storage
func NewTieredReader(tsm *TieredStorageManager) *TieredReader {
	return &TieredReader{
		tieredStorage: tsm,
		db:            tsm.GetDB(),
	}
}

// Query executes a query on a specific table across all volumes
func (tr *TieredReader) Query(tableName string, whereClause string, limit int) ([]map[string]interface{}, error) {
	volumes := tr.tieredStorage.GetVolumes()
	if len(volumes) == 0 {
		return nil, fmt.Errorf("no volumes configured")
	}

	var allResults []map[string]interface{}

	for _, vol := range volumes {
		tableFQN := fmt.Sprintf("%s.main.%s", vol.LakeName, tableName)
		selectColumns := fmt.Sprintf(
			"*, %s AS storage_lake, %s AS storage_volume",
			sqlStringLiteral(vol.LakeName),
			sqlStringLiteral(vol.Name),
		)

		// Check if table exists in this volume
		if !tr.tableExists(vol.LakeName, tableName) {
			continue
		}

		query := fmt.Sprintf("SELECT %s FROM %s", selectColumns, tableFQN)
		if whereClause != "" {
			query += " WHERE " + whereClause
		}
		query += " ORDER BY timestamp DESC"
		if limit > 0 {
			query += fmt.Sprintf(" LIMIT %d", limit)
		}

		rows, err := tr.db.Query(query)
		if err != nil {
			logger.Debug("TieredReader: Query failed for volume",
				"volume", vol.Name,
				"table", tableName,
				"error", err)
			continue
		}

		results, err := tr.scanRowsToMaps(rows)
		rows.Close()
		if err != nil {
			continue
		}

		allResults = append(allResults, results...)
	}

	// Sort by timestamp DESC
	tr.sortByTimestamp(allResults)

	// Apply limit after merge
	if limit > 0 && len(allResults) > limit {
		allResults = allResults[:limit]
	}

	return allResults, nil
}

// QueryByKey queries a specific table by TableKey across all volumes
func (tr *TieredReader) QueryByKey(key TableKey, whereClause string, limit int) ([]map[string]interface{}, error) {
	tableName := "hep_proto_" + key.String()
	return tr.Query(tableName, whereClause, limit)
}

// QueryAll queries all tables matching a pattern across all volumes
func (tr *TieredReader) QueryAll(whereClause string, limit int) ([]map[string]interface{}, error) {
	volumes := tr.tieredStorage.GetVolumes()
	if len(volumes) == 0 {
		return nil, fmt.Errorf("no volumes configured")
	}

	var allResults []map[string]interface{}

	for _, vol := range volumes {
		// Get all HEP tables in this volume
		tables, err := tr.tieredStorage.GetTableNames(vol)
		if err != nil {
			continue
		}

		for _, tableName := range tables {
			tableFQN := fmt.Sprintf("%s.main.%s", vol.LakeName, tableName)
			selectColumns := fmt.Sprintf(
				"*, %s AS storage_lake, %s AS storage_volume",
				sqlStringLiteral(vol.LakeName),
				sqlStringLiteral(vol.Name),
			)

			query := fmt.Sprintf("SELECT %s FROM %s", selectColumns, tableFQN)
			if whereClause != "" {
				query += " WHERE " + whereClause
			}
			query += " ORDER BY timestamp DESC"
			if limit > 0 {
				query += fmt.Sprintf(" LIMIT %d", limit)
			}

			rows, err := tr.db.Query(query)
			if err != nil {
				continue
			}

			results, err := tr.scanRowsToMaps(rows)
			rows.Close()
			if err != nil {
				continue
			}

			allResults = append(allResults, results...)
		}
	}

	// Sort by timestamp DESC
	tr.sortByTimestamp(allResults)

	// Apply limit after merge
	if limit > 0 && len(allResults) > limit {
		allResults = allResults[:limit]
	}

	return allResults, nil
}

// QueryUnion builds and executes a UNION ALL query across all volumes for a table
func (tr *TieredReader) QueryUnion(tableName string, whereClause string, limit int) ([]map[string]interface{}, error) {
	volumes := tr.tieredStorage.GetVolumes()
	if len(volumes) == 0 {
		return nil, fmt.Errorf("no volumes configured")
	}

	// Build UNION ALL query
	var unionParts []string
	for _, vol := range volumes {
		tableFQN := fmt.Sprintf("%s.main.%s", vol.LakeName, tableName)
		selectColumns := fmt.Sprintf(
			"*, %s AS storage_lake, %s AS storage_volume",
			sqlStringLiteral(vol.LakeName),
			sqlStringLiteral(vol.Name),
		)

		// Check if table exists
		if !tr.tableExists(vol.LakeName, tableName) {
			continue
		}

		subQuery := fmt.Sprintf("SELECT %s FROM %s", selectColumns, tableFQN)
		if whereClause != "" {
			subQuery += " WHERE " + whereClause
		}
		unionParts = append(unionParts, subQuery)
	}

	if len(unionParts) == 0 {
		return nil, fmt.Errorf("table %s not found in any volume", tableName)
	}

	// Build final query with UNION ALL
	var query string
	if len(unionParts) == 1 {
		query = unionParts[0]
	} else {
		query = "(" + strings.Join(unionParts, ") UNION ALL (") + ")"
	}
	query += " ORDER BY timestamp DESC"
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}

	rows, err := tr.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("query failed: %w", err)
	}
	defer rows.Close()

	return tr.scanRowsToMaps(rows)
}

// GetTimeRange returns the overall time range across all volumes
func (tr *TieredReader) GetTimeRange() (minTs, maxTs time.Time, err error) {
	volumes := tr.tieredStorage.GetVolumes()

	for _, vol := range volumes {
		tables, err := tr.tieredStorage.GetTableNames(vol)
		if err != nil {
			continue
		}

		for _, tableName := range tables {
			tableFQN := fmt.Sprintf("%s.main.%s", vol.LakeName, tableName)

			query := fmt.Sprintf("SELECT MIN(timestamp), MAX(timestamp) FROM %s", tableFQN)

			var minNull, maxNull sql.NullTime
			if err := tr.db.QueryRow(query).Scan(&minNull, &maxNull); err != nil {
				continue
			}

			if minNull.Valid && (minTs.IsZero() || minNull.Time.Before(minTs)) {
				minTs = minNull.Time
			}
			if maxNull.Valid && (maxTs.IsZero() || maxNull.Time.After(maxTs)) {
				maxTs = maxNull.Time
			}
		}
	}

	return minTs, maxTs, nil
}

// GetVolumeStats returns statistics for each volume
func (tr *TieredReader) GetVolumeStats() ([]map[string]interface{}, error) {
	volumes := tr.tieredStorage.GetVolumes()
	var stats []map[string]interface{}

	for _, vol := range volumes {
		tables, err := tr.tieredStorage.GetTableNames(vol)
		if err != nil {
			continue
		}

		var totalRows int64
		for _, tableName := range tables {
			tableFQN := fmt.Sprintf("%s.main.%s", vol.LakeName, tableName)

			var count int64
			query := fmt.Sprintf("SELECT COUNT(*) FROM %s", tableFQN)
			if err := tr.db.QueryRow(query).Scan(&count); err != nil {
				continue
			}
			totalRows += count
		}

		volStats := map[string]interface{}{
			"name":         vol.Name,
			"type":         vol.Type,
			"path":         vol.Path,
			"priority":     vol.Priority,
			"tables":       len(tables),
			"total_rows":   totalRows,
			"max_age_days": vol.MaxDataAgeDays,
		}
		stats = append(stats, volStats)
	}

	return stats, nil
}

// tableExists checks if a table exists in a specific lake
func (tr *TieredReader) tableExists(lakeName, tableName string) bool {
	query := fmt.Sprintf(`
		SELECT COUNT(*) FROM information_schema.tables 
		WHERE table_catalog = '%s' 
		  AND table_schema = 'main' 
		  AND table_name = '%s'
	`, lakeName, tableName)

	var count int
	if err := tr.db.QueryRow(query).Scan(&count); err != nil {
		return false
	}
	return count > 0
}

// scanRowsToMaps converts sql.Rows to []map[string]interface{}
func (tr *TieredReader) scanRowsToMaps(rows *sql.Rows) ([]map[string]interface{}, error) {
	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	var results []map[string]interface{}
	for rows.Next() {
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			continue
		}

		row := make(map[string]interface{})
		for i, col := range columns {
			row[col] = values[i]
		}
		results = append(results, row)
	}

	return results, nil
}

// sortByTimestamp sorts results by timestamp DESC
func (tr *TieredReader) sortByTimestamp(results []map[string]interface{}) {
	sort.Slice(results, func(i, j int) bool {
		tsI := getTimestampValue(results[i])
		tsJ := getTimestampValue(results[j])
		return tsI.After(tsJ)
	})
}

// getTimestampValue extracts timestamp from a result map
func getTimestampValue(row map[string]interface{}) time.Time {
	if ts, ok := row["timestamp"]; ok {
		switch v := ts.(type) {
		case time.Time:
			return v
		case int64:
			return time.Unix(0, v)
		case float64:
			return time.Unix(0, int64(v))
		}
	}
	return time.Time{}
}
