package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/sipcapture/homer-core/src/config"
	"github.com/sipcapture/homer-core/src/coordinator/sqlvalidator"
	logger "github.com/sipcapture/homer-core/src/utils/logging"
)

type Module struct {
	cfg        *config.MCPConfig
	httpClient *http.Client
	llm        *LLMClient
	server     *server.MCPServer

	mu      sync.Mutex
	running bool
	cancel  context.CancelFunc
	done    chan struct{}
	lastErr error
}

const (
	parserAuto  = "auto"
	parserLLM   = "llm"
	parserRegex = "regex"
)

type structuredArgs struct {
	QueryText    string `json:"query_text" jsonschema_description:"Natural language query" jsonschema:"required"`
	NowUTCUnixMS int64  `json:"now_utc_unix_ms,omitempty" jsonschema_description:"Optional current time in UTC milliseconds"`
	Limit        int    `json:"limit,omitempty" jsonschema_description:"Max rows for structured search (1..1000)"`
	Parser       string `json:"parser,omitempty" jsonschema_description:"Parser strategy: auto|llm|regex (default: auto)"`
}

type sqlArgs struct {
	QueryText string `json:"query_text" jsonschema_description:"Natural language query" jsonschema:"required"`
	Limit     int    `json:"limit,omitempty" jsonschema_description:"Max rows for SQL mode (1..50000)"`
	Parser    string `json:"parser,omitempty" jsonschema_description:"Parser strategy: auto|llm|regex (default: auto)"`
}

type hybridArgs struct {
	QueryText    string `json:"query_text" jsonschema_description:"Natural language query" jsonschema:"required"`
	Mode         string `json:"mode,omitempty" jsonschema_description:"auto|structured|sql"`
	NowUTCUnixMS int64  `json:"now_utc_unix_ms,omitempty" jsonschema_description:"Optional current time in UTC milliseconds"`
	Limit        int    `json:"limit,omitempty" jsonschema_description:"Max rows"`
	Parser       string `json:"parser,omitempty" jsonschema_description:"Parser strategy: auto|llm|regex (default: auto)"`
}

// parserMeta describes which NL parser produced the structured filters and
// auxiliary diagnostics that are exposed through the MCP tool meta.
type parserMeta struct {
	Used      string `json:"parser_used"`
	Model     string `json:"llm_model,omitempty"`
	LatencyMS int64  `json:"llm_latency_ms,omitempty"`
	Error     string `json:"llm_error,omitempty"`
}

type apiResponse struct {
	Data struct {
		Items []map[string]any `json:"items"`
		Keys  []string         `json:"keys"`
	} `json:"data"`
	Meta map[string]any `json:"meta"`
}

type searchPayload struct {
	Filter struct {
		ProtoType int    `json:"proto_type"`
		EventType string `json:"event_type"`
		Method    string `json:"method,omitempty"`
		CallID    string `json:"call_id,omitempty"`
		FromUser  string `json:"from_user,omitempty"`
		ToUser    string `json:"to_user,omitempty"`
		SrcIP     string `json:"src_ip,omitempty"`
		DstIP     string `json:"dst_ip,omitempty"`
	} `json:"filter"`
	Param struct {
		Limit   int    `json:"limit"`
		OrderBy string `json:"order_by"`
	} `json:"param"`
	Timestamp struct {
		From int64 `json:"from"`
		To   int64 `json:"to"`
	} `json:"timestamp"`
}

var (
	sipMethods = []string{
		"INVITE", "BYE", "REGISTER", "OPTIONS", "ACK", "CANCEL", "PRACK",
		"UPDATE", "INFO", "REFER", "SUBSCRIBE", "NOTIFY", "PUBLISH", "MESSAGE",
	}
)

func New(cfg *config.MCPConfig) (*Module, error) {
	if cfg == nil {
		return nil, fmt.Errorf("mcp config is nil")
	}

	mode := strings.ToLower(strings.TrimSpace(cfg.Mode))
	if mode == "" {
		mode = "hybrid"
	}
	if mode != "hybrid" && mode != "structured" && mode != "sql" {
		return nil, fmt.Errorf("invalid mcp.mode: %q", cfg.Mode)
	}

	baseURL := strings.TrimSpace(cfg.HomerBaseURL)
	if baseURL == "" {
		return nil, fmt.Errorf("mcp.homer_base_url is required")
	}

	if cfg.DefaultLimit <= 0 {
		cfg.DefaultLimit = 100
	}
	if cfg.SQLDefaultLimit <= 0 {
		cfg.SQLDefaultLimit = 100
	}
	if cfg.RequestTimeoutSec <= 0 {
		cfg.RequestTimeoutSec = 30
	}
	cfg.Mode = mode
	cfg.HomerBaseURL = strings.TrimRight(baseURL, "/")

	m := &Module{
		cfg: cfg,
		httpClient: &http.Client{
			Timeout: time.Duration(cfg.RequestTimeoutSec) * time.Second,
		},
		llm: NewLLMClient(&cfg.LLM),
	}
	m.server = m.buildServer()
	return m, nil
}

func (m *Module) Start() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.running {
		return fmt.Errorf("mcp module already running")
	}

	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	m.done = make(chan struct{})
	m.lastErr = nil
	m.running = true

	go func() {
		defer close(m.done)
		err := server.NewStdioServer(m.server).Listen(ctx, os.Stdin, os.Stdout)
		if err != nil && !errors.Is(err, context.Canceled) {
			logger.Error(fmt.Sprintf("MCP server stopped with error: %v", err))
			m.mu.Lock()
			m.lastErr = err
			m.mu.Unlock()
		}
	}()

	logger.Info("MCP module started", "mode", m.cfg.Mode, "base_url", m.cfg.HomerBaseURL)
	return nil
}

func (m *Module) Stop() error {
	m.mu.Lock()
	if !m.running {
		m.mu.Unlock()
		return nil
	}
	cancel := m.cancel
	done := m.done
	m.running = false
	m.mu.Unlock()

	if cancel != nil {
		cancel()
	}

	if done != nil {
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			return fmt.Errorf("timeout stopping mcp module")
		}
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	return m.lastErr
}

// Run blocks and serves stdio directly (useful for `homer mcp` command).
func (m *Module) Run() error {
	logger.Info("Starting MCP stdio server", "mode", m.cfg.Mode, "base_url", m.cfg.HomerBaseURL)
	return server.ServeStdio(m.server)
}

func (m *Module) buildServer() *server.MCPServer {
	s := server.NewMCPServer("homer-core-mcp", "0.1.0", server.WithToolCapabilities(false))

	structuredTool := mcp.NewTool("homer_search_transactions",
		mcp.WithDescription("Structured search: natural language -> /api/v4/transactions/search"),
		mcp.WithInputSchema[structuredArgs](),
	)
	s.AddTool(structuredTool, mcp.NewTypedToolHandler(m.handleStructuredTool))

	sqlTool := mcp.NewTool("homer_query_sql",
		mcp.WithDescription("SQL search: natural language -> generated SQL -> /api/v4/query"),
		mcp.WithInputSchema[sqlArgs](),
	)
	s.AddTool(sqlTool, mcp.NewTypedToolHandler(m.handleSQLTool))

	hybridTool := mcp.NewTool("homer_query",
		mcp.WithDescription("Hybrid search: structured by default, SQL on explicit request"),
		mcp.WithInputSchema[hybridArgs](),
	)
	s.AddTool(hybridTool, mcp.NewTypedToolHandler(m.handleHybridTool))
	return s
}

func (m *Module) handleStructuredTool(ctx context.Context, _ mcp.CallToolRequest, args structuredArgs) (*mcp.CallToolResult, error) {
	result, err := m.runStructured(ctx, args)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return toolResultJSON(result)
}

func (m *Module) handleSQLTool(ctx context.Context, _ mcp.CallToolRequest, args sqlArgs) (*mcp.CallToolResult, error) {
	result, err := m.runSQL(ctx, args)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return toolResultJSON(result)
}

func (m *Module) handleHybridTool(ctx context.Context, _ mcp.CallToolRequest, args hybridArgs) (*mcp.CallToolResult, error) {
	result, err := m.runHybrid(ctx, args)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return toolResultJSON(result)
}

// normalizeParserHint coerces user input into one of {auto, llm, regex}.
func normalizeParserHint(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case parserLLM:
		return parserLLM
	case parserRegex:
		return parserRegex
	default:
		return parserAuto
	}
}

func toolResultJSON(v any) (*mcp.CallToolResult, error) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, err
	}
	return mcp.NewToolResultText(string(b)), nil
}

func (m *Module) runStructured(ctx context.Context, args structuredArgs) (map[string]any, error) {
	payload, normalized, pmeta, err := m.parseQuery(ctx, args.QueryText, args.NowUTCUnixMS, args.Limit, args.Parser)
	if err != nil {
		return nil, err
	}

	var resp apiResponse
	if err := m.postJSON(ctx, "/api/v4/transactions/search", payload, &resp); err != nil {
		return nil, err
	}

	return map[string]any{
		"mode":               "structured",
		"normalized_filters": normalized,
		"api_payload":        payload,
		"total":              len(resp.Data.Items),
		"items":              resp.Data.Items,
		"keys":               resp.Data.Keys,
		"meta":               mergeMeta(resp.Meta, pmeta),
	}, nil
}

func (m *Module) runSQL(ctx context.Context, args sqlArgs) (map[string]any, error) {
	payload, _, pmeta, err := m.parseQuery(ctx, args.QueryText, 0, args.Limit, args.Parser)
	if err != nil {
		return nil, err
	}
	sql := buildSQL(payload)
	if err := validateSQL(sql); err != nil {
		return nil, err
	}

	limit := clampLimit(args.Limit, m.cfg.SQLDefaultLimit, 50000)
	body := map[string]any{"sql": sql, "limit": limit}

	var resp apiResponse
	if err := m.postJSON(ctx, "/api/v4/query", body, &resp); err != nil {
		return nil, err
	}

	return map[string]any{
		"mode":          "sql",
		"generated_sql": sql,
		"total":         len(resp.Data.Items),
		"items":         resp.Data.Items,
		"keys":          resp.Data.Keys,
		"meta":          mergeMeta(resp.Meta, pmeta),
	}, nil
}

func (m *Module) runHybrid(ctx context.Context, args hybridArgs) (map[string]any, error) {
	mode := strings.ToLower(strings.TrimSpace(args.Mode))
	if mode == "" {
		mode = "auto"
	}

	effectiveMode := mode
	if m.cfg.Mode == "structured" || m.cfg.Mode == "sql" {
		effectiveMode = m.cfg.Mode
	}
	if effectiveMode == "auto" {
		if shouldUseSQLMode(args.QueryText) {
			effectiveMode = "sql"
		} else {
			effectiveMode = "structured"
		}
	}

	switch effectiveMode {
	case "sql":
		return m.runSQL(ctx, sqlArgs{QueryText: args.QueryText, Limit: args.Limit, Parser: args.Parser})
	case "structured":
		return m.runStructured(ctx, structuredArgs{
			QueryText:    args.QueryText,
			NowUTCUnixMS: args.NowUTCUnixMS,
			Limit:        args.Limit,
			Parser:       args.Parser,
		})
	default:
		return nil, fmt.Errorf("unsupported mode: %s", effectiveMode)
	}
}

// parseQuery converts a natural-language query into a structured searchPayload.
// Strategy is controlled by parserHint:
//   - "regex": always use the deterministic regex parser
//   - "llm":   require LLM, return error if it is disabled or fails
//   - "auto" (default): try LLM first when configured, fall back to regex on
//     any error. Missing fields in LLM output are filled in from regex hints
//     so that LLM and regex complement each other.
func (m *Module) parseQuery(
	ctx context.Context,
	queryText string,
	nowMS int64,
	limit int,
	parserHint string,
) (searchPayload, map[string]any, parserMeta, error) {
	hint := normalizeParserHint(parserHint)
	now := nowMS
	if now <= 0 {
		now = time.Now().UTC().UnixMilli()
	}

	regexPayload, regexNorm := m.buildStructuredPayload(queryText, now, limit)

	if hint == parserRegex || m.llm == nil {
		if hint == parserLLM {
			return searchPayload{}, nil, parserMeta{}, fmt.Errorf("parser=llm requested but mcp.llm.enable=false")
		}
		return regexPayload, regexNorm, parserMeta{Used: parserRegex}, nil
	}

	llmRes, err := m.llm.Parse(ctx, queryText, now)
	if err != nil {
		if hint == parserLLM {
			return searchPayload{}, nil, parserMeta{}, fmt.Errorf("llm parse failed: %w", err)
		}
		logger.Warn("MCP LLM parse failed, falling back to regex", "error", err.Error())
		return regexPayload, regexNorm, parserMeta{
			Used:  "regex_fallback",
			Model: m.cfg.LLM.Model,
			Error: err.Error(),
		}, nil
	}

	llmPayload, llmNorm := m.applyLLMFilters(queryText, now, limit, llmRes.Filters, regexPayload, regexNorm)
	return llmPayload, llmNorm, parserMeta{
		Used:      parserLLM,
		Model:     llmRes.Model,
		LatencyMS: llmRes.LatencyMS,
	}, nil
}

// applyLLMFilters merges LLM-extracted filters with the regex baseline.
// LLM values win on conflicts; empty LLM values fall through to regex hints so
// the model never has to repeat what regex can already extract reliably.
func (m *Module) applyLLMFilters(
	queryText string,
	nowMS int64,
	limit int,
	llm *llmParsedFilters,
	regexPayload searchPayload,
	regexNorm map[string]any,
) (searchPayload, map[string]any) {
	payload := regexPayload
	if llm == nil {
		return payload, regexNorm
	}

	if v := strings.ToUpper(strings.TrimSpace(llm.Method)); v != "" {
		payload.Filter.Method = v
	}
	if v := strings.TrimSpace(llm.SrcIP); v != "" {
		payload.Filter.SrcIP = v
	}
	if v := strings.TrimSpace(llm.DstIP); v != "" {
		payload.Filter.DstIP = v
	}
	if v := strings.TrimSpace(llm.CallID); v != "" {
		payload.Filter.CallID = v
	}
	if v := strings.TrimSpace(llm.FromUser); v != "" {
		payload.Filter.FromUser = v
	}
	if v := strings.TrimSpace(llm.ToUser); v != "" {
		payload.Filter.ToUser = v
	}

	timeLabel, _ := regexNorm["time_range"].(string)
	if llm.FromMS > 0 && llm.ToMS > 0 && llm.ToMS >= llm.FromMS {
		payload.Timestamp.From = llm.FromMS
		payload.Timestamp.To = llm.ToMS
		if label := strings.TrimSpace(llm.TimeRangeLabel); label != "" {
			timeLabel = label
		} else {
			timeLabel = "llm_custom"
		}
	}

	payload.Param.Limit = clampLimit(limit, m.cfg.DefaultLimit, 1000)

	normalized := map[string]any{
		"query_text": queryText,
		"method":     nullIfEmpty(payload.Filter.Method),
		"time_range": timeLabel,
		"src_ip":     nullIfEmpty(payload.Filter.SrcIP),
		"dst_ip":     nullIfEmpty(payload.Filter.DstIP),
		"call_id":    nullIfEmpty(payload.Filter.CallID),
		"from_user":  nullIfEmpty(payload.Filter.FromUser),
		"to_user":    nullIfEmpty(payload.Filter.ToUser),
	}
	_ = nowMS
	return payload, normalized
}

// mergeMeta folds parserMeta diagnostics into the upstream API meta map so that
// callers always see which parser produced the filters.
func mergeMeta(apiMeta map[string]any, p parserMeta) map[string]any {
	out := apiMeta
	if out == nil {
		out = map[string]any{}
	}
	if p.Used != "" {
		out["parser_used"] = p.Used
	}
	if p.Model != "" {
		out["llm_model"] = p.Model
	}
	if p.LatencyMS > 0 {
		out["llm_latency_ms"] = p.LatencyMS
	}
	if p.Error != "" {
		out["llm_error"] = p.Error
	}
	return out
}

func (m *Module) postJSON(ctx context.Context, endpoint string, body any, out any) error {
	token := strings.TrimSpace(m.cfg.HomerToken)
	if token == "" {
		return fmt.Errorf("mcp.homer_token is required")
	}

	requestBody, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, m.cfg.HomerBaseURL+endpoint, bytes.NewReader(requestBody))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("http request failed: %w", err)
	}
	defer resp.Body.Close()

	respBytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("api error %d: %s", resp.StatusCode, strings.TrimSpace(string(respBytes)))
	}
	if err := json.Unmarshal(respBytes, out); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

func (m *Module) buildStructuredPayload(queryText string, nowUTCUnixMS int64, limit int) (searchPayload, map[string]any) {
	now := nowUTCUnixMS
	if now <= 0 {
		now = time.Now().UTC().UnixMilli()
	}
	from, to, rangeLabel := parseTimeRange(queryText, now)

	payload := searchPayload{}
	payload.Filter.ProtoType = 1
	payload.Filter.EventType = "call"
	payload.Param.Limit = clampLimit(limit, m.cfg.DefaultLimit, 1000)
	payload.Param.OrderBy = "timestamp DESC"
	payload.Timestamp.From = from
	payload.Timestamp.To = to

	payload.Filter.Method = extractSIPMethod(queryText)
	payload.Filter.SrcIP = extractPattern(queryText, `(?:src|source)\s+ip[:=]?\s*([0-9.]+)`)
	payload.Filter.DstIP = extractPattern(queryText, `(?:dst|destination)\s+ip[:=]?\s*([0-9.]+)`)
	payload.Filter.CallID = extractPattern(queryText, `(?:call[_\s-]?id|session[_\s-]?id)[:=]?\s*([^\s,]+)`)
	payload.Filter.FromUser = extractPattern(queryText, `(?:from[_\s-]?user|caller)[:=]?\s*([^\s,]+)`)
	payload.Filter.ToUser = extractPattern(queryText, `(?:to[_\s-]?user|callee)[:=]?\s*([^\s,]+)`)

	normalized := map[string]any{
		"query_text": queryText,
		"method":     nullIfEmpty(payload.Filter.Method),
		"time_range": rangeLabel,
		"src_ip":     nullIfEmpty(payload.Filter.SrcIP),
		"dst_ip":     nullIfEmpty(payload.Filter.DstIP),
		"call_id":    nullIfEmpty(payload.Filter.CallID),
	}
	return payload, normalized
}

func clampLimit(value, fallback, max int) int {
	if value <= 0 {
		value = fallback
	}
	if value <= 0 {
		value = 100
	}
	if value > max {
		value = max
	}
	return value
}

func parseTimeRange(queryText string, now int64) (from int64, to int64, label string) {
	q := strings.ToLower(queryText)
	const hour = int64(time.Hour / time.Millisecond)
	const day = int64((24 * time.Hour) / time.Millisecond)

	switch {
	case strings.Contains(q, "last hour"):
		return now - hour, now, "last_hour"
	case strings.Contains(q, "today") || strings.Contains(q, "heute"):
		t := time.UnixMilli(now).UTC()
		start := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC).UnixMilli()
		return start, now, "today"
	case strings.Contains(q, "yesterday") || strings.Contains(q, "gestern"):
		t := time.UnixMilli(now).UTC()
		startToday := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC).UnixMilli()
		return startToday - day, startToday - 1, "yesterday"
	default:
		return now - hour, now, "default_last_hour"
	}
}

func extractSIPMethod(queryText string) string {
	q := strings.ToUpper(queryText)
	for _, method := range sipMethods {
		re := regexp.MustCompile(`\b` + method + `\b`)
		if re.MatchString(q) {
			return method
		}
	}
	return ""
}

func extractPattern(queryText, pattern string) string {
	re := regexp.MustCompile(`(?i)` + pattern)
	m := re.FindStringSubmatch(queryText)
	if len(m) < 2 {
		return ""
	}
	return strings.TrimSpace(m[1])
}

func nullIfEmpty(v string) any {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	return v
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

func buildSQL(payload searchPayload) string {
	parts := []string{
		fmt.Sprintf("timestamp >= (to_timestamp(%d / 1000.0) AT TIME ZONE 'UTC')", payload.Timestamp.From),
		fmt.Sprintf("timestamp <= (to_timestamp(%d / 1000.0) AT TIME ZONE 'UTC')", payload.Timestamp.To),
	}

	if payload.Filter.Method != "" {
		parts = append(parts, fmt.Sprintf("method = '%s'", escapeSQL(payload.Filter.Method)))
	}
	if payload.Filter.CallID != "" {
		callID := escapeSQL(payload.Filter.CallID)
		parts = append(parts, fmt.Sprintf("(session_id LIKE '%%%s%%' OR cid LIKE '%%%s%%')", callID, callID))
	}
	if payload.Filter.FromUser != "" {
		parts = append(parts, fmt.Sprintf("caller LIKE '%%%s%%'", escapeSQL(payload.Filter.FromUser)))
	}
	if payload.Filter.ToUser != "" {
		parts = append(parts, fmt.Sprintf("callee LIKE '%%%s%%'", escapeSQL(payload.Filter.ToUser)))
	}
	if payload.Filter.SrcIP != "" {
		parts = append(parts, fmt.Sprintf("src_ip = '%s'", escapeSQL(payload.Filter.SrcIP)))
	}
	if payload.Filter.DstIP != "" {
		parts = append(parts, fmt.Sprintf("dst_ip = '%s'", escapeSQL(payload.Filter.DstIP)))
	}

	limit := clampLimit(payload.Param.Limit, 100, 50000)
	return fmt.Sprintf(
		"SELECT * FROM homer_lake.main.hep_proto_1_call WHERE %s ORDER BY timestamp DESC LIMIT %d",
		strings.Join(parts, " AND "),
		limit,
	)
}

func escapeSQL(v string) string {
	return strings.ReplaceAll(v, "'", "''")
}

func validateSQL(sql string) error {
	trimmed := strings.TrimSpace(sql)
	lower := strings.ToLower(trimmed)
	if !(strings.HasPrefix(lower, "select ") || strings.HasPrefix(lower, "with ")) {
		return fmt.Errorf("only SELECT/WITH is allowed")
	}
	if strings.Contains(trimmed, ";") {
		return fmt.Errorf("semicolon is not allowed")
	}
	if !strings.Contains(lower, "homer_lake.main.hep_proto_1_call") {
		return fmt.Errorf("only homer_lake.main.hep_proto_1_call is allowed")
	}

	if sqlvalidator.ContainsForbiddenIdentifier(trimmed, sqlvalidator.ForbiddenReadOnlyKeywords) {
		return fmt.Errorf("forbidden SQL token")
	}
	return nil
}
