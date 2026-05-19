// Copyright (C) 2026 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package correlation

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/sipcapture/homer-core/src/coordinator/sqlvalidator"
	logger "github.com/sipcapture/homer-core/src/utils/logging"
)

// sqlBridge holds per-request state for the Lua-exposed SQL helpers:
// call counter, cancellable context, limits from config and a reference
// to the SQL executor (FlightService in production).
type sqlBridge struct {
	ctx    context.Context
	engine *CorrelationEngine
	in     CorrelationInput
	calls  int
}

func newSQLBridge(ctx context.Context, e *CorrelationEngine, in CorrelationInput) *sqlBridge {
	return &sqlBridge{ctx: ctx, engine: e, in: in}
}

// executeSQL is exposed to Lua as `executeSQL(sql)`. It returns a slice of
// maps, which luar will marshal into a Lua table of string-keyed tables.
//
// Hard rules (enforced regardless of script content):
//   - Per-request call count is bounded by cfg.MaxSQLCalls.
//   - SQL must pass sqlvalidator.ValidateRawSQL (SELECT/WITH/etc. only).
//   - A LIMIT is appended when the query is LIMIT-able and none is present.
//   - Each call has its own timeout (cfg.SQLTimeoutMS), derived from the
//     script's parent context so the overall script budget still applies.
func (b *sqlBridge) executeSQL(sqlQuery string) []map[string]interface{} {
	if b == nil || b.engine == nil || b.engine.executor == nil {
		logger.Warn("correlation: executeSQL called without an executor")
		return nil
	}
	b.calls++
	if b.calls > b.engine.cfg.MaxSQLCalls {
		logger.Warn("correlation: executeSQL over quota", "max", b.engine.cfg.MaxSQLCalls)
		return nil
	}
	if err := b.ctx.Err(); err != nil {
		logger.Warn("correlation: executeSQL called after context expired")
		return nil
	}

	trimmed := strings.TrimSpace(sqlQuery)
	if trimmed == "" {
		return nil
	}
	if err := sqlvalidator.ValidateRawSQL(trimmed); err != nil {
		logger.Warn("correlation: executeSQL blocked by validator",
			"err", err.Error(), "sql", truncate(trimmed, 256))
		return nil
	}

	finalSQL := sqlvalidator.EnsureLimit(trimmed, b.engine.cfg.MaxSQLRows)

	cctx, cancel := context.WithTimeout(b.ctx,
		time.Duration(b.engine.cfg.SQLTimeoutMS)*time.Millisecond)
	defer cancel()

	rows, err := b.engine.executor.Query(cctx, finalSQL)
	if err != nil {
		logger.Warn("correlation: executeSQL query failed", "err", err.Error())
		return nil
	}
	if len(rows) > b.engine.cfg.MaxSQLRows {
		rows = rows[:b.engine.cfg.MaxSQLRows]
	}
	return rows
}

// getDataByField is a safe shortcut that composes a parameterised SELECT
// against a known DuckLake table. It avoids the Lua script having to build
// raw SQL for the most common lookup ("give me rows where <field> IN <values>
// within [from_ms, to_ms]"). All identifiers are whitelisted; values are
// quoted with sqlvalidator.SafeString.
//
// Lua signature:
//
//	getDataByField(proto, event, field, values, from_ms, to_ms) -> rows
//
// Example:
//
//	rows = getDataByField(1, "call", "x_call_id", {"abc","def"}, from, to)
func (b *sqlBridge) getDataByField(proto int, event, field string, values []string, fromMs, toMs int64) []map[string]interface{} {
	if b == nil || b.engine == nil {
		return nil
	}
	if !isSafeIdent(field) {
		logger.Warn("correlation: getDataByField rejected unsafe field", "field", field)
		return nil
	}
	event = strings.ToLower(strings.TrimSpace(event))
	if !isSafeIdent(event) {
		logger.Warn("correlation: getDataByField rejected unsafe event", "event", event)
		return nil
	}
	values = dedupNonEmpty(values)
	if len(values) == 0 {
		return nil
	}

	table := fmt.Sprintf("%s.hep_proto_%d_%s", b.engine.lakeName, proto, event)

	var inList strings.Builder
	for i, v := range values {
		if i > 0 {
			inList.WriteString(",")
		}
		inList.WriteByte('\'')
		inList.WriteString(sqlvalidator.SafeString(v))
		inList.WriteByte('\'')
	}

	sqlQuery := fmt.Sprintf(
		"SELECT * FROM %s WHERE %s IN (%s)",
		table, field, inList.String(),
	)
	if fromMs > 0 && toMs > 0 && fromMs < toMs {
		sqlQuery += fmt.Sprintf(
			" AND timestamp >= (to_timestamp(%d / 1000.0) AT TIME ZONE 'UTC')"+
				" AND timestamp <= (to_timestamp(%d / 1000.0) AT TIME ZONE 'UTC')",
			fromMs, toMs,
		)
	}
	return b.executeSQL(sqlQuery)
}

// isSafeIdent is a strict whitelist for column/table fragments we interpolate
// directly into SQL. Letters, digits and underscore only; must not be empty.
func isSafeIdent(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		ok := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '_'
		if !ok {
			return false
		}
	}
	return true
}

// truncate caps a string for log lines so a pathological script can't flood
// the logs with megabyte-sized payloads.
func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
