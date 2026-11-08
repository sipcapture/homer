// Copyright (C) 2026 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package sqlvalidator

import "testing"

// query() and query_table() execute their string argument, so a blocked
// reader hidden in the literal must not get through.
func TestValidateRawSQL_BlocksNestedQueryFunctions(t *testing.T) {
	blocked := []string{
		`SELECT * FROM query('SELECT content FROM read_text(''/etc/passwd'')')`,
		`SELECT content FROM QUERY('SELECT content FROM read_text(''/tmp/x'')')`,
		`SELECT * FROM query ('SELECT getenv(''HOME'')')`,
		`SELECT * FROM main.query('SELECT 1')`,
		`SELECT * FROM "query"('SELECT 1')`,
		`SELECT * FROM query_table('hep_proto_1_call')`,
		`SELECT * FROM query_table(['a', 'b'])`,
		`SELECT json_execute_serialized_sql('{}')`,
		`WITH x AS (SELECT * FROM query('SELECT 1')) SELECT * FROM x`,
		`SELECT * FROM hep WHERE sid IN (SELECT * FROM query('SELECT 1'))`,
	}
	for _, sql := range blocked {
		if err := ValidateRawSQL(sql); err == nil {
			t.Errorf("expected %q to be rejected", sql)
		}
	}
}

func TestValidateRawSQL_AllowsQueryAsColumnName(t *testing.T) {
	ok := []string{
		`SELECT query FROM hep_proto_1_call LIMIT 10`,
		`SELECT count(*) AS query_table FROM hep_proto_1_call`,
		`SELECT * FROM hep_proto_1_call WHERE data_header LIKE '%query(%'`,
	}
	for _, sql := range ok {
		if err := ValidateRawSQL(sql); err != nil {
			t.Errorf("expected %q to be allowed, got %v", sql, err)
		}
	}
}
