// Package sqlvalidator provides SQL injection prevention for Homer's coordinator.
// It validates raw SQL queries, SQL expression fragments, and sanitizes string values.
package sqlvalidator

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

// ---- Token types -----------------------------------------------------------

type tokenKind int

const (
	tkIdent  tokenKind = iota // identifier or keyword
	tkString                  // single-quoted string literal
	tkNumber                  // numeric literal
	tkOp                      // operator or punctuation (+, -, *, /, =, <, >, etc.)
	tkSemi                    // semicolon
	tkLParen                  // (
	tkRParen                  // )
	tkComma                   // ,
	tkDot                     // .
	tkStar                    // * (outside of string)
)

type token struct {
	kind  tokenKind
	value string // original text
	upper string // uppercased (for ident/keyword comparison)
}

// ---- Lexer -----------------------------------------------------------------

// tokenize splits SQL into tokens, correctly handling string literals so that
// keywords inside strings are not flagged. Comments are stripped.
func tokenize(sql string) []token {
	var tokens []token
	i := 0
	runes := []rune(sql)
	n := len(runes)

	for i < n {
		ch := runes[i]

		// Skip whitespace
		if unicode.IsSpace(ch) {
			i++
			continue
		}

		// Line comment: -- ...
		if ch == '-' && i+1 < n && runes[i+1] == '-' {
			for i < n && runes[i] != '\n' {
				i++
			}
			continue
		}

		// Block comment: /* ... */
		if ch == '/' && i+1 < n && runes[i+1] == '*' {
			i += 2
			for i+1 < n {
				if runes[i] == '*' && runes[i+1] == '/' {
					i += 2
					break
				}
				i++
			}
			continue
		}

		// Single-quoted string literal
		if ch == '\'' {
			start := i
			i++ // skip opening quote
			for i < n {
				if runes[i] == '\'' {
					if i+1 < n && runes[i+1] == '\'' {
						i += 2 // escaped quote ''
						continue
					}
					i++ // closing quote
					break
				}
				i++
			}
			tokens = append(tokens, token{kind: tkString, value: string(runes[start:i])})
			continue
		}

		// Double-quoted identifier
		if ch == '"' {
			start := i
			i++ // skip opening quote
			innerStart := i
			for i < n {
				if runes[i] == '"' {
					// Escaped double quote inside identifier: ""
					if i+1 < n && runes[i+1] == '"' {
						i += 2
						continue
					}
					// Unescaped closing quote
					break
				}
				i++
			}
			inner := string(runes[innerStart:i])
			if i < n && runes[i] == '"' {
				i++ // closing "
			}
			val := string(runes[start:i])
			tokens = append(tokens, token{kind: tkIdent, value: val, upper: strings.ToUpper(inner)})
			continue
		}

		// Number
		if unicode.IsDigit(ch) || (ch == '.' && i+1 < n && unicode.IsDigit(runes[i+1])) {
			start := i
			for i < n && (unicode.IsDigit(runes[i]) || runes[i] == '.' || runes[i] == 'e' || runes[i] == 'E' || runes[i] == '+' || runes[i] == '-') {
				// Handle 'e' and 'E' only if preceded by digit
				if (runes[i] == 'e' || runes[i] == 'E') && i > start && unicode.IsDigit(runes[i-1]) {
					i++
					continue
				}
				if (runes[i] == '+' || runes[i] == '-') && i > start && (runes[i-1] == 'e' || runes[i-1] == 'E') {
					i++
					continue
				}
				if unicode.IsDigit(runes[i]) || runes[i] == '.' {
					i++
					continue
				}
				break
			}
			tokens = append(tokens, token{kind: tkNumber, value: string(runes[start:i])})
			continue
		}

		// Dollar-quoted string: $$...$$ or $tag$...$tag$.
		// $1 stays a parameter (operator + number), not a string.
		if ch == '$' {
			if end, ok := tryDollarString(runes, i); ok {
				tokens = append(tokens, token{kind: tkString, value: string(runes[i:end])})
				i = end
				continue
			}
		}

		// Identifier or keyword
		if unicode.IsLetter(ch) || ch == '_' {
			start := i
			for i < n && (unicode.IsLetter(runes[i]) || unicode.IsDigit(runes[i]) || runes[i] == '_') {
				i++
			}
			val := string(runes[start:i])
			tokens = append(tokens, token{kind: tkIdent, value: val, upper: strings.ToUpper(val)})
			continue
		}

		// Special single characters
		switch ch {
		case ';':
			tokens = append(tokens, token{kind: tkSemi, value: ";"})
			i++
		case '(':
			tokens = append(tokens, token{kind: tkLParen, value: "("})
			i++
		case ')':
			tokens = append(tokens, token{kind: tkRParen, value: ")"})
			i++
		case ',':
			tokens = append(tokens, token{kind: tkComma, value: ","})
			i++
		case '.':
			tokens = append(tokens, token{kind: tkDot, value: "."})
			i++
		case '*':
			tokens = append(tokens, token{kind: tkStar, value: "*"})
			i++
		default:
			// Operators and other punctuation: consume multi-char ops
			start := i
			i++
			// Two-char operators
			if i < n {
				pair := string(runes[start : i+1])
				if pair == "::" || pair == "<=" || pair == ">=" || pair == "!=" || pair == "<>" || pair == "||" {
					i++
				}
			}
			tokens = append(tokens, token{kind: tkOp, value: string(runes[start:i])})
		}
	}

	return tokens
}

// tryDollarString reports whether runes[i] opens a dollar-quoted string and,
// when it does, the index just after the closing delimiter. An unclosed quote
// consumes the rest of the input so its contents are not tokenized as SQL.
func tryDollarString(runes []rune, i int) (int, bool) {
	n := len(runes)
	if i >= n || runes[i] != '$' {
		return i, false
	}
	j := i + 1
	if j < n && runes[j] == '$' {
		return closeDollarString(runes, j+1, "$$"), true
	}
	if j >= n || (!unicode.IsLetter(runes[j]) && runes[j] != '_') {
		return i, false
	}
	k := j
	for k < n && (unicode.IsLetter(runes[k]) || unicode.IsDigit(runes[k]) || runes[k] == '_') {
		k++
	}
	if k >= n || runes[k] != '$' {
		return i, false
	}
	tag := string(runes[j:k])
	return closeDollarString(runes, k+1, "$"+tag+"$"), true
}

func closeDollarString(runes []rune, bodyStart int, closer string) int {
	rest := string(runes[bodyStart:])
	idx := strings.Index(rest, closer)
	if idx < 0 {
		return len(runes)
	}
	return bodyStart + idx + len(closer)
}

// ---- Blocked keyword sets --------------------------------------------------

// blockedDML contains DML/DDL keywords that must never appear as standalone tokens
// in user-submitted SQL (outside of string literals).
var blockedDML = map[string]bool{
	"INSERT":   true,
	"UPDATE":   true,
	"DELETE":   true,
	"DROP":     true,
	"ALTER":    true,
	"CREATE":   true,
	"TRUNCATE": true,
	"GRANT":    true,
	"REVOKE":   true,
	"EXEC":     true,
	"EXECUTE":  true,
	"COPY":     true,
	"EXPORT":   true,
	"IMPORT":   true,
	"ATTACH":   true,
	"DETACH":   true,
	"LOAD":     true,
	"INSTALL":  true,
	"SET":      true,
}

// blockedFunctions are DuckDB functions that access the filesystem or network.
var blockedFunctions = map[string]bool{
	"READ_CSV":          true,
	"READ_CSV_AUTO":     true,
	"READ_PARQUET":      true,
	"READ_JSON":         true,
	"READ_JSON_AUTO":    true,
	"READ_BLOB":         true,
	"READ_TEXT":         true,
	"READ_NDJSON":       true,
	"READ_NDJSON_AUTO":  true,
	"WRITE_CSV":         true,
	"WRITE_PARQUET":     true,
	"HTTPFS":            true,
	"HTTP_GET":          true,
	"HTTP_POST":         true,
	"GLOB":              true,
	"COPY":              true,
	"PG_TYPEOF":         true,
	"SYSTEM":            true,
	"SHELL":             true,
	"READ_PARQUET_FUNC": true,
	// External table scanners (GHSA-4687-q698-mccv / CVE-2026-62251 bypass).
	"SQLITE_SCAN":   true,
	"POSTGRES_SCAN": true,
	"MYSQL_SCAN":    true,
	"ICEBERG_SCAN":  true,
	"DELTA_SCAN":    true,
	"SPATIAL_SCAN":  true,
	// Remote query wrappers and host disclosure (GHSA-c5mq-r27x-v29v).
	"POSTGRES_QUERY": true,
	"MYSQL_QUERY":    true,
	"GETENV":         true,
	"DUCKDB_SECRETS": true,
}

// isBlockedRoutine reports a DuckDB routine that can read or write files,
// open a network or external-database scan, or disclose host secrets.
// Prefixes and suffixes cover readers that are not spelled out above, so the
// next unlisted scanner does not reopen file access (GHSA-c5mq-r27x-v29v).
// Call sites still require '(', so a column that only shares the name is allowed.
func isBlockedRoutine(upper string) bool {
	if blockedFunctions[upper] {
		return true
	}
	switch {
	case strings.HasPrefix(upper, "READ_"),
		strings.HasPrefix(upper, "SNIFF_"),
		strings.HasPrefix(upper, "PARQUET_"),
		strings.HasPrefix(upper, "HTTP_"),
		strings.HasPrefix(upper, "ICEBERG_"),
		strings.HasPrefix(upper, "DELTA_"),
		strings.HasPrefix(upper, "ST_READ"):
		return true
	}
	return strings.HasSuffix(upper, "_SCAN") || strings.HasSuffix(upper, "_ATTACH")
}

// allowedStatementStarts are the only statement types allowed in raw SQL.
var allowedStatementStarts = map[string]bool{
	"SELECT":   true,
	"WITH":     true,
	"SHOW":     true,
	"DESCRIBE": true,
	"EXPLAIN":  true,
	"PRAGMA":   true,
}

// ---- ValidateRawSQL --------------------------------------------------------

// ValidateRawSQL validates a full SQL statement for safety.
// Returns nil if the SQL is safe to execute, or an error describing the violation.
func ValidateRawSQL(sql string) error {
	tokens, err := tokensForValidation(sql)
	if err != nil {
		return err
	}

	first := tokens[0]
	if first.kind != tkIdent || !allowedStatementStarts[first.upper] {
		return fmt.Errorf("query must start with SELECT, WITH, SHOW, DESCRIBE, EXPLAIN, or PRAGMA (got %q)", first.value)
	}

	return rejectSQLHazards(tokens, nil)
}

// hepProtoTableName is the only table name allowed on POST /exec (PCAP import).
// Matches LakeTableFQN: <catalog>.main.hep_proto_<proto>_<suffix>.
var hepProtoTableName = regexp.MustCompile(`(?i)^hep_proto_[0-9]+_[a-z][a-z0-9_]*$`)

var blockedWriteCatalogs = map[string]bool{
	"MEMORY":             true,
	"TEMP":               true,
	"SYSTEM":             true,
	"INFORMATION_SCHEMA": true,
	"PG_CATALOG":         true,
}

// ValidateWriteSQL validates coordinator-generated writes (PCAP import INSERT).
// Only INSERT INTO <catalog>.main.hep_proto_* (...) VALUES ... is allowed.
// SELECT/FROM, system catalogs, quoted identifiers, and DML/filesystem hazards stay blocked.
func ValidateWriteSQL(sql string) error {
	tokens, err := tokensForValidation(sql)
	if err != nil {
		return err
	}

	if err := requireInsertIntoHepProtoValues(tokens); err != nil {
		return err
	}

	return rejectSQLHazards(tokens, map[string]bool{"INSERT": true})
}

func requireInsertIntoHepProtoValues(tokens []token) error {
	for _, tok := range tokens {
		if tok.kind == tkSemi {
			return fmt.Errorf("semicolons are not allowed (prevents multi-statement injection)")
		}
		if tok.kind == tkIdent && strings.HasPrefix(tok.value, "\"") {
			return fmt.Errorf("quoted identifiers are not allowed in write SQL")
		}
		if tok.kind == tkOp && (tok.value == "[" || tok.value == "`") {
			return fmt.Errorf("quoted identifiers are not allowed in write SQL")
		}
	}

	if len(tokens) < 8 {
		return fmt.Errorf("write SQL must be INSERT INTO <catalog>.main.hep_proto_* VALUES ...")
	}
	if tokens[0].kind != tkIdent || tokens[0].upper != "INSERT" ||
		tokens[1].kind != tkIdent || tokens[1].upper != "INTO" {
		return fmt.Errorf("write SQL must start with INSERT INTO (got %q)", tokens[0].value)
	}
	if tokens[2].kind != tkIdent {
		return fmt.Errorf("write target catalog must be an identifier")
	}
	if blockedWriteCatalogs[tokens[2].upper] {
		return fmt.Errorf("system catalog %q is not allowed as write target", tokens[2].value)
	}
	if tokens[3].kind != tkDot ||
		tokens[4].kind != tkIdent || tokens[4].upper != "MAIN" ||
		tokens[5].kind != tkDot ||
		tokens[6].kind != tkIdent {
		return fmt.Errorf("write target must be <catalog>.main.hep_proto_<proto>_<name>")
	}
	if !hepProtoTableName.MatchString(tokens[6].value) {
		return fmt.Errorf("write target table must match hep_proto_<proto>_<name> (got %q)", tokens[6].value)
	}

	i := 7
	if i < len(tokens) && tokens[i].kind == tkLParen {
		depth := 0
		for i < len(tokens) {
			switch tokens[i].kind {
			case tkLParen:
				depth++
			case tkRParen:
				depth--
				i++
				if depth == 0 {
					goto afterCols
				}
				continue
			}
			i++
		}
		return fmt.Errorf("write SQL has an unclosed column list")
	}
afterCols:
	if i >= len(tokens) || tokens[i].kind != tkIdent || tokens[i].upper != "VALUES" {
		return fmt.Errorf("write SQL must use INSERT ... VALUES (SELECT/FROM is not allowed)")
	}

	for _, tok := range tokens {
		if tok.kind != tkIdent {
			continue
		}
		switch tok.upper {
		case "SELECT", "FROM", "JOIN", "UNION", "EXCEPT", "INTERSECT",
			"TABLE", "PIVOT", "UNPIVOT", "REPLACE", "OVERWRITE", "RETURNING":
			return fmt.Errorf("blocked keyword %q in write SQL", tok.value)
		}
	}
	return nil
}

func tokensForValidation(sql string) ([]token, error) {
	trimmed := strings.TrimSpace(sql)
	if trimmed == "" {
		return nil, fmt.Errorf("empty SQL query")
	}
	tokens := tokenize(trimmed)
	if len(tokens) == 0 {
		return nil, fmt.Errorf("empty SQL query after parsing")
	}
	return tokens, nil
}

// rejectSQLHazards applies the GHSA-rm5w-rqr7-2h54 checks shared by read and write SQL.
// skipDML lists keywords that may appear (the write path allows INSERT itself).
func rejectSQLHazards(tokens []token, skipDML map[string]bool) error {
	for _, tok := range tokens {
		if tok.kind == tkSemi {
			return fmt.Errorf("semicolons are not allowed (prevents multi-statement injection)")
		}
	}

	for _, tok := range tokens {
		if tok.kind != tkIdent {
			continue
		}
		if skipDML[tok.upper] {
			continue
		}
		if blockedDML[tok.upper] {
			return fmt.Errorf("blocked keyword %q (DML/DDL operations are not allowed)", tok.value)
		}
	}

	for i, tok := range tokens {
		if tok.kind != tkIdent {
			continue
		}
		if isBlockedRoutine(tok.upper) {
			if i+1 < len(tokens) && tokens[i+1].kind == tkLParen {
				return fmt.Errorf("blocked function %q (filesystem/network access is not allowed)", tok.value)
			}
		}
	}

	if err := rejectFilePathRelations(tokens); err != nil {
		return err
	}

	selectSeen := false
	fromSeen := false
	for _, tok := range tokens {
		if tok.kind != tkIdent {
			continue
		}
		if tok.upper == "SELECT" {
			selectSeen = true
			fromSeen = false
		}
		if tok.upper == "FROM" {
			fromSeen = true
		}
		if tok.upper == "INTO" && selectSeen && !fromSeen {
			return fmt.Errorf("SELECT INTO is not allowed")
		}
	}

	for _, tok := range tokens {
		if tok.kind == tkIdent && tok.upper == "CALL" {
			return fmt.Errorf("CALL statements are not allowed")
		}
	}

	return nil
}

// rejectFilePathRelations rejects DuckDB replacement scans, which read a file
// when a string literal is used as a table: SELECT * FROM '/path/file.parquet'.
// There is no function name for the routine denylist to see (GHSA-c5mq-r27x-v29v).
func rejectFilePathRelations(tokens []token) error {
	depth := 0
	relationList := map[int]bool{}
	expecting := map[int]bool{}

	for i, tok := range tokens {
		switch tok.kind {
		case tkLParen:
			if expecting[depth] {
				expecting[depth+1] = true
			}
			depth++
		case tkRParen:
			delete(expecting, depth)
			delete(relationList, depth)
			if depth > 0 {
				depth--
			}
		case tkComma:
			if relationList[depth] {
				expecting[depth] = true
			}
		case tkString:
			if expecting[depth] {
				return fmt.Errorf("file path table references are not allowed")
			}
		case tkIdent:
			upper := tok.upper
			if upper == "FROM" || upper == "JOIN" {
				expecting[depth] = true
				relationList[depth] = true
				continue
			}
			if isRelationNoise(upper) {
				continue
			}
			if relationList[depth] && isFromListEnd(upper) {
				relationList[depth] = false
				expecting[depth] = false
				continue
			}
			if upper == "ON" || upper == "USING" {
				expecting[depth] = false
				continue
			}
			if !expecting[depth] {
				continue
			}
			// EXTRACT(YEAR FROM TIMESTAMP '...') and similar typed literals are
			// values, not replacement-scan paths.
			if isTypedLiteralPrefix(upper) && i+1 < len(tokens) && tokens[i+1].kind == tkString {
				expecting[depth] = false
				continue
			}
			if isEscapeStringPrefix(upper) && i+1 < len(tokens) && tokens[i+1].kind == tkString {
				return fmt.Errorf("file path table references are not allowed")
			}
			if upper == "U" && i+2 < len(tokens) &&
				tokens[i+1].kind == tkOp && tokens[i+1].value == "&" &&
				tokens[i+2].kind == tkString {
				return fmt.Errorf("file path table references are not allowed")
			}
			if !startsSubquery(upper) {
				relationList[depth] = true
			}
			expecting[depth] = false
		}
	}
	return nil
}

func isRelationNoise(upper string) bool {
	switch upper {
	case "AS", "LATERAL", "ONLY", "NATURAL", "INNER", "OUTER",
		"LEFT", "RIGHT", "FULL", "CROSS", "SEMI", "ANTI", "POSITIONAL":
		return true
	default:
		return false
	}
}

func isFromListEnd(upper string) bool {
	switch upper {
	case "WHERE", "GROUP", "ORDER", "LIMIT", "OFFSET", "HAVING",
		"QUALIFY", "WINDOW", "UNION", "EXCEPT", "INTERSECT", "FETCH":
		return true
	default:
		return false
	}
}

func startsSubquery(upper string) bool {
	switch upper {
	case "SELECT", "WITH", "VALUES":
		return true
	default:
		return false
	}
}

func isEscapeStringPrefix(upper string) bool {
	switch upper {
	case "E", "X", "B", "N":
		return true
	default:
		return false
	}
}

func isTypedLiteralPrefix(upper string) bool {
	switch upper {
	case "TIMESTAMP", "TIMESTAMPTZ", "DATE", "TIME", "INTERVAL",
		"VARCHAR", "CHAR", "CHARACTER", "BOOLEAN", "BOOL", "BLOB",
		"JSON", "UUID", "TINYINT", "SMALLINT", "INTEGER", "INT", "BIGINT",
		"HUGEINT", "FLOAT", "DOUBLE", "DECIMAL", "NUMERIC", "REAL":
		return true
	default:
		return false
	}
}

// ---- ExprType --------------------------------------------------------------

// ExprType identifies the kind of SQL expression being validated.
type ExprType int

const (
	ExprSelect  ExprType = iota // SELECT clause (columns, aggregations)
	ExprGroupBy                 // GROUP BY clause
	ExprOrderBy                 // ORDER BY clause
)

// ---- ValidateExpression ----------------------------------------------------

// blockedExprKeywords are keywords that should never appear in SQL expression fragments.
var blockedExprKeywords = map[string]bool{
	"INSERT":    true,
	"UPDATE":    true,
	"DELETE":    true,
	"DROP":      true,
	"ALTER":     true,
	"CREATE":    true,
	"TRUNCATE":  true,
	"GRANT":     true,
	"REVOKE":    true,
	"EXEC":      true,
	"EXECUTE":   true,
	"COPY":      true,
	"EXPORT":    true,
	"IMPORT":    true,
	"ATTACH":    true,
	"DETACH":    true,
	"LOAD":      true,
	"INSTALL":   true,
	"SET":       true,
	"INTO":      true,
	"CALL":      true,
	"UNION":     true,
	"EXCEPT":    true,
	"INTERSECT": true,
	// Clause boundary keywords that must not appear inside expression fragments.
	// They indicate the start of a new SQL clause and would corrupt the query
	// when concatenated into SELECT/GROUP BY/ORDER BY positions.
	// Note: FROM is intentionally omitted because it appears in EXTRACT(... FROM ...).
	"WHERE":  true,
	"LIMIT":  true,
	"OFFSET": true,
	"HAVING": true,
}

// ValidateExpression validates a SQL expression fragment (for SELECT, GROUP BY, ORDER BY).
// Returns nil if safe, or an error describing the violation.
func ValidateExpression(expr string, exprType ExprType) error {
	trimmed := strings.TrimSpace(expr)
	if trimmed == "" {
		return nil // empty is fine, caller uses default
	}

	// Reject SQL comment markers BEFORE tokenization, because the tokenizer
	// strips comments but callers concatenate the raw string into SQL.
	// A comment like "timestamp DESC --" would pass token validation but
	// comment out server-appended clauses (e.g. LIMIT) in the final SQL.
	// Use token-aware scanning so "--" / "/*" inside string literals or
	// double-quoted identifiers are not false-positives (same as node SQL).
	if ContainsUnsafeComment(trimmed) {
		return fmt.Errorf("SQL comments are not allowed in %s expression", exprTypeName(exprType))
	}

	tokens := tokenize(trimmed)
	if len(tokens) == 0 {
		return nil
	}

	// 1. No semicolons
	for _, tok := range tokens {
		if tok.kind == tkSemi {
			return fmt.Errorf("semicolons are not allowed in %s expression", exprTypeName(exprType))
		}
	}

	// 2. No subqueries: reject SELECT keyword inside expression
	for _, tok := range tokens {
		if tok.kind == tkIdent && tok.upper == "SELECT" {
			return fmt.Errorf("subqueries (SELECT) are not allowed in %s expression", exprTypeName(exprType))
		}
	}

	// 3. No DML/DDL keywords
	for _, tok := range tokens {
		if tok.kind != tkIdent {
			continue
		}
		if blockedExprKeywords[tok.upper] {
			return fmt.Errorf("blocked keyword %q in %s expression", tok.value, exprTypeName(exprType))
		}
	}

	// 4. No blocked functions
	for i, tok := range tokens {
		if tok.kind != tkIdent {
			continue
		}
		if isBlockedRoutine(tok.upper) {
			if i+1 < len(tokens) && tokens[i+1].kind == tkLParen {
				return fmt.Errorf("blocked function %q in %s expression", tok.value, exprTypeName(exprType))
			}
		}
	}

	// 5. Balanced parentheses
	depth := 0
	for _, tok := range tokens {
		if tok.kind == tkLParen {
			depth++
		}
		if tok.kind == tkRParen {
			depth--
		}
		if depth < 0 {
			return fmt.Errorf("unbalanced parentheses in %s expression", exprTypeName(exprType))
		}
	}
	if depth != 0 {
		return fmt.Errorf("unbalanced parentheses in %s expression", exprTypeName(exprType))
	}

	return nil
}

func exprTypeName(t ExprType) string {
	switch t {
	case ExprSelect:
		return "SELECT"
	case ExprGroupBy:
		return "GROUP BY"
	case ExprOrderBy:
		return "ORDER BY"
	default:
		return "SQL"
	}
}

// ---- IsLimitableQuery ------------------------------------------------------

// IsLimitableQuery returns true if the SQL query is of a type that accepts a
// LIMIT clause (SELECT, WITH). Statements like SHOW, DESCRIBE, EXPLAIN, and
// PRAGMA do not accept LIMIT and should not have one appended automatically.
func IsLimitableQuery(sql string) bool {
	tokens := tokenize(strings.TrimSpace(sql))
	if len(tokens) == 0 {
		return false
	}
	first := tokens[0]
	if first.kind != tkIdent {
		return false
	}
	switch first.upper {
	case "SELECT", "WITH":
		return true
	default:
		return false
	}
}

// ---- HasLimitToken ---------------------------------------------------------

// HasLimitToken checks if the SQL contains a LIMIT clause as a real token
// (not inside a string literal or comment, and not just used as an alias).
// It requires that LIMIT is followed by a numeric literal or the keyword ALL
// to distinguish a real LIMIT clause from an identifier named "LIMIT".
func HasLimitToken(sql string) bool {
	tokens := tokenize(sql)
	for i := 0; i < len(tokens); i++ {
		tok := tokens[i]
		if tok.kind == tkIdent && tok.upper == "LIMIT" {
			// Treat as real LIMIT clause only if followed by a number or ALL
			if i+1 < len(tokens) {
				next := tokens[i+1]
				if next.kind == tkNumber || (next.kind == tkIdent && next.upper == "ALL") {
					return true
				}
			}
		}
	}
	return false
}

// EnsureLimit appends "LIMIT max" when the query is SELECT/WITH and has no
// real LIMIT clause. Non-limitable statements (SHOW, PRAGMA, …) are unchanged.
func EnsureLimit(sql string, max int) string {
	if max <= 0 {
		return sql
	}
	if !IsLimitableQuery(sql) {
		return sql
	}
	if HasLimitToken(sql) {
		return sql
	}
	return fmt.Sprintf("%s LIMIT %d", strings.TrimSpace(sql), max)
}

// ---- ContainsUnsafeComment -------------------------------------------------

// ContainsUnsafeComment reports whether sql contains SQL comment markers
// (--, /*, */) that appear outside single-quoted string literals or
// double-quoted identifiers.
// A naive strings.Contains would false-positive on legitimate data like
// session_ids with "--" in them when interpolated into string values.
func ContainsUnsafeComment(sql string) bool {
	runes := []rune(sql)
	n := len(runes)
	i := 0
	for i < n {
		ch := runes[i]

		// Skip single-quoted string literals (including '' escape)
		if ch == '\'' {
			i++
			for i < n {
				if runes[i] == '\'' {
					if i+1 < n && runes[i+1] == '\'' {
						i += 2 // escaped quote ''
						continue
					}
					i++ // closing quote
					break
				}
				i++
			}
			continue
		}

		// Skip dollar-quoted strings so "--" / "/*" inside them are data.
		if ch == '$' {
			if end, ok := tryDollarString(runes, i); ok {
				i = end
				continue
			}
		}

		// Skip double-quoted identifiers (including "" escape)
		if ch == '"' {
			i++
			for i < n {
				if runes[i] == '"' {
					if i+1 < n && runes[i+1] == '"' {
						i += 2 // escaped double quote ""
						continue
					}
					i++ // closing quote
					break
				}
				i++
			}
			continue
		}

		// Line comment: --
		if ch == '-' && i+1 < n && runes[i+1] == '-' {
			return true
		}

		// Block comment: /*
		if ch == '/' && i+1 < n && runes[i+1] == '*' {
			return true
		}

		// Closing block comment: */ (standing alone is unsafe)
		if ch == '*' && i+1 < n && runes[i+1] == '/' {
			return true
		}

		i++
	}
	return false
}

// ---- Forbidden identifiers (read-only paths) -------------------------------

// ForbiddenReadOnlyKeywords are statement keywords that must not appear as
// identifiers outside string literals on read-only SELECT paths (node / MCP).
// CALL remains blocked as a real DuckDB statement; matching is token-aware so
// Call-IDs / session_ids that embed words like "call" are not rejected.
var ForbiddenReadOnlyKeywords = map[string]bool{
	"ATTACH":   true,
	"DETACH":   true,
	"COPY":     true,
	"PRAGMA":   true,
	"INSTALL":  true,
	"LOAD":     true,
	"CALL":     true,
	"CREATE":   true,
	"ALTER":    true,
	"DROP":     true,
	"TRUNCATE": true,
	"INSERT":   true,
	"UPDATE":   true,
	"DELETE":   true,
	"MERGE":    true,
	"REPLACE":  true,
	"GRANT":    true,
	"REVOKE":   true,
	"VACUUM":   true,
	"ANALYZE":  true,
	"EXPORT":   true,
	"IMPORT":   true,
}

// ContainsForbiddenIdentifier reports whether sql contains any of the given
// keywords as identifier tokens outside string literals. A naive whole-string
// regex would false-positive on Call-IDs / session_ids that embed words like
// "call" or "delete".
func ContainsForbiddenIdentifier(sql string, forbidden map[string]bool) bool {
	if len(forbidden) == 0 {
		return false
	}
	for _, tok := range tokenize(sql) {
		if tok.kind != tkIdent {
			continue
		}
		if forbidden[tok.upper] {
			return true
		}
	}
	return false
}

// ---- SafeString ------------------------------------------------------------

const maxSafeStringLen = 1000

// SafeString sanitizes a user-supplied string value for safe interpolation
// into a SQL single-quoted string context. It escapes single quotes, strips
// null bytes and control characters, and enforces a length limit.
//
// Usage: fmt.Sprintf("column = '%s'", SafeString(userInput))
func SafeString(s string) string {
	// Length limit (truncate safely on rune boundaries to avoid splitting UTF-8)
	if len(s) > maxSafeStringLen {
		runes := []rune(s)
		if len(runes) > maxSafeStringLen {
			runes = runes[:maxSafeStringLen]
		}
		s = string(runes)
	}

	var b strings.Builder
	b.Grow(len(s) + 10)

	for _, r := range s {
		// Strip null bytes
		if r == 0 {
			continue
		}
		// Strip non-printable control characters (keep tab, newline, carriage return)
		if r < 0x20 && r != '\t' && r != '\n' && r != '\r' {
			continue
		}
		// Escape single quotes
		if r == '\'' {
			b.WriteString("''")
			continue
		}
		// Escape backslashes (prevent DuckDB escape interpretation)
		if r == '\\' {
			b.WriteString("\\\\")
			continue
		}
		b.WriteRune(r)
	}

	return b.String()
}
