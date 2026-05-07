// Copyright (C) 2026 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later
//
// Package sqlrewrite translates Grafana InfluxDB FlightSQL queries into
// DuckDB-compatible SQL for homer-core FlightSQL.

package sqlrewrite

import (
	"database/sql"
	"fmt"
	"strings"
)

// TableTimeHint is optional per-table metadata for Grafana time column mapping.
type TableTimeHint struct {
	TimeColumn string
}

// TableSettings maps logical table name (case-insensitive keys) to hints.
type TableSettings map[string]TableTimeHint

// Rewriter translates InfluxDB-dialect SQL into DuckDB SQL.
type Rewriter struct {
	LakeName      string
	TableSettings TableSettings
	DB            *sql.DB // may be nil — qualifyTableNames is skipped
}

// Rewrite returns the rewritten query and a short rule name for logging.
func (r *Rewriter) Rewrite(original string) (rewritten string, rule string) {
	upper := strings.ToUpper(strings.TrimSpace(original))

	if strings.Contains(upper, "INFORMATION_SCHEMA.TABLES") &&
		strings.Contains(upper, "TABLE_SCHEMA") &&
		strings.Contains(upper, "'IOX'") {
		catalog := r.LakeName
		if catalog == "" {
			catalog = "homer_lake"
		}
		q := fmt.Sprintf(
			`SELECT table_name FROM information_schema.tables`+
				` WHERE table_catalog = '%s' AND table_schema = 'main'`+
				` ORDER BY table_name`, catalog)
		return q, "table discovery (iox → DuckLake catalog)"
	}

	if strings.Contains(upper, "INFORMATION_SCHEMA.COLUMNS") &&
		strings.Contains(upper, "TABLE_SCHEMA") &&
		strings.Contains(upper, "'IOX'") {
		tableName := extractQuotedValue(original, "table_name")
		if tableName != "" && r.LakeName != "" {
			fqn := r.LakeName + ".main." + tableName
			var q string
			if tblCfg, ok := r.TableSettings[strings.ToLower(tableName)]; ok && tblCfg.TimeColumn != "" {
				q = fmt.Sprintf(
					`SELECT column_name, column_type AS data_type FROM (DESCRIBE %s) `+
						`UNION ALL SELECT 'time' AS column_name, 'TIMESTAMP' AS data_type `+
						`ORDER BY column_name`, fqn)
			} else {
				q = fmt.Sprintf(
					`SELECT column_name, column_type AS data_type FROM (DESCRIBE %s) ORDER BY column_name`, fqn)
			}
			return q, "column discovery (iox → DESCRIBE)"
		}
	}

	out := original
	if r.LakeName != "" && r.DB != nil {
		out = r.qualifyTableNames(out)
	}
	out = r.rewriteTimeColumn(out)

	if out != original {
		return out, "qualify table names + rewrite time column"
	}
	return original, "no rewrite needed"
}

func (r *Rewriter) rewriteTimeColumn(s string) string {
	timeCol := r.resolveTimeColumn(s)
	quotedCol := `"` + timeCol + `"`
	s = strings.ReplaceAll(s, `"time"`, quotedCol)
	s = strings.ReplaceAll(s, `as `+quotedCol, `as "time"`)
	s = strings.ReplaceAll(s, `AS `+quotedCol, `AS "time"`)
	return s
}

func (r *Rewriter) resolveTimeColumn(s string) string {
	lower := strings.ToLower(s)
	for tableName, tblCfg := range r.TableSettings {
		if tblCfg.TimeColumn != "" && strings.Contains(lower, strings.ToLower(tableName)) {
			return tblCfg.TimeColumn
		}
	}
	return "timestamp"
}

func (r *Rewriter) qualifyTableNames(s string) string {
	prefix := r.LakeName + "."
	if strings.Contains(s, prefix) {
		return s
	}

	rows, err := r.DB.Query(
		`SELECT table_name FROM information_schema.tables
		 WHERE table_catalog = ? AND table_schema = 'main'`, r.LakeName)
	if err != nil {
		return s
	}
	defer rows.Close()

	knownTables := make(map[string]bool)
	for rows.Next() {
		var name string
		if rows.Scan(&name) == nil {
			knownTables[strings.ToLower(name)] = true
		}
	}

	for tbl := range knownTables {
		quoted := `"` + tbl + `"`
		s = strings.ReplaceAll(s, quoted, prefix+quoted)
	}
	return s
}

func extractQuotedValue(s, key string) string {
	lower := strings.ToLower(s)
	keyLower := strings.ToLower(key)
	idx := strings.Index(lower, keyLower)
	if idx < 0 {
		return ""
	}
	rest := s[idx+len(key):]
	for len(rest) > 0 && (rest[0] == ' ' || rest[0] == '=') {
		rest = rest[1:]
	}
	if len(rest) == 0 || rest[0] != '\'' {
		return ""
	}
	rest = rest[1:]
	end := strings.IndexByte(rest, '\'')
	if end < 0 {
		return ""
	}
	return rest[:end]
}
