// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package ducklake

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Reader provides read operations including time travel
type Reader struct {
	db       *sql.DB
	tableFQN string
}

// NewReader creates a reader from an existing writer
func NewReader(w *Writer) *Reader {
	return &Reader{
		db:       w.db,
		tableFQN: w.tableFQN,
	}
}

// Query executes a query on current data
func (r *Reader) Query(whereClause string, limit int) ([]HEPRecord, error) {
	if err := ValidateWhereClause(whereClause, AllQueryColumns()); err != nil {
		return nil, err
	}
	limit = ClampLimit(limit, DefaultQueryLimit, MaxQueryLimit)
	query := fmt.Sprintf("SELECT * FROM %s", r.tableFQN)
	if whereClause != "" {
		query += " WHERE " + whereClause
	}
	query += " ORDER BY timestamp DESC"
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}

	return r.executeQuery(query)
}

// QueryAtSnapshot queries data at a specific snapshot
func (r *Reader) QueryAtSnapshot(snapshotID int64, whereClause string, limit int) ([]HEPRecord, error) {
	if err := ValidateWhereClause(whereClause, AllQueryColumns()); err != nil {
		return nil, err
	}
	limit = ClampLimit(limit, DefaultQueryLimit, MaxQueryLimit)
	// DuckLake syntax: SELECT * FROM table AT SNAPSHOT snapshot_id
	query := fmt.Sprintf("SELECT * FROM %s AT SNAPSHOT %d", r.tableFQN, snapshotID)
	if whereClause != "" {
		query += " WHERE " + whereClause
	}
	query += " ORDER BY timestamp DESC"
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}

	return r.executeQuery(query)
}

// QueryAtTime queries data as it was at a specific timestamp
func (r *Reader) QueryAtTime(asOf time.Time, whereClause string, limit int) ([]HEPRecord, error) {
	if err := ValidateWhereClause(whereClause, AllQueryColumns()); err != nil {
		return nil, err
	}
	limit = ClampLimit(limit, DefaultQueryLimit, MaxQueryLimit)
	// DuckLake syntax: SELECT * FROM table AT TIMESTAMP 'timestamp'
	timestamp := asOf.Format("2006-01-02 15:04:05")
	query := fmt.Sprintf("SELECT * FROM %s AT TIMESTAMP '%s'", r.tableFQN, timestamp)
	if whereClause != "" {
		query += " WHERE " + whereClause
	}
	query += " ORDER BY timestamp DESC"
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}

	return r.executeQuery(query)
}

// executeQuery executes a query and returns HEP records
func (r *Reader) executeQuery(query string) ([]HEPRecord, error) {
	rows, err := r.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("query failed: %w", err)
	}
	defer rows.Close()

	var records []HEPRecord
	for rows.Next() {
		var rec HEPRecord
		var dataExtra sql.NullString

		err := rows.Scan(
			&rec.UUID, &rec.Timestamp, &rec.SessionID, &rec.Caller, &rec.Callee,
			&rec.SrcIP, &rec.DstIP, &rec.SrcPort, &rec.DstPort, &rec.Event,
			&rec.ProtoType, &rec.Protocol, &rec.NodeID, &rec.CID, &rec.Payload,
			&dataExtra,
		)
		if err != nil {
			continue
		}

		if dataExtra.Valid {
			rec.DataExtra = dataExtra.String
		}
		records = append(records, rec)
	}

	return records, nil
}

// ListSnapshots returns all available snapshots
func (r *Reader) ListSnapshots(limit int) ([]Snapshot, error) {
	limit = ClampLimit(limit, DefaultQueryLimit, MaxQueryLimit)

	// DuckLake provides snapshot info via system function
	query := fmt.Sprintf(`
		SELECT snapshot_id, snapshot_time, row_count
		FROM ducklake_snapshots('%s')
		ORDER BY snapshot_time DESC
		LIMIT %d
	`, r.tableFQN, limit)

	rows, err := r.db.Query(query)
	if err != nil {
		// If ducklake_snapshots doesn't exist, return empty
		return nil, nil
	}
	defer rows.Close()

	var snapshots []Snapshot
	for rows.Next() {
		var s Snapshot
		var snapshotTime string
		if err := rows.Scan(&s.ID, &snapshotTime, &s.RowCount); err != nil {
			continue
		}
		if t, err := time.Parse("2006-01-02 15:04:05", snapshotTime); err == nil {
			s.CreatedAt = t
		}
		snapshots = append(snapshots, s)
	}

	return snapshots, nil
}

// GetSnapshotByID returns a specific snapshot
func (r *Reader) GetSnapshotByID(snapshotID int64) (*Snapshot, error) {
	query := fmt.Sprintf(`
		SELECT snapshot_id, snapshot_time, row_count
		FROM ducklake_snapshots('%s')
		WHERE snapshot_id = %d
	`, r.tableFQN, snapshotID)

	row := r.db.QueryRow(query)

	var s Snapshot
	var snapshotTime string
	if err := row.Scan(&s.ID, &snapshotTime, &s.RowCount); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	if t, err := time.Parse("2006-01-02 15:04:05", snapshotTime); err == nil {
		s.CreatedAt = t
	}

	return &s, nil
}

// GetTimeRange returns min/max timestamps for current data
func (r *Reader) GetTimeRange() (minTs, maxTs int64, err error) {
	query := fmt.Sprintf("SELECT MIN(timestamp), MAX(timestamp) FROM %s", r.tableFQN)

	var minNull, maxNull sql.NullInt64
	if err := r.db.QueryRow(query).Scan(&minNull, &maxNull); err != nil {
		return 0, 0, err
	}

	if minNull.Valid {
		minTs = minNull.Int64
	}
	if maxNull.Valid {
		maxTs = maxNull.Int64
	}

	return minTs, maxTs, nil
}

// HasDataInRange checks if data exists in the given time range
func (r *Reader) HasDataInRange(queryMinTs, queryMaxTs int64) (bool, error) {
	minTs, maxTs, err := r.GetTimeRange()
	if err != nil {
		return false, err
	}

	// No data
	if minTs == 0 && maxTs == 0 {
		return false, nil
	}

	// Check overlap
	hasData := queryMinTs <= maxTs && queryMaxTs >= minTs
	return hasData, nil
}

// GetRowCount returns total row count
func (r *Reader) GetRowCount() (int64, error) {
	var count int64
	query := fmt.Sprintf("SELECT COUNT(*) FROM %s", r.tableFQN)
	if err := r.db.QueryRow(query).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

// DiffSnapshots returns changes between two snapshots
func (r *Reader) DiffSnapshots(fromSnapshot, toSnapshot int64) (added, removed int64, err error) {
	// Count rows in each snapshot
	var fromCount, toCount int64

	query := fmt.Sprintf("SELECT COUNT(*) FROM %s AT SNAPSHOT %d", r.tableFQN, fromSnapshot)
	if err := r.db.QueryRow(query).Scan(&fromCount); err != nil {
		return 0, 0, fmt.Errorf("failed to count from snapshot: %w", err)
	}

	query = fmt.Sprintf("SELECT COUNT(*) FROM %s AT SNAPSHOT %d", r.tableFQN, toSnapshot)
	if err := r.db.QueryRow(query).Scan(&toCount); err != nil {
		return 0, 0, fmt.Errorf("failed to count to snapshot: %w", err)
	}

	if toCount > fromCount {
		added = toCount - fromCount
	} else {
		removed = fromCount - toCount
	}

	return added, removed, nil
}

// MultiTableReader provides read operations across multiple tables
type MultiTableReader struct {
	db           *sql.DB
	writer       *MultiTableWriter
	lakeName     string
	searchBuffer bool // include memory tables in queries
}

// NewMultiTableReader creates a reader for multi-table writer
func NewMultiTableReader(w *MultiTableWriter) *MultiTableReader {
	return &MultiTableReader{
		db:           w.GetDB(),
		writer:       w,
		lakeName:     w.GetLakeName(),
		searchBuffer: w.SearchBufferEnabled(),
	}
}

// allTablesForKey returns the DuckLake table and, when search_buffer is enabled,
// both in-memory buffer tables for a key.
func (r *MultiTableReader) allTablesForKey(key TableKey) ([]string, error) {
	fqn, err := ResolveTableFQN(r.writer, key)
	if err != nil {
		return nil, err
	}
	tables := []string{fqn}
	if r.searchBuffer {
		if tw := r.writer.GetTable(key); tw != nil {
			for _, mem := range tw.MemTableNames() {
				tables = append(tables, mem)
			}
		}
	}
	return tables, nil
}

// GetTimeRange returns min/max timestamps across all tables including buffers
func (r *MultiTableReader) GetTimeRange() (minTs, maxTs int64, err error) {
	keys := r.writer.ListTableKeys()
	if len(keys) == 0 {
		return 0, 0, nil
	}

	for _, key := range keys {
		tbls, tblErr := r.allTablesForKey(key)
		if tblErr != nil {
			continue
		}
		for _, tbl := range tbls {
			query := fmt.Sprintf("SELECT MIN(timestamp), MAX(timestamp) FROM %s", tbl)
			var minNull, maxNull sql.NullInt64
			if err := r.db.QueryRow(query).Scan(&minNull, &maxNull); err != nil {
				continue
			}
			if minNull.Valid && (minTs == 0 || minNull.Int64 < minTs) {
				minTs = minNull.Int64
			}
			if maxNull.Valid && maxNull.Int64 > maxTs {
				maxTs = maxNull.Int64
			}
		}
	}

	return minTs, maxTs, nil
}

// GetTimeRangeForTableKey returns time range for specific TableKey including buffers
func (r *MultiTableReader) GetTimeRangeForTableKey(key TableKey) (minTs, maxTs int64, err error) {
	if err := ValidateTableKey(key); err != nil {
		return 0, 0, err
	}
	tbls, err := r.allTablesForKey(key)
	if err != nil {
		return 0, 0, err
	}
	for _, tbl := range tbls {
		query := fmt.Sprintf("SELECT MIN(timestamp), MAX(timestamp) FROM %s", tbl)
		var minNull, maxNull sql.NullInt64
		if err := r.db.QueryRow(query).Scan(&minNull, &maxNull); err != nil {
			continue
		}
		if minNull.Valid && (minTs == 0 || minNull.Int64 < minTs) {
			minTs = minNull.Int64
		}
		if maxNull.Valid && maxNull.Int64 > maxTs {
			maxTs = maxNull.Int64
		}
	}
	return minTs, maxTs, nil
}

// HasDataInRange checks if data exists in the given time range across all tables
func (r *MultiTableReader) HasDataInRange(queryMinTs, queryMaxTs int64) (bool, error) {
	minTs, maxTs, err := r.GetTimeRange()
	if err != nil {
		return false, err
	}

	// No data
	if minTs == 0 && maxTs == 0 {
		return false, nil
	}

	// Check overlap
	hasData := queryMinTs <= maxTs && queryMaxTs >= minTs
	return hasData, nil
}

// GetRowCount returns total row count across all tables including buffers
func (r *MultiTableReader) GetRowCount() (int64, error) {
	keys := r.writer.ListTableKeys()
	var total int64

	for _, key := range keys {
		tbls, tblErr := r.allTablesForKey(key)
		if tblErr != nil {
			continue
		}
		for _, tbl := range tbls {
			var count int64
			query := fmt.Sprintf("SELECT COUNT(*) FROM %s", tbl)
			if err := r.db.QueryRow(query).Scan(&count); err != nil {
				continue
			}
			total += count
		}
	}

	return total, nil
}

// GetRowCountForTableKey returns row count for specific TableKey including buffers
func (r *MultiTableReader) GetRowCountForTableKey(key TableKey) (int64, error) {
	if err := ValidateTableKey(key); err != nil {
		return 0, err
	}
	tbls, err := r.allTablesForKey(key)
	if err != nil {
		return 0, err
	}
	var total int64
	for _, tbl := range tbls {
		var count int64
		query := fmt.Sprintf("SELECT COUNT(*) FROM %s", tbl)
		if err := r.db.QueryRow(query).Scan(&count); err != nil {
			continue
		}
		total += count
	}
	return total, nil
}

// buildUnionQuery builds a query for a specific table key. When search_buffer
// is enabled, it builds a UNION ALL across the DuckLake persistent table and
// both in-memory buffer tables so queries see the freshest data even before
// flush. When search_buffer is disabled, it queries only the DuckLake table.
func (r *MultiTableReader) buildUnionQuery(key TableKey, whereClause string, limit int) (string, error) {
	tables, err := r.allTablesForKey(key)
	if err != nil {
		return "", err
	}
	limit = ClampLimit(limit, DefaultQueryLimit, MaxQueryLimit)

	if len(tables) == 1 {
		query := fmt.Sprintf("SELECT * FROM %s", tables[0])
		if whereClause != "" {
			query += " WHERE " + whereClause
		}
		query += " ORDER BY timestamp DESC"
		if limit > 0 {
			query += fmt.Sprintf(" LIMIT %d", limit)
		}
		return query, nil
	}

	var parts []string
	for _, tbl := range tables {
		q := fmt.Sprintf("SELECT * FROM %s", tbl)
		if whereClause != "" {
			q += " WHERE " + whereClause
		}
		parts = append(parts, q)
	}

	query := "(" + strings.Join(parts, ") UNION ALL (") + ")"
	query += " ORDER BY timestamp DESC"
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}
	return query, nil
}

// Query executes a query on specific table by TableKey.
// Includes data from both in-memory buffers via UNION ALL so that
// un-flushed records are visible to search.
func (r *MultiTableReader) Query(key TableKey, whereClause string, limit int) ([]map[string]interface{}, error) {
	if err := ValidateTableKey(key); err != nil {
		return nil, err
	}
	cols, err := ColumnsForTableKey(key)
	if err != nil {
		return nil, err
	}
	if err := ValidateWhereClause(whereClause, cols); err != nil {
		return nil, err
	}
	query, err := r.buildUnionQuery(key, whereClause, limit)
	if err != nil {
		return nil, err
	}
	return r.executeQueryGeneric(query)
}

// QueryAll executes a query across all tables.
// Includes data from both in-memory buffers via UNION ALL.
func (r *MultiTableReader) QueryAll(whereClause string, limit int) ([]map[string]interface{}, error) {
	if err := ValidateWhereClause(whereClause, AllQueryColumns()); err != nil {
		return nil, err
	}
	limit = ClampLimit(limit, DefaultQueryLimit, MaxQueryLimit)
	keys := r.writer.ListTableKeys()
	if len(keys) == 0 {
		return nil, nil
	}

	var allResults []map[string]interface{}
	for _, key := range keys {
		if err := ValidateTableKey(key); err != nil {
			continue
		}
		query, err := r.buildUnionQuery(key, whereClause, limit)
		if err != nil {
			continue
		}
		results, err := r.executeQueryGeneric(query)
		if err != nil {
			continue
		}
		allResults = append(allResults, results...)
	}

	// Sort by timestamp and apply limit
	sortByTimestampDesc(allResults)
	if limit > 0 && len(allResults) > limit {
		allResults = allResults[:limit]
	}

	return allResults, nil
}

// executeQueryGeneric executes a query and returns generic map results
func (r *MultiTableReader) executeQueryGeneric(query string) ([]map[string]interface{}, error) {
	rows, err := r.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("query failed: %w", err)
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("failed to get columns: %w", err)
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

// sortByTimestampDesc sorts results by timestamp column in descending order.
func sortByTimestampDesc(results []map[string]interface{}) {
	sort.Slice(results, func(i, j int) bool {
		tsI := extractTimestamp(results[i])
		tsJ := extractTimestamp(results[j])
		return tsI.After(tsJ)
	})
}

// extractTimestamp pulls a time.Time from a result map.
func extractTimestamp(row map[string]interface{}) time.Time {
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

// ListSnapshots returns snapshots for a specific table by TableKey
func (r *MultiTableReader) ListSnapshots(key TableKey, limit int) ([]Snapshot, error) {
	limit = ClampLimit(limit, DefaultQueryLimit, MaxQueryLimit)
	tableFQN, err := ResolveTableFQN(r.writer, key)
	if err != nil {
		return nil, err
	}

	query := fmt.Sprintf(`
		SELECT snapshot_id, snapshot_time, row_count
		FROM ducklake_snapshots('%s')
		ORDER BY snapshot_time DESC
		LIMIT %d
	`, tableFQN, limit)

	rows, err := r.db.Query(query)
	if err != nil {
		return nil, nil
	}
	defer rows.Close()

	var snapshots []Snapshot
	for rows.Next() {
		var s Snapshot
		var snapshotTime string
		if err := rows.Scan(&s.ID, &snapshotTime, &s.RowCount); err != nil {
			continue
		}
		if t, err := time.Parse("2006-01-02 15:04:05", snapshotTime); err == nil {
			s.CreatedAt = t
		}
		snapshots = append(snapshots, s)
	}

	return snapshots, nil
}

// ListTableKeys returns all TableKeys with tables
func (r *MultiTableReader) ListTableKeys() []TableKey {
	return r.writer.ListTableKeys()
}

// ListProtoTypes returns unique proto_types with tables
func (r *MultiTableReader) ListProtoTypes() []uint32 {
	return r.writer.ListProtoTypes()
}

