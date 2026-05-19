// Package sqlvalidator provides SQL injection prevention for Homer's coordinator.
// It validates raw SQL queries, SQL expression fragments, and sanitizes string values.
package sqlvalidator

import (
	"fmt"
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
	"INSTALL": true,
	"SET":     true,
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
	trimmed := strings.TrimSpace(sql)
	if trimmed == "" {
		return fmt.Errorf("empty SQL query")
	}

	tokens := tokenize(trimmed)
	if len(tokens) == 0 {
		return fmt.Errorf("empty SQL query after parsing")
	}

	// 1. Must start with an allowed statement type
	first := tokens[0]
	if first.kind != tkIdent || !allowedStatementStarts[first.upper] {
		return fmt.Errorf("query must start with SELECT, WITH, SHOW, DESCRIBE, EXPLAIN, or PRAGMA (got %q)", first.value)
	}

	// 2. No semicolons (prevents statement stacking)
	for _, tok := range tokens {
		if tok.kind == tkSemi {
			return fmt.Errorf("semicolons are not allowed (prevents multi-statement injection)")
		}
	}

	// 3. Check for blocked DML/DDL keywords
	for _, tok := range tokens {
		if tok.kind != tkIdent {
			continue
		}
		if blockedDML[tok.upper] {
			return fmt.Errorf("blocked keyword %q (DML/DDL operations are not allowed)", tok.value)
		}
	}

	// 4. Check for blocked filesystem/network functions
	for i, tok := range tokens {
		if tok.kind != tkIdent {
			continue
		}
		if blockedFunctions[tok.upper] {
			// Only flag if it looks like a function call (followed by parenthesis)
			if i+1 < len(tokens) && tokens[i+1].kind == tkLParen {
				return fmt.Errorf("blocked function %q (filesystem/network access is not allowed)", tok.value)
			}
			// Also block as keyword for COPY, EXPORT, IMPORT (already in blockedDML)
		}
	}

	// 5. Check for SELECT ... INTO (prevents writing to tables/files)
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

	// 6. Check for CALL (except within string literals, already handled by tokenizer)
	for _, tok := range tokens {
		if tok.kind == tkIdent && tok.upper == "CALL" {
			return fmt.Errorf("CALL statements are not allowed")
		}
	}

	return nil
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
	"INSTALL":  true,
	"SET":      true,
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
	if strings.Contains(trimmed, "--") || strings.Contains(trimmed, "/*") || strings.Contains(trimmed, "*/") {
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
		if blockedFunctions[tok.upper] {
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
