// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package handlers

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/sipcapture/homer-core/src/coordinator/services"
)

type NodeInfo struct {
	Name     string
	Host     string
	Port     int
	Priority int
}

type NodesHandler struct {
	nodes   []NodeInfo
	flight  *services.FlightService
	config  nodesHandlerConfig
}

type nodesHandlerConfig struct {
	LakeName string
}

func NewNodesHandler(nodes []NodeInfo) *NodesHandler {
	copied := make([]NodeInfo, len(nodes))
	copy(copied, nodes)
	return &NodesHandler{nodes: copied}
}

func NewNodesHandlerWithFlight(nodes []NodeInfo, flight *services.FlightService, lakeName string) *NodesHandler {
	h := NewNodesHandler(nodes)
	h.flight = flight
	h.config.LakeName = lakeName
	return h
}

type NodeV4 struct {
	Arhive      bool   `json:"arhive,omitempty"`
	DBArchive   string `json:"db_archive,omitempty"`
	DBName      string `json:"db_name,omitempty"`
	Host        string `json:"host,omitempty"`
	Name        string `json:"name,omitempty"`
	Node        string `json:"node,omitempty"`
	Online      bool   `json:"online,omitempty"`
	Primary     bool   `json:"primary,omitempty"`
	TablePrefix string `json:"table_prefix,omitempty"`
	Value       string `json:"value,omitempty"`
}

type NodeListResponseV4 struct {
	Data struct {
		Items []NodeV4 `json:"items"`
	} `json:"data"`
	Meta Meta `json:"meta"`
}

// DuckLakeSettingV4 represents one row from ducklake_settings()
type DuckLakeSettingV4 struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type DuckLakeSettingsResponseV4 struct {
	Data struct {
		Items []DuckLakeSettingV4 `json:"items"`
	} `json:"data"`
	Meta Meta `json:"meta"`
}

// DbSqlAutocompleteSchemaResponseV4 is catalog → schema → table → column names for SQL editor completion.
type DbSqlAutocompleteSchemaResponseV4 struct {
	Data map[string]map[string]map[string][]string `json:"data"`
	Meta Meta                                     `json:"meta"`
}

func sqlAutocompleteRowString(row map[string]interface{}, want string) string {
	lw := strings.ToLower(want)
	for k, v := range row {
		if strings.ToLower(k) == lw {
			return strings.TrimSpace(fmt.Sprint(v))
		}
	}
	return ""
}

// V4DbSqlAutocompleteSchema returns live column metadata from the first connected storage node
// (information_schema), for CodeMirror / SQL UIs — not Influx-style statistics.
func (h *NodesHandler) V4DbSqlAutocompleteSchema(c echo.Context) error {
	if _, err := getUsernameFromContext(c); err != nil {
		return writeError(c, http.StatusUnauthorized, "Unauthorized", "Not authenticated")
	}
	if h.flight == nil {
		return writeError(c, http.StatusServiceUnavailable, "Unavailable", "Flight service not configured")
	}

	const q = `
SELECT table_catalog AS catalog, table_schema AS schema, table_name AS table_name, column_name AS column_name
FROM information_schema.columns
WHERE table_schema = 'main'
  AND table_catalog NOT IN ('system', 'temp', 'memory', 'information_schema', 'pg_catalog')
ORDER BY table_catalog, table_schema, table_name, column_name
`
	rows, err := h.flight.QueryFirstConnected(c.Request().Context(), q)
	if err != nil {
		return writeError(c, http.StatusInternalServerError, "Server Error", "Failed to load SQL schema from storage node")
	}

	out := make(map[string]map[string]map[string][]string)
	seen := make(map[string]map[string]map[string]map[string]struct{})

	for _, row := range rows {
		cat := sqlAutocompleteRowString(row, "catalog")
		sch := sqlAutocompleteRowString(row, "schema")
		tbl := sqlAutocompleteRowString(row, "table_name")
		col := sqlAutocompleteRowString(row, "column_name")
		if cat == "" || sch == "" || tbl == "" || col == "" {
			continue
		}
		if out[cat] == nil {
			out[cat] = make(map[string]map[string][]string)
		}
		if out[cat][sch] == nil {
			out[cat][sch] = make(map[string][]string)
		}
		if seen[cat] == nil {
			seen[cat] = make(map[string]map[string]map[string]struct{})
		}
		if seen[cat][sch] == nil {
			seen[cat][sch] = make(map[string]map[string]struct{})
		}
		if seen[cat][sch][tbl] == nil {
			seen[cat][sch][tbl] = make(map[string]struct{})
		}
		if _, dup := seen[cat][sch][tbl][col]; dup {
			continue
		}
		seen[cat][sch][tbl][col] = struct{}{}
		out[cat][sch][tbl] = append(out[cat][sch][tbl], col)
	}

	resp := DbSqlAutocompleteSchemaResponseV4{
		Data: out,
		Meta: buildMeta(c, ""),
	}
	return c.JSON(http.StatusOK, resp)
}

// V4DuckLakeSettings queries ducklake_settings() from the first available node
// and returns catalog type, extension version, and data path.
func (h *NodesHandler) V4DuckLakeSettings(c echo.Context) error {
	if _, err := getUsernameFromContext(c); err != nil {
		return writeError(c, http.StatusUnauthorized, "Unauthorized", "Not authenticated")
	}
	if h.flight == nil {
		return writeError(c, http.StatusServiceUnavailable, "Unavailable", "Flight service not configured")
	}

	lakeName := h.config.LakeName
	if lakeName == "" {
		lakeName = "homer_lake"
	}

	sql := fmt.Sprintf("SELECT name, value::VARCHAR FROM ducklake_settings('%s')", lakeName)
	rows, err := h.flight.Query(context.Background(), sql)
	if err != nil {
		return writeError(c, http.StatusInternalServerError, "Query Error", "Failed to query ducklake_settings: "+err.Error())
	}

	resp := DuckLakeSettingsResponseV4{}
	resp.Data.Items = make([]DuckLakeSettingV4, 0, len(rows))
	for _, row := range rows {
		item := DuckLakeSettingV4{}
		if v, ok := row["name"].(string); ok {
			item.Name = v
		}
		if v, ok := row["value"].(string); ok {
			item.Value = v
		}
		resp.Data.Items = append(resp.Data.Items, item)
	}
	resp.Meta = buildMeta(c, "")
	resp.Meta.Pagination = &Pagination{Limit: len(resp.Data.Items), Total: len(resp.Data.Items)}
	return c.JSON(http.StatusOK, resp)
}

func (h *NodesHandler) V4NodesList(c echo.Context) error {
	resp := NodeListResponseV4{}
	resp.Data.Items = make([]NodeV4, 0, len(h.nodes))

	minPriority := 0
	hasPriority := false
	for _, node := range h.nodes {
		if !hasPriority || node.Priority < minPriority {
			minPriority = node.Priority
			hasPriority = true
		}
	}

	for _, node := range h.nodes {
		nodeID := node.Name
		if nodeID == "" {
			nodeID = fmt.Sprintf("%s:%d", node.Host, node.Port)
		}
		primary := hasPriority && node.Priority == minPriority
		resp.Data.Items = append(resp.Data.Items, NodeV4{
			Name:    node.Name,
			Host:    node.Host,
			Node:    nodeID,
			Value:   nodeID,
			Primary: primary,
			Online:  node.Host != "",
		})
	}

	resp.Meta = buildMeta(c, "")
	resp.Meta.Pagination = &Pagination{
		Limit:   len(resp.Data.Items),
		Total:   len(resp.Data.Items),
		HasMore: false,
	}

	return c.JSON(http.StatusOK, resp)
}
