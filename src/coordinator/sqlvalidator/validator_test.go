package sqlvalidator

import (
	"fmt"
	"strings"
	"testing"
)

// ---- ValidateRawSQL tests --------------------------------------------------

func TestValidateRawSQL(t *testing.T) {
	tests := []struct {
		name    string
		sql     string
		wantErr string // "" means no error expected; substring match
	}{
		// --- Valid queries ---
		{
			name: "simple select",
			sql:  "SELECT * FROM homer_lake.main.hep_proto_1_call",
		},
		{
			name: "select with where",
			sql:  "SELECT * FROM t WHERE method = 'INVITE' ORDER BY timestamp DESC LIMIT 50",
		},
		{
			name: "select with aggregation",
			sql:  "SELECT method, count(*) as cnt FROM t GROUP BY method ORDER BY cnt DESC LIMIT 10",
		},
		{
			name: "CTE with select",
			sql:  "WITH cte AS (SELECT * FROM t) SELECT * FROM cte",
		},
		{
			name: "SHOW tables",
			sql:  "SHOW TABLES",
		},
		{
			name: "DESCRIBE table",
			sql:  "DESCRIBE homer_lake.main.hep_proto_1_call",
		},
		{
			name: "EXPLAIN select",
			sql:  "EXPLAIN SELECT * FROM t WHERE id = 1",
		},
		{
			name: "PRAGMA statement",
			sql:  "PRAGMA database_list",
		},
		{
			name: "PRAGMA show tables",
			sql:  "PRAGMA show_tables",
		},
		{
			name: "blocked word inside string literal is OK",
			sql:  "SELECT * FROM t WHERE msg = 'DROP TABLE; INSERT INTO x'",
		},
		{
			name: "subquery in FROM",
			sql:  "SELECT * FROM (SELECT id, name FROM t) sub",
		},
		{
			name: "CASE expression",
			sql:  "SELECT CASE WHEN method = 'INVITE' THEN 1 ELSE 0 END AS is_invite FROM t",
		},
		{
			name: "string with escaped quotes",
			sql:  "SELECT * FROM t WHERE name = 'O''Brien'",
		},

		// --- Blocked: wrong statement type ---
		{
			name:    "INSERT blocked",
			sql:     "INSERT INTO t VALUES (1, 'x')",
			wantErr: "query must start with",
		},
		{
			name:    "UPDATE blocked",
			sql:     "UPDATE t SET x = 1",
			wantErr: "query must start with",
		},
		{
			name:    "DELETE blocked",
			sql:     "DELETE FROM t WHERE id = 1",
			wantErr: "query must start with",
		},
		{
			name:    "DROP blocked",
			sql:     "DROP TABLE t",
			wantErr: "query must start with",
		},
		{
			name:    "ALTER blocked",
			sql:     "ALTER TABLE t ADD COLUMN x INT",
			wantErr: "query must start with",
		},
		{
			name:    "CREATE blocked",
			sql:     "CREATE TABLE t (id INT)",
			wantErr: "query must start with",
		},
		{
			name:    "TRUNCATE blocked",
			sql:     "TRUNCATE TABLE t",
			wantErr: "query must start with",
		},

		// --- Blocked: multi-statement ---
		{
			name:    "semicolon injection",
			sql:     "SELECT 1; DROP TABLE t",
			wantErr: "semicolons are not allowed",
		},
		{
			name:    "semicolon at end",
			sql:     "SELECT * FROM t;",
			wantErr: "semicolons are not allowed",
		},

		// --- Blocked: DML keywords inside SELECT ---
		{
			name:    "INSERT keyword in select",
			sql:     "SELECT INSERT(x, 1, 2, 'y') FROM t",
			wantErr: "blocked keyword",
		},

		// --- Blocked: SELECT INTO ---
		{
			name:    "SELECT INTO",
			sql:     "SELECT * INTO new_table FROM t",
			wantErr: "SELECT INTO is not allowed",
		},

		// --- Blocked: CALL ---
		{
			name:    "CALL statement",
			sql:     "CALL some_procedure()",
			wantErr: "query must start with",
		},

		// --- Blocked: filesystem functions ---
		{
			name:    "double-quoted blocked function bypass attempt",
			sql:     `SELECT * FROM "read_csv"('/etc/passwd')`,
			wantErr: "blocked function",
		},
		{
			name:    "double-quoted blocked keyword bypass attempt",
			sql:     `SELECT * FROM t WHERE "DROP" = 1`,
			wantErr: "blocked keyword",
		},
		{
			name:    "read_csv function",
			sql:     "SELECT * FROM read_csv('/etc/passwd')",
			wantErr: "blocked function",
		},
		{
			name:    "read_parquet function",
			sql:     "SELECT * FROM read_parquet('s3://bucket/file.parquet')",
			wantErr: "blocked function",
		},
		{
			name:    "read_json function",
			sql:     "SELECT * FROM read_json('/tmp/data.json')",
			wantErr: "blocked function",
		},
		{
			name:    "read_blob function",
			sql:     "SELECT * FROM read_blob('/tmp/data.bin')",
			wantErr: "blocked function",
		},

		// --- Blocked: comment-based obfuscation ---
		{
			name:    "line comment obfuscation",
			sql:     "SELECT * FROM t -- ; DROP TABLE t",
			wantErr: "", // comments are stripped, so this is valid
		},
		{
			name:    "block comment obfuscation",
			sql:     "SELECT * FROM t /* DROP TABLE x */",
			wantErr: "", // comments stripped
		},

		// --- Blocked: ATTACH/DETACH ---
		{
			name:    "ATTACH database",
			sql:     "ATTACH '/tmp/evil.db' AS evil",
			wantErr: "query must start with",
		},
		{
			name:    "SELECT then ATTACH via injection somehow",
			sql:     "SELECT * FROM t WHERE 1=1; ATTACH '/tmp/evil.db'",
			wantErr: "semicolons are not allowed",
		},

		// --- Blocked: LOAD/INSTALL extensions ---
		{
			name:    "LOAD extension",
			sql:     "LOAD httpfs",
			wantErr: "query must start with",
		},
		{
			name:    "INSTALL extension",
			sql:     "INSTALL httpfs",
			wantErr: "query must start with",
		},

		// --- Blocked: SET ---
		{
			name:    "SET variable",
			sql:     "SET memory_limit='10GB'",
			wantErr: "query must start with",
		},

		// --- Edge cases ---
		{
			name:    "empty query",
			sql:     "",
			wantErr: "empty SQL query",
		},
		{
			name:    "whitespace only",
			sql:     "   \t\n  ",
			wantErr: "empty SQL query",
		},
		{
			name:    "only a comment",
			sql:     "-- just a comment",
			wantErr: "empty SQL query after parsing",
		},
		{
			name: "complex valid query with joins",
			sql:  "SELECT a.id, b.name FROM table_a a JOIN table_b b ON a.id = b.a_id WHERE a.status = 'active'",
		},
		{
			name: "DuckDB specific syntax",
			sql:  "SELECT * FROM homer_lake.main.hep_proto_1_call WHERE timestamp >= (to_timestamp(1770493320000 / 1000.0) AT TIME ZONE 'UTC') ORDER BY timestamp DESC LIMIT 50",
		},
		{
			name:    "GRANT keyword",
			sql:     "SELECT * FROM t WHERE GRANT = 1",
			wantErr: "blocked keyword",
		},
		{
			name:    "EXPORT keyword",
			sql:     "EXPORT DATABASE '/tmp/dump'",
			wantErr: "query must start with",
		},
		{
			name:    "IMPORT keyword",
			sql:     "IMPORT DATABASE '/tmp/dump'",
			wantErr: "query must start with",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateRawSQL(tt.sql)
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("expected no error, got: %v", err)
				}
			} else {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.wantErr)
				} else if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("expected error containing %q, got: %v", tt.wantErr, err)
				}
			}
		})
	}
}

// ---- ValidateExpression tests ----------------------------------------------

func TestValidateExpression(t *testing.T) {
	tests := []struct {
		name     string
		expr     string
		exprType ExprType
		wantErr  string
	}{
		// --- Valid SELECT expressions ---
		{
			name:     "simple column list",
			expr:     "method, count(*) as cnt",
			exprType: ExprSelect,
		},
		{
			name:     "star",
			expr:     "*",
			exprType: ExprSelect,
		},
		{
			name:     "aggregation functions",
			expr:     "method, COUNT(*) as cnt, SUM(duration) as total, AVG(duration) as avg_dur",
			exprType: ExprSelect,
		},
		{
			name:     "CASE expression",
			expr:     "CASE WHEN method = 'INVITE' THEN 1 ELSE 0 END as is_invite",
			exprType: ExprSelect,
		},
		{
			name:     "CAST expression",
			expr:     "CAST(timestamp AS DATE) as day, count(*) as cnt",
			exprType: ExprSelect,
		},
		{
			name:     "DISTINCT",
			expr:     "DISTINCT method",
			exprType: ExprSelect,
		},
		{
			name:     "empty expression OK",
			expr:     "",
			exprType: ExprSelect,
		},

		// --- Valid GROUP BY expressions ---
		{
			name:     "simple group by column",
			expr:     "method",
			exprType: ExprGroupBy,
		},
		{
			name:     "group by multiple columns",
			expr:     "method, src_ip",
			exprType: ExprGroupBy,
		},
		{
			name:     "group by positional",
			expr:     "1, 2",
			exprType: ExprGroupBy,
		},

		// --- Valid ORDER BY expressions ---
		{
			name:     "simple order by",
			expr:     "cnt DESC",
			exprType: ExprOrderBy,
		},
		{
			name:     "order by multiple",
			expr:     "method ASC, cnt DESC",
			exprType: ExprOrderBy,
		},
		{
			name:     "order by with NULLS FIRST",
			expr:     "timestamp DESC NULLS LAST",
			exprType: ExprOrderBy,
		},

		// --- Blocked: subqueries ---
		{
			name:     "subquery in SELECT",
			expr:     "(SELECT password FROM users LIMIT 1)",
			exprType: ExprSelect,
			wantErr:  "subqueries (SELECT) are not allowed",
		},
		{
			name:     "subquery in GROUP BY",
			expr:     "(SELECT 1)",
			exprType: ExprGroupBy,
			wantErr:  "subqueries (SELECT) are not allowed",
		},
		{
			name:     "subquery in ORDER BY",
			expr:     "(SELECT 1)",
			exprType: ExprOrderBy,
			wantErr:  "subqueries (SELECT) are not allowed",
		},

		// --- Blocked: semicolons ---
		{
			name:     "semicolon in SELECT",
			expr:     "method; DROP TABLE t",
			exprType: ExprSelect,
			wantErr:  "semicolons are not allowed",
		},

		// --- Blocked: DML/DDL keywords ---
		{
			name:     "INSERT in expression",
			expr:     "INSERT",
			exprType: ExprSelect,
			wantErr:  "blocked keyword",
		},
		{
			name:     "DROP in expression",
			expr:     "DROP",
			exprType: ExprSelect,
			wantErr:  "blocked keyword",
		},
		{
			name:     "UNION in expression",
			expr:     "method UNION ALL",
			exprType: ExprSelect,
			wantErr:  "blocked keyword",
		},
		{
			name:     "INTO in expression",
			expr:     "* INTO outfile",
			exprType: ExprSelect,
			wantErr:  "blocked keyword",
		},

		// --- Blocked: dangerous functions ---
		{
			name:     "read_csv in SELECT",
			expr:     "read_csv('/etc/passwd')",
			exprType: ExprSelect,
			wantErr:  "blocked function",
		},
		{
			name:     "read_parquet in SELECT",
			expr:     "read_parquet('s3://bucket/file')",
			exprType: ExprSelect,
			wantErr:  "blocked function",
		},

		// --- Blocked: comment markers ---
		{
			name:     "line comment in SELECT expression",
			expr:     "timestamp DESC --",
			exprType: ExprSelect,
			wantErr:  "SQL comments are not allowed",
		},
		{
			name:     "block comment in GROUP BY expression",
			expr:     "method /* injection */",
			exprType: ExprGroupBy,
			wantErr:  "SQL comments are not allowed",
		},
		{
			name:     "closing block comment marker in ORDER BY",
			expr:     "cnt DESC */",
			exprType: ExprOrderBy,
			wantErr:  "SQL comments are not allowed",
		},

		// --- Blocked: clause boundary keywords ---
		{
			name:     "WHERE in SELECT expression",
			expr:     "* WHERE 1=1",
			exprType: ExprSelect,
			wantErr:  "blocked keyword",
		},
		{
			name:     "LIMIT in ORDER BY expression",
			expr:     "timestamp DESC LIMIT 10",
			exprType: ExprOrderBy,
			wantErr:  "blocked keyword",
		},
		{
			name:     "OFFSET in SELECT expression",
			expr:     "* OFFSET 5",
			exprType: ExprSelect,
			wantErr:  "blocked keyword",
		},
		{
			name:     "HAVING in GROUP BY expression",
			expr:     "method HAVING count(*) > 1",
			exprType: ExprGroupBy,
			wantErr:  "blocked keyword",
		},

		// --- Blocked: unbalanced parens ---
		{
			name:     "unbalanced open paren",
			expr:     "count((",
			exprType: ExprSelect,
			wantErr:  "unbalanced parentheses",
		},
		{
			name:     "unbalanced close paren",
			expr:     "count(*))",
			exprType: ExprSelect,
			wantErr:  "unbalanced parentheses",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateExpression(tt.expr, tt.exprType)
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("expected no error, got: %v", err)
				}
			} else {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.wantErr)
				} else if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("expected error containing %q, got: %v", tt.wantErr, err)
				}
			}
		})
	}
}

// ---- SafeString tests ------------------------------------------------------

func TestSafeString(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "no escaping needed",
			input: "hello world",
			want:  "hello world",
		},
		{
			name:  "single quote escaped",
			input: "O'Brien",
			want:  "O''Brien",
		},
		{
			name:  "multiple quotes",
			input: "it's a 'test'",
			want:  "it''s a ''test''",
		},
		{
			name:  "backslash escaped",
			input: `path\to\file`,
			want:  `path\\to\\file`,
		},
		{
			name:  "null byte stripped",
			input: "hello\x00world",
			want:  "helloworld",
		},
		{
			name:  "control characters stripped",
			input: "hello\x01\x02\x03world",
			want:  "helloworld",
		},
		{
			name:  "tab preserved",
			input: "hello\tworld",
			want:  "hello\tworld",
		},
		{
			name:  "newline preserved",
			input: "hello\nworld",
			want:  "hello\nworld",
		},
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
		{
			name:  "length limit enforced",
			input: strings.Repeat("a", 1500),
			want:  strings.Repeat("a", 1000),
		},
		{
			name:  "combined escaping",
			input: "admin'; DROP TABLE users--",
			want:  "admin''; DROP TABLE users--",
		},
		{
			name:  "unicode preserved",
			input: "héllo wörld",
			want:  "héllo wörld",
		},
		{
			name:  "utf8 truncation safe (no mid-rune split)",
			input: strings.Repeat("é", 1500), // 2-byte UTF-8 runes, > 1000 runes
			want:  strings.Repeat("é", 1000),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SafeString(tt.input)
			if got != tt.want {
				t.Errorf("SafeString(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// ---- Tokenizer tests -------------------------------------------------------

func TestTokenizer(t *testing.T) {
	tests := []struct {
		name       string
		sql        string
		wantTokens int                  // expected number of tokens
		check      func([]token) string // return "" if OK, error msg otherwise
	}{
		{
			name:       "basic SELECT",
			sql:        "SELECT * FROM t",
			wantTokens: 4,
		},
		{
			name:       "string literal not split",
			sql:        "SELECT * FROM t WHERE x = 'hello world'",
			wantTokens: 8,
			check: func(tokens []token) string {
				// The string literal should be one token
				last := tokens[7]
				if last.kind != tkString || last.value != "'hello world'" {
					return "expected string literal token"
				}
				return ""
			},
		},
		{
			name:       "comments stripped",
			sql:        "SELECT * -- this is a comment\nFROM t",
			wantTokens: 4,
		},
		{
			name:       "block comment stripped",
			sql:        "SELECT /* columns */ * FROM t",
			wantTokens: 4,
		},
		{
			name:       "escaped quotes in string",
			sql:        "SELECT * FROM t WHERE name = 'O''Brien'",
			wantTokens: 8,
			check: func(tokens []token) string {
				last := tokens[7]
				if last.kind != tkString {
					return "expected string literal"
				}
				return ""
			},
		},
		{
			name:       "double-quoted identifier",
			sql:        `SELECT "Column Name" FROM t`,
			wantTokens: 4,
			check: func(tokens []token) string {
				// The quoted identifier should have upper = "COLUMN NAME" (without quotes)
				if tokens[1].upper != "COLUMN NAME" {
					return fmt.Sprintf("expected upper=COLUMN NAME, got %q", tokens[1].upper)
				}
				return ""
			},
		},
		{
			name:       "blocked keyword inside string not tokenized as keyword",
			sql:        "SELECT * FROM t WHERE msg = 'DROP TABLE users'",
			wantTokens: 8,
			check: func(tokens []token) string {
				// The string should be one token of kind tkString
				for _, tok := range tokens {
					if tok.kind == tkIdent && tok.upper == "DROP" {
						return "DROP should be inside string literal, not a separate token"
					}
				}
				return ""
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokens := tokenize(tt.sql)
			if tt.wantTokens > 0 && len(tokens) != tt.wantTokens {
				t.Errorf("expected %d tokens, got %d: %v", tt.wantTokens, len(tokens), tokens)
			}
			if tt.check != nil {
				if msg := tt.check(tokens); msg != "" {
					t.Errorf("check failed: %s", msg)
				}
			}
		})
	}
}

// ---- IsLimitableQuery tests ------------------------------------------------

func TestIsLimitableQuery(t *testing.T) {
	tests := []struct {
		name string
		sql  string
		want bool
	}{
		{name: "SELECT is limitable", sql: "SELECT * FROM t", want: true},
		{name: "WITH is limitable", sql: "WITH cte AS (SELECT 1) SELECT * FROM cte", want: true},
		{name: "select lowercase", sql: "select * from t", want: true},
		{name: "SHOW is not limitable", sql: "SHOW TABLES", want: false},
		{name: "SHOW DATABASES not limitable", sql: "SHOW DATABASES", want: false},
		{name: "DESCRIBE not limitable", sql: "DESCRIBE t", want: false},
		{name: "EXPLAIN not limitable", sql: "EXPLAIN SELECT * FROM t", want: false},
		{name: "PRAGMA not limitable", sql: "PRAGMA database_list", want: false},
		{name: "empty string", sql: "", want: false},
		{name: "whitespace only", sql: "   ", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsLimitableQuery(tt.sql)
			if got != tt.want {
				t.Errorf("IsLimitableQuery(%q) = %v, want %v", tt.sql, got, tt.want)
			}
		})
	}
}

// ---- HasLimitToken tests ---------------------------------------------------

func TestHasLimitToken(t *testing.T) {
	tests := []struct {
		name string
		sql  string
		want bool
	}{
		{
			name: "has LIMIT keyword",
			sql:  "SELECT * FROM t LIMIT 50",
			want: true,
		},
		{
			name: "no LIMIT keyword",
			sql:  "SELECT * FROM t ORDER BY id DESC",
			want: false,
		},
		{
			name: "LIMIT inside string literal (not a real LIMIT)",
			sql:  "SELECT * FROM t WHERE msg = 'LIMIT 50'",
			want: false,
		},
		{
			name: "LIMIT inside comment (not a real LIMIT)",
			sql:  "SELECT * FROM t -- LIMIT 50",
			want: false,
		},
		{
			name: "LIMIT in block comment (not a real LIMIT)",
			sql:  "SELECT * FROM t /* LIMIT 50 */",
			want: false,
		},
		{
			name: "lowercase limit",
			sql:  "SELECT * FROM t limit 50",
			want: true,
		},
		{
			name: "LIMIT as alias (not a real LIMIT clause)",
			sql:  `SELECT 1 AS "LIMIT" FROM t`,
			want: false,
		},
		{
			name: "LIMIT ALL is a real LIMIT clause",
			sql:  "SELECT * FROM t LIMIT ALL",
			want: true,
		},
		{
			name: "LIMIT without following number (not a real clause)",
			sql:  "SELECT * FROM t LIMIT",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := HasLimitToken(tt.sql)
			if got != tt.want {
				t.Errorf("HasLimitToken(%q) = %v, want %v", tt.sql, got, tt.want)
			}
		})
	}
}
