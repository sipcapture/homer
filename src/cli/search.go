package cli

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/sipcapture/homer-core/src/pcapwriter"
)

// SearchFlags holds all CLI flags for the search sub-command.
type SearchFlags struct {
	// Connection
	Host  string // coordinator host:port
	User  string // username for login
	Pass  string // password for login
	Token string // session JWT from login, or Auth Token secret from Settings (same as UI)

	// Search filters
	From      string // time range start: "1h", "30m", ISO timestamp
	To        string // time range end (default: now)
	Proto     int    // proto_type sent to API: sip=1, rtcp=5, … otlp_traces=200, lp=300 (see ParseSearchProto)
	EventType string // SIP: "call"|"registration"|"default"; OTLP (otlp_* / 200–202): "default"; LP (lp / 300): "schema__table"
	Method    string // SIP method (INVITE, REGISTER, etc.)
	CallID    string // session/call ID
	FromUser  string // from_user
	ToUser    string // to_user
	SrcIP     string // source IP
	DstIP     string // destination IP
	Node      string // node name
	Limit     int    // result limit

	// Raw SQL mode (bypasses structured search)
	SQL string // raw SQL query (sent to /api/v4/query)

	// Aggregation (structured)
	Select  string // custom SELECT columns/aggregations (e.g. "method, count(*) as cnt")
	GroupBy string // GROUP BY clause (e.g. "method")
	OrderBy string // custom ORDER BY (e.g. "cnt DESC")

	// Output
	Format      string // "table", "csv", "json", "chart"
	Fields      string // comma-separated list of fields to display (empty = all)
	Interactive bool   // -i: launch TUI mode
	Debug       bool   // print request/response to stderr

	// Post-filters (client-side, applied after API response)
	Grep    string // include only rows matching pattern
	Exclude string // exclude rows matching pattern

	// Output file (e.g. --format pcap)
	Output string // set from --output or -o
}

// searchFlagRefs holds pointers for flag.FlagSet parsing.
type searchFlagRefs struct {
	Host        *string
	User        *string
	Pass        *string
	Token       *string
	From        *string
	To          *string
	Proto       *searchProtoFlag
	EventType   *string
	Method      *string
	CallID      *string
	FromUser    *string
	ToUser      *string
	SrcIP       *string
	DstIP       *string
	Node        *string
	Limit       *int
	SQL         *string
	Select      *string
	GroupBy     *string
	OrderBy     *string
	Format      *string
	Fields      *string
	Interactive *bool
	Debug       *bool
	Grep        *string
	Exclude     *string
	Output      *string
	OutputShort *string
}

// RegisterSearchFlags creates a FlagSet for "homer search" subcommand.
func RegisterSearchFlags() (*flag.FlagSet, *searchFlagRefs) {
	fs := flag.NewFlagSet("search", flag.ExitOnError)
	refs := &searchFlagRefs{}

	refs.Host = fs.String("host", "", "coordinator host:port (e.g. 10.0.0.1:8081)")
	refs.User = fs.String("user", "", "username for coordinator authentication")
	refs.Pass = fs.String("pass", "", "password for coordinator authentication")
	refs.Token = fs.String("token", "", "session JWT or Settings→Auth Token secret (skip user/pass); env HOMER_TOKEN; use --token=<value> if the next arg is another flag")
	refs.From = fs.String("from", "1h", "time range start (1h, 30m, 2h30m, 1d, or ISO 8601)")
	refs.To = fs.String("to", "", "time range end (default: now)")
	protoHelp := `protocol: sip|rtcp|rtp|dns|log|otlp_traces|otlp_metrics|otlp_logs|lp or decimal (1,5,34,35,100,200,201,202,300,…; default sip)`
	refs.Proto = &searchProtoFlag{}
	fs.Var(refs.Proto, "proto", protoHelp)
	refs.EventType = fs.String("event-type", "", "event type: call|registration|default (SIP); default (OTLP); schema__table (LP)")
	refs.Method = fs.String("method", "", "SIP method filter (INVITE, REGISTER, etc.)")
	refs.CallID = fs.String("call-id", "", "call/session ID filter")
	refs.FromUser = fs.String("from-user", "", "from_user filter")
	refs.ToUser = fs.String("to-user", "", "to_user filter")
	refs.SrcIP = fs.String("src-ip", "", "source IP filter")
	refs.DstIP = fs.String("dst-ip", "", "destination IP filter")
	refs.Node = fs.String("node", "", "node name filter")
	refs.Limit = fs.Int("limit", 50, "result limit (default: 50, max: 50000)")
	refs.SQL = fs.String("sql", "", "raw SQL query (bypasses structured search, sent to /api/v4/query)")
	refs.Select = fs.String("select", "", "custom SELECT columns/aggregations (e.g. \"method, count(*) as cnt\")")
	refs.GroupBy = fs.String("group-by", "", "GROUP BY clause (e.g. \"method\")")
	refs.OrderBy = fs.String("order-by", "", "custom ORDER BY (e.g. \"cnt DESC\", default: timestamp DESC)")
	refs.Format = fs.String("format", "table", "output format: table, vertical, csv, json, chart, callflow, pcap")
	refs.Fields = fs.String("fields", "", "comma-separated fields to display (e.g. timestamp,method,source_ip,call_id)")
	refs.Interactive = fs.Bool("interactive", false, "launch interactive TUI search mode")
	refs.Debug = fs.Bool("debug", false, "print request/response debug info to stderr")
	refs.Grep = fs.String("grep", "", "post-filter: include rows matching pattern (e.g. INVITE or event=INVITE)")
	refs.Exclude = fs.String("exclude", "", "post-filter: exclude rows matching pattern (e.g. \"100 Trying\")")
	refs.Output = fs.String("output", "", "output file path (required with --format pcap)")
	refs.OutputShort = fs.String("o", "", "shorthand for --output (pcap file path)")

	return fs, refs
}

// ParseSearchFlags extracts SearchFlags from parsed flag refs.
func ParseSearchFlags(refs *searchFlagRefs) SearchFlags {
	return SearchFlags{
		Host:        *refs.Host,
		User:        *refs.User,
		Pass:        *refs.Pass,
		Token:       *refs.Token,
		From:        *refs.From,
		To:          *refs.To,
		Proto:       refs.Proto.Int(),
		EventType:   *refs.EventType,
		Method:      *refs.Method,
		CallID:      *refs.CallID,
		FromUser:    *refs.FromUser,
		ToUser:      *refs.ToUser,
		SrcIP:       *refs.SrcIP,
		DstIP:       *refs.DstIP,
		Node:        *refs.Node,
		Limit:       *refs.Limit,
		SQL:         *refs.SQL,
		Select:      *refs.Select,
		GroupBy:     *refs.GroupBy,
		OrderBy:     *refs.OrderBy,
		Format:      *refs.Format,
		Fields:      *refs.Fields,
		Interactive: *refs.Interactive,
		Debug:       *refs.Debug,
		Grep:        *refs.Grep,
		Exclude:     *refs.Exclude,
		Output:      mergeOutputPath(*refs.Output, *refs.OutputShort),
	}
}

func mergeOutputPath(primary, alt string) string {
	primary = strings.TrimSpace(primary)
	alt = strings.TrimSpace(alt)
	if primary != "" {
		return primary
	}
	return alt
}

// RunSearch is the main entry point. It dispatches to either the interactive
// TUI or a one-shot CLI search depending on flags.
func RunSearch(f SearchFlags) error {
	if f.Host == "" {
		return fmt.Errorf("--host is required (coordinator address, e.g. 10.0.0.1:8081)")
	}

	if f.Interactive && ParseFormat(f.Format) == FormatPcap {
		return fmt.Errorf("--format pcap cannot be used with --interactive; run without -i and set --output (-o) for the pcap file path")
	}

	client := NewClient(f.Host)
	client.Debug = f.Debug
	if f.Token != "" {
		client.Token = f.Token
	}

	// Authenticate
	if err := client.EnsureAuth(f.User, f.Pass); err != nil {
		return fmt.Errorf("authentication: %w", err)
	}

	if f.Interactive {
		return RunTUI(client, f)
	}

	return runOneShotSearch(client, f)
}

// runOneShotSearch builds a SearchRequest from flags, executes it, and renders.
// If --sql is set, uses the raw query endpoint instead.
func runOneShotSearch(client *Client, f SearchFlags) error {
	var resp *SearchResponse
	var err error

	queryStart := time.Now()

	if f.SQL != "" {
		// Raw SQL mode: send to /api/v4/query
		fmt.Fprintf(os.Stderr, "Querying %s (raw SQL) ...\n", client.BaseURL)
		resp, err = client.RawQuery(f.SQL, f.Limit)
	} else {
		// Structured search mode
		var req SearchRequest
		req, err = buildSearchRequest(f)
		if err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "Searching %s ...\n", client.BaseURL)
		resp, err = client.Search(req)
	}

	if err != nil {
		return err
	}
	queryElapsed := time.Since(queryStart)

	items := resp.Data.Items

	// Post-filter: grep (include) and exclude
	if f.Grep != "" {
		items = PostFilterItems(items, f.Grep, true)
	}
	if f.Exclude != "" {
		items = PostFilterItems(items, f.Exclude, false)
	}

	format := ParseFormat(f.Format)
	if format == FormatPcap {
		if err := writeSearchPcap(items, f); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "Query completed. %d rows fetched in %.2f ms\n", len(items), float64(queryElapsed.Microseconds())/1000.0)
		return nil
	}

	keys := resp.Data.Keys
	if f.Fields != "" {
		keys = FilterKeys(keys, f.Fields, items)
	}
	RenderResults(items, keys, format)

	// Print query timing summary
	fmt.Fprintf(os.Stderr, "Query completed. %d rows returned in %.2f ms\n", len(items), float64(queryElapsed.Microseconds())/1000.0)
	return nil
}

// writeSearchPcap writes SIP search rows to a PCAP file (same framing as API export/pcap).
func writeSearchPcap(items []map[string]interface{}, f SearchFlags) error {
	proto := f.Proto
	if proto == 0 {
		proto = 1
	}
	if proto != 1 {
		return fmt.Errorf("--format pcap is only supported for SIP (use default or --proto sip / 1); got proto_type %d", f.Proto)
	}
	out := strings.TrimSpace(f.Output)
	if out == "" {
		return fmt.Errorf("--output (-o) is required when using --format pcap")
	}

	sort.Slice(items, func(i, j int) bool {
		return pcapwriter.RowTime(items[i], "timestamp").Before(pcapwriter.RowTime(items[j], "timestamp"))
	})

	raw, err := pcapwriter.SIPSearchRowsToPCAP(items)
	if err != nil {
		if errors.Is(err, pcapwriter.ErrNoSIPPacketsInRows) {
			return fmt.Errorf("no SIP payloads in results (need non-empty payload plus src_ip/dst_ip); widen search or use SIP proto")
		}
		return err
	}
	if err := os.WriteFile(out, raw, 0o644); err != nil {
		return fmt.Errorf("write pcap file: %w", err)
	}
	n := pcapwriter.SIPSearchRowsPacketCount(items)
	fmt.Fprintf(os.Stderr, "Wrote %d packets to %s\n", n, out)
	return nil
}

// buildSearchRequest converts SearchFlags to a SearchRequest.
func buildSearchRequest(f SearchFlags) (SearchRequest, error) {
	now := time.Now()

	fromTime, err := parseTimeSpec(f.From, now)
	if err != nil {
		return SearchRequest{}, fmt.Errorf("invalid --from: %w", err)
	}

	var toTime time.Time
	if f.To == "" {
		toTime = now
	} else {
		toTime, err = parseTimeSpec(f.To, now)
		if err != nil {
			return SearchRequest{}, fmt.Errorf("invalid --to: %w", err)
		}
	}

	limit := f.Limit
	if limit <= 0 {
		limit = 50
	}

	// Default proto_type to 1 (SIP) if not specified
	proto := f.Proto
	if proto == 0 {
		proto = 1
	}

	// Default event_type to "call" for SIP proto (1).
	// OTLP (otlp_traces / 200–202): all map to profile "default" — no event_type injection needed.
	// LP (lp / 300): event_type must be supplied by the caller as "schema__table" (e.g. "main__cpu").
	eventType := f.EventType
	if eventType == "" && proto == 1 {
		eventType = "call"
	}

	return SearchRequest{
		Filter: SearchFilter{
			ProtoType: proto,
			EventType: eventType,
			Method:    f.Method,
			CallID:    f.CallID,
			FromUser:  f.FromUser,
			ToUser:    f.ToUser,
			SrcIP:     f.SrcIP,
			DstIP:     f.DstIP,
			Node:      f.Node,
		},
		Param: SearchParam{
			Limit:   limit,
			Select:  f.Select,
			GroupBy: f.GroupBy,
			OrderBy: f.OrderBy,
		},
		Timestamp: SearchTimestamp{
			From: fromTime.UnixMilli(),
			To:   toTime.UnixMilli(),
		},
	}, nil
}

// parseTimeSpec parses a time specification. Supported formats:
//   - Duration shorthand: "1h", "30m", "2h30m", "1d" → relative to `now`
//   - Epoch milliseconds: "1707260400000"
//   - ISO 8601: "2025-01-30T12:00:00Z"
//   - Date-time: "2025-01-30 12:00:00"
func parseTimeSpec(spec string, now time.Time) (time.Time, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" || spec == "now" {
		return now, nil
	}

	// Try duration shorthand (possibly with "d" for days)
	if d, err := parseDuration(spec); err == nil {
		return now.Add(-d), nil
	}

	// Try epoch milliseconds
	if ms, err := strconv.ParseInt(spec, 10, 64); err == nil && ms > 1e11 {
		return time.Unix(ms/1000, (ms%1000)*1e6), nil
	}

	// Try common time layouts
	for _, layout := range []string{
		time.RFC3339,
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
		"2006-01-02",
	} {
		if t, err := time.Parse(layout, spec); err == nil {
			return t, nil
		}
	}

	return time.Time{}, fmt.Errorf("cannot parse %q as time (use e.g. 1h, 30m, 2h30m, 1d, or ISO 8601)", spec)
}

// durationRe matches shorthand like "1d", "2h30m", "45s", etc.
var durationRe = regexp.MustCompile(`^(\d+d)?(\d+h)?(\d+m)?(\d+s)?$`)

// parseDuration extends Go's time.ParseDuration with support for "d" (days).
func parseDuration(s string) (time.Duration, error) {
	s = strings.ToLower(strings.TrimSpace(s))

	// If it already works with Go's parser, use that
	if d, err := time.ParseDuration(s); err == nil {
		return d, nil
	}

	// Try our extended format with days
	if !durationRe.MatchString(s) {
		return 0, fmt.Errorf("invalid duration: %s", s)
	}

	var total time.Duration
	remaining := s

	// Extract days
	if idx := strings.Index(remaining, "d"); idx >= 0 {
		days, err := strconv.Atoi(remaining[:idx])
		if err != nil {
			return 0, fmt.Errorf("invalid days in %q: %w", s, err)
		}
		total += time.Duration(days) * 24 * time.Hour
		remaining = remaining[idx+1:]
	}

	// Parse the rest with Go's parser
	if remaining != "" {
		d, err := time.ParseDuration(remaining)
		if err != nil {
			return 0, fmt.Errorf("invalid duration %q: %w", s, err)
		}
		total += d
	}

	if total == 0 {
		return 0, fmt.Errorf("zero duration")
	}

	return total, nil
}
