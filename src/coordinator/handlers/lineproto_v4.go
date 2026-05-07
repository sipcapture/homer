// Copyright (C) 2026 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package handlers

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/sipcapture/homer-core/src/coordinator/services"
)

// LineProtoHandler exposes read-only discovery for the dynamic
// DuckLake tables created by the InfluxDB Line Protocol receiver
// (see src/lineprotoreceiver). Each measurement materialises into a
// table whose name is derived from the measurement plus a configurable
// prefix (default "lp_"); since the schema is not known up front,
// downstream consumers (the Proto Search widget, Smart Input, Grafana,
// etc.) discover it through these endpoints rather than via static
// mapping_schema rows.
//
// Routes (all under /api/v4):
//
//   GET /line_protocol/tables                  — list all matching tables
//   GET /line_protocol/tables/:schema/:table   — list one table's columns
//
// Responses follow the v4 envelope (Meta / Data / Pagination) used by
// the rest of the coordinator API.
type LineProtoHandler struct {
	flightService *services.FlightService
	// defaultPrefix is the value used when the client does not pass
	// ?prefix=. It mirrors LineProtoConfig.TablePrefix so the API surface
	// matches the receiver's defaults out of the box.
	defaultPrefix string
}

// NewLineProtoHandler constructs the handler. flightService must not be
// nil; defaultPrefix may be empty in which case "lp_" is used.
func NewLineProtoHandler(fs *services.FlightService, defaultPrefix string) *LineProtoHandler {
	if defaultPrefix == "" {
		defaultPrefix = "lp_"
	}
	return &LineProtoHandler{flightService: fs, defaultPrefix: defaultPrefix}
}

// LineProtoColumn describes one column in an LP-managed DuckLake table.
type LineProtoColumn struct {
	Name     string `json:"name"`
	DataType string `json:"data_type"`
	Nullable bool   `json:"nullable"`
	Position int    `json:"position"`
}

// LineProtoTable groups a discovered table with its enumerated columns.
// Catalog/Schema/Name are returned separately so the client can build a
// fully-qualified identifier without parsing string concatenations.
type LineProtoTable struct {
	Catalog string            `json:"catalog"`
	Schema  string            `json:"schema"`
	Name    string            `json:"name"`
	FQN     string            `json:"fqn"`
	Columns []LineProtoColumn `json:"columns"`
}

// LineProtoTablesResponseV4 is the envelope for the list endpoint.
type LineProtoTablesResponseV4 struct {
	Data struct {
		Items  []LineProtoTable `json:"items"`
		Prefix string           `json:"prefix"`
	} `json:"data"`
	Total int  `json:"total"`
	Meta  Meta `json:"meta"`
}

// LineProtoTableResponseV4 is the envelope for the single-table endpoint.
type LineProtoTableResponseV4 struct {
	Data LineProtoTable `json:"data"`
	Meta Meta           `json:"meta"`
}

// V4LineProtoTables handles GET /api/v4/line_protocol/tables.
//
// Optional query parameters:
//
//   ?prefix=<str>   — override the default "lp_" filter (e.g. "metric_")
//   ?schema=<str>   — restrict to a single DuckDB schema (the per-?db= one)
//   ?with_columns=false — skip column enumeration to keep the response small
func (h *LineProtoHandler) V4LineProtoTables(c echo.Context) error {
	limit, err := parseLimit(c)
	if err != nil {
		return writeError(c, http.StatusBadRequest, "Bad Request", "Invalid page[limit]")
	}

	prefix := strings.TrimSpace(c.QueryParam("prefix"))
	if prefix == "" {
		prefix = h.defaultPrefix
	}
	if !isSafePrefix(prefix) {
		return writeError(c, http.StatusBadRequest, "Bad Request", "prefix must be ASCII alphanumeric or underscore")
	}
	schemaFilter := strings.TrimSpace(c.QueryParam("schema"))
	if schemaFilter != "" && !isSafeIdent(schemaFilter) {
		return writeError(c, http.StatusBadRequest, "Bad Request", "schema must be a valid identifier")
	}
	withColumns := strings.ToLower(strings.TrimSpace(c.QueryParam("with_columns"))) != "false"

	tables, err := h.listTables(c, prefix, schemaFilter)
	if err != nil {
		return writeError(c, http.StatusInternalServerError, "Server Error", "Failed to list line-protocol tables")
	}
	if len(tables) > limit {
		tables = tables[:limit]
	}

	if withColumns {
		if err := h.attachColumns(c, tables); err != nil {
			return writeError(c, http.StatusInternalServerError, "Server Error", "Failed to read column metadata")
		}
	}

	resp := LineProtoTablesResponseV4{}
	resp.Data.Items = tables
	resp.Data.Prefix = prefix
	resp.Total = len(tables)
	resp.Meta = buildMeta(c, "")
	resp.Meta.Pagination = &Pagination{Limit: limit, Total: len(tables), HasMore: false}
	return c.JSON(http.StatusOK, resp)
}

// V4LineProtoTable handles GET /api/v4/line_protocol/tables/:schema/:table.
// Returns the catalog/schema/name plus the full ordered column list.
func (h *LineProtoHandler) V4LineProtoTable(c echo.Context) error {
	schema := strings.TrimSpace(c.Param("schema"))
	table := strings.TrimSpace(c.Param("table"))
	if schema == "" || table == "" {
		return writeError(c, http.StatusBadRequest, "Bad Request", "schema and table are required")
	}
	if !isSafeIdent(schema) || !isSafeIdent(table) {
		return writeError(c, http.StatusBadRequest, "Bad Request", "schema and table must be valid identifiers")
	}

	row, err := h.lookupTable(c, schema, table)
	if err != nil {
		return writeError(c, http.StatusInternalServerError, "Server Error", "Failed to look up table")
	}
	if row == nil {
		return writeError(c, http.StatusNotFound, "Not Found", fmt.Sprintf("table %s.%s not found", schema, table))
	}
	if err := h.attachColumns(c, []LineProtoTable{*row}); err != nil {
		return writeError(c, http.StatusInternalServerError, "Server Error", "Failed to read column metadata")
	}
	// attachColumns mutates by index — we passed a slice of one, fetch
	// it back out so the response carries the populated struct.
	resp := LineProtoTableResponseV4{
		Data: *row,
		Meta: buildMeta(c, ""),
	}
	return c.JSON(http.StatusOK, resp)
}

// listTables enumerates rows from information_schema.tables that match
// the given prefix (and optional schema). The DuckLake catalog name is
// returned per-row so callers can build the fully-qualified identifier.
func (h *LineProtoHandler) listTables(c echo.Context, prefix, schemaFilter string) ([]LineProtoTable, error) {
	var sb strings.Builder
	sb.WriteString("SELECT table_catalog, table_schema, table_name FROM information_schema.tables WHERE table_name LIKE '")
	sb.WriteString(escapeLiteral(prefix))
	sb.WriteString("%'")
	if schemaFilter != "" {
		sb.WriteString(" AND table_schema = '")
		sb.WriteString(escapeLiteral(schemaFilter))
		sb.WriteString("'")
	}
	sb.WriteString(" ORDER BY table_catalog, table_schema, table_name")

	rows, err := h.flightService.Query(c.Request().Context(), sb.String())
	if err != nil {
		return nil, err
	}
	out := make([]LineProtoTable, 0, len(rows))
	for _, r := range rows {
		t := LineProtoTable{
			Catalog: stringField(r, "table_catalog"),
			Schema:  stringField(r, "table_schema"),
			Name:    stringField(r, "table_name"),
		}
		if t.Name == "" {
			continue
		}
		t.FQN = fqnOf(t.Catalog, t.Schema, t.Name)
		out = append(out, t)
	}
	return out, nil
}

// lookupTable returns a single matching row or nil when not found.
func (h *LineProtoHandler) lookupTable(c echo.Context, schema, table string) (*LineProtoTable, error) {
	sql := "SELECT table_catalog, table_schema, table_name FROM information_schema.tables WHERE table_schema = '" +
		escapeLiteral(schema) + "' AND table_name = '" + escapeLiteral(table) + "' LIMIT 1"
	rows, err := h.flightService.Query(c.Request().Context(), sql)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	r := rows[0]
	t := &LineProtoTable{
		Catalog: stringField(r, "table_catalog"),
		Schema:  stringField(r, "table_schema"),
		Name:    stringField(r, "table_name"),
	}
	t.FQN = fqnOf(t.Catalog, t.Schema, t.Name)
	return t, nil
}

// attachColumns runs a single information_schema.columns query for the
// whole batch (if non-empty) and assigns the resulting columns back to
// the matching tables, preserving ordinal_position order.
func (h *LineProtoHandler) attachColumns(c echo.Context, tables []LineProtoTable) error {
	if len(tables) == 0 {
		return nil
	}

	// Build an OR'd predicate over (schema,name) pairs in a single
	// query — keeps the round-trip count constant regardless of how
	// many tables matched.
	var predicates []string
	for _, t := range tables {
		predicates = append(predicates,
			"(table_schema = '"+escapeLiteral(t.Schema)+"' AND table_name = '"+escapeLiteral(t.Name)+"')")
	}
	sql := "SELECT table_schema, table_name, column_name, data_type, is_nullable, ordinal_position " +
		"FROM information_schema.columns WHERE " + strings.Join(predicates, " OR ") +
		" ORDER BY table_schema, table_name, ordinal_position"

	rows, err := h.flightService.Query(c.Request().Context(), sql)
	if err != nil {
		return err
	}

	// Index tables by "schema.name" so the assignment loop stays O(N).
	idx := make(map[string]int, len(tables))
	for i, t := range tables {
		idx[t.Schema+"."+t.Name] = i
	}
	for _, r := range rows {
		key := stringField(r, "table_schema") + "." + stringField(r, "table_name")
		i, ok := idx[key]
		if !ok {
			continue
		}
		col := LineProtoColumn{
			Name:     stringField(r, "column_name"),
			DataType: stringField(r, "data_type"),
			Nullable: strings.EqualFold(stringField(r, "is_nullable"), "YES"),
			Position: intField(r, "ordinal_position"),
		}
		tables[i].Columns = append(tables[i].Columns, col)
	}
	return nil
}

// fqnOf returns "<catalog>.<schema>.<name>" with empty parts elided so
// it works for both DuckDB-only and DuckLake-attached deployments.
func fqnOf(catalog, schema, name string) string {
	switch {
	case catalog != "" && schema != "":
		return catalog + "." + schema + "." + name
	case schema != "":
		return schema + "." + name
	default:
		return name
	}
}

// isSafeIdent reports whether s is a non-empty ASCII identifier
// (alphanumeric or underscore, not starting with a digit). Used to
// guard the path/query parameters that are interpolated into SQL —
// callers can rely on isSafeIdent + escapeLiteral together for full
// safety.
func isSafeIdent(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		ok := r == '_' ||
			(r >= 'a' && r <= 'z') ||
			(r >= 'A' && r <= 'Z') ||
			(i > 0 && r >= '0' && r <= '9')
		if !ok {
			return false
		}
	}
	return true
}

// isSafePrefix is the prefix-form of isSafeIdent (allows leading digits
// because nothing forces an LP measurement to start with a letter).
func isSafePrefix(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		ok := r == '_' ||
			(r >= 'a' && r <= 'z') ||
			(r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9')
		if !ok {
			return false
		}
	}
	return true
}

// stringField returns the named field as a trimmed string, or "" when
// missing / nil. Mirrors the helper pattern used by other v4 handlers.
func stringField(row map[string]interface{}, key string) string {
	v, ok := row[key]
	if !ok || v == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(v))
}

// intField returns the named field as int, or 0 when missing / unparseable.
func intField(row map[string]interface{}, key string) int {
	v, ok := row[key]
	if !ok || v == nil {
		return 0
	}
	switch n := v.(type) {
	case int:
		return n
	case int32:
		return int(n)
	case int64:
		return int(n)
	case float64:
		return int(n)
	}
	var i int
	_, _ = fmt.Sscanf(fmt.Sprint(v), "%d", &i)
	return i
}
