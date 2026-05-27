package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"text/tabwriter"
	"time"
)

// ---- heplify API types -----------------------------------------------------

// HeplifyWebStats mirrors heplify's WebStats response from GET /api/stats.
type HeplifyWebStats struct {
	NodeName      string              `json:"node_name"`
	NodeID        int                 `json:"node_id"`
	UUID          string              `json:"uuid"`
	Interfaces    []string            `json:"interfaces"`
	CaptureModes  map[string][]string `json:"capture_modes"`
	UptimeSeconds int64               `json:"uptime_seconds"`
	Uptime        string              `json:"uptime"`
	Packets       struct {
		Total      int64 `json:"total"`
		SIP        int64 `json:"sip"`
		RTCP       int64 `json:"rtcp"`
		RTCPFail   int64 `json:"rtcp_fail"`
		RTP        int64 `json:"rtp"`
		DNS        int64 `json:"dns"`
		Log        int64 `json:"log"`
		HEPSent    int64 `json:"hep_sent"`
		Duplicates int64 `json:"duplicates"`
		Unknown    int64 `json:"unknown"`
	} `json:"packets"`
	Transport []HeplifyTransportInfo `json:"transport"`
}

// HeplifyTransportInfo mirrors heplify's TransportInfo for a single HEP connection.
type HeplifyTransportInfo struct {
	Addr       string `json:"addr"`
	Proto      string `json:"proto"`
	Connected  bool   `json:"connected"`
	Reconnects int64  `json:"reconnects"`
	Sent       int64  `json:"sent"`
	Errors     int64  `json:"errors"`
}

// HeplifyHealth is the response from GET /health.
type HeplifyHealth struct {
	Status              string `json:"status"`
	ConnectedTransports int    `json:"connected_transports"`
	QueueSize           int    `json:"queue_size"`
	BufferSizeBytes     int64  `json:"buffer_size_bytes"`
}

// ---- flags -----------------------------------------------------------------

// AgentFlags holds all CLI flags for the "homer agent" sub-command.
type AgentFlags struct {
	Action   string // set from positional arg in main.go, not a flag
	Host     string
	User     string
	Pass     string
	Interval time.Duration
	Format   string // "table" or "json"
}

type agentFlagRefs struct {
	Host     *string
	User     *string
	Pass     *string
	Interval *string
	Format   *string
}

// RegisterAgentFlags creates a FlagSet for "homer agent" subcommand.
func RegisterAgentFlags() (*flag.FlagSet, *agentFlagRefs) {
	fs := flag.NewFlagSet("agent", flag.ExitOnError)
	refs := &agentFlagRefs{}

	refs.Host = fs.String("host", "127.0.0.1:8008", "heplify API address (host:port)")
	refs.User = fs.String("user", "", "username for Basic Auth")
	refs.Pass = fs.String("pass", "", "password for Basic Auth")
	refs.Interval = fs.String("interval", "3s", "refresh interval for watch mode (e.g. 1s, 5s, 1m)")
	refs.Format = fs.String("format", "table", "output format: table, json")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `homer agent [action] [flags]

Query a heplify agent API and display statistics.

Actions:
  stats     Show packet capture statistics (default)
  health    Show agent health status
  watch     Continuously refresh stats (Ctrl+C to stop)

Flags:
`)
		fs.PrintDefaults()
	}
	return fs, refs
}

// ParseAgentFlags extracts AgentFlags from parsed flag refs.
func ParseAgentFlags(refs *agentFlagRefs) AgentFlags {
	interval, err := time.ParseDuration(*refs.Interval)
	if err != nil || interval <= 0 {
		interval = 3 * time.Second
	}
	return AgentFlags{
		Host:     *refs.Host,
		User:     *refs.User,
		Pass:     *refs.Pass,
		Interval: interval,
		Format:   *refs.Format,
	}
}

// RunAgentCmd dispatches to the appropriate agent action.
func RunAgentCmd(f AgentFlags) error {
	switch f.Action {
	case "health":
		return runAgentHealth(f)
	case "watch":
		return runAgentWatch(f)
	default: // "stats" or empty
		return runAgentStats(f)
	}
}

// ---- HTTP client -----------------------------------------------------------

func newAgentHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: cliTLSConfig(),
		},
	}
}

func agentBaseURL(host string) string {
	h := strings.TrimRight(host, "/")
	if !strings.HasPrefix(h, "http://") && !strings.HasPrefix(h, "https://") {
		h = "http://" + h
	}
	return h
}

func agentGet(client *http.Client, url, user, pass string) ([]byte, int, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("create request: %w", err)
	}
	if user != "" {
		req.SetBasicAuth(user, pass)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("connect to %s: %w", url, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("read response: %w", err)
	}
	return body, resp.StatusCode, nil
}

// ---- stats -----------------------------------------------------------------

func runAgentStats(f AgentFlags) error {
	client := newAgentHTTPClient()
	url := agentBaseURL(f.Host) + "/api/stats"
	body, code, err := agentGet(client, url, f.User, f.Pass)
	if err != nil {
		return err
	}
	if code != http.StatusOK {
		return fmt.Errorf("GET %s: HTTP %d: %s", url, code, strings.TrimSpace(string(body)))
	}

	if f.Format == "json" {
		fmt.Println(string(body))
		return nil
	}

	var ws HeplifyWebStats
	if err := json.Unmarshal(body, &ws); err != nil {
		return fmt.Errorf("decode stats response: %w", err)
	}

	printAgentStats(f.Host, ws)
	return nil
}

func printAgentStats(host string, ws HeplifyWebStats) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)

	fmt.Fprintf(w, "heplify @ %s\n\n", host)

	fmt.Fprintf(w, "-- Node\n")
	fmt.Fprintf(w, "  node_name\t%s\n", ws.NodeName)
	fmt.Fprintf(w, "  node_id\t%d\n", ws.NodeID)
	fmt.Fprintf(w, "  interfaces\t%s\n", strings.Join(ws.Interfaces, ", "))
	fmt.Fprintf(w, "  uptime\t%s\n", ws.Uptime)

	fmt.Fprintf(w, "\n-- Packets\n")
	fmt.Fprintf(w, "  total\t%d\n", ws.Packets.Total)
	fmt.Fprintf(w, "  sip\t%d\n", ws.Packets.SIP)
	fmt.Fprintf(w, "  rtcp\t%d\n", ws.Packets.RTCP)
	fmt.Fprintf(w, "  rtcp_fail\t%d\n", ws.Packets.RTCPFail)
	fmt.Fprintf(w, "  rtp\t%d\n", ws.Packets.RTP)
	fmt.Fprintf(w, "  dns\t%d\n", ws.Packets.DNS)
	fmt.Fprintf(w, "  log\t%d\n", ws.Packets.Log)
	fmt.Fprintf(w, "  hep_sent\t%d\n", ws.Packets.HEPSent)
	fmt.Fprintf(w, "  duplicates\t%d\n", ws.Packets.Duplicates)
	fmt.Fprintf(w, "  unknown\t%d\n", ws.Packets.Unknown)

	w.Flush()

	if len(ws.Transport) > 0 {
		fmt.Println("\n-- Transport")
		items := make([]map[string]interface{}, len(ws.Transport))
		for i, t := range ws.Transport {
			conn := "no"
			if t.Connected {
				conn = "yes"
			}
			items[i] = map[string]interface{}{
				"addr":       t.Addr,
				"proto":      t.Proto,
				"connected":  conn,
				"reconnects": t.Reconnects,
				"sent":       t.Sent,
				"errors":     t.Errors,
			}
		}
		keys := []string{"addr", "proto", "connected", "reconnects", "sent", "errors"}
		RenderResults(items, keys, FormatTable)
	}
}

// ---- health ----------------------------------------------------------------

func runAgentHealth(f AgentFlags) error {
	client := newAgentHTTPClient()
	// /health is always open (no auth required)
	url := agentBaseURL(f.Host) + "/health"
	body, code, err := agentGet(client, url, "", "")
	if err != nil {
		return err
	}
	if code != http.StatusOK && code != http.StatusServiceUnavailable {
		return fmt.Errorf("GET %s: HTTP %d: %s", url, code, strings.TrimSpace(string(body)))
	}

	if f.Format == "json" {
		fmt.Println(string(body))
		return nil
	}

	var h HeplifyHealth
	if err := json.Unmarshal(body, &h); err != nil {
		return fmt.Errorf("decode health response: %w", err)
	}

	statusMark := "ok"
	if h.Status != "ok" {
		statusMark = "DEGRADED"
	}
	fmt.Printf("heplify @ %s  status=%-8s  connected_transports=%d  queue=%d  buffer=%s\n",
		f.Host, statusMark, h.ConnectedTransports, h.QueueSize,
		agentFormatBytes(h.BufferSizeBytes))
	return nil
}

// ---- watch -----------------------------------------------------------------

func runAgentWatch(f AgentFlags) error {
	client := newAgentHTTPClient()
	url := agentBaseURL(f.Host) + "/api/stats"

	for {
		body, code, err := agentGet(client, url, f.User, f.Pass)

		fmt.Print("\033[2J\033[H") // clear screen
		fmt.Printf("heplify watch @ %s  [%s]  (Ctrl+C to stop)\n\n",
			f.Host, time.Now().Format("15:04:05"))

		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
		} else if code != http.StatusOK {
			fmt.Fprintf(os.Stderr, "HTTP %d: %s\n", code, strings.TrimSpace(string(body)))
		} else {
			var ws HeplifyWebStats
			if err := json.Unmarshal(body, &ws); err != nil {
				fmt.Fprintf(os.Stderr, "decode error: %v\n", err)
			} else {
				printAgentStats(f.Host, ws)
			}
		}

		time.Sleep(f.Interval)
	}
}

// ---- helpers ---------------------------------------------------------------

func agentFormatBytes(b int64) string {
	if b < 1024 {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(1024), 0
	for n := b / 1024; n >= 1024; n /= 1024 {
		div *= 1024
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}
