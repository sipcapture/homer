// Copyright (C) 2026 Homer Server Contributors
// SPDX-License-Identifier: AGPL-3.0-or-later

package services

import (
	"encoding/json"
	"strings"
)

// LPColumn is a minimal column descriptor consumed by BuildLPFieldsMapping.
// It mirrors the subset of INFORMATION_SCHEMA.columns the LP mapping sync
// service needs and matches the shape returned by the existing
// /api/v4/line_protocol/tables discovery endpoint, so the same structs
// round-trip from sync ↔ handler ↔ tests without an extra adapter.
type LPColumn struct {
	Name     string
	DataType string
	Position int
}

// lpFieldEntry is the internal struct that mirrors the JSON shape of the
// embedded fields_*.json seeds — kept local so the seeds remain the
// authoritative spec and this package only emits "the same shape, but
// derived from live columns".
type lpFieldEntry struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Type     string `json:"type"`
	Index    string `json:"index"`
	FormType string `json:"form_type"`
	Position int    `json:"position"`
	SIDType  bool   `json:"sid_type,omitempty"`
	Hide     bool   `json:"hide"`
}

// BuildLPFieldsMapping turns a list of live DuckLake columns into the
// fields_mapping JSON consumed by the Proto Search widget.
//
// Type mapping — DuckDB type → UI shape:
//
//	TIMESTAMP* / DATE          → datetime input
//	BOOLEAN                    → switch
//	BIGINT / INTEGER / DOUBLE  → numeric input
//	JSON / VARCHAR / *         → free-text input
//
// The `time` column (always created by the LP receiver) is marked as
// `sid_type=true` so the widget treats it as the primary correlation
// key, mirroring how `trace_id` is treated for OTLP traces.
//
// Columns are emitted in the order their `Position` indicates — when
// two columns share the same position we fall back to the input slice
// order, which matches what the receiver hands us from
// information_schema.columns.
func BuildLPFieldsMapping(columns []LPColumn) ([]byte, error) {
	if len(columns) == 0 {
		return []byte("[]"), nil
	}

	entries := make([]lpFieldEntry, 0, len(columns))
	pos := 0
	for _, c := range columns {
		name := strings.TrimSpace(c.Name)
		if name == "" {
			continue
		}
		pos++
		uiType, formType := lpUITypeFor(c.DataType)
		hide := lpFieldShouldHide(name)
		entry := lpFieldEntry{
			ID:       name,
			Name:     prettifyColumnName(name),
			Type:     uiType,
			Index:    "none",
			FormType: formType,
			Position: pos,
			Hide:     hide,
		}
		if name == "time" {
			entry.SIDType = true
			entry.Index = "primary"
		}
		entries = append(entries, entry)
	}
	return json.Marshal(entries)
}

// lpUITypeFor maps a DuckDB SQL type to the (Type, FormType) pair the
// Proto Search widget expects in fields_mapping JSON.
func lpUITypeFor(dataType string) (string, string) {
	switch normaliseDuckType(dataType) {
	case "TIMESTAMP", "TIMESTAMP_NS", "TIMESTAMP_MS", "TIMESTAMP_S", "DATE":
		return "string", "datetime"
	case "BOOLEAN":
		return "boolean", "switch"
	case "TINYINT", "SMALLINT", "INTEGER", "BIGINT", "HUGEINT",
		"UTINYINT", "USMALLINT", "UINTEGER", "UBIGINT",
		"FLOAT", "DOUBLE", "DECIMAL":
		return "number", "input"
	default:
		// JSON / VARCHAR / BLOB / unknown — treat as free-text. JSON
		// columns can still be filtered server-side via CAST(... AS
		// VARCHAR) LIKE.
		return "string", "input"
	}
}

// normaliseDuckType strips DECIMAL(p,s) / TIMESTAMP_TZ / array suffixes
// so a single switch above stays readable.
func normaliseDuckType(t string) string {
	t = strings.ToUpper(strings.TrimSpace(t))
	if i := strings.IndexAny(t, "(["); i >= 0 {
		t = t[:i]
	}
	t = strings.TrimSpace(t)
	// Common aliases used by DuckDB's information_schema output.
	switch t {
	case "TIMESTAMP WITH TIME ZONE", "TIMESTAMPTZ":
		return "TIMESTAMP"
	case "INT", "INT4":
		return "INTEGER"
	case "INT8":
		return "BIGINT"
	case "INT2":
		return "SMALLINT"
	case "INT1":
		return "TINYINT"
	case "BOOL":
		return "BOOLEAN"
	}
	return t
}

// lpFieldShouldHide picks a sensible default visibility for the
// auto-generated mapping. We always show `time` and primitive columns,
// but hide the catch-all `raw` blob (when present) and any column
// whose name starts with "_" (DuckDB hidden-style convention).
func lpFieldShouldHide(name string) bool {
	if name == "" {
		return true
	}
	if name == "raw" || name == "_raw" || name == "raw_payload" {
		return true
	}
	if strings.HasPrefix(name, "_") {
		return true
	}
	return false
}

// prettifyColumnName turns "usage_idle" into "Usage idle" — gives the
// widget label something nicer than the bare column name without
// requiring a hand-curated translation table per measurement.
func prettifyColumnName(name string) string {
	if name == "" {
		return ""
	}
	parts := strings.FieldsFunc(name, func(r rune) bool {
		return r == '_' || r == '-' || r == '.'
	})
	if len(parts) == 0 {
		return name
	}
	for i, p := range parts {
		if p == "" {
			continue
		}
		// Title-case only the first word — keep the rest lowercase
		// so identifier-style names ("user_id" → "User id") read like
		// English instead of camelCase shouting.
		if i == 0 {
			r := []rune(p)
			r[0] = upperRune(r[0])
			parts[i] = string(r)
		} else {
			parts[i] = strings.ToLower(p)
		}
	}
	return strings.Join(parts, " ")
}

func upperRune(r rune) rune {
	if r >= 'a' && r <= 'z' {
		return r - 32
	}
	return r
}
