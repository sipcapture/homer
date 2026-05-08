package cli

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ---- styles ----------------------------------------------------------------

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("205")).
			MarginBottom(1)

	focusedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))
	blurredStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	cursorStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))
	noStyle      = lipgloss.NewStyle()

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241")).
			MarginTop(1)

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")).
			Bold(true)

	successStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("82"))

	resultHeaderStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("39"))
)

// ---- field indices ---------------------------------------------------------

const (
	fieldHost = iota
	fieldFrom
	fieldTo
	fieldProto
	fieldEventType
	fieldMethod
	fieldCallID
	fieldFromUser
	fieldToUser
	fieldSrcIP
	fieldDstIP
	fieldNode
	fieldLimit
	fieldFormat
	fieldFields
	fieldGrep
	fieldExclude
	numFields
)

// ---- model -----------------------------------------------------------------

type tuiView int

const (
	viewForm tuiView = iota
	viewResults
)

type searchResult struct {
	items []map[string]interface{}
	keys  []string
	err   error
}

type model struct {
	client  *Client
	inputs  []textinput.Model
	focused int
	view    tuiView

	// results state
	result     *SearchResponse
	resultErr  error
	resultText string
	searching  bool

	// initial flags
	initFlags SearchFlags
	width     int
	height    int
}

// RunTUI starts the interactive TUI search application.
func RunTUI(client *Client, f SearchFlags) error {
	m := newModel(client, f)
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}

func newModel(client *Client, f SearchFlags) model {
	inputs := make([]textinput.Model, numFields)

	for i := range inputs {
		t := textinput.New()
		t.Cursor.Style = cursorStyle
		t.CharLimit = 256
		inputs[i] = t
	}

	inputs[fieldHost].Placeholder = "coordinator:8081"
	inputs[fieldHost].Prompt = "Host:       "
	if f.Host != "" {
		inputs[fieldHost].SetValue(f.Host)
	}

	inputs[fieldFrom].Placeholder = "1h (e.g. 30m, 2h, 1d)"
	inputs[fieldFrom].Prompt = "From:       "
	if f.From != "" {
		inputs[fieldFrom].SetValue(f.From)
	} else {
		inputs[fieldFrom].SetValue("1h")
	}

	inputs[fieldTo].Placeholder = "now"
	inputs[fieldTo].Prompt = "To:         "
	if f.To != "" {
		inputs[fieldTo].SetValue(f.To)
	}

	inputs[fieldProto].Placeholder = "sip  rtcp  rtp  dns  log  otlp_traces  otlp_metrics  otlp_logs  lp  (or 1,5,34,…)"
	inputs[fieldProto].Prompt = "Proto:      "
	if f.Proto > 0 {
		inputs[fieldProto].SetValue(FormatSearchProtoDisplay(f.Proto))
	} else {
		inputs[fieldProto].SetValue("sip")
	}

	inputs[fieldEventType].Placeholder = "call | registration | default | default (OTLP 200-202) | schema__table (LP 300)"
	inputs[fieldEventType].Prompt = "Event Type: "
	if f.EventType != "" {
		inputs[fieldEventType].SetValue(f.EventType)
	} else {
		inputs[fieldEventType].SetValue("call")
	}

	inputs[fieldMethod].Placeholder = "INVITE, REGISTER, OPTIONS..."
	inputs[fieldMethod].Prompt = "Method:     "
	if f.Method != "" {
		inputs[fieldMethod].SetValue(f.Method)
	}

	inputs[fieldCallID].Placeholder = "session/call ID"
	inputs[fieldCallID].Prompt = "Call-ID:    "
	if f.CallID != "" {
		inputs[fieldCallID].SetValue(f.CallID)
	}

	inputs[fieldFromUser].Placeholder = "from_user"
	inputs[fieldFromUser].Prompt = "From User:  "
	if f.FromUser != "" {
		inputs[fieldFromUser].SetValue(f.FromUser)
	}

	inputs[fieldToUser].Placeholder = "to_user"
	inputs[fieldToUser].Prompt = "To User:    "
	if f.ToUser != "" {
		inputs[fieldToUser].SetValue(f.ToUser)
	}

	inputs[fieldSrcIP].Placeholder = "source IP"
	inputs[fieldSrcIP].Prompt = "Src IP:     "
	if f.SrcIP != "" {
		inputs[fieldSrcIP].SetValue(f.SrcIP)
	}

	inputs[fieldDstIP].Placeholder = "destination IP"
	inputs[fieldDstIP].Prompt = "Dst IP:     "
	if f.DstIP != "" {
		inputs[fieldDstIP].SetValue(f.DstIP)
	}

	inputs[fieldNode].Placeholder = "node name"
	inputs[fieldNode].Prompt = "Node:       "
	if f.Node != "" {
		inputs[fieldNode].SetValue(f.Node)
	}

	inputs[fieldLimit].Placeholder = "50"
	inputs[fieldLimit].Prompt = "Limit:      "
	if f.Limit > 0 {
		inputs[fieldLimit].SetValue(strconv.Itoa(f.Limit))
	} else {
		inputs[fieldLimit].SetValue("50")
	}

	inputs[fieldFormat].Placeholder = "table (table, vertical, csv, json, chart, callflow; pcap is CLI-only)"
	inputs[fieldFormat].Prompt = "Format:     "
	if f.Format != "" {
		inputs[fieldFormat].SetValue(f.Format)
	} else {
		inputs[fieldFormat].SetValue("table")
	}

	inputs[fieldFields].Placeholder = "all (e.g. timestamp,method,source_ip,call_id)"
	inputs[fieldFields].Prompt = "Fields:     "
	inputs[fieldFields].CharLimit = 512
	if f.Fields != "" {
		inputs[fieldFields].SetValue(f.Fields)
	}

	inputs[fieldGrep].Placeholder = "include pattern (e.g. INVITE or event=INVITE)"
	inputs[fieldGrep].Prompt = "Grep:       "
	if f.Grep != "" {
		inputs[fieldGrep].SetValue(f.Grep)
	}

	inputs[fieldExclude].Placeholder = "exclude pattern (e.g. 100,183)"
	inputs[fieldExclude].Prompt = "Exclude:    "
	if f.Exclude != "" {
		inputs[fieldExclude].SetValue(f.Exclude)
	}

	// Focus first field
	inputs[0].Focus()
	inputs[0].PromptStyle = focusedStyle
	inputs[0].TextStyle = focusedStyle

	return model{
		client:    client,
		inputs:    inputs,
		focused:   0,
		view:      viewForm,
		initFlags: f,
	}
}

// ---- tea.Model implementation ----------------------------------------------

func (m model) Init() tea.Cmd {
	return textinput.Blink
}

// searchMsg carries the result of an async search.
type searchMsg struct {
	resp *SearchResponse
	err  error
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			if m.view == viewResults {
				m.view = viewForm
				return m, nil
			}
			return m, tea.Quit

		case "tab", "down":
			if m.view == viewForm {
				m.focused = (m.focused + 1) % numFields
				return m, m.updateFocus()
			}

		case "shift+tab", "up":
			if m.view == viewForm {
				m.focused = (m.focused - 1 + numFields) % numFields
				return m, m.updateFocus()
			}

		case "enter":
			if m.view == viewForm {
				m.searching = true
				m.view = viewResults
				return m, m.doSearch()
			}

		case "f":
			// Cycle format in results view
			if m.view == viewResults && !m.searching {
				cur := ParseFormat(m.inputs[fieldFormat].Value())
				switch cur {
				case FormatTable:
					m.inputs[fieldFormat].SetValue("vertical")
				case FormatVertical:
					m.inputs[fieldFormat].SetValue("csv")
				case FormatCSV:
					m.inputs[fieldFormat].SetValue("json")
				case FormatJSON:
					m.inputs[fieldFormat].SetValue("chart")
				case FormatChart:
					m.inputs[fieldFormat].SetValue("callflow")
				case FormatCallflow:
					m.inputs[fieldFormat].SetValue("table")
				case FormatPcap:
					m.inputs[fieldFormat].SetValue("table")
				}
				m.resultText = m.renderResultsToString()
				return m, nil
			}

		case "r":
			// Re-run search from results view
			if m.view == viewResults && !m.searching {
				m.searching = true
				return m, m.doSearch()
			}
		}

	case searchMsg:
		m.searching = false
		if msg.err != nil {
			m.resultErr = msg.err
			m.resultText = ""
		} else {
			m.result = msg.resp
			m.resultErr = nil
			m.resultText = m.renderResultsToString()
		}
		return m, nil
	}

	// Update focused input
	if m.view == viewForm {
		cmd := m.updateInputs(msg)
		return m, cmd
	}

	return m, nil
}

func (m *model) updateFocus() tea.Cmd {
	cmds := make([]tea.Cmd, len(m.inputs))
	for i := range m.inputs {
		if i == m.focused {
			cmds[i] = m.inputs[i].Focus()
			m.inputs[i].PromptStyle = focusedStyle
			m.inputs[i].TextStyle = focusedStyle
		} else {
			m.inputs[i].Blur()
			m.inputs[i].PromptStyle = noStyle
			m.inputs[i].TextStyle = noStyle
		}
	}
	return tea.Batch(cmds...)
}

func (m *model) updateInputs(msg tea.Msg) tea.Cmd {
	cmds := make([]tea.Cmd, len(m.inputs))
	for i := range m.inputs {
		m.inputs[i], cmds[i] = m.inputs[i].Update(msg)
	}
	return tea.Batch(cmds...)
}

func (m model) doSearch() tea.Cmd {
	return func() tea.Msg {
		f, err := m.flagsFromInputs()
		if err != nil {
			return searchMsg{err: err}
		}
		req, err := buildSearchRequest(f)
		if err != nil {
			return searchMsg{err: err}
		}

		// Re-create client if host changed
		host := m.inputs[fieldHost].Value()
		client := m.client
		if host != "" && host != m.initFlags.Host {
			client = NewClient(host)
			client.Token = m.client.Token
		}

		resp, err := client.Search(req)
		return searchMsg{resp: resp, err: err}
	}
}

func (m model) flagsFromInputs() (SearchFlags, error) {
	protoStr := strings.TrimSpace(m.inputs[fieldProto].Value())
	proto, err := ParseSearchProto(protoStr)
	if err != nil {
		return SearchFlags{}, fmt.Errorf("proto: %w", err)
	}
	limit, _ := strconv.Atoi(m.inputs[fieldLimit].Value())

	return SearchFlags{
		Host:      m.inputs[fieldHost].Value(),
		From:      m.inputs[fieldFrom].Value(),
		To:        m.inputs[fieldTo].Value(),
		Proto:     proto,
		EventType: m.inputs[fieldEventType].Value(),
		Method:    m.inputs[fieldMethod].Value(),
		CallID:    m.inputs[fieldCallID].Value(),
		FromUser:  m.inputs[fieldFromUser].Value(),
		ToUser:    m.inputs[fieldToUser].Value(),
		SrcIP:     m.inputs[fieldSrcIP].Value(),
		DstIP:     m.inputs[fieldDstIP].Value(),
		Node:      m.inputs[fieldNode].Value(),
		Limit:     limit,
		Format:    m.inputs[fieldFormat].Value(),
		Fields:    m.inputs[fieldFields].Value(),
		Grep:      m.inputs[fieldGrep].Value(),
		Exclude:   m.inputs[fieldExclude].Value(),
	}, nil
}

func (m model) renderResultsToString() string {
	if m.result == nil || len(m.result.Data.Items) == 0 {
		return "No results."
	}

	items := m.result.Data.Items

	// Apply post-filters
	grepStr := m.inputs[fieldGrep].Value()
	if grepStr != "" {
		items = PostFilterItems(items, grepStr, true)
	}
	excludeStr := m.inputs[fieldExclude].Value()
	if excludeStr != "" {
		items = PostFilterItems(items, excludeStr, false)
	}
	if len(items) == 0 {
		return "No results after post-filter."
	}

	keys := m.result.Data.Keys
	if len(keys) == 0 {
		keys = deriveKeys(items[0])
	}

	// Apply field filtering
	fieldsStr := m.inputs[fieldFields].Value()
	if fieldsStr != "" {
		keys = FilterKeys(keys, fieldsStr, items)
	}

	format := ParseFormat(m.inputs[fieldFormat].Value())

	var sb strings.Builder

	switch format {
	case FormatTable:
		sb.WriteString(renderTableToString(items, keys))
	case FormatVertical:
		sb.WriteString(renderVerticalToString(items, keys))
	case FormatCSV:
		sb.WriteString(renderCSVToString(items, keys))
	case FormatJSON:
		sb.WriteString(renderJSONToString(items))
	case FormatChart:
		sb.WriteString(renderChartToString(items))
	case FormatCallflow:
		sb.WriteString(buildCallflowString(items))
	case FormatPcap:
		sb.WriteString("Format \"pcap\" is only for one-shot CLI: homer search ... --format pcap --output trace.pcap\n")
	}

	sb.WriteString(fmt.Sprintf("\n%d rows in set", len(items)))
	return sb.String()
}

// ---- View ------------------------------------------------------------------

func (m model) View() string {
	switch m.view {
	case viewResults:
		return m.viewResults()
	default:
		return m.viewForm()
	}
}

func (m model) viewForm() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("Homer Search"))
	b.WriteString("\n\n")

	for i := range m.inputs {
		b.WriteString(m.inputs[i].View())
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(helpStyle.Render("Enter: search  |  Tab/Shift+Tab: navigate  |  Esc: quit"))
	b.WriteString("\n")
	b.WriteString(helpStyle.Render("Proto: sip | otlp_traces | lp | …  |  LP: event-type schema__table  |  Raw SQL: homer search --sql '...'"))

	return b.String()
}

func (m model) viewResults() string {
	var b strings.Builder

	b.WriteString(resultHeaderStyle.Render("Search Results"))
	b.WriteString("\n")
	b.WriteString(helpStyle.Render(fmt.Sprintf("Format: %s  |  f: cycle format  |  r: re-search  |  Esc: back to form",
		m.inputs[fieldFormat].Value())))
	b.WriteString("\n\n")

	if m.searching {
		b.WriteString("Searching...")
		return b.String()
	}

	if m.resultErr != nil {
		b.WriteString(errorStyle.Render(fmt.Sprintf("Error: %v", m.resultErr)))
		return b.String()
	}

	if m.resultText != "" {
		b.WriteString(m.resultText)
	} else {
		b.WriteString("No results.")
	}

	return b.String()
}

// ---- string renderers (for TUI buffer) -------------------------------------

func renderTableToString(items []map[string]interface{}, keys []string) string {
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
			if vl > widths[ci] && vl <= 40 {
				widths[ci] = vl
			} else if vl > 40 && widths[ci] < 40 {
				widths[ci] = 40
			}
		}
		rows[ri] = row
	}

	var sb strings.Builder
	sep := buildSep(widths)
	sb.WriteString(sep)
	sb.WriteString(buildRow(keys, widths))
	sb.WriteString(sep)
	for _, row := range rows {
		sb.WriteString(buildRow(row, widths))
	}
	sb.WriteString(sep)
	return sb.String()
}

func buildRow(vals []string, widths []int) string {
	var sb strings.Builder
	sb.WriteString("| ")
	for i, v := range vals {
		if len(v) > widths[i] {
			v = v[:widths[i]-3] + "..."
		}
		sb.WriteString(fmt.Sprintf("%-*s | ", widths[i], v))
	}
	sb.WriteString("\n")
	return sb.String()
}

func buildSep(widths []int) string {
	var sb strings.Builder
	sb.WriteString("+")
	for _, w := range widths {
		sb.WriteString(strings.Repeat("-", w+2) + "+")
	}
	sb.WriteString("\n")
	return sb.String()
}

func renderVerticalToString(items []map[string]interface{}, keys []string) string {
	maxKeyLen := 0
	for _, k := range keys {
		if len(k) > maxKeyLen {
			maxKeyLen = len(k)
		}
	}

	var sb strings.Builder
	for i, item := range items {
		sb.WriteString(fmt.Sprintf("*************************** %d. row ***************************\n", i+1))
		for _, k := range keys {
			sb.WriteString(fmt.Sprintf("%*s: %s\n", maxKeyLen, k, fmtVal(item[k])))
		}
	}
	return sb.String()
}

func renderCSVToString(items []map[string]interface{}, keys []string) string {
	var sb strings.Builder
	sb.WriteString(strings.Join(keys, ","))
	sb.WriteString("\n")
	for _, item := range items {
		vals := make([]string, len(keys))
		for i, k := range keys {
			v := fmtVal(item[k])
			if strings.ContainsAny(v, ",\"\n") {
				v = "\"" + strings.ReplaceAll(v, "\"", "\"\"") + "\""
			}
			vals[i] = v
		}
		sb.WriteString(strings.Join(vals, ","))
		sb.WriteString("\n")
	}
	return sb.String()
}

func renderJSONToString(items []map[string]interface{}) string {
	// Simple pretty print
	var sb strings.Builder
	sb.WriteString("[\n")
	for i, item := range items {
		sb.WriteString("  {")
		keys := deriveKeys(item)
		for ki, k := range keys {
			sb.WriteString(fmt.Sprintf("%q: %q", k, fmtVal(item[k])))
			if ki < len(keys)-1 {
				sb.WriteString(", ")
			}
		}
		sb.WriteString("}")
		if i < len(items)-1 {
			sb.WriteString(",")
		}
		sb.WriteString("\n")
	}
	sb.WriteString("]")
	return sb.String()
}

func renderChartToString(items []map[string]interface{}) string {
	// For TUI, just show a summary since full asciigraph needs stdout
	if len(items) == 0 {
		return "No data for chart."
	}
	return fmt.Sprintf("[Chart mode - %d data points. Use CLI mode for full ASCII graph.]", len(items))
}
