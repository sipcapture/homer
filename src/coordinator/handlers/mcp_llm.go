package handlers

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/sipcapture/homer-core/src/config"
	logger "github.com/sipcapture/homer-core/src/utils/logging"
)

// llmStructuredOutput is the JSON schema we ask the LLM to return when
// converting an NL query into Homer's structured search filters.
type llmStructuredOutput struct {
	Filter struct {
		Method   string `json:"method,omitempty"`
		CallID   string `json:"call_id,omitempty"`
		FromUser string `json:"from_user,omitempty"`
		ToUser   string `json:"to_user,omitempty"`
		SrcIP    string `json:"src_ip,omitempty"`
		DstIP    string `json:"dst_ip,omitempty"`
	} `json:"filter"`
	Timestamp struct {
		From int64 `json:"from,omitempty"`
		To   int64 `json:"to,omitempty"`
	} `json:"timestamp"`
	Limit int `json:"limit,omitempty"`
}

// llmSQLOutput is the JSON schema we ask the LLM to return when converting
// an NL query into a single DuckDB SELECT statement.
type llmSQLOutput struct {
	SQL string `json:"sql"`
}

type llmStatusResponse struct {
	Data map[string]any `json:"data"`
	Meta Meta           `json:"meta"`
}

// llmRunResult bundles the LLM-derived value with diagnostics that flow into
// the response meta, so callers can show "parser=llm, model=X, took=Yms" in
// the UI without a separate round-trip.
type llmRunResult struct {
	Used      bool
	Model     string
	LatencyMS int64
	Err       error
}

// llmActive reports whether the handler has a live LLM client. Returns the
// effective config alongside, so status checks and ping endpoints can reuse
// the same source of truth without re-validating.
func (h *SearchHandler) llmActive() (*config.MCPLLMConfig, bool) {
	if h == nil || h.mcpConfig == nil {
		return nil, false
	}
	cfg := &h.mcpConfig.LLM
	if !cfg.Enable {
		return nil, false
	}
	if h.llm == nil || !h.llm.Enabled() {
		return nil, false
	}
	return cfg, true
}

func (h *SearchHandler) V4MCPLLMStatus(c echo.Context) error {
	if h == nil || h.mcpConfig == nil {
		return writeError(c, http.StatusInternalServerError, "Server Error", "MCP config is not initialized")
	}

	cfg := h.mcpConfig.LLM
	provider := strings.ToLower(strings.TrimSpace(cfg.Provider))
	if provider == "" {
		provider = "openai"
	}
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		model = "gpt-4o-mini"
	}

	keyConfigured := strings.TrimSpace(cfg.APIKey) != ""
	enabled := cfg.Enable

	checkParam := strings.ToLower(strings.TrimSpace(c.QueryParam("check")))
	shouldCheck := checkParam == "" || checkParam == "1" || checkParam == "true" || checkParam == "yes"
	checked := false
	reachable := false
	var detail string

	// Note: we no longer require keyConfigured to attempt a ping. Many
	// OpenAI-compatible providers (Ollama, vLLM, LM Studio) do not require an
	// Authorization header and would otherwise be incorrectly reported as
	// unreachable purely because the operator did not set an API key.
	if shouldCheck && enabled && baseURL != "" {
		checked = true
		if err := h.pingLLMProvider(c.Request().Context(), &cfg); err != nil {
			detail = err.Error()
		} else {
			reachable = true
			detail = "ok"
		}
	}

	resp := llmStatusResponse{
		Data: map[string]any{
			"enabled":            enabled,
			"provider":           provider,
			"provider_supported": true, // any OpenAI-compatible endpoint
			"base_url":           baseURL,
			"model":              model,
			"key_configured":     keyConfigured,
			"checked":            checked,
			"reachable":          reachable,
			"detail":             detail,
		},
		Meta: buildMeta(c, ""),
	}
	return c.JSON(http.StatusOK, resp)
}

func (h *SearchHandler) pingLLMProvider(ctx context.Context, cfg *config.MCPLLMConfig) error {
	if cfg == nil {
		return fmt.Errorf("llm config is nil")
	}
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	timeout := cfg.TimeoutSec
	if timeout <= 0 {
		timeout = 15
	}

	pingCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(pingCtx, http.MethodGet, baseURL+"/models", nil)
	if err != nil {
		return err
	}
	if key := strings.TrimSpace(cfg.APIKey); key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}

	client := &http.Client{Timeout: time.Duration(timeout) * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("provider check failed (%d): %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

// tryLLMStructured asks the LLM (when enabled) to convert queryText into a
// structured search request. The fallback values are used when the LLM omits
// time range or limit. The returned llmRunResult carries diagnostics suitable
// for the response meta even when the request itself ends up being nil
// (LLM disabled, or an error happened — which is still a useful signal in
// "auto" mode where the regex parser is the safety net).
func (h *SearchHandler) tryLLMStructured(ctx context.Context, queryText string, fallbackFrom, fallbackTo int64, fallbackLimit int) (*SearchObjectV4, llmRunResult) {
	cfg, ok := h.llmActive()
	if !ok {
		return nil, llmRunResult{}
	}

	systemPrompt := "You convert user search text into Homer structured JSON. Return ONLY valid JSON, no explanation."
	userPrompt := fmt.Sprintf(
		`Query: %s
Defaults: proto_type=1, event_type="call", timestamp.from=%d, timestamp.to=%d, limit=%d.
Filter field mapping (use these exact keys):
  method      - SIP method (INVITE, BYE, REGISTER, etc.)
  call_id     - SIP Call-ID header, also known as CID, cid, call_id, callid, session_id
  from_user   - calling party / caller / from_user / from
  to_user     - called party / callee / to_user / to
  src_ip      - source IP address
  dst_ip      - destination IP address
Return JSON shape (omit empty string fields):
{"filter":{"method":"","call_id":"","from_user":"","to_user":"","src_ip":"","dst_ip":""},"timestamp":{"from":0,"to":0},"limit":0}`,
		queryText, fallbackFrom, fallbackTo, fallbackLimit,
	)

	var parsed llmStructuredOutput
	model, latency, err := h.llm.ChatJSON(ctx, systemPrompt, userPrompt, &parsed)
	if err != nil {
		return nil, llmRunResult{Used: false, Model: model, LatencyMS: latency, Err: err}
	}

	req := &SearchObjectV4{}
	req.Filter.ProtoType = 1
	req.Filter.EventType = "call"
	req.Filter.Method = strings.TrimSpace(parsed.Filter.Method)
	req.Filter.CallID = strings.TrimSpace(parsed.Filter.CallID)
	req.Filter.FromUser = strings.TrimSpace(parsed.Filter.FromUser)
	req.Filter.ToUser = strings.TrimSpace(parsed.Filter.ToUser)
	req.Filter.SrcIP = strings.TrimSpace(parsed.Filter.SrcIP)
	req.Filter.DstIP = strings.TrimSpace(parsed.Filter.DstIP)
	req.Timestamp.From = parsed.Timestamp.From
	req.Timestamp.To = parsed.Timestamp.To
	req.Param.Limit = parsed.Limit
	req.Param.OrderBy = "timestamp DESC"

	if req.Timestamp.From <= 0 {
		req.Timestamp.From = fallbackFrom
	}
	if req.Timestamp.To <= 0 {
		req.Timestamp.To = fallbackTo
	}
	req.Param.Limit = sanitizeLimit(req.Param.Limit, fallbackLimit, 1000)

	logger.Info("MCP LLM structured parser used", "provider", cfg.Provider, "model", model, "latency_ms", latency)
	return req, llmRunResult{Used: true, Model: model, LatencyMS: latency}
}

// tryLLMSQL asks the LLM (when enabled) to translate queryText into a single
// DuckDB SELECT. The returned string is empty when the LLM is disabled or the
// model gave us nothing usable; the llmRunResult carries diagnostics either
// way so the caller can surface them in the response meta.
func (h *SearchHandler) tryLLMSQL(ctx context.Context, queryText string, fallbackLimit int) (string, llmRunResult) {
	cfg, ok := h.llmActive()
	if !ok {
		return "", llmRunResult{}
	}

	systemPrompt := "You generate DuckDB SQL for Homer. Return ONLY valid JSON: {\"sql\":\"...\"}. SQL must be SELECT/WITH only, no semicolons."
	userPrompt := fmt.Sprintf(
		`Query: %s
Table: `+h.flightService.LakeName()+`.main.hep_proto_1_call
Columns:
  timestamp       TIMESTAMP  - packet capture time
  session_id      VARCHAR    - SIP dialog ID (from To-tag/From-tag)
  cid             VARCHAR    - SIP Call-ID header value (also called CID, call_id, callid)
  caller          VARCHAR    - calling party SIP URI / From user
  callee          VARCHAR    - called party SIP URI / To user
  src_ip          VARCHAR    - source IP address
  dst_ip          VARCHAR    - destination IP address
  src_port        UINTEGER   - source port
  dst_port        UINTEGER   - destination port
  method          VARCHAR    - SIP method (INVITE, BYE, REGISTER, OPTIONS, etc.)
  response_code   VARCHAR    - SIP response code (200, 404, 486, etc.)
  cseq_method     VARCHAR    - CSeq method
  protocol        UINTEGER   - transport protocol (1=UDP, 2=TCP, 3=TLS)
  node_id         VARCHAR    - capturing node identifier
  uuid            VARCHAR    - unique packet identifier
Rules:
- Use LIKE '%%value%%' for partial string matches
- Use = 'value' for exact matches
- Add ORDER BY timestamp DESC
- Include LIMIT <= %d
Return JSON shape: {"sql":"SELECT ..."}
`, queryText, fallbackLimit)

	var parsed llmSQLOutput
	model, latency, err := h.llm.ChatJSON(ctx, systemPrompt, userPrompt, &parsed)
	if err != nil {
		return "", llmRunResult{Used: false, Model: model, LatencyMS: latency, Err: err}
	}

	sql := strings.TrimSpace(parsed.SQL)
	if sql == "" {
		return "", llmRunResult{Used: false, Model: model, LatencyMS: latency, Err: fmt.Errorf("llm returned empty sql")}
	}
	logger.Info("MCP LLM sql parser used", "provider", cfg.Provider, "model", model, "latency_ms", latency)
	return sql, llmRunResult{Used: true, Model: model, LatencyMS: latency}
}
