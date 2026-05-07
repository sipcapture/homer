// Copyright (C) 2025 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	logger "github.com/sipcapture/homer-core/src/utils/logging"
)

const transactionViewPageTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Title}}</title>
<style>
:root {
  color-scheme: light dark;
  --tx-bg: #f4f4f5;
  --tx-surface: #ffffff;
  --tx-border: #e4e4e7;
  --tx-text: #18181b;
  --tx-muted: #71717a;
  --tx-accent: #0284c7;
  --tx-accent-soft: rgba(2, 132, 199, 0.12);
  --tx-row-a: #ffffff;
  --tx-row-b: #f0f9ff;
  --tx-titlebar: linear-gradient(180deg, #fafafa 0%, #f4f4f5 100%);
  --tx-pill-req-bg: #ecfdf5;
  --tx-pill-req-fg: #047857;
  --tx-pill-resp-bg: #fff7ed;
  --tx-pill-resp-fg: #c2410c;
}
@media (prefers-color-scheme: dark) {
  :root {
    --tx-bg: #09090b;
    --tx-surface: #18181b;
    --tx-border: #27272a;
    --tx-text: #fafafa;
    --tx-muted: #a1a1aa;
    --tx-accent: #38bdf8;
    --tx-accent-soft: rgba(56, 189, 248, 0.15);
    --tx-row-a: #18181b;
    --tx-row-b: #0c1929;
    --tx-titlebar: linear-gradient(180deg, #27272a 0%, #18181b 100%);
    --tx-pill-req-bg: #052e16;
    --tx-pill-req-fg: #4ade80;
    --tx-pill-resp-bg: #431407;
    --tx-pill-resp-fg: #fb923c;
  }
}
* { box-sizing: border-box; }
html, body { height: 100%; margin: 0; }
body.tx-page {
  font-family: system-ui, -apple-system, Segoe UI, Roboto, sans-serif;
  background: var(--tx-bg);
  color: var(--tx-text);
  line-height: 1.45;
  display: flex;
  flex-direction: column;
  min-height: 100%;
}
.tx-chrome {
  flex: 1;
  display: flex;
  flex-direction: column;
  min-height: 0;
  max-width: 1280px;
  width: 100%;
  margin: 0 auto;
  padding: 12px 16px 16px;
}
.tx-titlebar {
  background: var(--tx-titlebar);
  border: 1px solid var(--tx-border);
  border-radius: 10px 10px 0 0;
  padding: 10px 14px;
  box-shadow: 0 1px 2px rgba(0,0,0,.04);
}
.tx-titlebar h1 {
  font-size: 0.95rem;
  font-weight: 600;
  margin: 0 0 4px;
  letter-spacing: -0.01em;
}
.tx-titlebar .tx-session {
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, monospace;
  font-size: 0.8rem;
  color: var(--tx-muted);
  word-break: break-all;
}
.tx-body {
  flex: 1;
  display: flex;
  flex-direction: column;
  min-height: 0;
  background: var(--tx-surface);
  border: 1px solid var(--tx-border);
  border-top: none;
  border-radius: 0 0 10px 10px;
  box-shadow: 0 2px 8px rgba(0,0,0,.06);
}
@media (prefers-color-scheme: dark) {
  .tx-body { box-shadow: none; }
}
.tx-meta {
  padding: 8px 14px;
  font-size: 11px;
  color: var(--tx-muted);
  border-bottom: 1px solid var(--tx-border);
  background: var(--tx-accent-soft);
}
.tab-radios { position: absolute; width: 1px; height: 1px; opacity: 0; pointer-events: none; }
.tx-tab-labels {
  display: flex;
  gap: 0;
  padding: 0 8px;
  border-bottom: 1px solid var(--tx-border);
}
.tx-tab-labels label {
  display: inline-block;
  padding: 10px 16px;
  font-size: 12px;
  font-weight: 500;
  color: var(--tx-muted);
  cursor: pointer;
  border-bottom: 2px solid transparent;
  margin-bottom: -1px;
  user-select: none;
}
.tx-tab-labels label:hover { color: var(--tx-text); }
#tx-tab-messages:checked ~ .tx-tab-labels label[for="tx-tab-messages"],
#tx-tab-flow:checked ~ .tx-tab-labels label[for="tx-tab-flow"] {
  color: var(--tx-accent);
  font-weight: 600;
  border-bottom-color: var(--tx-accent);
}
.tx-panels { flex: 1; min-height: 0; display: flex; flex-direction: column; position: relative; }
.tx-panel {
  display: none;
  flex: 1;
  min-height: 0;
  flex-direction: column;
  overflow: hidden;
}
#tx-tab-messages:checked ~ .tx-panels .tx-panel-messages,
#tx-tab-flow:checked ~ .tx-panels .tx-panel-flow {
  display: flex;
}
.table-wrap {
  flex: 1;
  min-height: 200px;
  overflow: auto;
  margin: 0;
}
table.tx-grid {
  width: 100%;
  border-collapse: collapse;
  font-size: 11px;
}
table.tx-grid th, table.tx-grid td {
  text-align: left;
  padding: 7px 8px;
  border-bottom: 1px solid var(--tx-border);
  vertical-align: top;
}
table.tx-grid th {
  position: sticky;
  top: 0;
  z-index: 2;
  background: var(--tx-surface);
  font-weight: 600;
  font-size: 10px;
  text-transform: uppercase;
  letter-spacing: 0.04em;
  color: var(--tx-muted);
  white-space: nowrap;
  box-shadow: 0 1px 0 var(--tx-border);
}
table.tx-grid tbody tr:nth-child(even) { background: var(--tx-row-b); }
table.tx-grid tbody tr:nth-child(odd) { background: var(--tx-row-a); }
table.tx-grid tbody tr:hover { filter: brightness(0.97); }
@media (prefers-color-scheme: dark) {
  table.tx-grid tbody tr:hover { filter: brightness(1.08); }
}
.mono {
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, monospace;
  font-size: 10px;
  word-break: break-word;
}
.payload { max-height: 280px; overflow: auto; white-space: pre-wrap; margin: 0; }
details.payload-details > summary {
  cursor: pointer;
  font-size: 10px;
  color: var(--tx-accent);
  list-style: none;
}
details.payload-details > summary::-webkit-details-marker { display: none; }
.flow-scroll {
  flex: 1;
  min-height: 200px;
  overflow: auto;
  padding: 12px 14px 16px;
}
.flow-ladder { max-width: 720px; margin: 0 auto; }
.flow-step {
  display: grid;
  grid-template-columns: auto 1fr auto;
  gap: 10px 14px;
  align-items: start;
  padding: 8px 0;
  border-left: 3px solid var(--tx-border);
  padding-left: 14px;
  margin-left: 8px;
}
.flow-step-time { font-size: 10px; color: var(--tx-muted); white-space: nowrap; padding-top: 4px; }
.flow-step-main { min-width: 0; }
.flow-pill {
  display: inline-block;
  padding: 3px 10px;
  border-radius: 999px;
  font-size: 11px;
  font-weight: 600;
  font-family: ui-monospace, Menlo, monospace;
}
.flow-pill-req { background: var(--tx-pill-req-bg); color: var(--tx-pill-req-fg); }
.flow-pill-resp { background: var(--tx-pill-resp-bg); color: var(--tx-pill-resp-fg); }
.flow-endpoint {
  margin-top: 6px;
  font-size: 10px;
  color: var(--tx-muted);
  word-break: break-all;
}
.flow-idx {
  font-size: 10px;
  color: var(--tx-muted);
  padding-top: 4px;
}
.note {
  font-size: 10px;
  color: var(--tx-muted);
  padding: 10px 14px 12px;
  border-top: 1px solid var(--tx-border);
  margin-top: auto;
}
</style>
</head>
<body class="tx-page">
<div class="tx-chrome">
  <header class="tx-titlebar">
    <h1>{{.WindowTitle}}</h1>
    <div class="tx-session">{{.SessionLine}}</div>
  </header>
  <div class="tx-body">
    <div class="tx-meta">{{.Subtitle}}</div>
    <input type="radio" class="tab-radios" name="txtab" id="tx-tab-messages" checked>
    <input type="radio" class="tab-radios" name="txtab" id="tx-tab-flow">
    <div class="tx-tab-labels">
      <label for="tx-tab-messages">Messages</label>
      <label for="tx-tab-flow">Call flow</label>
    </div>
    <div class="tx-panels">
      <div class="tx-panel tx-panel-messages">
        <div class="table-wrap">
          <table class="tx-grid">
            <thead>
              <tr>
                <th>#</th>
                <th>Timestamp</th>
                <th>Call-ID</th>
                <th>Event</th>
                <th>src_ip</th>
                <th>src_port</th>
                <th>dst_ip</th>
                <th>dst_port</th>
                <th>caller</th>
                <th>callee</th>
                <th>Payload</th>
              </tr>
            </thead>
            <tbody>
            {{range .Rows}}
              <tr>
                <td class="mono">{{.Index}}</td>
                <td class="mono">{{.Time}}</td>
                <td class="mono">{{.CallID}}</td>
                <td class="mono">{{.Event}}</td>
                <td class="mono">{{.SrcIP}}</td>
                <td class="mono">{{.SrcPort}}</td>
                <td class="mono">{{.DstIP}}</td>
                <td class="mono">{{.DstPort}}</td>
                <td class="mono">{{.Caller}}</td>
                <td class="mono">{{.Callee}}</td>
                <td>
                  <details class="payload-details"><summary>View</summary>
                    <pre class="payload mono">{{.Payload}}</pre>
                  </details>
                </td>
              </tr>
            {{end}}
            </tbody>
          </table>
        </div>
      </div>
      <div class="tx-panel tx-panel-flow">
        <div class="flow-scroll">
          <div class="flow-ladder">
          {{range .FlowSteps}}
            <div class="flow-step">
              <div class="flow-step-time mono">{{.Time}}</div>
              <div class="flow-step-main">
                <span class="flow-pill flow-pill-{{.EventClass}}">{{.Event}}</span>
                <div class="flow-endpoint mono">{{.SrcEndpoint}} → {{.DstEndpoint}}</div>
                {{if .CallID}}<div class="flow-endpoint mono">Call-ID: {{.CallID}}</div>{{end}}
              </div>
              <div class="flow-idx mono">#{{.Index}}</div>
            </div>
          {{end}}
          </div>
        </div>
      </div>
    </div>
    <p class="note">{{.Footnote}}</p>
  </div>
</div>
</body>
</html>
`

var parsedTransactionViewTpl = template.Must(template.New("txview").Parse(transactionViewPageTemplate))

type transactionViewRow struct {
	Index   int
	Time    string
	Event   string
	CallID  string
	SrcIP   string
	SrcPort string
	DstIP   string
	DstPort string
	Caller  string
	Callee  string
	Payload string
}

// transactionFlowStep is a lightweight vertical timeline row (shared export
// "Call flow" tab) — same logical fields as the React CallFlow ladder.
type transactionFlowStep struct {
	Index       int
	Time        string
	Event       string
	EventClass  string // "req" or "resp"
	SrcEndpoint string
	DstEndpoint string
	CallID      string
}

func cellFromRow(row map[string]interface{}, keys ...string) string {
	for _, k := range keys {
		if v, ok := row[k]; ok && v != nil {
			s := strings.TrimSpace(fmt.Sprint(v))
			if s != "" {
				return s
			}
		}
	}
	return ""
}

func formatTimestampCell(row map[string]interface{}) string {
	if s := cellFromRow(row, "timestamp", "time", "create_date"); s != "" {
		return s
	}
	return ""
}

func payloadPreview(row map[string]interface{}) string {
	p := cellFromRow(row, "payload", "data", "raw")
	if len(p) > 65536 {
		return p[:65536] + "\n… (truncated)"
	}
	return p
}

func viewMessageCallID(row map[string]interface{}) string {
	return cellFromRow(row, "session_id", "cid", "call_id", "callid", "sip_call_id")
}

func viewMessageEventLabel(row map[string]interface{}) string {
	for _, k := range []string{"response_code", "resp_code", "status"} {
		if v, ok := row[k]; ok && v != nil {
			s := strings.TrimSpace(fmt.Sprint(v))
			if n, err := strconv.Atoi(s); err == nil && n > 0 {
				return s
			}
		}
	}
	for _, k := range []string{"method", "sip_method", "cseq_method", "event"} {
		if s := cellFromRow(row, k); s != "" {
			return s
		}
	}
	return ""
}

func sipEventClass(event string) string {
	s := strings.TrimSpace(event)
	if len(s) == 3 {
		if n, err := strconv.Atoi(s); err == nil && n >= 100 && n < 700 {
			return "resp"
		}
	}
	return "req"
}

func formatEndpoint(ip, port string) string {
	ip = strings.TrimSpace(ip)
	port = strings.TrimSpace(port)
	if ip == "" && port == "" {
		return "—"
	}
	if port == "" {
		return ip
	}
	if ip == "" {
		return ":" + port
	}
	return ip + ":" + port
}

// V4TransactionViewLinkCreate stores the session descriptor server-side and returns a viewer URL (no JWT to open).
// POST /api/v4/transactions/view/link — body same as /transactions/messages.
func (h *SearchHandler) V4TransactionViewLinkCreate(c echo.Context) error {
	if h.viewTokens == nil {
		return writeError(c, http.StatusServiceUnavailable, "Service Unavailable", "Transaction view token storage not available")
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
		logger.Error("V4TransactionViewLinkCreate: probe query failed", "error", qerr)
		return writeError(c, http.StatusInternalServerError, "Server Error", "Query failed")
	}
	if !ok {
		return writeError(c, http.StatusBadRequest, "Bad Request", "No messages for this session")
	}
	id, cerr := h.viewTokens.Create(c.Request().Context(), username, bodyBytes, 72*time.Hour, h.transactionViewMaxOpens)
	if cerr != nil {
		logger.Error("V4TransactionViewLinkCreate: persist failed", "error", cerr)
		return writeError(c, http.StatusInternalServerError, "Server Error", "Failed to create view link")
	}
	viewPath := "/export/view/" + id
	return c.JSON(http.StatusCreated, map[string]interface{}{
		"data": map[string]string{
			"uuid":     id,
			"url_view": viewPath,
		},
		"meta": buildMeta(c, ""),
	})
}

// TransactionViewHTMLPage serves an embedded HTML SIP transaction view at GET /export/view/:uuid (counts toward token open limit).
func (h *SearchHandler) TransactionViewHTMLPage(c echo.Context) error {
	if h.viewTokens == nil {
		return c.HTML(http.StatusServiceUnavailable, `<!DOCTYPE html><html><body><p>Transaction view is not configured.</p></body></html>`)
	}
	id := c.Param("uuid")
	payload, err := h.viewTokens.ConsumePayload(c.Request().Context(), id)
	if err != nil {
		return c.HTML(http.StatusGone, `<!DOCTYPE html><html><head><meta charset="utf-8"><title>Link unavailable</title></head><body style="font-family:system-ui;padding:24px"><p>This view link has expired, reached the maximum number of opens, or is invalid.</p></body></html>`)
	}
	var req TransactionSessionRequestV4
	if err := json.Unmarshal(payload, &req); err != nil {
		return c.HTML(http.StatusInternalServerError, `<!DOCTYPE html><html><body><p>Invalid view payload.</p></body></html>`)
	}
	rows, qerr := h.queryTransactionMessages(c.Request().Context(), &req)
	if qerr != nil {
		logger.Error("TransactionViewHTMLPage: query failed", "error", qerr)
		return c.HTML(http.StatusInternalServerError, `<!DOCTYPE html><html><body><p>Failed to load messages.</p></body></html>`)
	}

	ids, _ := resolvedSessionIDList(&req)
	windowTitle := "Transaction"
	sessionLine := "—"
	browserTitle := "Transaction"
	if len(ids) == 1 {
		sessionLine = ids[0]
		browserTitle = "Transaction · " + ids[0]
	} else if len(ids) > 1 {
		windowTitle = "Transactions"
		sessionLine = fmt.Sprintf("%d sessions", len(ids))
		browserTitle = fmt.Sprintf("Transactions · %d sessions", len(ids))
	}
	sub := fmt.Sprintf("%d message(s) · proto_type=%d · event_type=%s", len(rows), func() int {
		if req.ProtoType == 0 {
			return 1
		}
		return req.ProtoType
	}(), func() string {
		if strings.TrimSpace(req.EventType) == "" {
			return "call"
		}
		return req.EventType
	}())

	viewRows := make([]transactionViewRow, 0, len(rows))
	flowSteps := make([]transactionFlowStep, 0, len(rows))
	for i, row := range rows {
		event := viewMessageEventLabel(row)
		callID := viewMessageCallID(row)
		srcIP := cellFromRow(row, "src_ip", "source_ip")
		dstIP := cellFromRow(row, "dst_ip", "destination_ip")
		srcPort := cellFromRow(row, "src_port", "source_port")
		dstPort := cellFromRow(row, "dst_port", "destination_port")
		caller := cellFromRow(row, "caller", "from_user", "caller_id")
		callee := cellFromRow(row, "callee", "to_user", "called_party")
		viewRows = append(viewRows, transactionViewRow{
			Index:   i + 1,
			Time:    formatTimestampCell(row),
			Event:   event,
			CallID:  callID,
			SrcIP:   srcIP,
			SrcPort: srcPort,
			DstIP:   dstIP,
			DstPort: dstPort,
			Caller:  caller,
			Callee:  callee,
			Payload: payloadPreview(row),
		})
		flowSteps = append(flowSteps, transactionFlowStep{
			Index:       i + 1,
			Time:        formatTimestampCell(row),
			Event:       event,
			EventClass:  sipEventClass(event),
			SrcEndpoint: formatEndpoint(srcIP, srcPort),
			DstEndpoint: formatEndpoint(dstIP, dstPort),
			CallID:      callID,
		})
	}

	footnote := fmt.Sprintf(
		"This URL can be opened a limited number of times per token (current server default: %d; coordinator setting transaction_view_max_opens). Reload uses another open; after the limit or TTL, the link stops working.",
		h.transactionViewMaxOpens,
	)

	var buf bytes.Buffer
	if err := parsedTransactionViewTpl.Execute(&buf, map[string]interface{}{
		"Title":       browserTitle,
		"WindowTitle": windowTitle,
		"SessionLine": sessionLine,
		"Subtitle":    sub,
		"Rows":        viewRows,
		"FlowSteps":   flowSteps,
		"Footnote":    footnote,
	}); err != nil {
		logger.Error("TransactionViewHTMLPage: template failed", "error", err)
		return c.HTML(http.StatusInternalServerError, `<!DOCTYPE html><html><body><p>Render error.</p></body></html>`)
	}
	return c.HTMLBlob(http.StatusOK, buf.Bytes())
}
