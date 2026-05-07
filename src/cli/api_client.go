package cli

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Client is an HTTP client for the Homer coordinator API.
type Client struct {
	BaseURL    string
	Token      string
	Debug      bool
	HTTPClient *http.Client
}

// NewClient creates a new API client targeting the given coordinator host.
// host can be "host:port" or a full URL. TLS verification is skipped for
// self-signed certificates common in internal deployments.
func NewClient(host string) *Client {
	base := host
	if !strings.HasPrefix(base, "http://") && !strings.HasPrefix(base, "https://") {
		base = "http://" + base
	}
	base = strings.TrimRight(base, "/")

	return &Client{
		BaseURL: base,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			},
		},
	}
}

// ---- Authentication --------------------------------------------------------

// LoginRequest is the JSON body for POST /api/v4/auth/sessions.
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// loginResponse mirrors the coordinator's LoginResponseV4.
type loginResponse struct {
	Data struct {
		Token string `json:"token"`
		Scope string `json:"scope"`
	} `json:"data"`
}

// NormalizeAuthToken trims whitespace and strips a leading "Bearer " prefix
// (users often paste the full Authorization header value).
func NormalizeAuthToken(t string) string {
	t = strings.TrimSpace(t)
	if len(t) >= 7 && strings.EqualFold(t[:7], "bearer ") {
		t = strings.TrimSpace(t[7:])
	}
	return t
}

// TokenLooksMisparsedFlag is true when Go's flag parser likely attached the next
// flag name as this string's value (e.g. `homer search --token --sql "..."` → token "--sql").
func TokenLooksMisparsedFlag(t string) bool {
	return strings.HasPrefix(strings.TrimSpace(t), "--")
}

// tokenLooksLikeJWT matches compact signed JWTs (header.payload.sig).
func tokenLooksLikeJWT(t string) bool {
	parts := strings.Split(strings.TrimSpace(t), ".")
	if len(parts) != 3 {
		return false
	}
	for _, p := range parts {
		if len(p) == 0 {
			return false
		}
	}
	return true
}

// applyTokenAuth sets either Authorization: Bearer (session JWT from login) or
// Auth-Token (raw secret from Settings → Auth Tokens when enable_token_access is on).
// The coordinator validates JWT via Bearer first path; API keys use Auth-Token only.
func applyTokenAuth(httpReq *http.Request, token string) {
	if token == "" {
		return
	}
	if tokenLooksLikeJWT(token) {
		httpReq.Header.Set("Authorization", "Bearer "+token)
		return
	}
	httpReq.Header.Set("Auth-Token", token)
}

// Login authenticates against the coordinator and stores the JWT token.
func (c *Client) Login(user, pass string) error {
	body, err := json.Marshal(LoginRequest{Username: user, Password: pass})
	if err != nil {
		return fmt.Errorf("marshal login request: %w", err)
	}

	url := c.BaseURL + "/api/v4/auth/sessions"
	if c.Debug {
		fmt.Fprintf(os.Stderr, "[DEBUG] POST %s\n", url)
		fmt.Fprintf(os.Stderr, "[DEBUG] Request body: %s\n", string(body))
	}

	resp, err := c.HTTPClient.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("login request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if c.Debug {
		fmt.Fprintf(os.Stderr, "[DEBUG] Response: HTTP %d\n", resp.StatusCode)
		fmt.Fprintf(os.Stderr, "[DEBUG] Response body: %s\n", string(respBody))
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("login failed (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	var lr loginResponse
	if err := json.Unmarshal(respBody, &lr); err != nil {
		return fmt.Errorf("decode login response: %w", err)
	}
	if lr.Data.Token == "" {
		return fmt.Errorf("login succeeded but no token in response")
	}

	c.Token = lr.Data.Token
	if c.Debug {
		fmt.Fprintf(os.Stderr, "[DEBUG] Login OK, token: %s\n", truncateToken(c.Token))
	}
	return nil
}

// ---- Token caching ---------------------------------------------------------

const tokenFileName = ".homer_token"

func tokenFilePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, tokenFileName)
}

// SaveToken persists the current JWT token to ~/.homer_token.
func (c *Client) SaveToken() error {
	p := tokenFilePath()
	if p == "" {
		return nil
	}
	return os.WriteFile(p, []byte(c.Token), 0600)
}

// LoadToken reads a cached JWT token from ~/.homer_token.
func (c *Client) LoadToken() bool {
	p := tokenFilePath()
	if p == "" {
		return false
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return false
	}
	t := strings.TrimSpace(string(data))
	if t == "" {
		return false
	}
	c.Token = t
	return true
}

// EnsureAuth logs in if no token is available (either from flag or cache).
// Returns an error only if login is required but fails.
func (c *Client) EnsureAuth(user, pass string) error {
	if c.Token != "" {
		c.Token = NormalizeAuthToken(c.Token)
		if TokenLooksMisparsedFlag(c.Token) {
			return fmt.Errorf(`invalid token value %q — if your command uses "--token" followed by another flag (e.g. "--sql"), the parser treats that flag name as the token. Fix: use --token=<jwt>, put the JWT immediately after --token, or set env HOMER_TOKEN`, c.Token)
		}
		return nil
	}
	if env := strings.TrimSpace(os.Getenv("HOMER_TOKEN")); env != "" {
		c.Token = NormalizeAuthToken(env)
		if TokenLooksMisparsedFlag(c.Token) {
			return fmt.Errorf("invalid HOMER_TOKEN env value %q", c.Token)
		}
		return nil
	}
	if c.LoadToken() {
		c.Token = NormalizeAuthToken(c.Token)
		return nil
	}
	if user == "" || pass == "" {
		return fmt.Errorf("no token available; provide --user and --pass, --token / HOMER_TOKEN (JWT or Settings→Auth Token secret)")
	}
	if err := c.Login(user, pass); err != nil {
		return err
	}
	_ = c.SaveToken()
	return nil
}

// ---- Search ----------------------------------------------------------------

// SearchFilter contains all filter fields for a transaction search.
type SearchFilter struct {
	ProtoType int    `json:"proto_type,omitempty"`
	EventType string `json:"event_type,omitempty"`
	Method    string `json:"method,omitempty"`
	CallID    string `json:"call_id,omitempty"`
	RuriUser  string `json:"ruri_user,omitempty"`
	FromUser  string `json:"from_user,omitempty"`
	ToUser    string `json:"to_user,omitempty"`
	UserAgent string `json:"user_agent,omitempty"`
	SrcIP     string `json:"src_ip,omitempty"`
	DstIP     string `json:"dst_ip,omitempty"`
	SrcPort   int    `json:"src_port,omitempty"`
	DstPort   int    `json:"dst_port,omitempty"`
	CaptureID int    `json:"capture_id,omitempty"`
	Node      string `json:"node,omitempty"`
}

// SearchParam holds pagination and aggregation parameters.
type SearchParam struct {
	Limit   int    `json:"limit,omitempty"`
	Select  string `json:"select,omitempty"`   // custom SELECT columns/aggregations
	GroupBy string `json:"group_by,omitempty"` // GROUP BY clause
	OrderBy string `json:"order_by,omitempty"` // custom ORDER BY
}

// SearchTimestamp holds the time range as epoch milliseconds.
type SearchTimestamp struct {
	From int64 `json:"from"`
	To   int64 `json:"to"`
}

// SearchRequest is the full body for POST /api/v4/transactions/search.
type SearchRequest struct {
	Filter    SearchFilter    `json:"filter"`
	Param     SearchParam     `json:"param"`
	Timestamp SearchTimestamp `json:"timestamp"`
}

// SearchResponse mirrors TransactionListResponseV4.
type SearchResponse struct {
	Data struct {
		Items []map[string]interface{} `json:"items"`
		Keys  []string                 `json:"keys,omitempty"`
	} `json:"data"`
	Meta struct {
		Pagination struct {
			Limit int `json:"limit,omitempty"`
			Total int `json:"total,omitempty"`
		} `json:"pagination,omitempty"`
	} `json:"meta"`
}

// Search executes a transaction search against the coordinator API.
func (c *Client) Search(req SearchRequest) (*SearchResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal search request: %w", err)
	}

	url := c.BaseURL + "/api/v4/transactions/search"
	if c.Debug {
		prettyBody, _ := json.MarshalIndent(req, "", "  ")
		fmt.Fprintf(os.Stderr, "[DEBUG] POST %s\n", url)
		if c.Token != "" {
			if tokenLooksLikeJWT(c.Token) {
				fmt.Fprintf(os.Stderr, "[DEBUG] Authorization: Bearer %s\n", truncateToken(c.Token))
			} else {
				fmt.Fprintf(os.Stderr, "[DEBUG] Auth-Token: %s\n", truncateToken(c.Token))
			}
		}
		fmt.Fprintf(os.Stderr, "[DEBUG] Request body:\n%s\n", string(prettyBody))
	}

	httpReq, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	applyTokenAuth(httpReq, c.Token)

	start := time.Now()
	resp, err := c.HTTPClient.Do(httpReq)
	elapsed := time.Since(start)
	if err != nil {
		return nil, fmt.Errorf("search request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if c.Debug {
		fmt.Fprintf(os.Stderr, "[DEBUG] Response: HTTP %d (%s)\n", resp.StatusCode, elapsed.Round(time.Millisecond))
		if len(respBody) <= 4096 {
			fmt.Fprintf(os.Stderr, "[DEBUG] Response body:\n%s\n", string(respBody))
		} else {
			fmt.Fprintf(os.Stderr, "[DEBUG] Response body (%d bytes, truncated):\n%s\n...\n",
				len(respBody), string(respBody[:4096]))
		}
	}

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("unauthorized (HTTP 401): invalid session JWT or API token (Auth Tokens require coordinator api_settings.enable_token_access); use login --user/--pass for a Bearer JWT")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("search failed (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	var sr SearchResponse
	if err := json.Unmarshal(respBody, &sr); err != nil {
		return nil, fmt.Errorf("decode search response: %w", err)
	}

	if c.Debug {
		fmt.Fprintf(os.Stderr, "[DEBUG] Parsed: %d items, %d keys\n", len(sr.Data.Items), len(sr.Data.Keys))
	}

	return &sr, nil
}

// RawQueryRequest is the body for POST /api/v4/query.
type RawQueryRequest struct {
	SQL   string `json:"sql"`
	Limit int    `json:"limit,omitempty"`
}

// RawQuery executes a raw SQL query via the coordinator's /api/v4/query endpoint.
func (c *Client) RawQuery(sqlStr string, limit int) (*SearchResponse, error) {
	req := RawQueryRequest{SQL: sqlStr, Limit: limit}
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal raw query request: %w", err)
	}

	url := c.BaseURL + "/api/v4/query"
	if c.Debug {
		prettyBody, _ := json.MarshalIndent(req, "", "  ")
		fmt.Fprintf(os.Stderr, "[DEBUG] POST %s\n", url)
		if c.Token != "" {
			if tokenLooksLikeJWT(c.Token) {
				fmt.Fprintf(os.Stderr, "[DEBUG] Authorization: Bearer %s\n", truncateToken(c.Token))
			} else {
				fmt.Fprintf(os.Stderr, "[DEBUG] Auth-Token: %s\n", truncateToken(c.Token))
			}
		}
		fmt.Fprintf(os.Stderr, "[DEBUG] Request body:\n%s\n", string(prettyBody))
	}

	httpReq, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	applyTokenAuth(httpReq, c.Token)

	start := time.Now()
	resp, err := c.HTTPClient.Do(httpReq)
	elapsed := time.Since(start)
	if err != nil {
		return nil, fmt.Errorf("raw query request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if c.Debug {
		fmt.Fprintf(os.Stderr, "[DEBUG] Response: HTTP %d (%s)\n", resp.StatusCode, elapsed.Round(time.Millisecond))
		if len(respBody) <= 4096 {
			fmt.Fprintf(os.Stderr, "[DEBUG] Response body:\n%s\n", string(respBody))
		} else {
			fmt.Fprintf(os.Stderr, "[DEBUG] Response body (%d bytes, truncated):\n%s\n...\n",
				len(respBody), string(respBody[:4096]))
		}
	}

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("unauthorized (HTTP 401): invalid session JWT or API token (Auth Tokens require coordinator api_settings.enable_token_access); use login --user/--pass for a Bearer JWT")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("query failed (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	var sr SearchResponse
	if err := json.Unmarshal(respBody, &sr); err != nil {
		return nil, fmt.Errorf("decode query response: %w", err)
	}

	if c.Debug {
		fmt.Fprintf(os.Stderr, "[DEBUG] Parsed: %d items, %d keys\n", len(sr.Data.Items), len(sr.Data.Keys))
	}

	return &sr, nil
}

func truncateToken(t string) string {
	if len(t) <= 16 {
		return t
	}
	return t[:8] + "..." + t[len(t)-8:]
}
