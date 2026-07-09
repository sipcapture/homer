// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ijt/go-anytime/v2"
	"github.com/labstack/echo/v4"
	"github.com/sipcapture/homer-core/src/coordinator/services"
	"github.com/sipcapture/homer-core/src/coordinator/sqlvalidator"
	"github.com/sipcapture/homer-core/src/pcapwriter"
	logger "github.com/sipcapture/homer-core/src/utils/logging"
)

// b2bSuffixRe matches B2BUA suffixes appended to Call-IDs by SIP proxies
// (e.g. _b2b-1, _b2b-2). RTCP/RTP packets typically use the raw Call-ID.
var b2bSuffixRe = regexp.MustCompile(`_b2b-\d+$`)

// stripB2BSuffix removes B2BUA leg suffix from a session ID so that it can
// match RTCP/RTP records which store the original Call-ID.
func stripB2BSuffix(sid string) string {
	return b2bSuffixRe.ReplaceAllString(sid, "")
}

// sqlFormMatchClause builds a dashboard/form string filter: exact equality by
// default; SQL LIKE only when rawValue contains '%' (user-supplied wildcards).
func sqlFormMatchClause(columnExpr, rawValue string) string {
	esc := sqlvalidator.SafeString(rawValue)
	if strings.Contains(rawValue, "%") {
		return fmt.Sprintf("%s LIKE '%s'", columnExpr, esc)
	}
	return fmt.Sprintf("%s = '%s'", columnExpr, esc)
}

// sqlFormMatchClauseOr applies sqlFormMatchClause across two columns (OR).
func sqlFormMatchClauseOr(leftExpr, rightExpr, rawValue string) string {
	esc := sqlvalidator.SafeString(rawValue)
	if strings.Contains(rawValue, "%") {
		return fmt.Sprintf("(%s LIKE '%s' OR %s LIKE '%s')", leftExpr, esc, rightExpr, esc)
	}
	return fmt.Sprintf("(%s = '%s' OR %s = '%s')", leftExpr, esc, rightExpr, esc)
}

// sqlFormMatchClauseAny ORs the same value across multiple column expressions.
func sqlFormMatchClauseAny(columnExprs []string, rawValue string) string {
	esc := sqlvalidator.SafeString(rawValue)
	op := "="
	if strings.Contains(rawValue, "%") {
		op = "LIKE"
	}
	parts := make([]string, len(columnExprs))
	for i, col := range columnExprs {
		parts[i] = fmt.Sprintf("%s %s '%s'", col, op, esc)
	}
	return "(" + strings.Join(parts, " OR ") + ")"
}

// sipFilterMethodValues merges filter.method (including comma-separated tokens),
// filter.methods, trims and dedupes. Matches Homer UI multi-select + custom SIP methods.
func sipFilterMethodValues(method string, methods []string) []string {
	seen := make(map[string]struct{})
	out := make([]string, 0)
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" {
			return
		}
		if _, ok := seen[s]; ok {
			return
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	for _, m := range methods {
		add(m)
	}
	if method != "" {
		for _, part := range strings.Split(method, ",") {
			add(strings.TrimSpace(part))
		}
	}
	return out
}

// sipFilterResponseValues merges filter.response_code (comma-separated allowed),
// filter.response_codes — for multi-select + custom SIP response codes.
func sipFilterResponseValues(code string, codes []string) []string {
	seen := make(map[string]struct{})
	out := make([]string, 0)
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" {
			return
		}
		if _, ok := seen[s]; ok {
			return
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	for _, c := range codes {
		add(c)
	}
	if code != "" {
		for _, part := range strings.Split(code, ",") {
			add(strings.TrimSpace(part))
		}
	}
	return out
}

type TransactionListResponseV4 struct {
	Data struct {
		Items []map[string]interface{} `json:"items"`
		Keys  []string                 `json:"keys,omitempty"`
	} `json:"data"`
	Meta Meta `json:"meta"`
}

type MessageListResponseV4 struct {
	Data struct {
		Items []map[string]interface{} `json:"items"`
	} `json:"data"`
	Meta Meta `json:"meta"`
}

type MessageResponseV4 struct {
	Data map[string]interface{} `json:"data"`
	Meta Meta                   `json:"meta"`
}

type MessageDecodedResponseV4 struct {
	Data struct {
		Data []map[string]interface{} `json:"data"`
	} `json:"data"`
	Meta Meta `json:"meta"`
}

type QosResponseV4 struct {
	Data map[string]interface{} `json:"data"`
	Meta Meta                   `json:"meta"`
}

type LogListResponseV4 struct {
	Data struct {
		Items []map[string]interface{} `json:"items"`
	} `json:"data"`
	Meta Meta `json:"meta"`
}

type SearchObjectV4 struct {
	Filter struct {
		ProtoType     int      `json:"proto_type"`
		EventType     string   `json:"event_type"`
		Method        string   `json:"method"` // legacy single value; comma-separated OK; merged with methods
		Methods       []string `json:"methods,omitempty"` // multi-select + custom methods → SQL IN (...)
		ResponseCode  string   `json:"response_code,omitempty"` // comma-separated OK; merged with response_codes
		ResponseCodes []string `json:"response_codes,omitempty"` // multi-select + custom codes → SQL IN (...)
		CallID        string   `json:"call_id"`
		SessionID     string   `json:"session_id,omitempty"` // alias for call_id (matches DuckLake column name)
		CID           string   `json:"cid,omitempty"`        // search in cid column only
		RuriUser      string   `json:"ruri_user"`
		FromUser      string   `json:"from_user"`
		Caller        string   `json:"caller,omitempty"` // alias for from_user
		ToUser        string   `json:"to_user"`
		Callee        string   `json:"callee,omitempty"` // alias for to_user
		UserAgent     string   `json:"user_agent"`
		SrcIP         string   `json:"src_ip"`
		DstIP         string   `json:"dst_ip"`
		SrcPort       int      `json:"src_port"`
		DstPort       int      `json:"dst_port"`
		CaptureID     int      `json:"capture_id"`
		Node          string   `json:"node"`
		NodeID        string   `json:"node_id,omitempty"` // alias for node
		Aor           string   `json:"aor,omitempty"`           // SIP registration column
		Contact       string   `json:"contact,omitempty"`     // SIP registration column
		Expires       string   `json:"expires,omitempty"`     // SIP registration column
		CseqMethod    string   `json:"cseq_method,omitempty"` // SIP call column cseq_method
		Payload       string   `json:"payload,omitempty"` // full-text search in payload (LOG)
		// OTLP metrics (proto_type 201) — Protocol Search form + metric-name picker.
		Name        string   `json:"name,omitempty"`         // exact metric name (preferred over call_id→name LIKE)
		Type        string   `json:"type,omitempty"`         // single OTLP metric kind (gauge, sum, …)
		Types       []string `json:"types,omitempty"`        // multi-select → IN ("type", …)
		ServiceName string   `json:"service_name,omitempty"` // LIKE on service_name (narrowing)
		// Virtual carries structured-search values for fields_mapping[].virtual (data_extra JSON, etc.).
		// Keys are field ids; only keys declared in the active mapping row are applied.
		Virtual map[string]string `json:"virtual,omitempty"`
		// VirtualAbsent lists field ids (mapping virtual.match=absent) when the UI checkbox is on.
		VirtualAbsent []string `json:"virtual_absent,omitempty"`
		// VirtualPresent lists field ids (mapping virtual.match=present) when the UI checkbox is on.
		VirtualPresent []string `json:"virtual_present,omitempty"`
	} `json:"filter"`
	Param struct {
		Limit   int    `json:"limit"`
		Select  string `json:"select,omitempty"`   // custom SELECT columns/aggregations (e.g. "method, count(*) as cnt")
		GroupBy string `json:"group_by,omitempty"` // GROUP BY clause (e.g. "method")
		OrderBy string `json:"order_by,omitempty"` // custom ORDER BY (e.g. "cnt DESC")
	} `json:"param"`
	Timestamp struct {
		From int64 `json:"from"`
		To   int64 `json:"to"`
	} `json:"timestamp"`
}

// maxTransactionSessionIDs caps OR-clauses for multi-session message queries.
const maxTransactionSessionIDs = 50

// maxOTLPTraceSpansPerQuery caps rows returned for a single trace_id via
// POST /transactions/messages on otlp_traces (no uuid / session_id column).
const maxOTLPTraceSpansPerQuery = 5000

// maxOTLPLogsPerQuery caps log rows for a single trace_id on otlp_logs.
const maxOTLPLogsPerQuery = 5000

// maxOTLPMetricsPerQuery caps metric points for a single metric name on otlp_metrics.
const maxOTLPMetricsPerQuery = 5000

// maxOTLPMetricNamesDistinct caps rows returned for POST /transactions/otlp-metric-names.
const maxOTLPMetricNamesDistinct = 2000

// normalizeTransactionSessionIDs trims, deduplicates, and caps session id list for SQL IN/OR use.
func normalizeTransactionSessionIDs(ids []string) []string {
	if len(ids) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(ids))
	out := make([]string, 0, len(ids))
	for _, raw := range ids {
		s := strings.TrimSpace(raw)
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
		if len(out) >= maxTransactionSessionIDs {
			break
		}
	}
	return out
}

func buildSessionIDOrWhere(sessionIDs []string) string {
	parts := make([]string, len(sessionIDs))
	for i, sid := range sessionIDs {
		parts[i] = fmt.Sprintf("session_id = '%s'", sqlvalidator.SafeString(sid))
	}
	return "(" + strings.Join(parts, " OR ") + ")"
}

// sessionMatchOneSQL matches one Call-ID on session_id, including B2B leg suffix stripping for RTCP/RTP correlation.
func sessionMatchOneSQL(sid string) string {
	safe := sqlvalidator.SafeString(strings.TrimSpace(sid))
	if safe == "" {
		return ""
	}
	base := stripB2BSuffix(safe)
	baseSafe := sqlvalidator.SafeString(base)
	if baseSafe != safe {
		return fmt.Sprintf("(session_id = '%s' OR session_id = '%s')", safe, baseSafe)
	}
	return fmt.Sprintf("session_id = '%s'", safe)
}

// buildSessionIDMatchOrChain ORs B2B-aware session_id clauses for multiple Call-IDs (QoS, logs, callinfo, etc.).
func buildSessionIDMatchOrChain(sessionIDs []string) string {
	return buildColumnMatchOrChain("session_id", sessionIDs)
}

// buildCallIDMatchOrChain ORs B2B-aware callid clauses for VQ-RTCPXR stats correlation.
func buildCallIDMatchOrChain(callIDs []string) string {
	return buildColumnMatchOrChain("callid", callIDs)
}

func buildColumnMatchOrChain(column string, ids []string) string {
	parts := make([]string, 0, len(ids))
	for _, sid := range ids {
		if p := columnMatchOneSQL(column, sid); p != "" {
			parts = append(parts, p)
		}
	}
	if len(parts) == 0 {
		return "FALSE"
	}
	return "(" + strings.Join(parts, " OR ") + ")"
}

func columnMatchOneSQL(column, sid string) string {
	safe := sqlvalidator.SafeString(strings.TrimSpace(sid))
	if safe == "" {
		return ""
	}
	base := stripB2BSuffix(safe)
	baseSafe := sqlvalidator.SafeString(base)
	if baseSafe != safe {
		return fmt.Sprintf("(%s = '%s' OR %s = '%s')", column, safe, column, baseSafe)
	}
	return fmt.Sprintf("%s = '%s'", column, safe)
}

// buildExactColumnMatchOrChain builds (col = 'a' OR col = 'b') with escaped
// string values. Used for OTLP trace_id (no B2B Call-ID suffix stripping).
func buildExactColumnMatchOrChain(column string, ids []string) string {
	switch column {
	case "trace_id", "name":
	default:
		return "FALSE"
	}
	parts := make([]string, 0, len(ids))
	for _, raw := range ids {
		s := strings.TrimSpace(raw)
		if s == "" {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s = '%s'", column, sqlvalidator.SafeString(s)))
	}
	if len(parts) == 0 {
		return "FALSE"
	}
	return "(" + strings.Join(parts, " OR ") + ")"
}

func isOTLPTracesDuckLakeTable(table string) bool {
	return strings.HasSuffix(strings.TrimSpace(table), "otlp_traces")
}

func isOTLPLogsDuckLakeTable(table string) bool {
	return strings.HasSuffix(strings.TrimSpace(table), "otlp_logs")
}

func isOTLPTableCorrelatedByTraceID(table string) bool {
	return isOTLPTracesDuckLakeTable(table) || isOTLPLogsDuckLakeTable(table)
}

func isOTLPMetricsDuckLakeTable(table string) bool {
	return strings.HasSuffix(strings.TrimSpace(table), "otlp_metrics")
}

// buildSubMatchOrChain matches SUB/NOTIFY traffic: (session_id OR cid) per Call-ID, B2B-aware.
func buildSubMatchOrChain(sessionIDs []string) string {
	parts := make([]string, 0, len(sessionIDs))
	for _, sid := range sessionIDs {
		safe := sqlvalidator.SafeString(strings.TrimSpace(sid))
		if safe == "" {
			continue
		}
		base := stripB2BSuffix(safe)
		baseSafe := sqlvalidator.SafeString(base)
		if baseSafe != safe {
			parts = append(parts, fmt.Sprintf("((session_id = '%s' OR session_id = '%s') OR (cid = '%s' OR cid = '%s'))", safe, baseSafe, safe, baseSafe))
		} else {
			parts = append(parts, fmt.Sprintf("(session_id = '%s' OR cid = '%s')", safe, safe))
		}
	}
	if len(parts) == 0 {
		return "FALSE"
	}
	return "(" + strings.Join(parts, " OR ") + ")"
}

// resolvedSessionIDList returns normalized ids from session_ids (priority) or a single session_id.
func resolvedSessionIDList(req *TransactionSessionRequestV4) ([]string, error) {
	multi := normalizeTransactionSessionIDs(req.SessionIDs)
	if len(multi) > 0 {
		return multi, nil
	}
	s := strings.TrimSpace(req.SessionID)
	if s == "" {
		return nil, fmt.Errorf("session_id or non-empty session_ids is required")
	}
	return []string{s}, nil
}

type TransactionSessionRequestV4 struct {
	SessionID  string   `json:"session_id"`
	SessionIDs []string `json:"session_ids,omitempty"`
	ProtoType  int      `json:"proto_type"`
	EventType  string   `json:"event_type"`
	// Whitelist lists IPs to exclude from PCAP/text export (legacy Homer 7 name; not an allow-list).
	Whitelist []string `json:"whitelist,omitempty"`
	Timestamp struct {
		From int64 `json:"from,omitempty"`
		To   int64 `json:"to,omitempty"`
	} `json:"timestamp,omitempty"`
}

// TransactionOtlpLogsRequestV4 searches otlp_logs for rows containing call_id in body / JSON blobs within an optional time window.
type TransactionOtlpLogsRequestV4 struct {
	SessionID  string   `json:"session_id"`
	SessionIDs []string `json:"session_ids,omitempty"`
	Timestamp  struct {
		From int64 `json:"from,omitempty"`
		To   int64 `json:"to,omitempty"`
	} `json:"timestamp,omitempty"`
	CallID string `json:"call_id"` // substring match; if empty, first resolved session id is used
}

// TransactionOtlpLogsTraceRequestV4 loads all otlp_logs rows for one trace_id in the optional time window (session context required).
type TransactionOtlpLogsTraceRequestV4 struct {
	SessionID  string   `json:"session_id"`
	SessionIDs []string `json:"session_ids,omitempty"`
	Timestamp  struct {
		From int64 `json:"from,omitempty"`
		To   int64 `json:"to,omitempty"`
	} `json:"timestamp,omitempty"`
	TraceID string `json:"trace_id"`
}

// TransactionOtlpMetricNamesRequestV4 lists distinct metric names in otlp_metrics for a time window.
type TransactionOtlpMetricNamesRequestV4 struct {
	Timestamp struct {
		From int64 `json:"from,omitempty"`
		To   int64 `json:"to,omitempty"`
	} `json:"timestamp"`
	ServiceName string `json:"service_name,omitempty"` // optional LIKE narrow on service_name
}

func buildOTLPMetricNamesSQL(lake string, fromMs, toMs int64, serviceFilter string) string {
	conds := []string{
		fmt.Sprintf("timestamp >= (to_timestamp(%d / 1000.0) AT TIME ZONE 'UTC')", fromMs),
		fmt.Sprintf("timestamp <= (to_timestamp(%d / 1000.0) AT TIME ZONE 'UTC')", toMs),
		"name IS NOT NULL",
		"CAST(name AS VARCHAR) != ''",
	}
	if s := strings.TrimSpace(serviceFilter); s != "" {
		conds = append(conds, fmt.Sprintf("service_name LIKE '%%%s%%'", sqlvalidator.SafeString(s)))
	}
	table := fmt.Sprintf("%s.otlp_metrics", lake)
	return fmt.Sprintf(
		"SELECT DISTINCT name FROM %s WHERE %s ORDER BY name LIMIT %d",
		table,
		strings.Join(conds, " AND "),
		maxOTLPMetricNamesDistinct,
	)
}

// MessageGetRequestV4 is the body for POST /api/v4/messages and POST /api/v4/messages/decoded.
type MessageGetRequestV4 struct {
	UUID      string `json:"uuid"`
	ProtoType int    `json:"proto_type"`
	EventType string `json:"event_type"`
	Timestamp struct {
		From int64 `json:"from,omitempty"`
		To   int64 `json:"to,omitempty"`
	} `json:"timestamp,omitempty"`
}

// RawQueryRequest is the body for POST /api/v4/query (raw SQL passthrough).
type RawQueryRequest struct {
	SQL   string `json:"sql"`
	Limit int    `json:"limit,omitempty"` // safety cap, default 1000
}

// MCPQueryRequest is the body for POST /api/v4/mcp/query.
// It converts natural language query into structured or SQL execution.
type MCPQueryRequest struct {
	QueryText string `json:"query_text"`
	Mode      string `json:"mode,omitempty"`   // auto|structured|sql
	Parser    string `json:"parser,omitempty"` // auto|llm|regex (default: auto)
	Limit     int    `json:"limit,omitempty"`
	Timestamp struct {
		From int64 `json:"from,omitempty"`
		To   int64 `json:"to,omitempty"`
	} `json:"timestamp,omitempty"`
	NowUTCUnixMS int64 `json:"now_utc_unix_ms,omitempty"`
}

// normalizeParserHint sanitizes the user-supplied parser strategy. Unknown or
// empty values map to "auto" so old clients keep their previous behavior.
func normalizeParserHint(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "llm":
		return "llm"
	case "regex", "rule", "rules":
		return "regex"
	default:
		return "auto"
	}
}

func (h *SearchHandler) V4TransactionsList(c echo.Context) error {
	req := SimpleSearchRequest{
		TransactionType: c.QueryParam("filter[event_type]"),
		CallID:          c.QueryParam("filter[transaction_id]"),
		SrcIP:           c.QueryParam("filter[src_ip]"),
		DstIP:           c.QueryParam("filter[dst_ip]"),
		Method:          c.QueryParam("filter[method]"),
		NodeID:          c.QueryParam("filter[node]"),
	}

	if proto := c.QueryParam("filter[protocol]"); proto != "" {
		if v, err := strconv.Atoi(proto); err == nil {
			req.ProtoType = v
		}
	}
	req.From = c.QueryParam("from")
	req.To = c.QueryParam("to")

	limit := 0
	if limitStr := c.QueryParam("page[limit]"); limitStr != "" {
		if v, err := strconv.Atoi(limitStr); err == nil {
			limit = v
		}
	}
	req.Limit = limit

	sql, err := h.buildSimpleSearchSQL(&req)
	if err != nil {
		return writeError(c, http.StatusBadRequest, "Bad Request", err.Error())
	}
	results, err := h.flightService.Query(c.Request().Context(), sql)
	if err != nil {
		return writeError(c, http.StatusInternalServerError, "Server Error", "Query failed")
	}

	h.enrichRowsWithIPAliases(c.Request().Context(), results)

	resp := TransactionListResponseV4{}
	resp.Data.Items = results
	resp.Data.Keys = getColumns(results)
	resp.Meta = buildMeta(c, "")
	resp.Meta.Pagination = &Pagination{Limit: req.Limit, Total: len(results)}
	return c.JSON(http.StatusOK, resp)
}

// transactionSearchChunkMs is the slice size for time-sliced transaction
// searches. Long bounded ranges are executed newest-first in chunks of this
// size and stop as soon as the row limit is reached, so non-prunable filters
// (e.g. session_id LIKE '%…%') over a week of data do not have to scan the
// whole range in a single query that then hits the coordinator query timeout
// (sipcapture/homer#785).
const transactionSearchChunkMs = int64(24 * time.Hour / time.Millisecond)

// searchTimeChunk is one newest-first slice of a chunked transaction search.
// ToInclusive is true only for the newest chunk: interior chunks use an
// exclusive upper bound so rows with sub-millisecond timestamps on a chunk
// boundary are neither lost nor duplicated.
type searchTimeChunk struct {
	FromMs      int64
	ToMs        int64
	ToInclusive bool
}

// splitSearchTimeRange slices [fromMs, toMs] into chunks of at most chunkMs,
// ordered newest-first. Unbounded or small ranges yield a single chunk.
func splitSearchTimeRange(fromMs, toMs, chunkMs int64) []searchTimeChunk {
	if fromMs <= 0 || toMs <= 0 || toMs <= fromMs || chunkMs <= 0 || toMs-fromMs <= chunkMs {
		return []searchTimeChunk{{FromMs: fromMs, ToMs: toMs, ToInclusive: true}}
	}
	chunks := make([]searchTimeChunk, 0, (toMs-fromMs)/chunkMs+1)
	hi := toMs
	inclusive := true
	for hi > fromMs {
		lo := hi - chunkMs
		if lo < fromMs {
			lo = fromMs
		}
		chunks = append(chunks, searchTimeChunk{FromMs: lo, ToMs: hi, ToInclusive: inclusive})
		hi = lo
		inclusive = false
	}
	return chunks
}

// transactionSearchChunkable reports whether a search request can be executed
// as independent newest-first time slices with early exit. Aggregations,
// custom projections and non-default ordering must see the whole range in a
// single query, so they are excluded.
func transactionSearchChunkable(req *SearchObjectV4) bool {
	if req.Timestamp.From <= 0 || req.Timestamp.To <= 0 {
		return false
	}
	if req.Timestamp.To-req.Timestamp.From <= transactionSearchChunkMs {
		return false
	}
	if strings.TrimSpace(req.Param.Select) != "" || strings.TrimSpace(req.Param.GroupBy) != "" {
		return false
	}
	orderBy := strings.TrimSpace(req.Param.OrderBy)
	return orderBy == "" ||
		strings.EqualFold(orderBy, "timestamp desc") ||
		strings.EqualFold(orderBy, "time desc")
}

// Lake top-N execution strategies (mirrors storage.ducklake.search.lake_topn_strategy).
const (
	topNStream  = "stream"  // drop ORDER BY, single streaming LIMIT, Go-side sort
	topNChunked = "chunked" // newest-first time slices with early exit
	topNFull    = "full"    // single ORDER BY query over the whole range
)

// rowTimestampSortKey extracts a comparable newest-first key from a result
// row's "timestamp" field, tolerating the representations FlightSQL/Arrow can
// hand back (time.Time, string, []byte, epoch numbers).
func rowTimestampSortKey(m map[string]interface{}) int64 {
	v, ok := m["timestamp"]
	if !ok || v == nil {
		return 0
	}
	switch t := v.(type) {
	case time.Time:
		return t.UnixNano()
	case string:
		return parseTimestampKey(t)
	case []byte:
		return parseTimestampKey(string(t))
	case int64:
		return t
	case int:
		return int64(t)
	case float64:
		return int64(t)
	}
	return 0
}

func parseTimestampKey(s string) int64 {
	for _, layout := range []string{
		"2006-01-02 15:04:05.999999999",
		"2006-01-02 15:04:05",
		time.RFC3339Nano,
		time.RFC3339,
	} {
		if ts, err := time.Parse(layout, s); err == nil {
			return ts.UnixNano()
		}
	}
	return 0
}

// sortRowsByTimestampDescLimit orders rows newest-first and trims to limit.
// Used to restore ordering after the "stream" strategy drops ORDER BY at the
// database to keep memory flat.
func sortRowsByTimestampDescLimit(rows []map[string]interface{}, limit int) []map[string]interface{} {
	sort.SliceStable(rows, func(i, j int) bool {
		return rowTimestampSortKey(rows[i]) > rowTimestampSortKey(rows[j])
	})
	if limit > 0 && len(rows) > limit {
		rows = rows[:limit]
	}
	return rows
}

// lakeTopNStrategy returns the configured strategy for long-range timestamp-DESC
// top-N searches, defaulting to "stream".
func (h *SearchHandler) lakeTopNStrategy() string {
	switch h.lakeTopNStrategyCfg {
	case topNChunked:
		return topNChunked
	case topNFull:
		return topNFull
	default:
		return topNStream
	}
}

// outerChunkMs is the coordinator's OUTER window width for the chunked strategy,
// defaulting to 24h (transactionSearchChunkMs). The node sub-slices each window
// to 1h for memory safety, so this only governs coordinator round-trips.
func (h *SearchHandler) outerChunkMs() int64 {
	if h.lakeChunkSec <= 0 {
		return transactionSearchChunkMs
	}
	return int64(h.lakeChunkSec) * 1000
}

// transactionSearchTopNShape reports whether the request is a plain
// timestamp-DESC top-N (no aggregation, custom projection or non-default
// ordering) — the only shape for which the stream/chunked strategies are
// correct. Unlike transactionSearchChunkable it does not require the range to
// exceed a chunk, because a streaming LIMIT helps at any range.
func transactionSearchTopNShape(req *SearchObjectV4) bool {
	if strings.TrimSpace(req.Param.Select) != "" || strings.TrimSpace(req.Param.GroupBy) != "" {
		return false
	}
	orderBy := strings.TrimSpace(req.Param.OrderBy)
	return orderBy == "" ||
		strings.EqualFold(orderBy, "timestamp desc") ||
		strings.EqualFold(orderBy, "time desc")
}

// queryTransactionSearch executes a v4 transaction search. fullSQL is the
// already-validated query for the whole time range. The lake_topn_strategy
// config selects how a long-range timestamp-DESC top-N is run:
//   - stream (default): one query over the WHOLE range with ORDER BY dropped
//     (the DB streams a flat LIMIT and stops after N rows); the rows are
//     re-sorted newest-first in Go. No time-slicing — fastest, simplest.
//   - chunked: newest-first 24h time slices, each ORDER BY timestamp DESC, with
//     early exit; the node further sub-splits each window to 1h for memory.
//   - full: a single ORDER BY query over the whole range.
func (h *SearchHandler) queryTransactionSearch(ctx context.Context, fullSQL string, req *SearchObjectV4, virtualRules map[string]services.VirtualFieldRule) ([]map[string]interface{}, error) {
	strategy := h.lakeTopNStrategy()

	// lazy: phase 1 is SELECT uuid, timestamp; full rows loaded by uuid after.
	lazy := h.lazyPayloadEligible(req)

	rows, err := h.runTransactionSearch(ctx, fullSQL, req, virtualRules, strategy, lazy)
	if err != nil {
		return nil, err
	}
	if lazy {
		rows = h.hydrateHeavyColumns(ctx, req, rows)
	}
	return rows, nil
}

// runTransactionSearch executes the search/sort phase for the configured
// strategy. When narrow is true the wide blob columns are dropped from the
// projection (the caller re-attaches them by uuid).
func (h *SearchHandler) runTransactionSearch(ctx context.Context, fullSQL string, req *SearchObjectV4, virtualRules map[string]services.VirtualFieldRule, strategy string, narrow bool) ([]map[string]interface{}, error) {
	// full, or shapes we cannot safely rewrite (aggregations, custom projection
	// or non-default ordering must see the whole range at once): single query.
	if strategy == topNFull || !transactionSearchTopNShape(req) {
		if !narrow {
			return h.flightService.Query(ctx, fullSQL)
		}
		sql, err := buildSearchSQLV4WithOpts(h.flightService.LakeName(), req, virtualRules, searchSQLOpts{narrowNoHeavy: true})
		if err != nil {
			return nil, err
		}
		return h.flightService.Query(ctx, sql)
	}

	// stream: a single query over the whole range, ORDER BY dropped, re-sorted
	// in Go. No chunking.
	if strategy == topNStream {
		limit := effectiveSearchLimit(req.Param.Limit)
		streamSQL, err := buildSearchSQLV4WithOpts(h.flightService.LakeName(), req, virtualRules, searchSQLOpts{noOrderBy: true, narrowNoHeavy: narrow})
		if err != nil {
			return nil, err
		}
		logger.Info("V4TransactionsSearch: stream execution (single query)", "limit", limit,
			"narrow", narrow, "from", req.Timestamp.From, "to", req.Timestamp.To)
		rows, err := h.flightService.Query(ctx, streamSQL)
		if err != nil {
			return nil, err
		}
		return sortRowsByTimestampDescLimit(rows, limit), nil
	}

	// chunked: newest-first time slices with early exit. Each window keeps
	// ORDER BY timestamp DESC, so the node sub-splits it into 1h windows for
	// memory; the concatenation is already globally newest-first.
	chunks := splitSearchTimeRange(req.Timestamp.From, req.Timestamp.To, h.outerChunkMs())
	limit := effectiveSearchLimit(req.Param.Limit)
	logger.Info("V4TransactionsSearch: chunked execution",
		"strategy", strategy, "chunks", len(chunks), "limit", limit, "narrow", narrow,
		"from", req.Timestamp.From, "to", req.Timestamp.To)

	results := make([]map[string]interface{}, 0, limit)
	for i, chunk := range chunks {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		chunkReq := *req
		chunkReq.Timestamp.From = chunk.FromMs
		chunkReq.Timestamp.To = chunk.ToMs
		sql, err := buildSearchSQLV4WithOpts(h.flightService.LakeName(), &chunkReq, virtualRules,
			searchSQLOpts{toExclusive: !chunk.ToInclusive, narrowNoHeavy: narrow})
		if err != nil {
			return nil, err
		}
		rows, err := h.flightService.Query(ctx, sql)
		if err != nil {
			return nil, err
		}
		results = append(results, rows...)
		if len(results) >= limit {
			logger.Info("V4TransactionsSearch: chunked early exit",
				"chunks_done", i+1, "chunks_total", len(chunks), "rows", len(results))
			break
		}
	}
	if len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}

// lazyPayloadEligible reports whether a request can use the two-phase
// (narrow search + by-uuid hydration) path. It requires the default full-row
// projection (no custom select / group by) on a HEP proto table — OTLP and LP
// tables have a different column layout with no uuid / payload pair.
func (h *SearchHandler) lazyPayloadEligible(req *SearchObjectV4) bool {
	if !h.lazyPayloadHydration {
		return false
	}
	if strings.TrimSpace(req.Param.Select) != "" || strings.TrimSpace(req.Param.GroupBy) != "" {
		return false
	}
	protoType := req.Filter.ProtoType
	if protoType == 0 {
		protoType = 1
	}
	if isOTLPProtoType(protoType) || isLPProtoType(protoType) {
		return false
	}
	return true
}

// hydrateHeavyColumns loads full rows for a uuid+timestamp-only search phase.
// Phase 1 filters/sorts/LIMITs over SELECT uuid, timestamp; this phase issues a
// bounded point-lookup (SELECT * … WHERE uuid IN (…) AND timestamp range) so
// wide columns (payload, data_extra, caller, …) are decompressed only for the
// <=LIMIT rows returned. Any failure degrades gracefully: narrow rows are kept.
func (h *SearchHandler) hydrateHeavyColumns(ctx context.Context, req *SearchObjectV4, rows []map[string]interface{}) []map[string]interface{} {
	if len(rows) == 0 {
		return rows
	}

	uuids := make([]string, 0, len(rows))
	seen := make(map[string]struct{}, len(rows))
	var minMs, maxMs int64
	for _, r := range rows {
		u := rowStringValue(r["uuid"])
		if u == "" {
			continue
		}
		if _, dup := seen[u]; !dup {
			seen[u] = struct{}{}
			uuids = append(uuids, u)
		}
		if ns := rowTimestampSortKey(r); ns > 0 {
			ms := ns / int64(time.Millisecond)
			if minMs == 0 || ms < minMs {
				minMs = ms
			}
			if ms > maxMs {
				maxMs = ms
			}
		}
	}
	if len(uuids) == 0 {
		// No uuid column (unexpected for HEP tables) — nothing to hydrate.
		return rows
	}

	// Bound the lookup by the timestamp span of the matched rows (padded for
	// sub-ms rounding), falling back to the request range. The timestamp
	// predicate is REQUIRED for memory safety: DuckDB pushes a timestamp range
	// down into the Parquet scan and late-materializes payload only for the
	// surviving rows, but a bare "uuid IN (…)" (uuid has no useful min/max
	// stats) forces a full eager decompression of the wide column and OOMs.
	// If we cannot establish any bound, skip hydration and return narrow rows.
	const padMs = int64(60 * 1000)
	fromMs, toMs := minMs-padMs, maxMs+padMs
	if minMs <= 0 || maxMs <= 0 {
		fromMs, toMs = req.Timestamp.From, req.Timestamp.To
	}
	if fromMs <= 0 && toMs <= 0 {
		logger.Warn("V4TransactionsSearch: no timestamp bound for payload hydration, returning narrow rows")
		return rows
	}

	protoType := req.Filter.ProtoType
	if protoType == 0 {
		protoType = 1
	}
	tableName := getTableName(h.flightService.LakeName(), protoType, req.Filter.EventType)

	conditions := timestampRangeConditions("timestamp", fromMs, toMs, false)
	escaped := make([]string, len(uuids))
	for i, u := range uuids {
		escaped[i] = "'" + sqlvalidator.SafeString(u) + "'"
	}
	conditions = append(conditions, "uuid IN ("+strings.Join(escaped, ",")+")")
	sql := fmt.Sprintf("SELECT * FROM %s WHERE %s", tableName, strings.Join(conditions, " AND "))

	logger.Info("V4TransactionsSearch: hydrating full rows by uuid", "uuids", len(uuids),
		"from", fromMs, "to", toMs)
	fullRows, err := h.flightService.Query(ctx, sql)
	if err != nil {
		logger.Warn("V4TransactionsSearch: row hydration failed, returning narrow rows",
			"error", err)
		return rows
	}

	byUUID := make(map[string]map[string]interface{}, len(fullRows))
	for _, r := range fullRows {
		if u := rowStringValue(r["uuid"]); u != "" {
			byUUID[u] = r
		}
	}

	out := make([]map[string]interface{}, 0, len(rows))
	for _, narrow := range rows {
		u := rowStringValue(narrow["uuid"])
		if full, ok := byUUID[u]; ok {
			out = append(out, full)
			continue
		}
		out = append(out, narrow)
	}
	return out
}

// rowStringValue coerces a result-set cell to a string, handling the string /
// []byte shapes the Flight result decoder may produce.
func rowStringValue(v interface{}) string {
	switch s := v.(type) {
	case string:
		return s
	case []byte:
		return string(s)
	}
	return ""
}

func (h *SearchHandler) V4TransactionsSearch(c echo.Context) error {
	var req SearchObjectV4
	if err := c.Bind(&req); err != nil {
		return writeError(c, http.StatusBadRequest, "Bad Request", "Invalid request body")
	}

	virtualRules := h.loadVirtualRulesForReq(c.Request().Context(), &req)
	sql, err := buildSearchSQLV4(h.flightService.LakeName(), &req, virtualRules)
	if err != nil {
		logger.Error(fmt.Sprintf("V4TransactionsSearch: SQL validation failed: %v", err))
		return writeError(c, http.StatusBadRequest, "Bad Request", fmt.Sprintf("SQL validation failed: %v", err))
	}
	logger.Info("V4TransactionsSearch", "proto", req.Filter.ProtoType, "event", req.Filter.EventType, "sql", sql)
	results, err := h.queryTransactionSearch(c.Request().Context(), sql, &req, virtualRules)
	if err != nil {
		logger.Error(fmt.Sprintf("V4TransactionsSearch: query error: %v", err))
		return writeError(c, http.StatusInternalServerError, "Server Error", "Query failed")
	}
	logger.Info("V4TransactionsSearch: got results", "count", len(results))

	h.enrichRowsWithIPAliases(c.Request().Context(), results)

	resp := TransactionListResponseV4{}
	resp.Data.Items = results
	resp.Data.Keys = getColumns(results)
	resp.Meta = buildMeta(c, "")
	resp.Meta.Pagination = &Pagination{Limit: req.Param.Limit, Total: len(results)}
	if req.Timestamp.From > 0 && req.Timestamp.To > 0 {
		resp.Meta.TimeRange = &TimeRangeMeta{From: req.Timestamp.From, To: req.Timestamp.To}
	}
	return c.JSON(http.StatusOK, resp)
}

// queryTransactionMessages loads SIP (or other proto) rows for a transaction session request (same rules as POST /transactions/messages).
//
// When a Lua correlation script is registered for (proto_type, event_type) the
// function runs two phases:
//  1. Query the base session_id set (as before).
//  2. Pass the base rows into the script; if the script returns extra
//     session_ids, reissue the query on the expanded set and return the
//     merged result. Any script/SQL failure is non-fatal — the handler falls
//     back to the base rows and logs a warning.
func (h *SearchHandler) queryTransactionMessages(ctx context.Context, req *TransactionSessionRequestV4) ([]map[string]interface{}, error) {
	multi := normalizeTransactionSessionIDs(req.SessionIDs)
	if len(multi) == 0 {
		if strings.TrimSpace(req.SessionID) == "" {
			return nil, fmt.Errorf("session_id or non-empty session_ids is required")
		}
		multi = []string{strings.TrimSpace(req.SessionID)}
	}

	protoType := req.ProtoType
	if protoType == 0 {
		protoType = 1
	}
	eventType := req.EventType
	if eventType == "" {
		eventType = "call"
	}
	table := getTableName(h.flightService.LakeName(), protoType, eventType)

	baseRows, err := h.executeTransactionMessagesSQL(ctx, table, multi, req.Timestamp.From, req.Timestamp.To)
	if err != nil {
		return nil, err
	}
	sortTransactionMessageRowsByTimestampAsc(baseRows)

	if h.correlation == nil || !h.correlation.Has(protoType, eventType) {
		return baseRows, nil
	}

	corrRes := h.correlation.Correlate(ctx, CorrelationInput{
		HepID:      protoType,
		Profile:    eventType,
		ProtoType:  protoType,
		EventType:  eventType,
		BaseRows:   baseRows,
		SessionIDs: multi,
		TimeFrom:   req.Timestamp.From,
		TimeTo:     req.Timestamp.To,
	})
	if corrRes == nil || len(corrRes.ExtraSessionIDs) == 0 {
		return baseRows, nil
	}

	expanded := mergeSessionIDs(multi, corrRes.ExtraSessionIDs)
	if len(expanded) == len(multi) {
		return baseRows, nil
	}

	expandedRows, err := h.executeTransactionMessagesSQL(ctx, table, expanded, req.Timestamp.From, req.Timestamp.To)
	if err != nil {
		// Fail-open: log and return base rows so a broken correlation path
		// never manifests as a user-visible 500.
		logger.Warn("V4TransactionMessages: correlated requery failed, returning base rows",
			"err", err.Error(), "proto", protoType, "event", eventType)
		return baseRows, nil
	}
	sortTransactionMessageRowsByTimestampAsc(expandedRows)
	return expandedRows, nil
}

// transactionMessagesSelectSQL builds the SELECT used by POST /transactions/messages.
// Package tests assert OTLP trace/log tables filter on trace_id and apply a row cap.
func transactionMessagesSelectSQL(table string, sessionIDs []string, from, to int64) string {
	var where string
	switch {
	case isOTLPTableCorrelatedByTraceID(table):
		where = buildExactColumnMatchOrChain("trace_id", sessionIDs)
	case isOTLPMetricsDuckLakeTable(table):
		where = buildExactColumnMatchOrChain("name", sessionIDs)
	default:
		where = buildSessionIDMatchOrChain(sessionIDs)
	}
	if from > 0 && to > 0 {
		where += fmt.Sprintf(" AND timestamp >= (to_timestamp(%d / 1000.0) AT TIME ZONE 'UTC') AND timestamp <= (to_timestamp(%d / 1000.0) AT TIME ZONE 'UTC')",
			from, to)
	}
	limit := ""
	switch {
	case isOTLPTracesDuckLakeTable(table):
		limit = fmt.Sprintf(" LIMIT %d", maxOTLPTraceSpansPerQuery)
	case isOTLPLogsDuckLakeTable(table):
		limit = fmt.Sprintf(" LIMIT %d", maxOTLPLogsPerQuery)
	case isOTLPMetricsDuckLakeTable(table):
		limit = fmt.Sprintf(" LIMIT %d", maxOTLPMetricsPerQuery)
	}
	return fmt.Sprintf("SELECT * FROM %s WHERE %s ORDER BY timestamp ASC NULLS LAST%s", table, where, limit)
}

// executeTransactionMessagesSQL builds and runs the B2B-aware SELECT for a
// given session_id list. Broken out of queryTransactionMessages so the
// correlation second pass can reuse it without recursion.
func (h *SearchHandler) executeTransactionMessagesSQL(ctx context.Context, table string, sessionIDs []string, from, to int64) ([]map[string]interface{}, error) {
	if len(sessionIDs) == 0 {
		return nil, fmt.Errorf("no session_id provided")
	}
	sql := transactionMessagesSelectSQL(table, sessionIDs, from, to)
	return h.flightService.Query(ctx, sql)
}

// mergeSessionIDs returns base ∪ extras with order preserved, whitespace
// trimmed and empty strings dropped. The returned slice is the cap-bounded
// input to the expanded SELECT (capped by buildSessionIDMatchOrChain).
func mergeSessionIDs(base, extras []string) []string {
	seen := make(map[string]struct{}, len(base)+len(extras))
	out := make([]string, 0, len(base)+len(extras))
	for _, s := range base {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	for _, s := range extras {
		if len(out) >= maxTransactionSessionIDs {
			break
		}
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

// sortTransactionMessageRowsByTimestampAsc orders messages for transaction view / API:
// oldest capture time first (ASC), null or unparseable timestamps last, stable tie-break on ids.
func sortTransactionMessageRowsByTimestampAsc(rows []map[string]interface{}) {
	if len(rows) < 2 {
		return
	}
	sort.SliceStable(rows, func(i, j int) bool {
		ni := transactionRowSortNanos(rows[i])
		nj := transactionRowSortNanos(rows[j])
		if ni != nj {
			return ni < nj
		}
		return transactionRowSortTie(rows[i]) < transactionRowSortTie(rows[j])
	})
}

func transactionRowSortNanos(row map[string]interface{}) int64 {
	for _, key := range []string{"timestamp", "time"} {
		if t, ok := pcapwriter.RowTimeOptional(row, key); ok {
			return t.UnixNano()
		}
	}
	return math.MaxInt64
}

func transactionRowSortTie(row map[string]interface{}) string {
	for _, k := range []string{"uuid", "span_id", "trace_id", "name"} {
		if v, ok := row[k]; ok && v != nil {
			s := strings.TrimSpace(fmt.Sprint(v))
			if s != "" {
				return s
			}
		}
	}
	return ""
}

func (h *SearchHandler) V4TransactionMessages(c echo.Context) error {
	var req TransactionSessionRequestV4
	if err := c.Bind(&req); err != nil {
		return writeError(c, http.StatusBadRequest, "Bad Request", "Invalid request body")
	}
	results, err := h.queryTransactionMessages(c.Request().Context(), &req)
	if err != nil {
		if strings.Contains(err.Error(), "session_id") {
			return writeError(c, http.StatusBadRequest, "Bad Request", err.Error())
		}
		return writeError(c, http.StatusInternalServerError, "Server Error", "Query failed")
	}

	h.enrichRowsWithIPAliases(c.Request().Context(), results)

	resp := MessageListResponseV4{}
	resp.Data.Items = results
	resp.Meta = buildMeta(c, "")
	return c.JSON(http.StatusOK, resp)
}

func (h *SearchHandler) V4MessageGet(c echo.Context) error {
	var req MessageGetRequestV4
	if err := c.Bind(&req); err != nil {
		return writeError(c, http.StatusBadRequest, "Bad Request", "Invalid request body")
	}
	if strings.TrimSpace(req.UUID) == "" {
		return writeError(c, http.StatusBadRequest, "Bad Request", "uuid is required")
	}

	protoType := req.ProtoType
	if protoType == 0 {
		protoType = 1
	}
	eventType := req.EventType
	if eventType == "" {
		eventType = "call"
	}
	table := getTableName(h.flightService.LakeName(), protoType, eventType)
	where := fmt.Sprintf("uuid = '%s'", sqlvalidator.SafeString(req.UUID))
	if req.Timestamp.From > 0 && req.Timestamp.To > 0 {
		where += fmt.Sprintf(" AND timestamp >= (to_timestamp(%d / 1000.0) AT TIME ZONE 'UTC') AND timestamp <= (to_timestamp(%d / 1000.0) AT TIME ZONE 'UTC')",
			req.Timestamp.From, req.Timestamp.To)
	}
	sql := fmt.Sprintf("SELECT * FROM %s WHERE %s LIMIT 1", table, where)
	results, err := h.flightService.Query(c.Request().Context(), sql)
	if err != nil {
		return writeError(c, http.StatusInternalServerError, "Server Error", "Query failed")
	}
	if len(results) == 0 {
		return writeError(c, http.StatusNotFound, "Not Found", "Message not found")
	}
	h.enrichRowsWithIPAliases(c.Request().Context(), results)
	resp := MessageResponseV4{
		Data: results[0],
		Meta: buildMeta(c, ""),
	}
	return c.JSON(http.StatusOK, resp)
}

func (h *SearchHandler) V4MessageDecoded(c echo.Context) error {
	var req MessageGetRequestV4
	if err := c.Bind(&req); err != nil {
		return writeError(c, http.StatusBadRequest, "Bad Request", "Invalid request body")
	}
	if strings.TrimSpace(req.UUID) == "" {
		return writeError(c, http.StatusBadRequest, "Bad Request", "uuid is required")
	}

	protoType := req.ProtoType
	if protoType == 0 {
		protoType = 1
	}
	eventType := req.EventType
	if eventType == "" {
		eventType = "call"
	}
	table := getTableName(h.flightService.LakeName(), protoType, eventType)
	where := fmt.Sprintf("uuid = '%s'", sqlvalidator.SafeString(req.UUID))
	if req.Timestamp.From > 0 && req.Timestamp.To > 0 {
		where += fmt.Sprintf(" AND timestamp >= (to_timestamp(%d / 1000.0) AT TIME ZONE 'UTC') AND timestamp <= (to_timestamp(%d / 1000.0) AT TIME ZONE 'UTC')",
			req.Timestamp.From, req.Timestamp.To)
	}
	sql := fmt.Sprintf("SELECT * FROM %s WHERE %s LIMIT 1", table, where)
	results, err := h.flightService.Query(c.Request().Context(), sql)
	if err != nil {
		return writeError(c, http.StatusInternalServerError, "Server Error", "Query failed")
	}
	decoded := make([]map[string]interface{}, 0)
	if len(results) > 0 {
		decoded = append(decoded, results[0])
	}
	h.enrichRowsWithIPAliases(c.Request().Context(), decoded)
	resp := MessageDecodedResponseV4{}
	resp.Data.Data = decoded
	resp.Meta = buildMeta(c, "")
	return c.JSON(http.StatusOK, resp)
}

func (h *SearchHandler) V4TransactionQos(c echo.Context) error {
	var req TransactionSessionRequestV4
	if err := c.Bind(&req); err != nil {
		return writeError(c, http.StatusBadRequest, "Bad Request", "Invalid request body")
	}
	ids, err := resolvedSessionIDList(&req)
	if err != nil {
		return writeError(c, http.StatusBadRequest, "Bad Request", err.Error())
	}
	sidCondition := buildSessionIDMatchOrChain(ids)

	tsCondition := ""
	if req.Timestamp.From > 0 && req.Timestamp.To > 0 {
		tsCondition = fmt.Sprintf(" AND timestamp >= (to_timestamp(%d / 1000.0) AT TIME ZONE 'UTC') AND timestamp <= (to_timestamp(%d / 1000.0) AT TIME ZONE 'UTC')",
			req.Timestamp.From, req.Timestamp.To)
	}

	rtcpTable := getTableName(h.flightService.LakeName(), 5, "default")
	rtcpSQL := fmt.Sprintf(
		"SELECT * FROM %s WHERE %s%s ORDER BY timestamp ASC",
		rtcpTable, sidCondition, tsCondition,
	)
	rtcpResults, rtcpErr := h.flightService.Query(c.Request().Context(), rtcpSQL)
	if rtcpErr != nil {
		rtcpResults = []map[string]interface{}{}
	}
	h.enrichRowsWithIPAliases(c.Request().Context(), rtcpResults)

	rtpTable := getTableName(h.flightService.LakeName(), 35, "default")
	rtpSQL := fmt.Sprintf(
		"SELECT * FROM %s WHERE %s%s ORDER BY timestamp ASC",
		rtpTable, sidCondition, tsCondition,
	)
	rtpResults, rtpErr := h.flightService.Query(c.Request().Context(), rtpSQL)
	if rtpErr != nil {
		rtpResults = []map[string]interface{}{}
	}
	h.enrichRowsWithIPAliases(c.Request().Context(), rtpResults)

	vqrtcpTable := fmt.Sprintf("%s.vqrtcpxr_stats", h.flightService.LakeName())
	callidCondition := buildCallIDMatchOrChain(ids)
	vqrtcpSQL := fmt.Sprintf(
		"SELECT * FROM %s WHERE %s%s ORDER BY timestamp ASC",
		vqrtcpTable, callidCondition, tsCondition,
	)
	vqrtcpResults, vqrtcpErr := h.flightService.Query(c.Request().Context(), vqrtcpSQL)
	if vqrtcpErr != nil {
		vqrtcpResults = []map[string]interface{}{}
	}
	h.enrichRowsWithIPAliases(c.Request().Context(), vqrtcpResults)

	resp := QosResponseV4{
		Data: map[string]interface{}{
			"rtcp":   map[string]interface{}{"data": rtcpResults},
			"rtp":    map[string]interface{}{"data": rtpResults},
			"vqrtcp": map[string]interface{}{"data": vqrtcpResults},
		},
		Meta: buildMeta(c, ""),
	}
	return c.JSON(http.StatusOK, resp)
}

// V4TransactionCallInfo returns an aggregated summary for a call session.
// POST /api/v4/transactions/callinfo
func (h *SearchHandler) V4TransactionCallInfo(c echo.Context) error {
	var req TransactionSessionRequestV4
	if err := c.Bind(&req); err != nil {
		return writeError(c, http.StatusBadRequest, "Bad Request", "Invalid request body")
	}
	ids, err := resolvedSessionIDList(&req)
	if err != nil {
		return writeError(c, http.StatusBadRequest, "Bad Request", err.Error())
	}

	protoType := req.ProtoType
	if protoType == 0 {
		protoType = 1
	}
	eventType := req.EventType
	if eventType == "" {
		eventType = "call"
	}
	sipEvent := strings.ToLower(strings.TrimSpace(eventType))
	switch sipEvent {
	case "calls":
		sipEvent = "call"
	case "registrations", "register":
		sipEvent = "registration"
	}

	table := getTableName(h.flightService.LakeName(), protoType, eventType)
	sessionWhere := buildSessionIDMatchOrChain(ids)

	tsFilter := ""
	if req.Timestamp.From > 0 && req.Timestamp.To > 0 {
		tsFilter = fmt.Sprintf(
			" AND timestamp >= (to_timestamp(%d / 1000.0) AT TIME ZONE 'UTC') AND timestamp <= (to_timestamp(%d / 1000.0) AT TIME ZONE 'UTC')",
			req.Timestamp.From, req.Timestamp.To,
		)
	}

	var results []map[string]interface{}
	if protoType == 1 && sipEvent == "call" {
		detailSQL := fmt.Sprintf(
			`SELECT session_id, timestamp, method, response_code, cseq_method, caller, callee,
				src_ip, CAST(src_port AS VARCHAR) AS src_port, dst_ip, CAST(dst_port AS VARCHAR) AS dst_port,
				payload, data_extra, uuid
			FROM %s WHERE %s%s ORDER BY timestamp ASC LIMIT %d`,
			table, sessionWhere, tsFilter, callInfoMaxRows,
		)
		rows, err := h.flightService.Query(c.Request().Context(), detailSQL)
		if err != nil {
			return writeError(c, http.StatusInternalServerError, "Server Error", "Query failed")
		}
		results = computeSIPCallInfoRows(rows)
		if results == nil {
			results = []map[string]interface{}{}
		}
	} else {
		sql := fmt.Sprintf(`SELECT
		session_id,
		MAX(caller) AS caller,
		MAX(callee) AS callee,
		CAST(MIN(timestamp) AS VARCHAR) AS first_seen,
		CAST(MAX(timestamp) AS VARCHAR) AS last_seen,
		date_diff('second', MIN(timestamp), MAX(timestamp)) AS duration_sec,
		COUNT(*) AS message_count,
		string_agg(DISTINCT method, ', ' ORDER BY method) AS methods,
		string_agg(DISTINCT CAST(response_code AS VARCHAR), ', ' ORDER BY response_code)
			FILTER (WHERE response_code > 0) AS response_codes,
		string_agg(DISTINCT src_ip, ', ') AS src_ips,
		string_agg(DISTINCT dst_ip, ', ') AS dst_ips,
		string_agg(DISTINCT CAST(node_id AS VARCHAR), ', ') AS nodes
	FROM %s WHERE %s%s GROUP BY session_id`,
			table, sessionWhere, tsFilter)
		var err error
		results, err = h.flightService.Query(c.Request().Context(), sql)
		if err != nil {
			return writeError(c, http.StatusInternalServerError, "Server Error", "Query failed")
		}
	}

	resp := LogListResponseV4{}
	resp.Data.Items = results
	resp.Meta = buildMeta(c, "")
	return c.JSON(http.StatusOK, resp)
}

// V4TransactionEvents returns application log rows (HEP proto 100) correlated to a session.
// POST /api/v4/transactions/events
func (h *SearchHandler) V4TransactionEvents(c echo.Context) error {
	var req TransactionSessionRequestV4
	if err := c.Bind(&req); err != nil {
		return writeError(c, http.StatusBadRequest, "Bad Request", "Invalid request body")
	}
	ids, err := resolvedSessionIDList(&req)
	if err != nil {
		return writeError(c, http.StatusBadRequest, "Bad Request", err.Error())
	}

	table := getTableName(h.flightService.LakeName(), 100, "default")
	where := buildSessionIDMatchOrChain(ids)
	if req.Timestamp.From > 0 && req.Timestamp.To > 0 {
		where += fmt.Sprintf(
			" AND timestamp >= (to_timestamp(%d / 1000.0) AT TIME ZONE 'UTC') AND timestamp <= (to_timestamp(%d / 1000.0) AT TIME ZONE 'UTC')",
			req.Timestamp.From, req.Timestamp.To,
		)
	}
	sql := fmt.Sprintf("SELECT * FROM %s WHERE %s ORDER BY timestamp ASC LIMIT 5000", table, where)
	results, err := h.flightService.Query(c.Request().Context(), sql)
	if err != nil {
		return writeError(c, http.StatusInternalServerError, "Server Error", "Query failed")
	}

	resp := LogListResponseV4{}
	resp.Data.Items = results
	resp.Meta = buildMeta(c, "")
	return c.JSON(http.StatusOK, resp)
}

// V4TransactionSub returns SIP messages from the default table (SUBSCRIBE/NOTIFY/OPTIONS)
// correlated to a session by Call-ID.
// POST /api/v4/transactions/sub
func (h *SearchHandler) V4TransactionSub(c echo.Context) error {
	var req TransactionSessionRequestV4
	if err := c.Bind(&req); err != nil {
		return writeError(c, http.StatusBadRequest, "Bad Request", "Invalid request body")
	}
	ids, err := resolvedSessionIDList(&req)
	if err != nil {
		return writeError(c, http.StatusBadRequest, "Bad Request", err.Error())
	}

	table := getTableName(h.flightService.LakeName(), 1, "default")
	where := buildSubMatchOrChain(ids)
	if req.Timestamp.From > 0 && req.Timestamp.To > 0 {
		where += fmt.Sprintf(
			" AND timestamp >= (to_timestamp(%d / 1000.0) AT TIME ZONE 'UTC') AND timestamp <= (to_timestamp(%d / 1000.0) AT TIME ZONE 'UTC')",
			req.Timestamp.From, req.Timestamp.To,
		)
	}
	sql := fmt.Sprintf(
		"SELECT timestamp, src_ip, src_port, dst_ip, dst_port, method, response_code, caller, callee, node_id FROM %s WHERE %s ORDER BY timestamp ASC LIMIT 5000",
		table, where,
	)
	results, err := h.flightService.Query(c.Request().Context(), sql)
	if err != nil {
		return writeError(c, http.StatusInternalServerError, "Server Error", "Query failed")
	}

	resp := LogListResponseV4{}
	resp.Data.Items = results
	resp.Meta = buildMeta(c, "")
	return c.JSON(http.StatusOK, resp)
}

// V4TransactionOtlpLogs returns OTLP log rows whose body or JSON columns contain the given Call-ID (same time range as other transaction tabs).
// POST /api/v4/transactions/otlp-logs
func (h *SearchHandler) V4TransactionOtlpLogs(c echo.Context) error {
	var req TransactionOtlpLogsRequestV4
	if err := c.Bind(&req); err != nil {
		return writeError(c, http.StatusBadRequest, "Bad Request", "Invalid request body")
	}
	ids, err := resolvedSessionIDList(&TransactionSessionRequestV4{
		SessionID:  req.SessionID,
		SessionIDs: req.SessionIDs,
		Timestamp:  req.Timestamp,
	})
	if err != nil {
		return writeError(c, http.StatusBadRequest, "Bad Request", err.Error())
	}
	callID := strings.TrimSpace(req.CallID)
	if callID == "" && len(ids) > 0 {
		callID = ids[0]
	}
	if callID == "" {
		return writeError(c, http.StatusBadRequest, "Bad Request", "call_id or session_id is required")
	}
	escaped := sqlvalidator.SafeString(callID)
	match := fmt.Sprintf(`(
		body LIKE '%%%[1]s%%' OR
		CAST(attributes AS VARCHAR) LIKE '%%%[1]s%%' OR
		CAST(resource_attrs AS VARCHAR) LIKE '%%%[1]s%%' OR
		CAST(body_json AS VARCHAR) LIKE '%%%[1]s%%' OR
		CAST(raw AS VARCHAR) LIKE '%%%[1]s%%'
	)`, escaped)

	whereParts := []string{match}
	if req.Timestamp.From > 0 && req.Timestamp.To > 0 {
		whereParts = append([]string{fmt.Sprintf(
			"timestamp >= (to_timestamp(%d / 1000.0) AT TIME ZONE 'UTC') AND timestamp <= (to_timestamp(%d / 1000.0) AT TIME ZONE 'UTC')",
			req.Timestamp.From, req.Timestamp.To,
		)}, whereParts...)
	}
	where := strings.Join(whereParts, " AND ")

	lake := h.flightService.LakeName()
	sql := fmt.Sprintf("SELECT * FROM %s.otlp_logs WHERE %s ORDER BY timestamp ASC LIMIT %d", lake, where, maxOTLPLogsPerQuery)
	results, err := h.flightService.Query(c.Request().Context(), sql)
	if err != nil {
		return writeError(c, http.StatusInternalServerError, "Server Error", "Query failed")
	}

	resp := LogListResponseV4{}
	resp.Data.Items = results
	resp.Meta = buildMeta(c, "")
	return c.JSON(http.StatusOK, resp)
}

// V4TransactionOtlpLogsTrace returns all OTLP log rows for a single trace_id (same time window as other transaction tabs).
// POST /api/v4/transactions/otlp-logs-trace
func (h *SearchHandler) V4TransactionOtlpLogsTrace(c echo.Context) error {
	var req TransactionOtlpLogsTraceRequestV4
	if err := c.Bind(&req); err != nil {
		return writeError(c, http.StatusBadRequest, "Bad Request", "Invalid request body")
	}
	if _, err := resolvedSessionIDList(&TransactionSessionRequestV4{
		SessionID:  req.SessionID,
		SessionIDs: req.SessionIDs,
		Timestamp:  req.Timestamp,
	}); err != nil {
		return writeError(c, http.StatusBadRequest, "Bad Request", err.Error())
	}
	tid := strings.TrimSpace(req.TraceID)
	if tid == "" {
		return writeError(c, http.StatusBadRequest, "Bad Request", "trace_id is required")
	}
	escaped := sqlvalidator.SafeString(tid)
	whereParts := []string{fmt.Sprintf("trace_id = '%s'", escaped)}
	if req.Timestamp.From > 0 && req.Timestamp.To > 0 {
		whereParts = append([]string{fmt.Sprintf(
			"timestamp >= (to_timestamp(%d / 1000.0) AT TIME ZONE 'UTC') AND timestamp <= (to_timestamp(%d / 1000.0) AT TIME ZONE 'UTC')",
			req.Timestamp.From, req.Timestamp.To,
		)}, whereParts...)
	}
	where := strings.Join(whereParts, " AND ")
	lake := h.flightService.LakeName()
	sql := fmt.Sprintf("SELECT * FROM %s.otlp_logs WHERE %s ORDER BY timestamp ASC LIMIT %d", lake, where, maxOTLPLogsPerQuery)
	results, err := h.flightService.Query(c.Request().Context(), sql)
	if err != nil {
		return writeError(c, http.StatusInternalServerError, "Server Error", "Query failed")
	}
	resp := LogListResponseV4{}
	resp.Data.Items = results
	resp.Meta = buildMeta(c, "")
	return c.JSON(http.StatusOK, resp)
}

// V4TransactionOtlpMetricNames returns distinct metric names present in otlp_metrics for the given time window.
// POST /api/v4/transactions/otlp-metric-names
func (h *SearchHandler) V4TransactionOtlpMetricNames(c echo.Context) error {
	var req TransactionOtlpMetricNamesRequestV4
	if err := c.Bind(&req); err != nil {
		return writeError(c, http.StatusBadRequest, "Bad Request", "Invalid request body")
	}
	if req.Timestamp.From <= 0 || req.Timestamp.To <= 0 {
		return writeError(c, http.StatusBadRequest, "Bad Request", "timestamp.from and timestamp.to are required")
	}
	if req.Timestamp.From > req.Timestamp.To {
		return writeError(c, http.StatusBadRequest, "Bad Request", "timestamp.from must be <= timestamp.to")
	}
	lake := h.flightService.LakeName()
	sql := buildOTLPMetricNamesSQL(lake, req.Timestamp.From, req.Timestamp.To, req.ServiceName)
	results, err := h.flightService.Query(c.Request().Context(), sql)
	if err != nil {
		logger.Error(fmt.Sprintf("V4TransactionOtlpMetricNames: query error: %v", err))
		return writeError(c, http.StatusInternalServerError, "Server Error", "Query failed")
	}
	resp := TransactionListResponseV4{}
	resp.Data.Items = results
	resp.Data.Keys = getColumns(results)
	resp.Meta = buildMeta(c, "")
	resp.Meta.Pagination = &Pagination{Limit: maxOTLPMetricNamesDistinct, Total: len(results)}
	resp.Meta.TimeRange = &TimeRangeMeta{From: req.Timestamp.From, To: req.Timestamp.To}
	return c.JSON(http.StatusOK, resp)
}

// V4TransactionExportText exports all SIP messages for a session as plain text.
// POST /api/v4/transactions/export/text
// Response: text/plain attachment, one SIP message per block.
func (h *SearchHandler) V4TransactionExportText(c echo.Context) error {
	rows, filename, err := h.fetchTransactionRows(c, "txt")
	if err != nil {
		return err
	}
	return h.writeTransactionExportText(c, rows, filename)
}

func (h *SearchHandler) writeTransactionExportText(c echo.Context, rows []map[string]interface{}, filename string) error {
	var buf strings.Builder
	for _, row := range rows {
		ts := pcapwriter.RowTime(row, "timestamp")
		srcIP := pcapwriter.RowStr(row, "src_ip")
		dstIP := pcapwriter.RowStr(row, "dst_ip")
		srcPort := pcapwriter.RowInt(row, "src_port")
		dstPort := pcapwriter.RowInt(row, "dst_port")
		payload := pcapwriter.RowStr(row, "payload")
		if payload == "" {
			continue
		}
		buf.WriteString(FormatTextLine(srcIP, dstIP, srcPort, dstPort, ts, "SIP"))
		buf.WriteString(payload)
		buf.WriteString("\r\n\r\n")
	}

	c.Response().Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	return c.Blob(http.StatusOK, "text/plain; charset=utf-8", []byte(buf.String()))
}

// V4TransactionExportPcap exports all SIP messages for a session as a PCAP file.
// POST /api/v4/transactions/export/pcap
// Response: application/vnd.tcpdump.pcap attachment.
func (h *SearchHandler) V4TransactionExportPcap(c echo.Context) error {
	rows, filename, err := h.fetchTransactionRows(c, "pcap")
	if err != nil {
		return err
	}
	return h.writeTransactionExportPcap(c, rows, filename)
}

func (h *SearchHandler) writeTransactionExportPcap(c echo.Context, rows []map[string]interface{}, filename string) error {
	raw, err := pcapwriter.SIPSearchRowsToPCAP(rows)
	if err != nil {
		if errors.Is(err, pcapwriter.ErrNoSIPPacketsInRows) {
			return writeError(c, http.StatusBadRequest, "Bad Request", "No exportable SIP payload in result set")
		}
		return writeError(c, http.StatusInternalServerError, "Server Error", "Failed to build PCAP")
	}

	c.Response().Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	return c.Blob(http.StatusOK, "application/vnd.tcpdump.pcap", raw)
}

// buildTransactionExportSQL builds the full SELECT for SIP export rows and returns session ids for naming.
func (h *SearchHandler) buildTransactionExportSQL(req *TransactionSessionRequestV4) (string, []string, error) {
	ids, rerr := resolvedSessionIDList(req)
	if rerr != nil {
		return "", nil, rerr
	}
	protoType := req.ProtoType
	if protoType == 0 {
		protoType = 1
	}
	eventType := req.EventType
	if eventType == "" {
		eventType = "call"
	}
	table := getTableName(h.flightService.LakeName(), protoType, eventType)
	where := buildSessionIDMatchOrChain(ids)
	if req.Timestamp.From > 0 && req.Timestamp.To > 0 {
		where += fmt.Sprintf(
			" AND timestamp >= (to_timestamp(%d / 1000.0) AT TIME ZONE 'UTC') AND timestamp <= (to_timestamp(%d / 1000.0) AT TIME ZONE 'UTC')",
			req.Timestamp.From, req.Timestamp.To,
		)
	}
	where += buildExportExcludeIPClause(req.Whitelist)
	sql := fmt.Sprintf("SELECT * FROM %s WHERE %s ORDER BY timestamp ASC LIMIT 10000", table, where)
	return sql, ids, nil
}

// buildExportExcludeIPClause appends AND conditions so rows with src_ip or dst_ip matching any
// listed address are omitted from export (Homer 7 export exclusion / param.whitelist parity).
func buildExportExcludeIPClause(whitelist []string) string {
	var b strings.Builder
	for _, ip := range whitelist {
		ip = strings.TrimSpace(ip)
		if ip == "" {
			continue
		}
		safe := sqlvalidator.SafeString(ip)
		b.WriteString(fmt.Sprintf(" AND src_ip != '%s' AND dst_ip != '%s'", safe, safe))
	}
	return b.String()
}

func exportFilenameForSessionIDs(ids []string, ext string) string {
	safe := strings.NewReplacer("/", "_", "\\", "_", ":", "_", " ", "_", "@", "_")
	var sid string
	if len(ids) > 1 {
		sid = fmt.Sprintf("multi_%d_sessions", len(ids))
	} else {
		sid = safe.Replace(ids[0])
	}
	if len(sid) > 64 {
		sid = sid[:64]
	}
	return fmt.Sprintf("homer_%s.%s", sid, ext)
}

func (h *SearchHandler) hasTransactionMessages(ctx context.Context, req *TransactionSessionRequestV4) (bool, error) {
	sqlStr, _, err := h.buildTransactionExportSQL(req)
	if err != nil {
		return false, err
	}
	idx := strings.Index(sqlStr, "ORDER BY timestamp")
	if idx < 0 {
		return false, fmt.Errorf("invalid export SQL")
	}
	prefix := strings.Replace(sqlStr[:idx], "SELECT *", "SELECT 1", 1)
	probeSQL := prefix + " LIMIT 1"
	rows, err := h.flightService.Query(ctx, probeSQL)
	if err != nil {
		return false, err
	}
	return len(rows) > 0, nil
}

// fetchTransactionRowsWithRequest loads SIP rows for export (authenticated or share replay).
func (h *SearchHandler) fetchTransactionRowsWithRequest(c echo.Context, req *TransactionSessionRequestV4, ext string) ([]map[string]interface{}, string, error) {
	sqlStr, ids, err := h.buildTransactionExportSQL(req)
	if err != nil {
		return nil, "", writeError(c, http.StatusBadRequest, "Bad Request", err.Error())
	}
	rows, err := h.flightService.Query(c.Request().Context(), sqlStr)
	if err != nil {
		return nil, "", writeError(c, http.StatusInternalServerError, "Server Error", "Query failed")
	}
	return rows, exportFilenameForSessionIDs(ids, ext), nil
}

// fetchTransactionRows binds JSON from the request then loads export rows.
func (h *SearchHandler) fetchTransactionRows(c echo.Context, ext string) ([]map[string]interface{}, string, error) {
	var req TransactionSessionRequestV4
	if err := c.Bind(&req); err != nil {
		return nil, "", writeError(c, http.StatusBadRequest, "Bad Request", "Invalid request body")
	}
	return h.fetchTransactionRowsWithRequest(c, &req, ext)
}

// transactionExportLinkDataV4 is the JSON object under "data" for POST /transactions/export/link.
type transactionExportLinkDataV4 struct {
	UUID     string `json:"uuid"`
	URLText  string `json:"url_text"`
	URLPcap  string `json:"url_pcap"`
	URLView  string `json:"url_view,omitempty"`
	ViewUUID string `json:"view_uuid,omitempty"`
}

// V4TransactionExportLinkCreate stores the export request server-side and returns share URLs (no JWT for download)
// plus an optional one-time browser URL url_view (/export/view/<view_uuid>) when view token storage is enabled.
// POST /api/v4/transactions/export/link — body same as /transactions/export/text.
func (h *SearchHandler) V4TransactionExportLinkCreate(c echo.Context) error {
	if h.shareExports == nil {
		return writeError(c, http.StatusServiceUnavailable, "Service Unavailable", "Share link storage not available")
	}
	username, err := getUsernameFromContext(c)
	if err != nil {
		return writeError(c, http.StatusUnauthorized, "Unauthorized", "Not authenticated")
	}
	bodyBytes, err := io.ReadAll(c.Request().Body)
	if err != nil {
		return writeError(c, http.StatusBadRequest, "Bad Request", "Invalid request body")
	}
	var probe TransactionSessionRequestV4
	if err := json.Unmarshal(bodyBytes, &probe); err != nil {
		return writeError(c, http.StatusBadRequest, "Bad Request", "Invalid JSON")
	}
	ok, qerr := h.hasTransactionMessages(c.Request().Context(), &probe)
	if qerr != nil {
		logger.Error("V4TransactionExportLinkCreate: probe query failed", "error", qerr)
		return writeError(c, http.StatusInternalServerError, "Server Error", "Query failed")
	}
	if !ok {
		return writeError(c, http.StatusBadRequest, "Bad Request", "No exportable data for this session")
	}
	id, cerr := h.shareExports.Create(c.Request().Context(), username, bodyBytes, 72*time.Hour)
	if cerr != nil {
		logger.Error("V4TransactionExportLinkCreate: persist failed", "error", cerr)
		return writeError(c, http.StatusInternalServerError, "Server Error", "Failed to create share link")
	}
	base := "/api/v4/share/transactions/export"
	out := transactionExportLinkDataV4{
		UUID:    id,
		URLText: fmt.Sprintf("%s/text/%s", base, id),
		URLPcap: fmt.Sprintf("%s/pcap/%s", base, id),
	}
	meta := buildMeta(c, "")
	if h.viewTokens != nil {
		viewID, verr := h.viewTokens.Create(c.Request().Context(), username, bodyBytes, 72*time.Hour, h.transactionViewMaxOpens)
		if verr != nil {
			logger.Error("V4TransactionExportLinkCreate: view token failed", "error", verr)
			meta.Message = "browser_view_unavailable: " + verr.Error()
		} else {
			out.URLView = "/export/view/" + viewID
			out.ViewUUID = viewID
		}
	}
	return c.JSON(http.StatusCreated, map[string]interface{}{
		"data": out,
		"meta": meta,
	})
}

func (h *SearchHandler) loadShareTransactionRequest(c echo.Context) (*TransactionSessionRequestV4, error) {
	if h.shareExports == nil {
		return nil, writeError(c, http.StatusServiceUnavailable, "Service Unavailable", "Share export not available")
	}
	payload, err := h.shareExports.GetPayload(c.Request().Context(), c.Param("shareId"))
	if err != nil {
		return nil, writeError(c, http.StatusGone, "Gone", "Share link not found or expired")
	}
	var req TransactionSessionRequestV4
	if err := json.Unmarshal(payload, &req); err != nil {
		return nil, writeError(c, http.StatusInternalServerError, "Server Error", "Invalid share payload")
	}
	return &req, nil
}

// V4ShareTransactionExportText serves text export for a previously created share id (public, no auth).
// GET /api/v4/share/transactions/export/text/:shareId
func (h *SearchHandler) V4ShareTransactionExportText(c echo.Context) error {
	req, err := h.loadShareTransactionRequest(c)
	if err != nil {
		return err
	}
	rows, filename, err := h.fetchTransactionRowsWithRequest(c, req, "txt")
	if err != nil {
		return err
	}
	return h.writeTransactionExportText(c, rows, filename)
}

// V4ShareTransactionExportPcap serves PCAP export for a previously created share id (public, no auth).
// GET /api/v4/share/transactions/export/pcap/:shareId
func (h *SearchHandler) V4ShareTransactionExportPcap(c echo.Context) error {
	req, err := h.loadShareTransactionRequest(c)
	if err != nil {
		return err
	}
	rows, filename, err := h.fetchTransactionRowsWithRequest(c, req, "pcap")
	if err != nil {
		return err
	}
	return h.writeTransactionExportPcap(c, rows, filename)
}

// hepTableRe matches unqualified HEP table references.
// It handles three cases (with word-boundary check via look-behind simulation):
//
//	hep_proto_1_call            → {lake}.main.hep_proto_1_call
//	main.hep_proto_1_call       → {lake}.main.hep_proto_1_call
//	{lake}.main.hep_proto_1_call → left unchanged (already fully qualified)
//
// Captures up to 3 qualifier segments (e.g. "homer_lake.main.") so we can
// detect already-fully-qualified names like "homer_lake.main.hep_proto_1_call".
var hepTableRe = regexp.MustCompile(`(?i)\b((?:\w+\.){0,3})(hep_proto_\d+_\w+)`)

// qualifyTableNames rewrites bare / schema-only HEP table references so they
// include the DuckLake catalog name.
//
// Examples (lake = "homer_lake"):
//
//	hep_proto_1_call          → homer_lake.main.hep_proto_1_call
//	main.hep_proto_1_call     → homer_lake.main.hep_proto_1_call
//	homer_lake.main.hep_proto_1_call → unchanged
func qualifyTableNames(sql, lake string) string {
	if lake == "" {
		return sql
	}
	prefix := lake + ".main."
	return hepTableRe.ReplaceAllStringFunc(sql, func(match string) string {
		sub := hepTableRe.FindStringSubmatch(match)
		if len(sub) < 3 {
			return match
		}
		qualifier := sub[1] // e.g. "" / "main." / "homer_lake.main."
		table := sub[2]     // e.g. "hep_proto_1_call"

		// Already fully qualified with the lake name — leave as-is.
		if strings.HasPrefix(qualifier, lake+".") {
			return match
		}
		// Strip any partial qualifier ("main.") and apply the full prefix.
		return prefix + table
	})
}

// V4RawQuery executes a raw SQL query (SELECT only) against the nodes.
func (h *SearchHandler) V4RawQuery(c echo.Context) error {
	var req RawQueryRequest
	if err := c.Bind(&req); err != nil {
		return writeError(c, http.StatusBadRequest, "Bad Request", "Invalid request body")
	}

	sql := strings.TrimSpace(req.SQL)
	if sql == "" {
		return writeError(c, http.StatusBadRequest, "Bad Request", "sql field is required")
	}

	// Auto-qualify bare HEP table names with the lake catalog prefix.
	// e.g. "hep_proto_1_call" → "homer_lake.main.hep_proto_1_call"
	sql = qualifyTableNames(sql, h.flightService.LakeName())

	// Validate SQL for safety (prevents injection, DML, file access, etc.)
	if err := sqlvalidator.ValidateRawSQL(sql); err != nil {
		logger.Error(fmt.Sprintf("V4RawQuery: SQL validation failed: %v", err))
		return writeError(c, http.StatusBadRequest, "Bad Request", fmt.Sprintf("SQL validation failed: %v", err))
	}

	// Inject LIMIT if not present, but only for query types that accept it
	// (SELECT, WITH). SHOW, DESCRIBE, EXPLAIN, and PRAGMA do not accept LIMIT.
	effectiveLimit := req.Limit
	if effectiveLimit <= 0 || effectiveLimit > 50000 {
		effectiveLimit = 1000
	}
	if sqlvalidator.IsLimitableQuery(sql) && !sqlvalidator.HasLimitToken(sql) {
		sql += fmt.Sprintf(" LIMIT %d", effectiveLimit)
	}

	logger.Info("V4RawQuery", "sql", sql)
	results, err := h.flightService.Query(c.Request().Context(), sql)
	if err != nil {
		logger.Error(fmt.Sprintf("V4RawQuery: query error: %v", err))
		return writeError(c, http.StatusInternalServerError, "Server Error", fmt.Sprintf("Query failed: %v", err))
	}
	logger.Info("V4RawQuery: got results", "count", len(results))

	h.enrichRowsWithIPAliases(c.Request().Context(), results)

	resp := TransactionListResponseV4{}
	resp.Data.Items = results
	resp.Data.Keys = getColumns(results)
	resp.Meta = buildMeta(c, "")
	resp.Meta.Pagination = &Pagination{Limit: effectiveLimit, Total: len(results)}
	return c.JSON(http.StatusOK, resp)
}

func (h *SearchHandler) V4MCPQuery(c echo.Context) error {
	var req MCPQueryRequest
	if err := c.Bind(&req); err != nil {
		return writeError(c, http.StatusBadRequest, "Bad Request", "Invalid request body")
	}

	req.QueryText = strings.TrimSpace(req.QueryText)
	if req.QueryText == "" {
		return writeError(c, http.StatusBadRequest, "Bad Request", "query_text is required")
	}

	mode := strings.ToLower(strings.TrimSpace(req.Mode))
	if mode == "" {
		mode = "auto"
	}
	if mode != "auto" && mode != "structured" && mode != "sql" {
		return writeError(c, http.StatusBadRequest, "Bad Request", "mode must be one of: auto, structured, sql")
	}
	if mode == "auto" {
		if shouldUseSQLMode(req.QueryText) {
			mode = "sql"
		} else {
			mode = "structured"
		}
	}

	switch mode {
	case "sql":
		return h.runMCPAsSQL(c, &req)
	default:
		return h.runMCPAsStructured(c, &req)
	}
}

func (h *SearchHandler) runMCPAsStructured(c echo.Context, req *MCPQueryRequest) error {
	parserHint := normalizeParserHint(req.Parser)

	searchReq := SearchObjectV4{}
	searchReq.Filter.ProtoType = 1
	searchReq.Filter.EventType = "call"
	searchReq.Param.OrderBy = "timestamp DESC"
	searchReq.Param.Limit = sanitizeLimit(req.Limit, 100, 1000)

	var fallbackFrom, fallbackTo int64
	if req.Timestamp.From > 0 || req.Timestamp.To > 0 {
		fallbackFrom = req.Timestamp.From
		fallbackTo = req.Timestamp.To
	} else {
		fallbackFrom, fallbackTo = inferTimeRange(req.QueryText, req.NowUTCUnixMS)
	}

	parserMeta := &ParserMeta{Used: "regex", Requested: parserHint}

	// parser=regex skips the LLM entirely; parser=llm requires it; parser=auto
	// (the default) attempts LLM first and silently falls back to regex when
	// the model is unreachable, slow, or returns garbage.
	if parserHint != "regex" {
		llmReq, llmRes := h.tryLLMStructured(c.Request().Context(), req.QueryText, fallbackFrom, fallbackTo, searchReq.Param.Limit)
		switch {
		case llmRes.Used && llmReq != nil:
			searchReq = *llmReq
			if methods := extractSIPMethods(req.QueryText); len(methods) > 1 {
				searchReq.Filter.Methods = methods
				searchReq.Filter.Method = ""
			}
			parserMeta.Used = "llm"
			parserMeta.Model = llmRes.Model
			parserMeta.LatencyMS = llmRes.LatencyMS
		case llmRes.Err != nil:
			if parserHint == "llm" {
				return writeError(c, http.StatusBadGateway, "LLM Error", fmt.Sprintf("structured LLM parser failed: %v", llmRes.Err))
			}
			logger.Warn(fmt.Sprintf("MCP LLM structured parser failed, fallback to rules: %v", llmRes.Err))
			parserMeta.Used = "regex_fallback"
			parserMeta.Model = llmRes.Model
			parserMeta.LatencyMS = llmRes.LatencyMS
			parserMeta.Error = llmRes.Err.Error()
		case parserHint == "llm":
			return writeError(c, http.StatusFailedDependency, "LLM Disabled", "parser=llm requested but LLM bridge is not enabled")
		}
	}

	if parserMeta.Used != "llm" {
		methods := extractSIPMethods(req.QueryText)
		if len(methods) == 1 {
			searchReq.Filter.Method = methods[0]
		} else if len(methods) > 1 {
			searchReq.Filter.Methods = methods
		}
		searchReq.Filter.SrcIP = extractByRegex(req.QueryText, `(?:src|source)\s+ip[:=]?\s*([0-9.]+)`)
		searchReq.Filter.DstIP = extractByRegex(req.QueryText, `(?:dst|destination)\s+ip[:=]?\s*([0-9.]+)`)
		searchReq.Filter.CallID = extractCallID(req.QueryText)
		searchReq.Filter.FromUser = extractFromUser(req.QueryText)
		searchReq.Filter.ToUser = extractToUser(req.QueryText)
		searchReq.Timestamp.From = fallbackFrom
		searchReq.Timestamp.To = fallbackTo
	}

	virtualRules := h.loadVirtualRulesForReq(c.Request().Context(), &searchReq)
	sql, err := buildSearchSQLV4(h.flightService.LakeName(), &searchReq, virtualRules)
	if err != nil {
		return writeError(c, http.StatusBadRequest, "Bad Request", fmt.Sprintf("SQL validation failed: %v", err))
	}
	results, err := h.queryTransactionSearch(c.Request().Context(), sql, &searchReq, virtualRules)
	if err != nil {
		return writeError(c, http.StatusInternalServerError, "Server Error", "Query failed")
	}

	h.enrichRowsWithIPAliases(c.Request().Context(), results)

	resp := TransactionListResponseV4{}
	resp.Data.Items = results
	resp.Data.Keys = getColumns(results)
	resp.Meta = buildMeta(c, "")
	resp.Meta.Pagination = &Pagination{Limit: searchReq.Param.Limit, Total: len(results)}
	if searchReq.Timestamp.From > 0 || searchReq.Timestamp.To > 0 {
		resp.Meta.TimeRange = &TimeRangeMeta{From: searchReq.Timestamp.From, To: searchReq.Timestamp.To}
	}
	resp.Meta.Parser = parserMeta
	return c.JSON(http.StatusOK, resp)
}

func (h *SearchHandler) runMCPAsSQL(c echo.Context, req *MCPQueryRequest) error {
	parserHint := normalizeParserHint(req.Parser)

	sqlReq := SearchObjectV4{}
	sqlReq.Filter.ProtoType = 1
	sqlReq.Filter.EventType = "call"
	if methods := extractSIPMethods(req.QueryText); len(methods) == 1 {
		sqlReq.Filter.Method = methods[0]
	} else if len(methods) > 1 {
		sqlReq.Filter.Methods = methods
	}
	sqlReq.Filter.SrcIP = extractByRegex(req.QueryText, `(?:src|source)\s+ip[:=]?\s*([0-9.]+)`)
	sqlReq.Filter.DstIP = extractByRegex(req.QueryText, `(?:dst|destination)\s+ip[:=]?\s*([0-9.]+)`)
	sqlReq.Filter.CallID = extractByRegex(req.QueryText, `(?:call[_\s-]?id|session[_\s-]?id)[:=]?\s*([^\s,]+)`)
	sqlReq.Filter.FromUser = extractByRegex(req.QueryText, `(?:from[_\s-]?user|caller)[:=]?\s*([^\s,]+)`)
	sqlReq.Filter.ToUser = extractByRegex(req.QueryText, `(?:to[_\s-]?user|callee)[:=]?\s*([^\s,]+)`)
	sqlReq.Param.Limit = sanitizeLimit(req.Limit, 100, 50000)
	sqlReq.Param.OrderBy = "timestamp DESC"
	if req.Timestamp.From > 0 || req.Timestamp.To > 0 {
		sqlReq.Timestamp.From = req.Timestamp.From
		sqlReq.Timestamp.To = req.Timestamp.To
	} else {
		from, to := inferTimeRange(req.QueryText, req.NowUTCUnixMS)
		sqlReq.Timestamp.From = from
		sqlReq.Timestamp.To = to
	}

	parserMeta := &ParserMeta{Used: "regex", Requested: parserHint}

	sql := ""
	if parserHint != "regex" {
		llmSQL, llmRes := h.tryLLMSQL(c.Request().Context(), req.QueryText, sqlReq.Param.Limit)
		switch {
		case llmRes.Used && strings.TrimSpace(llmSQL) != "":
			sql = llmSQL
			parserMeta.Used = "llm"
			parserMeta.Model = llmRes.Model
			parserMeta.LatencyMS = llmRes.LatencyMS
		case llmRes.Err != nil:
			if parserHint == "llm" {
				return writeError(c, http.StatusBadGateway, "LLM Error", fmt.Sprintf("sql LLM parser failed: %v", llmRes.Err))
			}
			logger.Warn(fmt.Sprintf("MCP LLM sql parser failed, fallback to rules: %v", llmRes.Err))
			parserMeta.Used = "regex_fallback"
			parserMeta.Model = llmRes.Model
			parserMeta.LatencyMS = llmRes.LatencyMS
			parserMeta.Error = llmRes.Err.Error()
		case parserHint == "llm":
			return writeError(c, http.StatusFailedDependency, "LLM Disabled", "parser=llm requested but LLM bridge is not enabled")
		}
	}
	if strings.TrimSpace(sql) == "" {
		sql = buildMCPRawSQL(h.flightService.LakeName(), &sqlReq)
	}
	if err := sqlvalidator.ValidateRawSQL(sql); err != nil {
		return writeError(c, http.StatusBadRequest, "Bad Request", fmt.Sprintf("SQL validation failed: %v", err))
	}

	results, err := h.flightService.Query(c.Request().Context(), sql)
	if err != nil {
		return writeError(c, http.StatusInternalServerError, "Server Error", "Query failed")
	}

	h.enrichRowsWithIPAliases(c.Request().Context(), results)

	resp := TransactionListResponseV4{}
	resp.Data.Items = results
	resp.Data.Keys = getColumns(results)
	resp.Meta = buildMeta(c, sql)
	resp.Meta.Pagination = &Pagination{Limit: sqlReq.Param.Limit, Total: len(results)}
	if sqlReq.Timestamp.From > 0 || sqlReq.Timestamp.To > 0 {
		resp.Meta.TimeRange = &TimeRangeMeta{From: sqlReq.Timestamp.From, To: sqlReq.Timestamp.To}
	}
	resp.Meta.Parser = parserMeta
	return c.JSON(http.StatusOK, resp)
}

func sanitizeLimit(v, fallback, max int) int {
	if v <= 0 {
		v = fallback
	}
	if v > max {
		v = max
	}
	return v
}

// englishHourWordToN supports "last two hours" (regex below only matches digits).
var englishHourWordToN = map[string]int64{
	"one": 1, "two": 2, "three": 3, "four": 4, "five": 5,
	"six": 6, "seven": 7, "eight": 8, "nine": 9, "ten": 10,
	"eleven": 11, "twelve": 12,
}

// lastEnglishWordHoursRE matches "last/past … one|two|… twelve hours".
var lastEnglishWordHoursRE = regexp.MustCompile(`(?i)(?:from\s+(?:the\s+)?|within\s+(?:the\s+)?|over\s+(?:the\s+)?|for\s+(?:the\s+)?)?(?:last|past)\s+(one|two|three|four|five|six|seven|eight|nine|ten|eleven|twelve)\s+hours?\b`)

// lastNDurationRe matches "last/past/from (the) last/within (the) last N unit" in EN, RU, DE, UK.
// Groups: 1=number, 2=unit
var lastNDurationRe = regexp.MustCompile(`(?i)(?:from\s+(?:the\s+)?)?(?:within\s+(?:the\s+)?)?(?:over\s+(?:the\s+)?)?(?:for\s+(?:the\s+)?)?(?:last|past|letzten?|letzte|за\s+последн(?:ие|ий|их)|последн(?:ие|ий|их)|за\s+останн(?:ій|і|іх|ю)|останн(?:ій|і|іх|ю))\s*(\d+)\s*(minute|minutes?|min|mins|minuten?|мин|минут|хвилин(?:и|у)?|хв|часа?|часов|hour|hours?|hr|hrs|stunde|stunden|годин(?:у|и|а)?|год|tag|tage|tagen|день|дн(?:я|ей)|днів|дні)`)

// lastSingularRe matches "last/past/from (the) last hour/day/minute" WITHOUT a number.
var lastSingularRe = regexp.MustCompile(`(?i)(?:from\s+(?:the\s+)?|within\s+(?:the\s+)?|over\s+(?:the\s+)?|for\s+(?:the\s+)?)?(?:last|past)\s+(hour|minute|min|day|stunde|tag|день|час|годину?)`)

func inferTimeRange(queryText string, nowUTCUnixMS int64) (int64, int64) {
	now := nowUTCUnixMS
	if now <= 0 {
		now = time.Now().UTC().UnixMilli()
	}
	refTime := time.UnixMilli(now).UTC()
	lower := strings.ToLower(strings.TrimSpace(queryText))
	hour := int64(time.Hour / time.Millisecond)
	day := int64(24 * time.Hour / time.Millisecond)

	// ── 1. Our reliable patterns first (prevent anytime from misinterpreting them) ──

	// "last two hours" etc. (English words — lastNDurationRe only matches digits)
	if m := lastEnglishWordHoursRE.FindStringSubmatch(lower); len(m) >= 2 {
		if n, ok := englishHourWordToN[strings.ToLower(m[1])]; ok && n > 0 && n <= 168 {
			return now - n*hour, now
		}
	}

	// "last N minutes/hours/days" with explicit number
	if m := lastNDurationRe.FindStringSubmatch(queryText); len(m) >= 3 {
		n, err := strconv.ParseInt(m[1], 10, 64)
		if err == nil && n > 0 && n <= 10000 {
			unit := strings.ToLower(m[2])
			var dur int64
			switch {
			case strings.HasPrefix(unit, "min") || unit == "мин" || strings.HasPrefix(unit, "минут") || strings.HasPrefix(unit, "хвил") || unit == "хв":
				dur = n * int64(time.Minute/time.Millisecond)
			case strings.HasPrefix(unit, "h") || strings.HasPrefix(unit, "stund") || strings.HasPrefix(unit, "час") || strings.HasPrefix(unit, "годин") || unit == "год":
				dur = n * hour
			case strings.HasPrefix(unit, "tag") || strings.HasPrefix(unit, "дн") || strings.HasPrefix(unit, "дні") || strings.HasPrefix(unit, "day"):
				dur = n * day
			default:
				dur = hour
			}
			return now - dur, now
		}
	}

	// Singular "last hour / last day / last minute" (with any preposition prefix)
	if m := lastSingularRe.FindStringSubmatch(lower); len(m) >= 2 {
		unit := strings.ToLower(m[1])
		switch {
		case strings.HasPrefix(unit, "min"):
			return now - int64(time.Minute/time.Millisecond), now
		case strings.HasPrefix(unit, "h") || strings.HasPrefix(unit, "час") || strings.HasPrefix(unit, "годин"):
			return now - hour, now
		case strings.HasPrefix(unit, "day") || strings.HasPrefix(unit, "tag") || strings.HasPrefix(unit, "день"):
			return now - day, now
		}
	}

	// Fixed phrases (multilingual)
	switch {
	case strings.Contains(lower, "last day"), strings.Contains(lower, "for the last day"), strings.Contains(lower, "for last day"),
		strings.Contains(lower, "from the last day"), strings.Contains(lower, "from last day"),
		strings.Contains(lower, "within the last day"), strings.Contains(lower, "over the last day"),
		strings.Contains(lower, "последний день"), strings.Contains(lower, "за последний день"),
		strings.Contains(lower, "letzter tag"), strings.Contains(lower, "letzten tag"),
		strings.Contains(lower, "останній день"), strings.Contains(lower, "за останній день"):
		return now - day, now
	case strings.Contains(lower, "last hour"), strings.Contains(lower, "for the last hour"), strings.Contains(lower, "for last hour"),
		strings.Contains(lower, "from the last hour"), strings.Contains(lower, "from last hour"),
		strings.Contains(lower, "within the last hour"), strings.Contains(lower, "over the last hour"),
		strings.Contains(lower, "last 1 hour"), strings.Contains(lower, "past hour"),
		strings.Contains(lower, "последний час"), strings.Contains(lower, "за последний час"),
		strings.Contains(lower, "letzte stunde"), strings.Contains(lower, "letzten stunde"),
		strings.Contains(lower, "останній час"), strings.Contains(lower, "останню годину"):
		return now - hour, now
	case strings.Contains(lower, "сегодня"), strings.Contains(lower, "today"), strings.Contains(lower, "heute"), strings.Contains(lower, "сьогодні"):
		t := time.UnixMilli(now).UTC()
		start := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC).UnixMilli()
		return start, now
	case strings.Contains(lower, "вчера"), strings.Contains(lower, "yesterday"), strings.Contains(lower, "gestern"), strings.Contains(lower, "вчора"):
		t := time.UnixMilli(now).UTC()
		startToday := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC).UnixMilli()
		return startToday - day, startToday - 1
	}

	// ── 2. anytime library for everything else ("5 minutes ago", "last week", etc.) ──
	if r, _, err := anytime.ParseRange(queryText, refTime, anytime.Past); err == nil {
		start := r.Start().UTC().UnixMilli()
		end := r.End().UTC().UnixMilli()
		if start < end && (end-start) <= 365*24*int64(time.Hour/time.Millisecond) {
			return start, end
		}
	}
	var found []anytime.Range
	_ = anytime.ReplaceAllRangesByFunc(queryText, refTime, anytime.Past, func(src string, r anytime.Range) string {
		found = append(found, r)
		return src
	})
	if len(found) > 0 {
		last := found[len(found)-1]
		start := last.Start().UTC().UnixMilli()
		end := last.End().UTC().UnixMilli()
		if start < end && (end-start) <= 365*24*int64(time.Hour/time.Millisecond) {
			return start, end
		}
	}

	// ── 3. Default: last hour ──
	return now - hour, now
}

func extractSIPMethods(queryText string) []string {
	known := []string{
		"INVITE", "BYE", "REGISTER", "OPTIONS", "ACK", "CANCEL", "PRACK",
		"UPDATE", "INFO", "REFER", "SUBSCRIBE", "NOTIFY", "PUBLISH", "MESSAGE",
	}
	upper := strings.ToUpper(queryText)
	var found []string
	seen := make(map[string]bool)
	for _, method := range known {
		re := regexp.MustCompile(`\b` + method + `\b`)
		if re.MatchString(upper) && !seen[method] {
			seen[method] = true
			found = append(found, method)
		}
	}
	return found
}

// timeWords are common words that appear after "from"/"to" in time expressions
// and should NOT be captured as SIP user values.
var timeWords = map[string]bool{
	"the": true, "a": true, "an": true,
	"last": true, "past": true, "next": true, "this": true,
	"hour": true, "hours": true, "day": true, "days": true,
	"minute": true, "minutes": true, "min": true, "mins": true,
	"week": true, "weeks": true, "month": true, "months": true,
	"now": true, "today": true, "yesterday": true, "tomorrow": true,
	"morning": true, "night": true, "noon": true, "midnight": true,
	"second": true, "seconds": true, "sec": true,
}

// extractFromUser safely extracts the SIP from_user / caller filter.
// It requires an explicit "from_user", "from user", "caller" keyword or
// a bare "from <value>" where value is NOT a time-related word.
func extractFromUser(text string) string {
	// Explicit form: from_user:, from_user=, from user:, caller:
	if v := extractByRegex(text, `(?:from[_\s-]user|caller)[:=]\s*["']?([^\s,"']+)["']?`); v != "" {
		return v
	}
	// Bare "from <value>" — only if value doesn't look like a time word
	re := regexp.MustCompile(`(?i)\bfrom\s+["']?([^\s,"']+)["']?`)
	for _, m := range re.FindAllStringSubmatch(text, -1) {
		if len(m) >= 2 {
			v := strings.ToLower(strings.Trim(m[1], `"'`))
			if !timeWords[v] {
				return strings.Trim(m[1], `"'`)
			}
		}
	}
	return ""
}

// extractToUser safely extracts the SIP to_user / callee filter.
func extractToUser(text string) string {
	// Explicit form: to_user:, to_user=, callee:
	if v := extractByRegex(text, `(?:to[_\s-]user|callee)[:=]\s*["']?([^\s,"']+)["']?`); v != "" {
		return v
	}
	// Bare "to <value>" — only if value doesn't look like a time word
	re := regexp.MustCompile(`(?i)\bto\s+["']?([^\s,"']+)["']?`)
	for _, m := range re.FindAllStringSubmatch(text, -1) {
		if len(m) >= 2 {
			v := strings.ToLower(strings.Trim(m[1], `"'`))
			if !timeWords[v] {
				return strings.Trim(m[1], `"'`)
			}
		}
	}
	return ""
}

func extractByRegex(text, pattern string) string {
	re := regexp.MustCompile(`(?i)` + pattern)
	m := re.FindStringSubmatch(text)
	if len(m) < 2 {
		return ""
	}
	return strings.TrimSpace(strings.Trim(m[1], `"'`))
}

// extractCallID recognises: call_id, call-id, callid, session_id, cid, CID
// followed by optional =/:space and an optionally-quoted value.
func extractCallID(text string) string {
	re := regexp.MustCompile(`(?i)(?:call[_\s-]?id|session[_\s-]?id|\bcid\b)[\s:=]*["']?([^\s,"']+)["']?`)
	m := re.FindStringSubmatch(text)
	if len(m) >= 2 {
		return strings.TrimSpace(m[1])
	}
	// Also match: `with CID "value"` or `with CID value`
	re2 := regexp.MustCompile(`(?i)\bcid\b[\s:=]*["']?([^\s,"']+)["']?`)
	m2 := re2.FindStringSubmatch(text)
	if len(m2) >= 2 {
		return strings.TrimSpace(m2[1])
	}
	return ""
}

func shouldUseSQLMode(queryText string) bool {
	q := strings.ToLower(queryText)
	return strings.Contains(q, "mode=sql") ||
		strings.Contains(q, " raw sql") ||
		strings.HasPrefix(q, "sql:") ||
		strings.Contains(q, "show sql") ||
		strings.Contains(q, "display sql") ||
		strings.Contains(q, "print sql")
}

func buildMCPRawSQL(lakeName string, req *SearchObjectV4) string {
	conditions := make([]string, 0, 8)
	if req.Timestamp.From > 0 {
		conditions = append(conditions, fmt.Sprintf("timestamp >= (to_timestamp(%d / 1000.0) AT TIME ZONE 'UTC')", req.Timestamp.From))
	}
	if req.Timestamp.To > 0 {
		conditions = append(conditions, fmt.Sprintf("timestamp <= (to_timestamp(%d / 1000.0) AT TIME ZONE 'UTC')", req.Timestamp.To))
	}
	methodVals := sipFilterMethodValues(req.Filter.Method, req.Filter.Methods)
	if len(methodVals) > 0 {
		escaped := make([]string, len(methodVals))
		for i, m := range methodVals {
			escaped[i] = "'" + sqlvalidator.SafeString(m) + "'"
		}
		conditions = append(conditions, "method IN ("+strings.Join(escaped, ",")+")")
	}
	rcVals := sipFilterResponseValues(req.Filter.ResponseCode, req.Filter.ResponseCodes)
	if len(rcVals) > 0 {
		escaped := make([]string, len(rcVals))
		for i, rc := range rcVals {
			escaped[i] = "'" + sqlvalidator.SafeString(rc) + "'"
		}
		conditions = append(conditions, "response_code IN ("+strings.Join(escaped, ",")+")")
	}
	if req.Filter.CallID != "" {
		conditions = append(conditions, sqlFormMatchClauseOr("session_id", "cid", req.Filter.CallID))
	}
	if req.Filter.FromUser != "" {
		conditions = append(conditions, sqlFormMatchClause("caller", req.Filter.FromUser))
	}
	if req.Filter.ToUser != "" {
		conditions = append(conditions, sqlFormMatchClause("callee", req.Filter.ToUser))
	}
	if req.Filter.SrcIP != "" {
		conditions = append(conditions, fmt.Sprintf("src_ip = '%s'", sqlvalidator.SafeString(req.Filter.SrcIP)))
	}
	if req.Filter.DstIP != "" {
		conditions = append(conditions, fmt.Sprintf("dst_ip = '%s'", sqlvalidator.SafeString(req.Filter.DstIP)))
	}
	if req.Filter.CaptureID > 0 {
		capStr := sqlvalidator.SafeString(strconv.Itoa(req.Filter.CaptureID))
		conditions = append(conditions, fmt.Sprintf(
			"(CAST(node_id AS VARCHAR) = '%s' OR json_extract_string(data_extra, '$.capture_id') = '%s' OR json_extract_string(data_extra, '$.captureId') = '%s')",
			capStr, capStr, capStr))
	}
	if req.Filter.Payload != "" {
		conditions = append(conditions, sqlFormMatchClause("payload", req.Filter.Payload))
	}

	sql := "SELECT * FROM " + lakeName + ".main.hep_proto_1_call"
	if len(conditions) > 0 {
		sql += " WHERE " + strings.Join(conditions, " AND ")
	}
	sql += " ORDER BY timestamp DESC"
	sql += fmt.Sprintf(" LIMIT %d", sanitizeLimit(req.Param.Limit, 100, 50000))
	return sql
}

func appendVirtualDataExtraConditions(conditions []string, req *SearchObjectV4, rules map[string]services.VirtualFieldRule) []string {
	if len(rules) == 0 {
		return conditions
	}
	conditions = appendVirtualValueConditions(conditions, req.Filter.Virtual, rules)
	conditions = appendVirtualAbsentPresentConditions(conditions, req.Filter.VirtualAbsent, req.Filter.VirtualPresent, rules)
	return conditions
}

func appendVirtualValueConditions(conditions []string, values map[string]string, rules map[string]services.VirtualFieldRule) []string {
	if len(values) == 0 {
		return conditions
	}
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, id := range keys {
		rule, ok := rules[id]
		if !ok {
			continue
		}
		if rule.Match == services.VirtualMatchAbsent || rule.Match == services.VirtualMatchPresent {
			continue
		}
		raw := strings.TrimSpace(values[id])
		if raw == "" {
			continue
		}
		if rule.Kind != services.VirtualKindDataExtraJSON {
			continue
		}
		jp := services.DuckJSONPath(rule.Path)
		clause := sqlFormMatchClause(fmt.Sprintf("json_extract_string(data_extra, '%s')", jp), raw)
		conditions = append(conditions, clause)
	}
	return conditions
}

func appendVirtualAbsentPresentConditions(conditions []string, absentIDs, presentIDs []string, rules map[string]services.VirtualFieldRule) []string {
	normalizeIDs := func(ids []string) []string {
		seen := make(map[string]struct{})
		out := make([]string, 0, len(ids))
		for _, raw := range ids {
			id := strings.TrimSpace(raw)
			if id == "" {
				continue
			}
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			out = append(out, id)
		}
		sort.Strings(out)
		return out
	}
	for _, id := range normalizeIDs(absentIDs) {
		rule, ok := rules[id]
		if !ok || rule.Kind != services.VirtualKindDataExtraJSON || rule.Match != services.VirtualMatchAbsent {
			continue
		}
		jp := services.DuckJSONPath(rule.Path)
		inner := fmt.Sprintf("trim(json_extract_string(data_extra, '%s'))", jp)
		clause := fmt.Sprintf("(json_extract_string(data_extra, '%s') IS NULL OR %s = '' OR lower(%s) = 'null')", jp, inner, inner)
		conditions = append(conditions, clause)
	}
	for _, id := range normalizeIDs(presentIDs) {
		rule, ok := rules[id]
		if !ok || rule.Kind != services.VirtualKindDataExtraJSON || rule.Match != services.VirtualMatchPresent {
			continue
		}
		jp := services.DuckJSONPath(rule.Path)
		inner := fmt.Sprintf("trim(json_extract_string(data_extra, '%s'))", jp)
		clause := fmt.Sprintf("(json_extract_string(data_extra, '%s') IS NOT NULL AND %s <> '' AND lower(%s) <> 'null')", jp, inner, inner)
		conditions = append(conditions, clause)
	}
	return conditions
}

// loadVirtualRulesForReq resolves fields_mapping virtual rules for structured search.
func (h *SearchHandler) loadVirtualRulesForReq(ctx context.Context, req *SearchObjectV4) map[string]services.VirtualFieldRule {
	if h.mappingService == nil || (len(req.Filter.Virtual) == 0 && len(req.Filter.VirtualAbsent) == 0 && len(req.Filter.VirtualPresent) == 0) {
		return nil
	}
	proto := req.Filter.ProtoType
	if proto == 0 {
		proto = 1
	}
	if isOTLPProtoType(proto) || isLPProtoType(proto) {
		return nil
	}
	profile := strings.TrimSpace(req.Filter.EventType)
	if profile == "" {
		if proto == 1 {
			profile = "call"
		} else {
			profile = "default"
		}
	}
	m, err := h.mappingService.GetMappingByProfile(ctx, proto, profile)
	if err != nil {
		logger.Warn("virtual search: mapping load failed", "error", err.Error())
		return nil
	}
	if m == nil {
		return nil
	}
	rules, err := services.VirtualRulesFromFieldsMapping(m.FieldsMapping)
	if err != nil {
		logger.Warn("virtual search: fields_mapping virtual parse failed", "error", err.Error())
		return nil
	}
	return rules
}

// searchSQLOpts tweaks SQL generation for chunked search execution.
type searchSQLOpts struct {
	// toExclusive renders the upper time bound as "<" instead of "<=" so
	// interior slices of a chunked search neither lose nor duplicate rows
	// on chunk boundaries (timestamps can carry sub-millisecond precision).
	toExclusive bool
	// noOrderBy omits the ORDER BY clause so DuckDB executes a plain streaming
	// LIMIT (stops after N rows, flat memory) instead of a Top-N over the whole
	// range. The caller is expected to re-sort the rows in Go. Only honored for
	// the default timestamp-DESC ordering (custom order_by still emits ORDER BY).
	noOrderBy bool
	// narrowNoHeavy replaces a default "SELECT *" with uuid+timestamp only for
	// the filter/sort/LIMIT phase. Full rows are loaded afterwards by uuid
	// (see hydrateHeavyColumns). Ignored when a custom select is supplied.
	narrowNoHeavy bool
}

// narrowProjectionExpr is the phase-1 projection for lazy transaction search:
// only the correlation key and sort key — everything else is hydrated by uuid.
const narrowProjectionExpr = "uuid, timestamp"

// timestampRangeConditions renders WHERE bounds for a [fromMs, toMs] window
// in epoch milliseconds. Zero values omit the corresponding bound.
func timestampRangeConditions(column string, fromMs, toMs int64, toExclusive bool) []string {
	conds := make([]string, 0, 2)
	if fromMs > 0 {
		conds = append(conds, fmt.Sprintf("%s >= (to_timestamp(%d / 1000.0) AT TIME ZONE 'UTC')", column, fromMs))
	}
	if toMs > 0 {
		op := "<="
		if toExclusive {
			op = "<"
		}
		conds = append(conds, fmt.Sprintf("%s %s (to_timestamp(%d / 1000.0) AT TIME ZONE 'UTC')", column, op, toMs))
	}
	return conds
}

// effectiveSearchLimit mirrors the LIMIT clamp applied by the SQL builders.
func effectiveSearchLimit(limit int) int {
	if limit <= 0 || limit > 50000 {
		return 50
	}
	return limit
}

func buildSearchSQLV4(lakeName string, req *SearchObjectV4, virtualRules map[string]services.VirtualFieldRule) (string, error) {
	return buildSearchSQLV4WithOpts(lakeName, req, virtualRules, searchSQLOpts{})
}

func buildSearchSQLV4WithOpts(lakeName string, req *SearchObjectV4, virtualRules map[string]services.VirtualFieldRule, opts searchSQLOpts) (string, error) {
	protoType := req.Filter.ProtoType
	if protoType == 0 {
		protoType = 1
	}
	transactionType := req.Filter.EventType
	tableName := getTableName(lakeName, protoType, transactionType)
	// OTLP virtual mappings (traces / metrics / logs) use a different
	// column layout than HEP — none of the SIP-specific columns
	// (caller, callee, method, response_code, src_port, dst_port,
	// node_id, payload, …) exist on otlp_*. Build SQL separately so we
	// don't generate WHERE clauses that fail at parse time.
	if isOTLPProtoType(protoType) {
		return buildOTLPSearchSQLV4(tableName, protoType, req, opts)
	}
	// Line Protocol virtual mappings address dynamic lp_<measurement>
	// tables whose only guaranteed columns are `time` and the user
	// tags/fields. None of the SIP-specific WHERE clauses below
	// (caller, callee, method, …) make sense for them — route to the
	// LP-specific builder.
	if isLPProtoType(protoType) {
		return buildLPSearchSQLV4(tableName, req, opts)
	}
	txType := normalizeSIPTransactionType(protoType, transactionType)

	conditions := make([]string, 0)
	conditions = append(conditions, timestampRangeConditions("timestamp", req.Timestamp.From, req.Timestamp.To, opts.toExclusive)...)
	// session_id / call_id / correlation: column layout depends on proto (see ducklake.TableSchema).
	sessionFilter := firstNonEmpty(req.Filter.CallID, req.Filter.SessionID)
	if sessionFilter != "" {
		switch protoType {
		case 53:
			conditions = append(conditions, sqlFormMatchClause("payload", sessionFilter))
		case 100:
			conditions = append(conditions, sqlFormMatchClause("session_id", sessionFilter))
		default:
			conditions = append(conditions, sqlFormMatchClauseOr("session_id", "cid", sessionFilter))
		}
	} else if req.Filter.CID != "" {
		switch protoType {
		case 53:
			conditions = append(conditions, sqlFormMatchClause("payload", req.Filter.CID))
		case 100:
			conditions = append(conditions, sqlFormMatchClause("session_id", req.Filter.CID))
		default:
			conditions = append(conditions, sqlFormMatchClause("cid", req.Filter.CID))
		}
	}
	// Party / identity filters — column layout depends on SIP profile (ducklake.TableSchema).
	if protoType == 1 {
		callerFilter := firstNonEmpty(req.Filter.FromUser, req.Filter.Caller)
		calleeFilter := firstNonEmpty(req.Filter.ToUser, req.Filter.Callee)
		switch txType {
		case "call":
			if callerFilter != "" {
				conditions = append(conditions, sqlFormMatchClause("caller", callerFilter))
			}
			if calleeFilter != "" {
				conditions = append(conditions, sqlFormMatchClause("callee", calleeFilter))
			}
		case "registration":
			aorFilter := firstNonEmpty(req.Filter.Aor, callerFilter)
			if aorFilter != "" {
				conditions = append(conditions, sqlFormMatchClause("aor", aorFilter))
			}
			contactFilter := firstNonEmpty(req.Filter.Contact, calleeFilter)
			if contactFilter != "" {
				conditions = append(conditions, sqlFormMatchClause("contact", contactFilter))
			}
			if ex := strings.TrimSpace(req.Filter.Expires); ex != "" {
				conditions = append(conditions, sqlFormMatchClause("expires", ex))
			}
		case "default":
			if callerFilter != "" {
				conditions = append(conditions, sqlFormMatchClause("json_extract_string(data_extra, '$.from_host')", callerFilter))
			}
			if calleeFilter != "" {
				conditions = append(conditions, sqlFormMatchClause("json_extract_string(data_extra, '$.to_host')", calleeFilter))
			}
		}
	}
	if protoType == 1 {
		methodVals := sipFilterMethodValues(req.Filter.Method, req.Filter.Methods)
		if len(methodVals) > 0 {
			escaped := make([]string, len(methodVals))
			for i, m := range methodVals {
				escaped[i] = "'" + sqlvalidator.SafeString(m) + "'"
			}
			conditions = append(conditions, "method IN ("+strings.Join(escaped, ",")+")")
		}
		rcVals := sipFilterResponseValues(req.Filter.ResponseCode, req.Filter.ResponseCodes)
		if len(rcVals) > 0 {
			escaped := make([]string, len(rcVals))
			for i, rc := range rcVals {
				escaped[i] = "'" + sqlvalidator.SafeString(rc) + "'"
			}
			conditions = append(conditions, "response_code IN ("+strings.Join(escaped, ",")+")")
		}
	}
	if protoType == 1 && txType == "call" && strings.TrimSpace(req.Filter.CseqMethod) != "" {
		conditions = append(conditions, fmt.Sprintf("cseq_method = '%s'", sqlvalidator.SafeString(strings.TrimSpace(req.Filter.CseqMethod))))
	}
	if req.Filter.SrcIP != "" {
		conditions = append(conditions, fmt.Sprintf("src_ip = '%s'", sqlvalidator.SafeString(req.Filter.SrcIP)))
	}
	if req.Filter.DstIP != "" {
		conditions = append(conditions, fmt.Sprintf("dst_ip = '%s'", sqlvalidator.SafeString(req.Filter.DstIP)))
	}
	if req.Filter.SrcPort > 0 && protoType != 100 {
		conditions = append(conditions, fmt.Sprintf("src_port = %d", req.Filter.SrcPort))
	}
	if req.Filter.DstPort > 0 && protoType != 100 {
		conditions = append(conditions, fmt.Sprintf("dst_port = %d", req.Filter.DstPort))
	}
	if req.Filter.CaptureID > 0 {
		capStr := sqlvalidator.SafeString(strconv.Itoa(req.Filter.CaptureID))
		// Classic Homer captureId corresponds to HEP chunk 0x000c (capture agent); we persist it as node_id.
		// Also match capture_id embedded in data_extra when present (custom ingest).
		conditions = append(conditions, fmt.Sprintf(
			"(CAST(node_id AS VARCHAR) = '%s' OR json_extract_string(data_extra, '$.capture_id') = '%s' OR json_extract_string(data_extra, '$.captureId') = '%s')",
			capStr, capStr, capStr))
	}
	// node / node_id
	nodeFilter := firstNonEmpty(req.Filter.Node, req.Filter.NodeID)
	if nodeFilter != "" {
		conditions = append(conditions, fmt.Sprintf("node_id = '%s'", sqlvalidator.SafeString(nodeFilter)))
	}
	if ua := strings.TrimSpace(req.Filter.UserAgent); ua != "" {
		if protoType == 1 && txType == "registration" {
			conditions = append(conditions, sqlFormMatchClause("user_agent", ua))
		} else {
			conditions = append(conditions, sqlFormMatchClause("json_extract_string(data_extra, '$.user_agent')", ua))
		}
	}
	if ru := strings.TrimSpace(req.Filter.RuriUser); ru != "" {
		conditions = append(conditions, sqlFormMatchClause("json_extract_string(data_extra, '$.request_uri')", ru))
	}
	if req.Filter.Payload != "" {
		conditions = append(conditions, sqlFormMatchClause("payload", req.Filter.Payload))
	}
	conditions = appendVirtualDataExtraConditions(conditions, req, virtualRules)

	// Validate and use custom SELECT columns/aggregations or default to SELECT *
	selectClause := "*"
	if strings.TrimSpace(req.Param.Select) != "" {
		if err := sqlvalidator.ValidateExpression(req.Param.Select, sqlvalidator.ExprSelect); err != nil {
			return "", fmt.Errorf("invalid select: %w", err)
		}
		selectClause = strings.TrimSpace(req.Param.Select)
	} else if opts.narrowNoHeavy {
		// Phase 1: uuid+timestamp only; hydrateHeavyColumns loads full rows.
		selectClause = narrowProjectionExpr
	}

	sql := fmt.Sprintf("SELECT %s FROM %s", selectClause, tableName)
	if len(conditions) > 0 {
		sql += " WHERE " + strings.Join(conditions, " AND ")
	}

	// GROUP BY (for aggregation queries) -- validate
	if strings.TrimSpace(req.Param.GroupBy) != "" {
		if err := sqlvalidator.ValidateExpression(req.Param.GroupBy, sqlvalidator.ExprGroupBy); err != nil {
			return "", fmt.Errorf("invalid group_by: %w", err)
		}
		sql += " GROUP BY " + strings.TrimSpace(req.Param.GroupBy)
	}

	// ORDER BY: use custom or default to timestamp DESC -- validate
	if strings.TrimSpace(req.Param.OrderBy) != "" {
		if err := sqlvalidator.ValidateExpression(req.Param.OrderBy, sqlvalidator.ExprOrderBy); err != nil {
			return "", fmt.Errorf("invalid order_by: %w", err)
		}
		sql += " ORDER BY " + strings.TrimSpace(req.Param.OrderBy)
	} else if !opts.noOrderBy {
		// noOrderBy drops the default timestamp-DESC sort so the query streams
		// (the caller re-sorts in Go). A custom order_by is always honored.
		sql += " ORDER BY timestamp DESC"
	}

	sql += fmt.Sprintf(" LIMIT %d", effectiveSearchLimit(req.Param.Limit))
	return sql, nil
}

// buildOTLPSearchSQLV4 builds a SELECT for the OTLP virtual mappings. The
// OTLP DuckLake tables (otlp_traces / otlp_metrics / otlp_logs) live
// outside the hep_proto_* schema and expose their own columns — see
// storage/ducklake/otlp_storage.go. Generic Filter fields that do not map
// cleanly onto OTLP columns (method, response_code, caller/callee, src/dst
// ports, node_id, …) are intentionally ignored; cross-cutting filters
// (call/session id, payload contains, src_ip/dst_ip when meaningful) are
// translated to the appropriate OTLP column or scoped to the raw blob.
func buildOTLPSearchSQLV4(tableName string, protoType int, req *SearchObjectV4, opts searchSQLOpts) (string, error) {
	conditions := make([]string, 0, 8)
	conditions = append(conditions, timestampRangeConditions("timestamp", req.Timestamp.From, req.Timestamp.To, opts.toExclusive)...)
	// Trace / span correlation. session_id is treated as a synonym for
	// trace_id so callers wired up for HEP CID-style searches still work.
	// OTLP metrics: explicit filter.name (exact) takes precedence over
	// call_id/session_id → name LIKE (legacy).
	if protoType == otlpHepIDMetrics {
		if n := strings.TrimSpace(req.Filter.Name); n != "" {
			conditions = append(conditions, fmt.Sprintf("name = '%s'", sqlvalidator.SafeString(n)))
		} else {
			traceFilter := firstNonEmpty(req.Filter.CallID, req.Filter.SessionID, req.Filter.CID)
			if traceFilter != "" {
				conditions = append(conditions, sqlFormMatchClause("name", traceFilter))
			}
		}
	} else {
		traceFilter := firstNonEmpty(req.Filter.CallID, req.Filter.SessionID, req.Filter.CID)
		if traceFilter != "" {
			esc := sqlvalidator.SafeString(traceFilter)
			conditions = append(conditions, fmt.Sprintf("trace_id = '%s'", esc))
		}
	}
	if protoType == otlpHepIDMetrics {
		typeVals := make([]string, 0, len(req.Filter.Types)+1)
		seen := make(map[string]struct{})
		for _, t := range req.Filter.Types {
			s := strings.TrimSpace(t)
			if s == "" {
				continue
			}
			if _, ok := seen[s]; ok {
				continue
			}
			seen[s] = struct{}{}
			typeVals = append(typeVals, s)
		}
		if ts := strings.TrimSpace(req.Filter.Type); ts != "" {
			if _, ok := seen[ts]; !ok {
				typeVals = append(typeVals, ts)
			}
		}
		if len(typeVals) > 0 {
			parts := make([]string, len(typeVals))
			for i, tv := range typeVals {
				parts[i] = "'" + sqlvalidator.SafeString(tv) + "'"
			}
			conditions = append(conditions, `"type" IN (`+strings.Join(parts, ",")+`)`)
		}
	}
	// Free-text search: for traces/logs a payload filter most naturally
	// maps onto the body / span name, with raw JSON as a fallback. For
	// metrics it filters by metric name.
	if p := strings.TrimSpace(req.Filter.Payload); p != "" {
		switch protoType {
		case otlpHepIDLogs:
			conditions = append(conditions, sqlFormMatchClauseAny([]string{"body", "CAST(raw AS VARCHAR)"}, p))
		case otlpHepIDMetrics:
			conditions = append(conditions, sqlFormMatchClauseAny([]string{"name", "CAST(raw AS VARCHAR)"}, p))
		default:
			conditions = append(conditions, sqlFormMatchClauseAny([]string{"name", "CAST(raw AS VARCHAR)"}, p))
		}
	}
	// service_name: dedicated field for OTLP (mapping id service_name) or
	// legacy UserAgent slot used as service substring search.
	if sn := strings.TrimSpace(req.Filter.ServiceName); sn != "" {
		conditions = append(conditions, sqlFormMatchClause("service_name", sn))
	} else if ua := strings.TrimSpace(req.Filter.UserAgent); ua != "" {
		conditions = append(conditions, sqlFormMatchClause("service_name", ua))
	}

	selectClause := "*"
	if strings.TrimSpace(req.Param.Select) != "" {
		if err := sqlvalidator.ValidateExpression(req.Param.Select, sqlvalidator.ExprSelect); err != nil {
			return "", fmt.Errorf("invalid select: %w", err)
		}
		selectClause = strings.TrimSpace(req.Param.Select)
	}

	sql := fmt.Sprintf("SELECT %s FROM %s", selectClause, tableName)
	if len(conditions) > 0 {
		sql += " WHERE " + strings.Join(conditions, " AND ")
	}
	if strings.TrimSpace(req.Param.GroupBy) != "" {
		if err := sqlvalidator.ValidateExpression(req.Param.GroupBy, sqlvalidator.ExprGroupBy); err != nil {
			return "", fmt.Errorf("invalid group_by: %w", err)
		}
		sql += " GROUP BY " + strings.TrimSpace(req.Param.GroupBy)
	}
	if strings.TrimSpace(req.Param.OrderBy) != "" {
		if err := sqlvalidator.ValidateExpression(req.Param.OrderBy, sqlvalidator.ExprOrderBy); err != nil {
			return "", fmt.Errorf("invalid order_by: %w", err)
		}
		sql += " ORDER BY " + strings.TrimSpace(req.Param.OrderBy)
	} else {
		sql += " ORDER BY timestamp DESC"
	}
	sql += fmt.Sprintf(" LIMIT %d", effectiveSearchLimit(req.Param.Limit))
	return sql, nil
}

// buildLPSearchSQLV4 builds a SELECT for a Line Protocol virtual
// mapping (hepid 300). LP tables are dynamic — the only column we can
// always assume is `time` (TIMESTAMP, written by the receiver). All
// other columns are user-defined tags/fields.
//
// Filter handling intentionally mirrors buildOTLPSearchSQLV4: timestamp
// range maps to `time`, free-text Payload becomes a CAST-to-VARCHAR
// LIKE on the row, and SIP-specific filters (method, response_code,
// caller/callee, src/dst ports, node_id, …) are silently ignored.
//
// SELECT/GROUP BY/ORDER BY clauses pass through the same sqlvalidator
// gate as elsewhere so callers can craft custom widgets without
// compromising the SQL injection posture.
func buildLPSearchSQLV4(tableName string, req *SearchObjectV4, opts searchSQLOpts) (string, error) {
	conditions := make([]string, 0, 4)
	conditions = append(conditions, timestampRangeConditions("time", req.Timestamp.From, req.Timestamp.To, opts.toExclusive)...)
	// Free-text payload search: dynamic schemas mean we don't know
	// which column the user wants, so cast the whole row to VARCHAR
	// via DuckDB's record stringifier and LIKE on it. Slow on huge
	// tables, but correct, and only kicks in when the operator
	// actually types something.
	if p := strings.TrimSpace(req.Filter.Payload); p != "" {
		conditions = append(conditions, sqlFormMatchClause("CAST(ROW(*) AS VARCHAR)", p))
	}

	selectClause := "*"
	if strings.TrimSpace(req.Param.Select) != "" {
		if err := sqlvalidator.ValidateExpression(req.Param.Select, sqlvalidator.ExprSelect); err != nil {
			return "", fmt.Errorf("invalid select: %w", err)
		}
		selectClause = strings.TrimSpace(req.Param.Select)
	}

	sql := fmt.Sprintf("SELECT %s FROM %s", selectClause, tableName)
	if len(conditions) > 0 {
		sql += " WHERE " + strings.Join(conditions, " AND ")
	}
	if strings.TrimSpace(req.Param.GroupBy) != "" {
		if err := sqlvalidator.ValidateExpression(req.Param.GroupBy, sqlvalidator.ExprGroupBy); err != nil {
			return "", fmt.Errorf("invalid group_by: %w", err)
		}
		sql += " GROUP BY " + strings.TrimSpace(req.Param.GroupBy)
	}
	if strings.TrimSpace(req.Param.OrderBy) != "" {
		if err := sqlvalidator.ValidateExpression(req.Param.OrderBy, sqlvalidator.ExprOrderBy); err != nil {
			return "", fmt.Errorf("invalid order_by: %w", err)
		}
		sql += " ORDER BY " + strings.TrimSpace(req.Param.OrderBy)
	} else {
		sql += " ORDER BY time DESC"
	}
	sql += fmt.Sprintf(" LIMIT %d", effectiveSearchLimit(req.Param.Limit))
	return sql, nil
}

// firstNonEmpty returns the first non-empty string from the provided values.
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
