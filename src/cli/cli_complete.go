// Copyright (C) 2026 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package cli

import (
	"sort"
	"strings"
	"unicode"
)

// sqlCLICompleter provides TAB completion for the interactive SQL shell.
type sqlCLICompleter struct {
	lakeName  string
	dataPath  string
	tablesFn  func() []string
	keywords  []string
	columns   []string
	functions []string
	meta      []string
}

func newSQLCLICompleter(lakeName, dataPath string, tablesFn func() []string) *sqlCLICompleter {
	return &sqlCLICompleter{
		lakeName:  lakeName,
		dataPath:  strings.TrimSuffix(dataPath, "/"),
		tablesFn:  tablesFn,
		keywords:  sqlCLIKeywords,
		columns:   sqlCLIColumnNames,
		functions: sqlCLIFunctions,
		meta:      sqlCLIMetaCommands,
	}
}

func (c *sqlCLICompleter) Do(line []rune, pos int) ([][]rune, int) {
	if pos < 0 {
		pos = 0
	}
	if pos > len(line) {
		pos = len(line)
	}

	start, inQuote, _ := wordBoundsAt(line, pos)
	prefix := string(line[start:pos])
	offset := pos - start

	if inQuote {
		return prefixMatchRunes(prefix, c.pathCandidates(prefix)), offset
	}

	// Line start: meta-commands (help, exit, …) and SQL keywords (SELECT, …).
	if start == 0 && !strings.Contains(string(line[:pos]), " ") {
		cands := dedupeSorted(append(append([]string{}, c.meta...), c.keywords...))
		return prefixMatchRunes(prefix, cands), offset
	}

	ctx := analyzeSQLContext(string(line[:pos]))
	cands := c.candidatesForContext(ctx, prefix)
	return prefixMatchRunes(prefix, cands), offset
}

func (c *sqlCLICompleter) candidatesForContext(ctx sqlLineContext, prefix string) []string {
	var out []string
	switch ctx.kind {
	case ctxAfterFrom, ctxAfterJoin:
		out = append(out, c.tableNames()...)
		out = append(out, c.lakeName)
	case ctxAfterDot:
		out = append(out, c.columns...)
		if short := tableShortName(ctx.qualifier); short != "" {
			out = append(out, c.columnsForTable(short)...)
		}
	default:
		out = append(out, c.keywords...)
		out = append(out, c.functions...)
		out = append(out, c.tableNames()...)
		out = append(out, c.columns...)
	}
	return dedupeSorted(out)
}

func (c *sqlCLICompleter) tableNames() []string {
	raw := c.tablesFn()
	if len(raw) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(raw)*2)
	var out []string
	add := func(s string) {
		if s == "" {
			return
		}
		if _, ok := seen[s]; ok {
			return
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	add(c.lakeName)
	for _, t := range raw {
		add(t)
		if i := strings.LastIndex(t, "."); i >= 0 {
			add(t[i+1:])
		}
	}
	sort.Strings(out)
	return out
}

func (c *sqlCLICompleter) pathCandidates(typed string) []string {
	base := c.dataPath
	if base == "" {
		return nil
	}
	var out []string
	add := func(p string) {
		if p != "" {
			out = append(out, p)
		}
	}
	add(base)
	add(base + "/main")
	for _, tbl := range c.tablesFn() {
		short := tableShortName(tbl)
		if short == "" || short == c.lakeName {
			continue
		}
		add(base + "/main/" + short)
		add(base + "/main/" + short + "/date=*/**/*.parquet")
	}
	// Filter by typed prefix inside the quoted path.
	if typed == "" {
		return dedupeSorted(out)
	}
	var filtered []string
	for _, p := range out {
		if strings.HasPrefix(p, typed) {
			filtered = append(filtered, p)
		}
	}
	return dedupeSorted(filtered)
}

type sqlCtx int

const (
	ctxDefault sqlCtx = iota
	ctxAfterFrom
	ctxAfterJoin
	ctxAfterDot
)

type sqlLineContext struct {
	kind      sqlCtx
	qualifier string // table/schema before trailing dot
}

func analyzeSQLContext(beforeCursor string) sqlLineContext {
	tokens := tokenizeSQL(beforeCursor)
	if len(tokens) == 0 {
		return sqlLineContext{kind: ctxDefault}
	}
	last := strings.ToUpper(tokens[len(tokens)-1])
	if last == "" && len(tokens) >= 2 {
		// Cursor right after "tbl."
		prev := tokens[len(tokens)-2]
		return sqlLineContext{kind: ctxAfterDot, qualifier: prev}
	}
	if strings.HasSuffix(beforeCursor, ".") && len(tokens) >= 1 {
		return sqlLineContext{kind: ctxAfterDot, qualifier: tokens[len(tokens)-1]}
	}
	if len(tokens) >= 2 {
		prev := strings.ToUpper(tokens[len(tokens)-2])
		switch prev {
		case "FROM", "JOIN", "INTO", "UPDATE", "TABLE":
			return sqlLineContext{kind: ctxAfterFrom}
		}
	}
	lastUp := strings.ToUpper(last)
	switch lastUp {
	case "FROM", "JOIN":
		return sqlLineContext{kind: ctxAfterFrom}
	}
	return sqlLineContext{kind: ctxDefault}
}

func tokenizeSQL(s string) []string {
	var tokens []string
	var b strings.Builder
	inQuote := byte(0)
	flush := func() {
		if b.Len() > 0 {
			tokens = append(tokens, b.String())
			b.Reset()
		}
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if inQuote != 0 {
			if c == inQuote {
				inQuote = 0
			}
			continue
		}
		if c == '\'' || c == '"' {
			inQuote = c
			flush()
			continue
		}
		if c == '.' {
			flush()
			continue
		}
		if c == ' ' || c == '\t' || c == '\n' || c == '(' || c == ')' || c == ',' || c == ';' {
			flush()
			continue
		}
		b.WriteByte(c)
	}
	flush()
	return tokens
}

func wordBoundsAt(line []rune, pos int) (start int, inQuote bool, quote byte) {
	if pos > len(line) {
		pos = len(line)
	}
	// Find if cursor is inside a quoted string (scan from line start).
	inQ := false
	var q byte
	i := 0
	for i < pos {
		r := line[i]
		if r > 127 {
			i++
			continue
		}
		c := byte(r)
		if inQ {
			if c == q {
				inQ = false
			}
			i++
			continue
		}
		if c == '\'' || c == '"' {
			inQ = true
			q = c
			i++
			continue
		}
		i++
	}
	if inQ {
		// Walk back to opening quote for path prefix.
		j := pos - 1
		for j >= 0 && line[j] != rune(q) {
			j--
		}
		if j >= 0 {
			return j + 1, true, q
		}
		return 0, true, q
	}
	start = pos
	for start > 0 {
		r := line[start-1]
		if r == '.' || r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r) {
			start--
			continue
		}
		break
	}
	return start, false, 0
}

func prefixMatchRunes(prefix string, candidates []string) [][]rune {
	if len(candidates) == 0 {
		return nil
	}
	pl := strings.ToLower(prefix)
	var out [][]rune
	for _, cand := range candidates {
		cl := strings.ToLower(cand)
		if pl != "" && !strings.HasPrefix(cl, pl) {
			continue
		}
		suffix := cand[len(prefix):]
		if suffix == "" {
			continue
		}
		out = append(out, []rune(suffix))
	}
	return out
}

func dedupeSorted(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	sort.Strings(in)
	out := in[:0]
	var prev string
	for _, s := range in {
		if s == prev {
			continue
		}
		out = append(out, s)
		prev = s
	}
	return out
}

func tableShortName(qualifier string) string {
	qualifier = strings.Trim(qualifier, `"'`)
	if i := strings.LastIndex(qualifier, "."); i >= 0 {
		return qualifier[i+1:]
	}
	return qualifier
}

func (c *sqlCLICompleter) columnsForTable(short string) []string {
	// Per-table extras beyond the shared column list.
	switch short {
	case "hep_proto_1_registration":
		return []string{"aor", "contact", "expires", "user_agent"}
	case "otlp_traces", "otlp_logs", "otlp_metrics":
		return []string{"trace_id", "span_id", "name", "service_name", "body", "severity_text"}
	}
	return nil
}

var sqlCLIMetaCommands = []string{
	"help", "exit", "quit", "tables", "clear", "show tables",
}

var sqlCLIKeywords = []string{
	"SELECT", "FROM", "WHERE", "JOIN", "LEFT", "RIGHT", "INNER", "OUTER", "CROSS", "ON",
	"AND", "OR", "NOT", "IN", "IS", "NULL", "LIKE", "ILIKE", "BETWEEN", "EXISTS",
	"AS", "DISTINCT", "ALL", "UNION", "EXCEPT", "INTERSECT",
	"GROUP", "BY", "ORDER", "HAVING", "LIMIT", "OFFSET",
	"ASC", "DESC", "WITH", "CASE", "WHEN", "THEN", "ELSE", "END",
	"SHOW", "TABLES", "DESCRIBE", "EXPLAIN", "CALL",
	"TRUE", "FALSE", "CAST", "AT", "TIME", "ZONE", "UTC",
	"INSERT", "INTO", "VALUES", "CREATE", "VIEW", "REPLACE", "TABLE", "SCHEMA", "IF", "EXISTS",
}

var sqlCLIFunctions = []string{
	"read_parquet", "glob", "count", "sum", "avg", "min", "max",
	"json_extract", "json_extract_string",
	"ducklake_snapshots", "ducklake_merge_adjacent_files", "ducklake_delete_orphaned_files",
	"to_timestamp", "current_date", "now", "lower", "upper", "trim", "length",
}

var sqlCLIColumnNames = []string{
	"uuid", "date", "timestamp", "session_id", "caller", "callee",
	"src_ip", "dst_ip", "src_port", "dst_port",
	"method", "response_code", "cseq_method", "protocol", "node_id", "cid",
	"payload", "data_extra", "ruri_user", "auth_user",
}
