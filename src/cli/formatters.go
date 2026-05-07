package cli

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/guptarohit/asciigraph"
)

// OutputFormat enumerates the supported output formats.
type OutputFormat string

const (
	FormatTable    OutputFormat = "table"
	FormatVertical OutputFormat = "vertical"
	FormatCSV      OutputFormat = "csv"
	FormatJSON     OutputFormat = "json"
	FormatChart    OutputFormat = "chart"
	FormatCallflow OutputFormat = "callflow"
)

// ParseFormat converts a user-supplied string to an OutputFormat.
func ParseFormat(s string) OutputFormat {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "vertical", "\\g":
		return FormatVertical
	case "csv":
		return FormatCSV
	case "json":
		return FormatJSON
	case "chart":
		return FormatChart
	case "callflow", "flow", "ladder":
		return FormatCallflow
	default:
		return FormatTable
	}
}

// RenderResults formats items returned by the search API.
// keys is the ordered column list; if empty, keys are derived from items.
func RenderResults(items []map[string]interface{}, keys []string, format OutputFormat) {
	if len(items) == 0 {
		fmt.Fprintln(os.Stderr, "No results.")
		return
	}

	// Derive keys from first item if not supplied by the API
	if len(keys) == 0 {
		keys = deriveKeys(items[0])
	}

	switch format {
	case FormatVertical:
		renderVertical(items, keys)
	case FormatCSV:
		renderCSV(items, keys)
	case FormatJSON:
		renderJSON(items)
	case FormatChart:
		renderChart(items)
	case FormatCallflow:
		renderCallflow(items)
	default:
		renderTable(items, keys)
	}
}

// ---- Vertical (\G style) ---------------------------------------------------

func renderVertical(items []map[string]interface{}, keys []string) {
	// Find longest key name for alignment
	maxKeyLen := 0
	for _, k := range keys {
		if len(k) > maxKeyLen {
			maxKeyLen = len(k)
		}
	}

	for i, item := range items {
		fmt.Printf("*************************** %d. row ***************************\n", i+1)
		for _, k := range keys {
			fmt.Printf("%*s: %s\n", maxKeyLen, k, fmtVal(item[k]))
		}
	}
	fmt.Fprintf(os.Stderr, "\n%d rows in set\n\n", len(items))
}

// ---- Table -----------------------------------------------------------------

func renderTable(items []map[string]interface{}, keys []string) {
	// Calculate column widths
	widths := make([]int, len(keys))
	for i, k := range keys {
		widths[i] = len(k)
		if widths[i] < 4 {
			widths[i] = 4
		}
	}

	rows := make([][]string, len(items))
	for ri, item := range items {
		row := make([]string, len(keys))
		for ci, k := range keys {
			row[ci] = fmtVal(item[k])
			vl := len(row[ci])
			if vl > widths[ci] && vl <= 60 {
				widths[ci] = vl
			} else if vl > 60 && widths[ci] < 60 {
				widths[ci] = 60
			}
		}
		rows[ri] = row
	}

	// Header
	printSep(widths)
	printRow(keys, widths)
	printSep(widths)

	// Data
	for _, row := range rows {
		printRow(row, widths)
	}
	printSep(widths)

	fmt.Fprintf(os.Stderr, "\n%d rows in set\n\n", len(items))
}

func printRow(vals []string, widths []int) {
	fmt.Print("| ")
	for i, v := range vals {
		if len(v) > widths[i] {
			v = v[:widths[i]-3] + "..."
		}
		fmt.Printf("%-*s | ", widths[i], v)
	}
	fmt.Println()
}

func printSep(widths []int) {
	fmt.Print("+")
	for _, w := range widths {
		fmt.Print(strings.Repeat("-", w+2) + "+")
	}
	fmt.Println()
}

// ---- CSV -------------------------------------------------------------------

func renderCSV(items []map[string]interface{}, keys []string) {
	w := csv.NewWriter(os.Stdout)
	_ = w.Write(keys)
	for _, item := range items {
		row := make([]string, len(keys))
		for i, k := range keys {
			row[i] = fmtVal(item[k])
		}
		_ = w.Write(row)
	}
	w.Flush()
}

// ---- JSON ------------------------------------------------------------------

func renderJSON(items []map[string]interface{}) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(items)
}

// ---- Chart -----------------------------------------------------------------

// renderChart aggregates items by timestamp bucket and draws an ASCII graph
// of events over time with X-axis time labels.
func renderChart(items []map[string]interface{}) {
	if len(items) == 0 {
		fmt.Fprintln(os.Stderr, "No data to chart.")
		return
	}

	// Collect all timestamps
	var times []time.Time
	for _, item := range items {
		t := extractTime(item)
		if !t.IsZero() {
			times = append(times, t)
		}
	}
	if len(times) == 0 {
		fmt.Fprintln(os.Stderr, "No timestamp data found for chart.")
		return
	}

	sort.Slice(times, func(i, j int) bool { return times[i].Before(times[j]) })

	// Determine bucket size (aim for ~60 buckets max for terminal width)
	span := times[len(times)-1].Sub(times[0])
	bucketSize := time.Minute
	if span > 2*time.Hour {
		bucketSize = 5 * time.Minute
	}
	if span > 12*time.Hour {
		bucketSize = 30 * time.Minute
	}
	if span > 48*time.Hour {
		bucketSize = time.Hour
	}

	// Build buckets
	start := times[0].Truncate(bucketSize)
	end := times[len(times)-1].Truncate(bucketSize).Add(bucketSize)
	numBuckets := int(math.Ceil(float64(end.Sub(start)) / float64(bucketSize)))
	if numBuckets < 1 {
		numBuckets = 1
	}

	counts := make([]float64, numBuckets)
	for _, t := range times {
		idx := int(t.Sub(start) / bucketSize)
		if idx >= numBuckets {
			idx = numBuckets - 1
		}
		counts[idx]++
	}

	// Build x-axis time labels per bucket
	timeFmt := "15:04"
	if bucketSize >= time.Hour || span > 24*time.Hour {
		timeFmt = "01-02 15:04"
	}
	bucketLabels := make([]string, numBuckets)
	for i := range bucketLabels {
		bucketLabels[i] = start.Add(time.Duration(i) * bucketSize).Format(timeFmt)
	}

	const plotWidth = 80

	// Render graph without caption (we draw X axis and summary ourselves)
	graph := asciigraph.Plot(counts,
		asciigraph.Height(15),
		asciigraph.Width(plotWidth),
	)
	fmt.Println(graph)

	// ── X-axis labels ──

	// Find left margin: count visual runes up to and including ┤ or ┼
	leftMargin := 0
	for _, line := range strings.Split(graph, "\n") {
		for i, r := range []rune(line) {
			if r == '┤' || r == '┼' {
				leftMargin = i + 1
				break
			}
		}
		if leftMargin > 0 {
			break
		}
	}

	// Choose number of tick labels that don't overlap
	labelWidth := len(bucketLabels[0]) + 2 // label + 2 spaces padding
	maxTicks := plotWidth / labelWidth
	if maxTicks < 2 {
		maxTicks = 2
	}
	if maxTicks > 10 {
		maxTicks = 10
	}
	numTicks := maxTicks
	if numBuckets < numTicks {
		numTicks = numBuckets
	}
	if numTicks < 2 && numBuckets >= 2 {
		numTicks = 2
	}

	totalCols := leftMargin + plotWidth + 2

	// Tick-mark line: "|" at each tick position
	tickBuf := make([]rune, totalCols)
	for i := range tickBuf {
		tickBuf[i] = ' '
	}
	for i := 0; i < numTicks; i++ {
		pos := leftMargin
		if numTicks > 1 {
			pos += i * plotWidth / (numTicks - 1)
		}
		if pos < totalCols {
			tickBuf[pos] = '|'
		}
	}
	fmt.Println(strings.TrimRight(string(tickBuf), " "))

	// Label line: time strings centered under each tick
	labelBuf := make([]rune, totalCols+20)
	for i := range labelBuf {
		labelBuf[i] = ' '
	}
	for i := 0; i < numTicks; i++ {
		bucketIdx := 0
		if numTicks > 1 {
			bucketIdx = i * (numBuckets - 1) / (numTicks - 1)
		}
		label := bucketLabels[bucketIdx]

		pos := leftMargin
		if numTicks > 1 {
			pos += i*plotWidth/(numTicks-1) - len(label)/2
		}
		if pos < 0 {
			pos = 0
		}
		for ci, ch := range label {
			p := pos + ci
			if p >= 0 && p < len(labelBuf) {
				labelBuf[p] = ch
			}
		}
	}
	fmt.Println(strings.TrimRight(string(labelBuf), " "))

	// Summary line
	fmt.Printf("\n       Events per %s  |  %s .. %s  |  Total: %d\n",
		bucketSize,
		times[0].Format("2006-01-02 15:04"),
		times[len(times)-1].Format("2006-01-02 15:04"),
		len(times))
}

// ---- Call Flow (ladder diagram) --------------------------------------------

// callflowMsg represents one SIP message in the flow.
type callflowMsg struct {
	timestamp time.Time
	srcIP     string
	srcPort   string
	dstIP     string
	dstPort   string
	event     string // SIP method or response (INVITE, 200 OK, BYE, etc.)
	callID    string
}

// renderCallflow draws an ASCII ladder diagram like Homer/Wireshark call flow.
//
//	10.0.0.1          10.0.0.2          10.0.0.3
//	    |                 |                 |
//	    |--- INVITE ----->|                 |
//	    |<-- 100 Trying --|                 |
//	    |<-- 200 OK ------|                 |
//	    |--- ACK -------->|                 |
//	    |                 |--- BYE -------->|
//	    |                 |<-- 200 OK ------|
func renderCallflow(items []map[string]interface{}) {
	fmt.Print(buildCallflowString(items))
}

// sngrep-style layout constants (matches irontec/sngrep ui_call_flow.c)
const (
	cfTimePos  = 2  // timestamp starts at column 2
	cfFirstCol = 20 // first column pipe at column 20
	cfColSpan  = 30 // 30 chars between column pipes
)

func buildCallflowString(items []map[string]interface{}) string {
	if len(items) == 0 {
		return "No results.\n"
	}

	// 1. Parse & sort messages by timestamp
	msgs := parseCallflowMsgs(items)
	if len(msgs) == 0 {
		return "No SIP messages with src_ip/dst_ip found for call flow.\n"
	}

	// 2. Collect unique endpoints in order of appearance
	endpointOrder := []string{}
	endpointSet := map[string]bool{}
	for _, m := range msgs {
		src := endpointLabel(m.srcIP, m.srcPort)
		dst := endpointLabel(m.dstIP, m.dstPort)
		if !endpointSet[src] {
			endpointSet[src] = true
			endpointOrder = append(endpointOrder, src)
		}
		if !endpointSet[dst] {
			endpointSet[dst] = true
			endpointOrder = append(endpointOrder, dst)
		}
	}

	numEP := len(endpointOrder)
	if numEP < 2 {
		var sb strings.Builder
		for _, m := range msgs {
			sb.WriteString(fmt.Sprintf("%s  %s -> %s  %s\n",
				m.timestamp.Format("15:04:05.000"), m.srcIP, m.dstIP, m.event))
		}
		sb.WriteString(fmt.Sprintf("\n%d messages\n", len(msgs)))
		return sb.String()
	}

	// 3. Build endpoint → index map
	epIdx := map[string]int{}
	for i, ep := range endpointOrder {
		epIdx[ep] = i
	}

	// Total line width
	totalWidth := cfFirstCol + cfColSpan*(numEP-1) + 1
	if totalWidth < 80 {
		totalWidth = 80
	}

	var sb strings.Builder

	// ── Header row: IP labels centered above each column pipe ──
	hdr := cfMakeLine(totalWidth, ' ')
	for i, ep := range endpointOrder {
		pipePos := cfFirstCol + cfColSpan*i
		// center label: 22 chars wide like sngrep
		labelW := 22
		if len(ep)+2 > labelW {
			labelW = len(ep) + 2
		}
		start := pipePos - labelW/2
		if start < 0 {
			start = 0
		}
		for ci, ch := range []byte(ep) {
			pos := start + (labelW-len(ep))/2 + ci
			if pos >= 0 && pos < len(hdr) {
				hdr[pos] = ch
			}
		}
	}
	sb.WriteString(strings.TrimRight(string(hdr), " "))
	sb.WriteString("\n")

	// ── Separator line: horizontal dashes with T-junction at pipes ──
	sep := cfMakeLine(totalWidth, ' ')
	for i := 0; i < numEP; i++ {
		pipePos := cfFirstCol + cfColSpan*i
		// draw horizontal dashes 10 chars on each side of pipe
		dashStart := pipePos - 10
		if dashStart < cfTimePos+14 {
			dashStart = cfTimePos + 14
		}
		if i == 0 {
			dashStart = pipePos - 10
			if dashStart < 0 {
				dashStart = 0
			}
		}
		dashEnd := pipePos + 10
		if dashEnd >= totalWidth {
			dashEnd = totalWidth - 1
		}
		for j := dashStart; j <= dashEnd; j++ {
			if j >= 0 && j < len(sep) {
				sep[j] = '-'
			}
		}
		if pipePos < len(sep) {
			sep[pipePos] = '+'
		}
	}
	sb.WriteString(strings.TrimRight(string(sep), " "))
	sb.WriteString("\n")

	// ── Message rows: each message = 2 lines (method + arrow) ──
	for _, m := range msgs {
		src := endpointLabel(m.srcIP, m.srcPort)
		dst := endpointLabel(m.dstIP, m.dstPort)
		si, sok := epIdx[src]
		di, dok := epIdx[dst]
		if !sok || !dok || si == di {
			continue
		}

		srcPipe := cfFirstCol + cfColSpan*si
		dstPipe := cfFirstCol + cfColSpan*di

		leftPipe := srcPipe
		rightPipe := dstPipe
		leftToRight := true
		if srcPipe > dstPipe {
			leftPipe, rightPipe = dstPipe, srcPipe
			leftToRight = false
		}

		timeStr := m.timestamp.Format("15:04:05.000")

		// ── Line 1: timestamp + method label centered above arrow ──
		line1 := cfMakeLine(totalWidth, ' ')
		// Place pipes
		for j := 0; j < numEP; j++ {
			pos := cfFirstCol + cfColSpan*j
			if pos < len(line1) {
				line1[pos] = '|'
			}
		}
		// Timestamp at position 2
		copy(line1[cfTimePos:], []byte(timeStr))
		// Restore pipes that timestamp might have overwritten
		for j := 0; j < numEP; j++ {
			pos := cfFirstCol + cfColSpan*j
			if pos < len(line1) {
				line1[pos] = '|'
			}
		}
		// Method label centered between source and dest pipes
		label := m.event
		if len(label) > 26 {
			label = label[:26]
		}
		mid := (leftPipe + rightPipe) / 2
		labelStart := mid - len(label)/2
		if labelStart <= leftPipe {
			labelStart = leftPipe + 2
		}
		if labelStart+len(label) >= rightPipe {
			labelStart = rightPipe - len(label) - 1
		}
		if labelStart < 0 {
			labelStart = leftPipe + 2
		}
		for ci, ch := range []byte(label) {
			pos := labelStart + ci
			if pos > leftPipe && pos < rightPipe && pos < len(line1) {
				line1[pos] = ch
			}
		}
		sb.WriteString(strings.TrimRight(string(line1), " "))
		sb.WriteString("\n")

		// ── Line 2: arrow line with pipes ──
		line2 := cfMakeLine(totalWidth, ' ')
		// Place all pipes first
		for j := 0; j < numEP; j++ {
			pos := cfFirstCol + cfColSpan*j
			if pos < len(line2) {
				line2[pos] = '|'
			}
		}
		// Draw horizontal line between src and dst pipes
		for j := leftPipe + 1; j < rightPipe; j++ {
			if j < len(line2) {
				line2[j] = '-'
			}
		}
		// Arrow head
		if leftToRight {
			if rightPipe-1 > leftPipe && rightPipe-1 < len(line2) {
				line2[rightPipe-1] = '>'
			}
		} else {
			if leftPipe+1 < rightPipe && leftPipe+1 < len(line2) {
				line2[leftPipe+1] = '<'
			}
		}
		sb.WriteString(strings.TrimRight(string(line2), " "))
		sb.WriteString("\n")
	}

	// ── Footer ──
	sb.WriteString(fmt.Sprintf("\n%d messages, %d endpoints\n", len(msgs), numEP))

	// Call-ID legend
	callIDs := map[string]bool{}
	for _, m := range msgs {
		if m.callID != "" {
			callIDs[m.callID] = true
		}
	}
	if len(callIDs) > 0 {
		sb.WriteString("Call-IDs: ")
		first := true
		for cid := range callIDs {
			if !first {
				sb.WriteString(", ")
			}
			sb.WriteString(cid)
			first = false
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

func cfMakeLine(width int, fill byte) []byte {
	line := make([]byte, width)
	for i := range line {
		line[i] = fill
	}
	return line
}

// endpointLabel returns "ip:port" if port is available, otherwise just "ip".
func endpointLabel(ip, port string) string {
	if port != "" && port != "0" {
		return ip + ":" + port
	}
	return ip
}

func parseCallflowMsgs(items []map[string]interface{}) []callflowMsg {
	msgs := make([]callflowMsg, 0, len(items))
	for _, item := range items {
		srcIP := getStr(item, "src_ip")
		dstIP := getStr(item, "dst_ip")
		if srcIP == "" || dstIP == "" {
			continue
		}

		event := getStr(item, "event")
		if event == "" {
			event = getStr(item, "method")
		}
		if event == "" {
			event = "?"
		}

		m := callflowMsg{
			timestamp: extractTime(item),
			srcIP:     srcIP,
			srcPort:   getStr(item, "src_port"),
			dstIP:     dstIP,
			dstPort:   getStr(item, "dst_port"),
			event:     event,
			callID:    firstNonEmpty(getStr(item, "session_id"), getStr(item, "cid"), getStr(item, "call_id")),
		}
		msgs = append(msgs, m)
	}

	// Sort by timestamp
	sort.Slice(msgs, func(i, j int) bool {
		return msgs[i].timestamp.Before(msgs[j].timestamp)
	})

	return msgs
}

func getStr(item map[string]interface{}, key string) string {
	v, ok := item[key]
	if !ok || v == nil {
		return ""
	}
	switch sv := v.(type) {
	case string:
		return sv
	case float64:
		if sv == math.Trunc(sv) {
			return strconv.FormatInt(int64(sv), 10)
		}
		return strconv.FormatFloat(sv, 'f', -1, 64)
	default:
		return fmt.Sprintf("%v", sv)
	}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// FilterKeys returns only the keys specified in the comma-separated `fields` string.
// If keys is empty/nil, it derives them from items first. Only fields that actually
// exist in the data are returned, preserving the user's requested order.
func FilterKeys(keys []string, fields string, items []map[string]interface{}) []string {
	parts := strings.Split(fields, ",")
	wanted := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			wanted = append(wanted, p)
		}
	}
	if len(wanted) == 0 {
		return keys
	}

	// Build a set of available keys for fast lookup
	available := make(map[string]bool, len(keys))
	if len(keys) > 0 {
		for _, k := range keys {
			available[k] = true
		}
	} else if len(items) > 0 {
		for k := range items[0] {
			available[k] = true
		}
	}

	// Return only wanted fields that exist in the data
	result := make([]string, 0, len(wanted))
	for _, w := range wanted {
		if available[w] {
			result = append(result, w)
		}
	}

	if len(result) == 0 {
		// None matched — show all available keys and warn
		fmt.Fprintf(os.Stderr, "Warning: none of the requested fields (%s) found. Available: %s\n",
			fields, strings.Join(keys, ", "))
		return keys
	}

	return result
}

// ---- Post-filter -----------------------------------------------------------

// PostFilterItems filters items client-side based on a pattern string.
//
// Pattern syntax:
//   - Simple pattern: "INVITE" — matches if any field value contains "INVITE" (case-insensitive)
//   - Field-specific: "event=INVITE" — matches only if the "event" field contains "INVITE"
//   - Multiple patterns: "INVITE,BYE" — matches if any pattern matches (OR logic)
//   - Field-specific multiple: "event=INVITE,event=BYE" — OR across field-specific patterns
//
// If include is true, only matching rows are kept (grep).
// If include is false, matching rows are removed (exclude).
func PostFilterItems(items []map[string]interface{}, pattern string, include bool) []map[string]interface{} {
	if pattern == "" {
		return items
	}

	// Parse comma-separated patterns
	type filterRule struct {
		field string // empty = match any field
		value string // lowercase for case-insensitive matching
	}
	var rules []filterRule
	for _, p := range strings.Split(pattern, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if eqIdx := strings.Index(p, "="); eqIdx > 0 {
			rules = append(rules, filterRule{
				field: strings.TrimSpace(p[:eqIdx]),
				value: strings.ToLower(strings.TrimSpace(p[eqIdx+1:])),
			})
		} else {
			rules = append(rules, filterRule{value: strings.ToLower(p)})
		}
	}
	if len(rules) == 0 {
		return items
	}

	result := make([]map[string]interface{}, 0, len(items))
	for _, item := range items {
		matched := false
		for _, r := range rules {
			if r.field != "" {
				// Field-specific match
				v := strings.ToLower(fmtVal(item[r.field]))
				if strings.Contains(v, r.value) {
					matched = true
					break
				}
			} else {
				// Match against any field value
				for _, fv := range item {
					if strings.Contains(strings.ToLower(fmtVal(fv)), r.value) {
						matched = true
						break
					}
				}
				if matched {
					break
				}
			}
		}
		if (include && matched) || (!include && !matched) {
			result = append(result, item)
		}
	}

	if include {
		fmt.Fprintf(os.Stderr, "Post-filter (grep %q): %d → %d rows\n", pattern, len(items), len(result))
	} else {
		fmt.Fprintf(os.Stderr, "Post-filter (exclude %q): %d → %d rows\n", pattern, len(items), len(result))
	}

	return result
}

// ---- helpers ---------------------------------------------------------------

func fmtVal(v interface{}) string {
	if v == nil {
		return "NULL"
	}
	switch val := v.(type) {
	case string:
		if len(val) > 200 {
			return val[:197] + "..."
		}
		return val
	case float64:
		// JSON numbers are float64 by default
		if val == math.Trunc(val) && math.Abs(val) < 1e15 {
			return strconv.FormatInt(int64(val), 10)
		}
		return strconv.FormatFloat(val, 'f', -1, 64)
	case bool:
		if val {
			return "true"
		}
		return "false"
	default:
		return fmt.Sprintf("%v", val)
	}
}

func deriveKeys(item map[string]interface{}) []string {
	keys := make([]string, 0, len(item))
	for k := range item {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// extractTime tries to pull a time.Time from various timestamp formats.
func extractTime(item map[string]interface{}) time.Time {
	for _, key := range []string{"timestamp", "create_date", "date"} {
		v, ok := item[key]
		if !ok {
			continue
		}
		switch tv := v.(type) {
		case string:
			for _, layout := range []string{
				time.RFC3339Nano,
				time.RFC3339,
				"2006-01-02T15:04:05",
				"2006-01-02 15:04:05",
				"2006-01-02 15:04:05.000",
			} {
				if t, err := time.Parse(layout, tv); err == nil {
					return t
				}
			}
		case float64:
			// Could be epoch seconds or milliseconds
			if tv > 1e15 {
				// nanoseconds
				return time.Unix(0, int64(tv))
			}
			if tv > 1e12 {
				// milliseconds
				return time.Unix(int64(tv)/1000, (int64(tv)%1000)*1e6)
			}
			return time.Unix(int64(tv), 0)
		}
	}
	return time.Time{}
}
