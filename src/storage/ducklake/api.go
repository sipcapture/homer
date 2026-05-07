// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package ducklake

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"
)

// API provides HTTP API for DuckLake operations (legacy single-table)
type API struct {
	reader *Reader
	writer *Writer
}

// NewAPI creates a new DuckLake API handler (legacy single-table)
func NewAPI(writer *Writer) *API {
	return &API{
		reader: NewReader(writer),
		writer: writer,
	}
}

// MultiTableAPI provides HTTP API for multi-table DuckLake operations
type MultiTableAPI struct {
	reader *MultiTableReader
	writer *MultiTableWriter
}

// NewMultiTableAPI creates a new multi-table DuckLake API handler
func NewMultiTableAPI(writer *MultiTableWriter) *MultiTableAPI {
	return &MultiTableAPI{
		reader: NewMultiTableReader(writer),
		writer: writer,
	}
}

// TimeRangeRequest represents a time range check request
type TimeRangeRequest struct {
	MinTs int64 `json:"min_ts"`
	MaxTs int64 `json:"max_ts"`
}

// TimeRangeResponse represents a time range check response
type TimeRangeResponse struct {
	HasData       bool   `json:"has_data"`
	NodeMinTs     int64  `json:"node_min_ts"`
	NodeMaxTs     int64  `json:"node_max_ts"`
	OldestData    string `json:"oldest_data"`
	NewestData    string `json:"newest_data"`
	DataSpanHours int    `json:"data_span_hours"`
}

// StatsResponse represents node statistics
type StatsResponse struct {
	RowCount     int64  `json:"row_count"`
	MinTimestamp int64  `json:"min_timestamp,omitempty"`
	MaxTimestamp int64  `json:"max_timestamp,omitempty"`
	OldestData   string `json:"oldest_data,omitempty"`
	NewestData   string `json:"newest_data,omitempty"`
	CatalogType  string `json:"catalog_type"`
	DataPath     string `json:"data_path"`
	BufferSize   int    `json:"buffer_size"`
}

// SnapshotResponse represents snapshot info
type SnapshotResponse struct {
	ID        int64  `json:"id"`
	CreatedAt string `json:"created_at"`
	RowCount  int64  `json:"row_count"`
}

// HandleCheck handles POST /api/v1/ducklake/check
// Checks if node has data in time range (for smart routing)
func (a *API) HandleCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req TimeRangeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	minTs, maxTs, err := a.reader.GetTimeRange()
	if err != nil {
		http.Error(w, "Failed to get time range", http.StatusInternalServerError)
		return
	}

	resp := TimeRangeResponse{
		NodeMinTs: minTs,
		NodeMaxTs: maxTs,
	}

	if minTs > 0 {
		resp.OldestData = time.Unix(0, minTs).Format(time.RFC3339)
	}
	if maxTs > 0 {
		resp.NewestData = time.Unix(0, maxTs).Format(time.RFC3339)
	}
	if minTs > 0 && maxTs > 0 {
		resp.DataSpanHours = int((maxTs - minTs) / int64(time.Hour))
	}

	// Check if data exists in requested range
	if req.MinTs > 0 || req.MaxTs > 0 {
		hasData, _ := a.reader.HasDataInRange(req.MinTs, req.MaxTs)
		resp.HasData = hasData
	} else {
		resp.HasData = minTs > 0 || maxTs > 0
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// HandleStats handles GET /api/v1/ducklake/stats
func (a *API) HandleStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	stats, err := a.writer.GetStats()
	if err != nil {
		http.Error(w, "Failed to get stats", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// HandleSnapshots handles GET /api/v1/ducklake/snapshots
func (a *API) HandleSnapshots(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	limit := 100
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	snapshots, err := a.reader.ListSnapshots(limit)
	if err != nil {
		http.Error(w, "Failed to list snapshots", http.StatusInternalServerError)
		return
	}

	var resp []SnapshotResponse
	for _, s := range snapshots {
		resp = append(resp, SnapshotResponse{
			ID:        s.ID,
			CreatedAt: s.CreatedAt.Format(time.RFC3339),
			RowCount:  s.RowCount,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// HandleQuery handles POST /api/v1/ducklake/query
// Supports time travel queries
func (a *API) HandleQuery(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Where      string `json:"where"`
		Limit      int    `json:"limit"`
		SnapshotID *int64 `json:"snapshot_id,omitempty"`
		AsOfTime   string `json:"as_of_time,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Limit <= 0 || req.Limit > 1000 {
		req.Limit = 100
	}

	var records []HEPRecord
	var err error

	if req.SnapshotID != nil {
		// Query at specific snapshot
		records, err = a.reader.QueryAtSnapshot(*req.SnapshotID, req.Where, req.Limit)
	} else if req.AsOfTime != "" {
		// Query at specific time
		asOf, parseErr := time.Parse(time.RFC3339, req.AsOfTime)
		if parseErr != nil {
			http.Error(w, "Invalid as_of_time format", http.StatusBadRequest)
			return
		}
		records, err = a.reader.QueryAtTime(asOf, req.Where, req.Limit)
	} else {
		// Query current data
		records, err = a.reader.Query(req.Where, req.Limit)
	}

	if err != nil {
		http.Error(w, "Query failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"count":   len(records),
		"records": records,
	})
}

// RegisterRoutes registers DuckLake API routes
func (a *API) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/ducklake/check", a.HandleCheck)
	mux.HandleFunc("/api/v1/ducklake/stats", a.HandleStats)
	mux.HandleFunc("/api/v1/ducklake/snapshots", a.HandleSnapshots)
	mux.HandleFunc("/api/v1/ducklake/query", a.HandleQuery)

	// Also register on metadata path for compatibility with smart router
	mux.HandleFunc("/api/v1/metadata/check", a.HandleCheck)
	mux.HandleFunc("/api/v1/metadata/stats", a.HandleStats)
}

// MultiTableTimeRangeResponse represents time range response for multi-table
type MultiTableTimeRangeResponse struct {
	HasData       bool                   `json:"has_data"`
	NodeMinTs     int64                  `json:"node_min_ts"`
	NodeMaxTs     int64                  `json:"node_max_ts"`
	OldestData    string                 `json:"oldest_data"`
	NewestData    string                 `json:"newest_data"`
	DataSpanHours int                    `json:"data_span_hours"`
	TableCount    int                    `json:"table_count"`
	ProtoTypes    []uint32               `json:"proto_types"`
	Tables        []map[string]interface{} `json:"tables,omitempty"`
}

// MultiTableStatsResponse represents stats for multi-table
type MultiTableStatsResponse struct {
	TotalRowCount   int64                    `json:"total_row_count"`
	TotalBufferSize int                      `json:"total_buffer_size"`
	TableCount      int                      `json:"table_count"`
	CatalogType     string                   `json:"catalog_type"`
	DataPath        string                   `json:"data_path"`
	MinTimestamp    int64                    `json:"min_timestamp,omitempty"`
	MaxTimestamp    int64                    `json:"max_timestamp,omitempty"`
	OldestData      string                   `json:"oldest_data,omitempty"`
	NewestData      string                   `json:"newest_data,omitempty"`
	Tables          []map[string]interface{} `json:"tables"`
}

// HandleCheck handles POST /api/v1/ducklake/check for multi-table
func (a *MultiTableAPI) HandleCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req TimeRangeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	minTs, maxTs, err := a.reader.GetTimeRange()
	if err != nil {
		http.Error(w, "Failed to get time range", http.StatusInternalServerError)
		return
	}

	resp := MultiTableTimeRangeResponse{
		NodeMinTs:  minTs,
		NodeMaxTs:  maxTs,
		TableCount: len(a.writer.ListProtoTypes()),
		ProtoTypes: a.writer.ListProtoTypes(),
	}

	if minTs > 0 {
		resp.OldestData = time.Unix(0, minTs).Format(time.RFC3339)
	}
	if maxTs > 0 {
		resp.NewestData = time.Unix(0, maxTs).Format(time.RFC3339)
	}
	if minTs > 0 && maxTs > 0 {
		resp.DataSpanHours = int((maxTs - minTs) / int64(time.Hour))
	}

	// Check if data exists in requested range
	if req.MinTs > 0 || req.MaxTs > 0 {
		hasData, _ := a.reader.HasDataInRange(req.MinTs, req.MaxTs)
		resp.HasData = hasData
	} else {
		resp.HasData = minTs > 0 || maxTs > 0
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// HandleStats handles GET /api/v1/ducklake/stats for multi-table
func (a *MultiTableAPI) HandleStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	stats, err := a.writer.GetStats()
	if err != nil {
		http.Error(w, "Failed to get stats", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// HandleTables handles GET /api/v1/ducklake/tables
func (a *MultiTableAPI) HandleTables(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	tableKeys := a.writer.ListTableKeys()
	tables := make([]map[string]interface{}, 0, len(tableKeys))

	for _, key := range tableKeys {
		tableFQN := a.writer.GetTableFQN(key)
		count, _ := a.reader.GetRowCountForTableKey(key)
		minTs, maxTs, _ := a.reader.GetTimeRangeForTableKey(key)

		tableInfo := map[string]interface{}{
			"proto_type": key.ProtoType,
			"sub_type":   key.SubType,
			"table_key":  key.String(),
			"table_name": tableFQN,
			"row_count":  count,
		}

		if minTs > 0 {
			tableInfo["min_timestamp"] = minTs
			tableInfo["oldest_data"] = time.Unix(0, minTs).Format(time.RFC3339)
		}
		if maxTs > 0 {
			tableInfo["max_timestamp"] = maxTs
			tableInfo["newest_data"] = time.Unix(0, maxTs).Format(time.RFC3339)
		}

		tables = append(tables, tableInfo)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"tables": tables,
		"count":  len(tables),
	})
}

// HandleSnapshots handles GET /api/v1/ducklake/snapshots for multi-table
func (a *MultiTableAPI) HandleSnapshots(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Build TableKey from query params
	key := TableKey{ProtoType: 1, SubType: SIPTypeCall} // Default to SIP calls

	if ptStr := r.URL.Query().Get("proto_type"); ptStr != "" {
		if pt, err := strconv.ParseUint(ptStr, 10, 32); err == nil {
			key.ProtoType = uint32(pt)
		}
	}
	if subType := r.URL.Query().Get("sub_type"); subType != "" {
		key.SubType = subType
	}

	limit := 100
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	snapshots, err := a.reader.ListSnapshots(key, limit)
	if err != nil {
		http.Error(w, "Failed to list snapshots", http.StatusInternalServerError)
		return
	}

	var resp []SnapshotResponse
	for _, s := range snapshots {
		resp = append(resp, SnapshotResponse{
			ID:        s.ID,
			CreatedAt: s.CreatedAt.Format(time.RFC3339),
			RowCount:  s.RowCount,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"table_key": key.String(),
		"snapshots": resp,
	})
}

// HandleQuery handles POST /api/v1/ducklake/query for multi-table
func (a *MultiTableAPI) HandleQuery(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		ProtoType *uint32 `json:"proto_type,omitempty"`
		SubType   string  `json:"sub_type,omitempty"`
		Where     string  `json:"where"`
		Limit     int     `json:"limit"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Limit <= 0 || req.Limit > 1000 {
		req.Limit = 100
	}

	var records []map[string]interface{}
	var err error

	if req.ProtoType != nil {
		// Query specific table by TableKey
		key := TableKey{ProtoType: *req.ProtoType, SubType: req.SubType}
		records, err = a.reader.Query(key, req.Where, req.Limit)
	} else {
		// Query all tables
		records, err = a.reader.QueryAll(req.Where, req.Limit)
	}

	if err != nil {
		http.Error(w, "Query failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"count":   len(records),
		"records": records,
	})
}

// RegisterRoutes registers multi-table DuckLake API routes
func (a *MultiTableAPI) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/ducklake/check", a.HandleCheck)
	mux.HandleFunc("/api/v1/ducklake/stats", a.HandleStats)
	mux.HandleFunc("/api/v1/ducklake/tables", a.HandleTables)
	mux.HandleFunc("/api/v1/ducklake/snapshots", a.HandleSnapshots)
	mux.HandleFunc("/api/v1/ducklake/query", a.HandleQuery)

	// Also register on metadata path for compatibility with smart router
	mux.HandleFunc("/api/v1/metadata/check", a.HandleCheck)
	mux.HandleFunc("/api/v1/metadata/stats", a.HandleStats)
}
