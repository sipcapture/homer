package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/sipcapture/homer-core/src/config"
)

// LLMClient is a thin OpenAI-compatible chat-completions client used to convert
// natural-language MCP queries into structured filters. It works with OpenAI,
// Ollama (http://localhost:11434/v1), vLLM, LM Studio, OpenRouter, Groq and
// any other OpenAI-compatible endpoint.
//
// The client is reused by both the stdio MCP server in this package and by the
// Coordinator HTTP handler (`/api/v4/mcp/query`); see ChatJSON for a low-level
// entry point and Parse for the built-in filter-extraction prompt.
type LLMClient struct {
	cfg  *config.MCPLLMConfig
	http *http.Client
}

// llmParsedFilters is the structured output we ask the LLM to return.
// All fields are optional; missing fields are filled in by the regex parser
// during the merge step in parseQuery.
type llmParsedFilters struct {
	Method         string `json:"method,omitempty"`
	SrcIP          string `json:"src_ip,omitempty"`
	DstIP          string `json:"dst_ip,omitempty"`
	CallID         string `json:"call_id,omitempty"`
	FromUser       string `json:"from_user,omitempty"`
	ToUser         string `json:"to_user,omitempty"`
	TimeRangeLabel string `json:"time_range_label,omitempty"`
	FromMS         int64  `json:"from_ms,omitempty"`
	ToMS           int64  `json:"to_ms,omitempty"`
}

// llmParseResult bundles parsed filters with diagnostics so callers can expose
// it through the MCP tool meta.
type llmParseResult struct {
	Filters   *llmParsedFilters
	Model     string
	LatencyMS int64
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model          string         `json:"model"`
	Messages       []chatMessage  `json:"messages"`
	Temperature    float64        `json:"temperature"`
	MaxTokens      int            `json:"max_tokens,omitempty"`
	ResponseFormat map[string]any `json:"response_format,omitempty"`
}

type chatResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
}

const llmSystemPrompt = `You convert natural-language SIP/HEP search queries into a JSON object describing structured filters for a search API.
Return ONLY a single JSON object, no prose, no markdown fences.

Allowed top-level keys (all optional, omit if not mentioned in the query):
- method: one of INVITE, BYE, REGISTER, OPTIONS, ACK, CANCEL, PRACK, UPDATE, INFO, REFER, SUBSCRIBE, NOTIFY, PUBLISH, MESSAGE
- src_ip: source IPv4/IPv6 string
- dst_ip: destination IPv4/IPv6 string
- call_id: SIP Call-ID substring
- from_user: caller / from username
- to_user: callee / to username
- time_range_label: short human label such as "last_hour", "last_24h", "today", "yesterday", "custom"
- from_ms: start of time range, UTC unix timestamp in milliseconds (integer)
- to_ms: end of time range, UTC unix timestamp in milliseconds (integer)

Rules:
- If the query mentions "last hour" / "last N minutes" / "today" / "yesterday", compute from_ms/to_ms relative to the user-provided "now_utc_unix_ms".
- If no time range is mentioned, omit time fields and the server will default to the last hour.
- Do not invent values that are not present in the query.
- Keep IPs and Call-IDs verbatim as they appear.
- Output a single valid JSON object and nothing else.`

// NewLLMClient returns an OpenAI-compatible chat-completions client when
// the LLM bridge is enabled, or nil otherwise. The returned client is safe
// for concurrent use.
func NewLLMClient(cfg *config.MCPLLMConfig) *LLMClient {
	if cfg == nil || !cfg.Enable {
		return nil
	}
	timeout := cfg.TimeoutSec
	if timeout <= 0 {
		timeout = 15
	}
	return &LLMClient{
		cfg: cfg,
		http: &http.Client{
			Timeout: time.Duration(timeout) * time.Second,
		},
	}
}

// Enabled reports whether the client is non-nil and the underlying provider
// is enabled by config. Cheap helper to avoid nil checks at call sites.
func (c *LLMClient) Enabled() bool {
	return c != nil && c.cfg != nil && c.cfg.Enable
}

// Model returns the configured model name, with whitespace trimmed. Empty
// string means "not configured".
func (c *LLMClient) Model() string {
	if c == nil || c.cfg == nil {
		return ""
	}
	return strings.TrimSpace(c.cfg.Model)
}

// Provider returns the configured provider label (e.g. "openai", "ollama").
// It is informational only; the client always speaks the OpenAI chat API.
func (c *LLMClient) Provider() string {
	if c == nil || c.cfg == nil {
		return ""
	}
	return strings.TrimSpace(c.cfg.Provider)
}

// ChatJSON sends a chat-completions request with json_object response_format
// and decodes the (possibly fenced) response body into out. It returns the
// model name actually used and the round-trip latency for diagnostics.
//
// It is the lowest-level entry point and is used by both the built-in Parse
// helper (filter extraction) and by the Coordinator HTTP handler (which uses
// its own structured-output and SQL prompts).
func (c *LLMClient) ChatJSON(ctx context.Context, systemPrompt, userPrompt string, out any) (model string, latencyMS int64, err error) {
	if !c.Enabled() {
		return "", 0, fmt.Errorf("llm client is disabled")
	}
	baseURL := strings.TrimRight(strings.TrimSpace(c.cfg.BaseURL), "/")
	if baseURL == "" {
		return "", 0, fmt.Errorf("llm base_url is empty")
	}
	model = c.Model()
	if model == "" {
		return "", 0, fmt.Errorf("llm model is empty")
	}

	reqBody := chatRequest{
		Model:       model,
		Temperature: c.cfg.Temperature,
		MaxTokens:   c.cfg.MaxTokens,
		Messages: []chatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		ResponseFormat: map[string]any{"type": "json_object"},
	}

	payload, err := json.Marshal(reqBody)
	if err != nil {
		return model, 0, fmt.Errorf("marshal llm request: %w", err)
	}

	endpoint := baseURL + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return model, 0, fmt.Errorf("create llm request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if key := strings.TrimSpace(c.cfg.APIKey); key != "" {
		httpReq.Header.Set("Authorization", "Bearer "+key)
	}

	start := time.Now()
	resp, err := c.http.Do(httpReq)
	if err != nil {
		return model, 0, fmt.Errorf("llm http request failed: %w", err)
	}
	defer resp.Body.Close()

	respBytes, _ := io.ReadAll(resp.Body)
	latencyMS = time.Since(start).Milliseconds()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return model, latencyMS, fmt.Errorf("llm api error %d: %s", resp.StatusCode, strings.TrimSpace(string(respBytes)))
	}

	var chat chatResponse
	if err := json.Unmarshal(respBytes, &chat); err != nil {
		return model, latencyMS, fmt.Errorf("decode llm response: %w", err)
	}
	if len(chat.Choices) == 0 {
		return model, latencyMS, fmt.Errorf("llm returned no choices")
	}

	content := strings.TrimSpace(chat.Choices[0].Message.Content)
	if content == "" {
		return model, latencyMS, fmt.Errorf("llm returned empty content")
	}

	jsonStr, err := extractJSONObject(content)
	if err != nil {
		return model, latencyMS, fmt.Errorf("extract llm json: %w", err)
	}

	if out != nil {
		if err := json.Unmarshal([]byte(jsonStr), out); err != nil {
			return model, latencyMS, fmt.Errorf("unmarshal llm json: %w", err)
		}
	}
	return model, latencyMS, nil
}

// Parse asks the configured LLM to extract structured filters from queryText.
// The provided nowMS is forwarded to the model so it can resolve relative time
// expressions consistently with the rest of the request.
func (c *LLMClient) Parse(ctx context.Context, queryText string, nowMS int64) (*llmParseResult, error) {
	if !c.Enabled() {
		return nil, fmt.Errorf("llm client is disabled")
	}

	userPrompt := fmt.Sprintf(
		"now_utc_unix_ms=%d\nquery=%s",
		nowMS,
		strings.TrimSpace(queryText),
	)

	var filters llmParsedFilters
	model, latency, err := c.ChatJSON(ctx, llmSystemPrompt, userPrompt, &filters)
	if err != nil {
		return nil, err
	}
	return &llmParseResult{
		Filters:   &filters,
		Model:     model,
		LatencyMS: latency,
	}, nil
}

// extractJSONObject tolerates models that wrap their answer in markdown fences
// or add explanatory prose around the JSON object.
func extractJSONObject(s string) (string, error) {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		if idx := strings.Index(s, "\n"); idx >= 0 {
			s = s[idx+1:]
		}
		s = strings.TrimSuffix(s, "```")
		s = strings.TrimSpace(s)
	}

	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start < 0 || end < 0 || end <= start {
		return "", fmt.Errorf("no json object found")
	}
	return s[start : end+1], nil
}
