// Copyright (C) 2026 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package ducklake

import (
	"database/sql"
	"fmt"
	"strings"

	logger "github.com/sipcapture/homer-core/src/utils/logging"
)

// ApplyDuckDBTuning issues the per-connection DuckDB SET statements
// implied by the operator-supplied DuckDBTuning config block:
//
//   - threads        -> SET threads = N
//   - memory_limit   -> SET memory_limit = '<string>'
//   - temp_directory -> SET temp_directory = '<path>'
//
// Empty / zero values mean "do not touch this setting" so the function
// is safe to call unconditionally on every freshly-opened DuckDB
// connection (writer, node-side reader, CLI, ...).
//
// Errors are logged at WARN level and swallowed: a bad memory_limit
// string (e.g. "8 angstrom") should not stop the writer from coming up,
// it should just leave DuckDB on its built-in default. Pass `who` to
// describe the call site in the warning ("writer", "node", ...).
func ApplyDuckDBTuning(db *sql.DB, threads int, memoryLimit, tempDirectory, who string) {
	if db == nil {
		return
	}
	if threads > 0 {
		if _, err := db.Exec(fmt.Sprintf("SET threads = %d", threads)); err != nil {
			logger.Warn(fmt.Sprintf("DuckDB tuning (%s): SET threads = %d failed: %v", who, threads, err))
		} else {
			logger.Info("DuckDB tuning: threads set", "where", who, "threads", threads)
		}
	}
	if s := strings.TrimSpace(memoryLimit); s != "" {
		// Single-quote-escape the operator-supplied string so a value
		// like "O'Brien" can't break the SET statement. DuckDB accepts
		// double single quotes as an embedded quote.
		safe := strings.ReplaceAll(s, "'", "''")
		if _, err := db.Exec(fmt.Sprintf("SET memory_limit = '%s'", safe)); err != nil {
			logger.Warn(fmt.Sprintf("DuckDB tuning (%s): SET memory_limit = %q failed: %v", who, s, err))
		} else {
			logger.Info("DuckDB tuning: memory_limit set", "where", who, "memory_limit", s)
		}
	}
	if s := strings.TrimSpace(tempDirectory); s != "" {
		safe := strings.ReplaceAll(s, "'", "''")
		if _, err := db.Exec(fmt.Sprintf("SET temp_directory = '%s'", safe)); err != nil {
			logger.Warn(fmt.Sprintf("DuckDB tuning (%s): SET temp_directory = %q failed: %v", who, s, err))
		} else {
			logger.Info("DuckDB tuning: temp_directory set", "where", who, "temp_directory", s)
		}
	}
}
