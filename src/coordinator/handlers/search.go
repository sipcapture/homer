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
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/sipcapture/homer-core/src/config"
	"github.com/sipcapture/homer-core/src/coordinator/services"
	"github.com/sipcapture/homer-core/src/coordinator/sqlvalidator"
	mcppkg "github.com/sipcapture/homer-core/src/mcp"
)

// Correlator is the subset of the Lua correlation engine consumed by the
// transaction-messages handler. Keeping it as an interface here avoids an
// import cycle between handlers and the correlation package and lets tests
// inject a fake.
type Correlator interface {
	Has(hepid int, profile string) bool
	Correlate(ctx context.Context, in CorrelationInput) *CorrelationResult
}

// CorrelationInput and CorrelationResult mirror the correlation package
// types so handlers can depend on Correlator without importing correlation
// directly (breaks an otherwise-cyclic dep during wiring).
type CorrelationInput struct {
	HepID      int
	Profile    string
	ProtoType  int
	EventType  string
	BaseRows   []map[string]interface{}
	SessionIDs []string
	Nodes      []string
	TimeFrom   int64
	TimeTo     int64
}

// CorrelationResult carries the additional session_ids and debug lines a
// correlation script emitted.
type CorrelationResult struct {
	ExtraSessionIDs []string
	Debug           []string
}

// SearchHandler handles search-related API endpoints
type SearchHandler struct {
	flightService *services.FlightService
	aliasService  *services.AliasService
	mcpConfig     *config.MCPConfig
	// llm is the shared OpenAI-compatible chat client used for MCP NL→search
	// translation. nil when LLM is disabled (regex-only mode).
	llm          *mcppkg.LLMClient
	shareExports *services.ShareExportService
	viewTokens   *services.TransactionViewTokenService
	// transactionViewMaxOpens is stored on each view token (GET /export/view/:uuid success budget).
	transactionViewMaxOpens int
	// correlation runs Lua-based session_id expansion inside /transactions/messages.
	// nil means correlation is disabled; handlers must nil-check.
	correlation Correlator
	// mappingService loads mapping_schema rows for fields_mapping virtual filters (optional).
	mappingService *services.MappingService
	// lakeTopNStrategyCfg selects how long-range timestamp-DESC top-N searches
	// run (stream | chunked | full). Empty defaults to stream. Mirrors
	// storage.ducklake.search.lake_topn_strategy.
	lakeTopNStrategyCfg string
	// lakeChunkSec is the stream-strategy time-window width in seconds (<=0 =>
	// 1h default). Mirrors storage.ducklake.search.lake_chunk_sec.
	lakeChunkSec int
	// lazyPayloadHydration runs transaction searches in two phases: phase 1
	// SELECT uuid, timestamp (filter/sort/LIMIT), then a bounded SELECT * by
	// uuid IN (…). Mirrors storage.ducklake.search.lazy_payload (default on).
	lazyPayloadHydration bool
}

// NewSearchHandler creates a new search handler.
// aliasSvc may be nil; transaction rows will not get aliasSrc/aliasDst in that case.
func NewSearchHandler(fs *services.FlightService, aliasSvc *services.AliasService, mcpCfg *config.MCPConfig, share *services.ShareExportService, viewTok *services.TransactionViewTokenService, transactionViewMaxOpens int, mappingSvc *services.MappingService, lakeTopNStrategy string, lakeChunkSec int) *SearchHandler {
	if transactionViewMaxOpens <= 0 {
		transactionViewMaxOpens = 3
	}
	if transactionViewMaxOpens > 1000 {
		transactionViewMaxOpens = 1000
	}
	var llm *mcppkg.LLMClient
	if mcpCfg != nil {
		// NewLLMClient returns nil when llm.enable=false, so the handler can
		// rely on h.llm.Enabled() / nil-checks without re-reading the config.
		llm = mcppkg.NewLLMClient(&mcpCfg.LLM)
	}
	return &SearchHandler{
		flightService:           fs,
		aliasService:            aliasSvc,
		mcpConfig:               mcpCfg,
		llm:                     llm,
		shareExports:            share,
		viewTokens:              viewTok,
		transactionViewMaxOpens: transactionViewMaxOpens,
		mappingService:          mappingSvc,
		lakeTopNStrategyCfg:     lakeTopNStrategy,
		lakeChunkSec:            lakeChunkSec,
		lazyPayloadHydration:    true,
	}
}

// SetLazyPayloadHydration toggles the two-phase (narrow search + by-uuid
// hydration) execution of transaction searches. Defaults to enabled.
func (h *SearchHandler) SetLazyPayloadHydration(enabled bool) {
	h.lazyPayloadHydration = enabled
}

// SetCorrelator attaches (or clears) the Lua-based correlation engine used
// by /transactions/messages. Safe to call before the server starts; must not
// be called concurrently with in-flight requests.
func (h *SearchHandler) SetCorrelator(c Correlator) {
	h.correlation = c
}

// SimpleSearchRequest represents the simple UI search request
type SimpleSearchRequest struct {
	Limit           int    `json:"limit"`
	Offset          int    `json:"offset"`
	From            string `json:"from"`
	To              string `json:"to"`
	CallID          string `json:"call_id"`
	FromUser        string `json:"from_user"`
	ToUser          string `json:"to_user"`
	Method          string `json:"method"`
	SrcIP           string `json:"src_ip"`
	DstIP           string `json:"dst_ip"`
	ProtoType       int    `json:"proto_type"`       // HEP type: 1=SIP, 5=RTCP, 34=RTP Agent, 35=RTP Stats, 100=LOG
	TransactionType string `json:"transaction_type"` // call, registration, default
	NodeID          string `json:"node_id"`
	SQL             string `json:"sql"`
}

// SearchCalls handles POST /api/v1/search and /api/v3/search/call/data
func (h *SearchHandler) SearchCalls(c echo.Context) error {
	var req SimpleSearchRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"error":   "Invalid request body",
		})
	}

	// Build SQL query
	sql, err := h.buildSimpleSearchSQL(&req)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
	}

	fromNs, toNs := simpleSearchRangeNs(&req)
	results, err := h.flightService.QueryWithRange(c.Request().Context(), sql, fromNs, toNs)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"error":   fmt.Sprintf("Query failed: %v", err),
		})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"success": true,
		"data": map[string]interface{}{
			"count":   len(results),
			"rows":    results,
			"columns": getColumns(results),
		},
	})
}

// buildSimpleSearchSQL builds SQL from simple UI request.
// Returns the SQL string and an error if raw SQL validation fails.
func (h *SearchHandler) buildSimpleSearchSQL(req *SimpleSearchRequest) (string, error) {
	// If raw SQL provided, validate it for safety
	if req.SQL != "" {
		if err := sqlvalidator.ValidateRawSQL(req.SQL); err != nil {
			return "", fmt.Errorf("SQL validation failed: %w", err)
		}
		return req.SQL, nil
	}

	var conditions []string

	// Time range filter (milliseconds)
	if req.From != "" {
		if ts, err := parseTimestamp(req.From); err == nil {
			conditions = append(conditions, fmt.Sprintf("timestamp >= (to_timestamp(%d / 1000.0) AT TIME ZONE 'UTC')", ts))
		}
	}
	if req.To != "" {
		if ts, err := parseTimestamp(req.To); err == nil {
			conditions = append(conditions, fmt.Sprintf("timestamp <= (to_timestamp(%d / 1000.0) AT TIME ZONE 'UTC')", ts))
		}
	}

	// Field filters
	if req.CallID != "" {
		txType := normalizeSIPTransactionType(req.ProtoType, req.TransactionType)
		conditions = append(conditions, sipCallIDMatchClause(txType, req.CallID))
	}
	if req.FromUser != "" {
		conditions = append(conditions, sqlFormMatchClause("caller", req.FromUser))
	}
	if req.ToUser != "" {
		conditions = append(conditions, sqlFormMatchClause("callee", req.ToUser))
	}
	if req.Method != "" {
		conditions = append(conditions, fmt.Sprintf("method = '%s'", sqlvalidator.SafeString(req.Method)))
	}
	if req.SrcIP != "" {
		conditions = append(conditions, fmt.Sprintf("src_ip = '%s'", sqlvalidator.SafeString(req.SrcIP)))
	}
	if req.DstIP != "" {
		conditions = append(conditions, fmt.Sprintf("dst_ip = '%s'", sqlvalidator.SafeString(req.DstIP)))
	}
	if req.NodeID != "" {
		conditions = append(conditions, fmt.Sprintf("node_id = '%s'", sqlvalidator.SafeString(req.NodeID)))
	}

	// Build table name based on proto_type and transaction_type
	tableName := getTableName(h.flightService.LakeName(), req.ProtoType, req.TransactionType)

	sql := fmt.Sprintf("SELECT * FROM %s", tableName)
	if len(conditions) > 0 {
		sql += " WHERE " + strings.Join(conditions, " AND ")
	}
	sql += " ORDER BY timestamp DESC"

	// Apply limit and offset
	limit := req.Limit
	if limit <= 0 || limit > 50000 {
		limit = 50
	}
	sql += fmt.Sprintf(" LIMIT %d", limit)

	if req.Offset > 0 {
		sql += fmt.Sprintf(" OFFSET %d", req.Offset)
	}

	return sql, nil
}

// normalizeSIPTransactionType maps UI / filter event_type to DuckLake SIP sub-type
// (must stay in sync with getTableName).
//
// "all" (see isAllEventType) is NOT resolved here: it addresses every
// physical event-type table at once and callers that support merging
// (currently V4TransactionsSearch, see transactionSearchWantsAllEventTypes)
// must intercept it before reaching this function / getTableName. Any "all"
// value that does reach here (e.g. aggregation searches, which cannot be
// merged in Go) falls into the default branch below and resolves to the
// single "default" table, matching the historical behavior for unknown
// event_type values.
func normalizeSIPTransactionType(protoType int, transactionType string) string {
	if protoType == 0 {
		protoType = 1
	}
	if strings.TrimSpace(transactionType) == "" {
		transactionType = "call"
	}
	switch strings.ToLower(strings.TrimSpace(transactionType)) {
	case "call", "calls":
		transactionType = "call"
	case "registration", "registrations", "register":
		transactionType = "registration"
	case "siprec":
		transactionType = "siprec"
	default:
		transactionType = "default"
	}
	if protoType != 1 {
		return "default"
	}
	return transactionType
}

// isAllEventType reports whether a UI/API event_type filter requests a
// merged view across every physical SIP event-type table (call/registration/
// default) instead of addressing a single one. See
// transactionSearchWantsAllEventTypes / queryTransactionSearchAllEventTypes
// for where this merge is actually performed.
func isAllEventType(eventType string) bool {
	switch strings.ToLower(strings.TrimSpace(eventType)) {
	case "all", "*":
		return true
	}
	return false
}

// OTLP "virtual" hepid range, mirrored in the seeded mapping_schema rows
// (see services/mapping_seed.go). These do not collide with real HEP types
// — they let the Proto Search widget address the dedicated otlp_* DuckLake
// tables created by the OTLP receiver (storage/ducklake/otlp_storage.go).
const (
	otlpHepIDTraces  = 200
	otlpHepIDMetrics = 201
	otlpHepIDLogs    = 202
)

// isOTLPProtoType reports whether the given proto_type addresses one of the
// OTLP virtual mappings (traces / metrics / logs).
func isOTLPProtoType(protoType int) bool {
	switch protoType {
	case otlpHepIDTraces, otlpHepIDMetrics, otlpHepIDLogs:
		return true
	}
	return false
}

// otlpTableForHepID returns the unqualified OTLP table name for the given
// virtual hepid, or "" if the hepid is not an OTLP signal.
func otlpTableForHepID(protoType int) string {
	switch protoType {
	case otlpHepIDTraces:
		return "otlp_traces"
	case otlpHepIDMetrics:
		return "otlp_metrics"
	case otlpHepIDLogs:
		return "otlp_logs"
	}
	return ""
}

// lpHepID mirrors services.LPVirtualHepID so this package can do the
// routing decision without a circular import. Keep the two in sync —
// services.LPVirtualHepID is the source of truth.
const lpHepID = 300

// isLPProtoType reports whether the given proto_type addresses a Line
// Protocol virtual mapping (one synthetic hepid covers every dynamic
// lp_<measurement> table; the actual table name lives in the profile).
func isLPProtoType(protoType int) bool {
	return protoType == lpHepID
}

// lpTableForProfile decodes the "<schema>__<table>" profile encoding
// emitted by services.LPProfileFor and returns the fully-qualified
// DuckLake table name. Schema and table segments MUST be safe SQL
// identifiers (see isSafeIdent); catalog/system names are rejected so
// a crafted event_type cannot break out of the FROM clause
// (GHSA-94cf-g6mg-6gv7). When the profile lacks the separator we fall
// back to "main.<profile>" so legacy operators who hand-edited the
// mapping_schema row still get a usable address.
func lpTableForProfile(lakeName, profile string) (string, bool) {
	profile = strings.TrimSpace(profile)
	if profile == "" {
		return "", false
	}
	sep := "__"
	var schema, table string
	if i := strings.Index(profile, sep); i > 0 && i+len(sep) < len(profile) {
		schema = profile[:i]
		table = profile[i+len(sep):]
	} else {
		schema = "main"
		table = profile
	}
	if !isSafeIdent(schema) || !isSafeIdent(table) {
		return "", false
	}
	if isReservedLPSchema(schema) || isReservedLPTable(table) {
		return "", false
	}
	return fmt.Sprintf("%s.%s.%s", lakeName, schema, table), true
}

// isReservedLPSchema reports schemas that must never be addressed via
// the LP virtual mapping (system / catalog namespaces). Kept in sync
// with the purge predicates in services.LPMappingSync.
func isReservedLPSchema(schema string) bool {
	switch strings.ToLower(schema) {
	case "information_schema", "pg_catalog":
		return true
	default:
		return false
	}
}

// isReservedLPTable reports table names that must never be addressed
// via the LP virtual mapping (DuckLake internal catalog tables).
func isReservedLPTable(table string) bool {
	return strings.HasPrefix(strings.ToLower(table), "ducklake_")
}

// getTableName returns the DuckLake table name based on proto_type and transaction_type
func getTableName(lakeName string, protoType int, transactionType string) string {
	if protoType == 0 {
		protoType = 1 // SIP
	}
	// OTLP virtual mappings live in their own top-level tables (see
	// storage/ducklake/otlp_storage.go) and do not follow the
	// hep_proto_<id>_<profile> naming scheme.
	if t := otlpTableForHepID(protoType); t != "" {
		return fmt.Sprintf("%s.%s", lakeName, t)
	}
	// Line Protocol virtual mappings: profile encodes the dynamic
	// lp_<measurement> table. Resolved via lpTableForProfile so a
	// missing/blank profile collapses to a syntactically valid name
	// (caller will get a "no such table" instead of malformed SQL).
	if isLPProtoType(protoType) {
		if t, ok := lpTableForProfile(lakeName, transactionType); ok {
			return t
		}
	}
	suffix := normalizeSIPTransactionType(protoType, transactionType)
	return fmt.Sprintf("%s.main.hep_proto_%d_%s", lakeName, protoType, suffix)
}

// GetCallByID handles GET /api/v1/call/:callid
func (h *SearchHandler) GetCallByID(c echo.Context) error {
	callID := c.Param("callid")
	if callID == "" {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"error":   "callid is required",
		})
	}

	// URL decode the callID (e.g., %40 -> @)
	if decoded, err := url.QueryUnescape(callID); err == nil {
		callID = decoded
	}

	fromTs := c.QueryParam("from")
	toTs := c.QueryParam("to")
	protoTypeStr := c.QueryParam("proto_type")
	transactionType := c.QueryParam("transaction_type")

	protoType := 1 // Default to SIP
	if protoTypeStr != "" {
		if pt, err := strconv.Atoi(protoTypeStr); err == nil {
			protoType = pt
		}
	}

	// Get single table based on proto_type and transaction_type
	tableName := getTableName(h.flightService.LakeName(), protoType, transactionType)

	// Use LIKE for partial match since call_id may have different formats
	sql := fmt.Sprintf(
		"SELECT * FROM %s WHERE session_id LIKE '%%%s%%'",
		tableName, sqlvalidator.SafeString(callID),
	)

	if fromTs != "" {
		if ts, err := parseTimestamp(fromTs); err == nil {
			sql += fmt.Sprintf(" AND timestamp >= (to_timestamp(%d / 1000.0) AT TIME ZONE 'UTC')", ts)
		}
	}
	if toTs != "" {
		if ts, err := parseTimestamp(toTs); err == nil {
			sql += fmt.Sprintf(" AND timestamp <= (to_timestamp(%d / 1000.0) AT TIME ZONE 'UTC')", ts)
		}
	}

	sql += " ORDER BY timestamp ASC LIMIT 1000"

	slog.Info("GetCallByID query", "sql", sql, "callid", callID)

	var fromNs, toNs int64
	if fromTs != "" {
		if ts, err := parseTimestamp(fromTs); err == nil {
			fromNs = msToNs(ts)
		}
	}
	if toTs != "" {
		if ts, err := parseTimestamp(toTs); err == nil {
			toNs = msToNs(ts)
		}
	}
	results, err := h.flightService.QueryWithRange(c.Request().Context(), sql, fromNs, toNs)
	if err != nil {
		slog.Error("GetCallByID query failed", "error", err)
		return c.JSON(http.StatusOK, map[string]interface{}{
			"success": true,
			"data": map[string]interface{}{
				"count":   0,
				"rows":    []map[string]interface{}{},
				"callid":  callID,
				"columns": []string{},
			},
		})
	}

	slog.Info("GetCallByID results", "count", len(results))

	return c.JSON(http.StatusOK, map[string]interface{}{
		"success": true,
		"data": map[string]interface{}{
			"count":   len(results),
			"rows":    results,
			"callid":  callID,
			"columns": getColumns(results),
		},
	})
}

// GetPayload handles GET /api/v1/payload/:uuid
func (h *SearchHandler) GetPayload(c echo.Context) error {
	uuid := c.Param("uuid")
	if uuid == "" {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"error":   "uuid is required",
		})
	}

	// Search across all tables for the UUID
	ln := h.flightService.LakeName()
	tables := []string{
		ln + ".main.hep_proto_1_call",
		ln + ".main.hep_proto_1_registration",
		ln + ".main.hep_proto_1_default",
	}

	for _, table := range tables {
		sql := fmt.Sprintf(
			"SELECT * FROM %s WHERE uuid = '%s' LIMIT 1",
			table, sqlvalidator.SafeString(uuid),
		)

		results, err := h.flightService.Query(c.Request().Context(), sql)
		if err != nil {
			continue
		}

		if len(results) > 0 {
			return c.JSON(http.StatusOK, map[string]interface{}{
				"success": true,
				"data":    results[0],
			})
		}
	}

	return c.JSON(http.StatusNotFound, map[string]interface{}{
		"success": false,
		"error":   "Record not found",
	})
}

// simpleSearchRangeNs converts SimpleSearchRequest from/to strings to epoch ns
// for smart-routing pruning. Missing/invalid values stay 0 (no pruning).
func simpleSearchRangeNs(req *SimpleSearchRequest) (fromNs, toNs int64) {
	if req == nil {
		return 0, 0
	}
	if req.From != "" {
		if ts, err := parseTimestamp(req.From); err == nil {
			fromNs = msToNs(ts)
		}
	}
	if req.To != "" {
		if ts, err := parseTimestamp(req.To); err == nil {
			toNs = msToNs(ts)
		}
	}
	return fromNs, toNs
}

// parseTimestamp parses ISO timestamp or unix milliseconds
func parseTimestamp(s string) (int64, error) {
	if ts, err := strconv.ParseInt(s, 10, 64); err == nil {
		return ts, nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t, err = time.Parse("2006-01-02T15:04:05", s)
		if err != nil {
			return 0, err
		}
	}
	return t.UnixMilli(), nil
}

// getColumns extracts column names from results
func getColumns(results []map[string]interface{}) []string {
	if len(results) == 0 {
		return nil
	}
	// Union of keys across all rows (not just results[0]) so columns present on
	// only some rows — enriched custom headers, aliasSrc/aliasDst — are not dropped
	// just because the first row happens to lack them.
	seen := make(map[string]struct{}, len(results[0]))
	columns := make([]string, 0, len(results[0]))
	for _, row := range results {
		for k := range row {
			if _, ok := seen[k]; ok {
				continue
			}
			seen[k] = struct{}{}
			columns = append(columns, k)
		}
	}
	return columns
}

// GetCallTransactionRequest represents the transaction request
type GetCallTransactionRequest struct {
	Param struct {
		Search struct {
			CallID string `json:"callid"`
		} `json:"search"`
		Location struct{} `json:"location"`
	} `json:"param"`
	Timestamp struct {
		From int64 `json:"from"`
		To   int64 `json:"to"`
	} `json:"timestamp"`
}

// GetCallTransaction handles POST /api/v3/call/transaction
func (h *SearchHandler) GetCallTransaction(c echo.Context) error {
	var req GetCallTransactionRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"error":   "Invalid request body",
		})
	}

	callID := req.Param.Search.CallID
	if callID == "" {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"error":   "callid is required",
		})
	}

	sid := sqlvalidator.SafeString(callID)
	sidCondition := fmt.Sprintf("session_id = '%s'", sid)
	if baseSID := stripB2BSuffix(callID); baseSID != callID {
		baseSID = sqlvalidator.SafeString(baseSID)
		sidCondition = fmt.Sprintf("(session_id = '%s' OR session_id = '%s')", sid, baseSID)
	}

	// Search across all SIP tables
	ln2 := h.flightService.LakeName()
	tables := []string{
		ln2 + ".main.hep_proto_1_call",
		ln2 + ".main.hep_proto_1_registration",
		ln2 + ".main.hep_proto_1_default",
	}

	var allResults []map[string]interface{}

	// DuckDB timestamp filter for partition pruning (same format as transactions_v4)
	tsCondition := ""
	if req.Timestamp.From > 0 && req.Timestamp.To > 0 {
		tsCondition = fmt.Sprintf(" AND timestamp >= (to_timestamp(%d / 1000.0) AT TIME ZONE 'UTC') AND timestamp <= (to_timestamp(%d / 1000.0) AT TIME ZONE 'UTC')",
			req.Timestamp.From, req.Timestamp.To)
	}

	for _, table := range tables {
		sql := fmt.Sprintf(
			"SELECT * FROM %s WHERE %s%s ORDER BY timestamp ASC",
			table, sidCondition, tsCondition,
		)

		results, err := h.flightService.QueryWithRange(c.Request().Context(), sql, msToNs(req.Timestamp.From), msToNs(req.Timestamp.To))
		if err != nil {
			continue
		}
		allResults = append(allResults, results...)
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"success": true,
		"data": map[string]interface{}{
			"total":    len(allResults),
			"messages": allResults,
			"callid":   callID,
		},
	})
}

// escapeString escapes a string for SQL.
// Deprecated: use sqlvalidator.SafeString instead.
func escapeString(s string) string {
	return sqlvalidator.SafeString(s)
}
