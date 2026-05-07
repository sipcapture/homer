// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package input

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/sipcapture/homer-core/src/storage/ducklake"
)

// MetadataAPI provides HTTP API for metadata/storage queries
type MetadataAPI struct {
	manager *ducklake.Manager
}

// NewMetadataAPI creates a new metadata API handler
func NewMetadataAPI(manager *ducklake.Manager) *MetadataAPI {
	return &MetadataAPI{manager: manager}
}

// TimeRangeRequest represents a time range check request
type TimeRangeRequest struct {
	MinTs int64 `json:"min_ts"` // minimum timestamp (nanoseconds)
	MaxTs int64 `json:"max_ts"` // maximum timestamp (nanoseconds)
}

// TimeRangeResponse represents a time range check response
type TimeRangeResponse struct {
	HasData       bool   `json:"has_data"`        // true if node has data in range
	NodeMinTs     int64  `json:"node_min_ts"`     // node's oldest timestamp
	NodeMaxTs     int64  `json:"node_max_ts"`     // node's newest timestamp
	OldestData    string `json:"oldest_data"`     // RFC3339 formatted
	NewestData    string `json:"newest_data"`     // RFC3339 formatted
	DataSpanHours int    `json:"data_span_hours"`
}

// HandleCheck handles POST /api/v1/metadata/check
// Checks if node has data in the requested time range
func (m *MetadataAPI) HandleCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if m.manager == nil {
		// No storage configured - respond with has_data=true (don't skip this node)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(TimeRangeResponse{HasData: true})
		return
	}

	var req TimeRangeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Get node's time range
	minTs, maxTs, err := m.manager.GetTimeRange()
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

	// Check if node has data in requested range
	if req.MinTs > 0 || req.MaxTs > 0 {
		hasData, _ := m.manager.HasDataInRange(req.MinTs, req.MaxTs)
		resp.HasData = hasData
	} else {
		// No time range specified - assume node has data if there's any data
		resp.HasData = minTs > 0 || maxTs > 0
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// HandleStats handles GET /api/v1/metadata/stats
// Returns node storage statistics
func (m *MetadataAPI) HandleStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if m.manager == nil {
		http.Error(w, "Storage not configured", http.StatusNotFound)
		return
	}

	stats, err := m.manager.GetStats()
	if err != nil {
		http.Error(w, "Failed to get stats", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// RegisterRoutes registers metadata API routes on an HTTP mux
func (m *MetadataAPI) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/metadata/check", m.HandleCheck)
	mux.HandleFunc("/api/v1/metadata/stats", m.HandleStats)
}
