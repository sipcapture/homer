// Copyright (C) 2026 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later
//
// This file is adapted from hepic-lake/src/writer/lineproto_parser.go
// (also AGPL-3.0-or-later) — both projects share this implementation
// of the InfluxDB Line Protocol so that wire-format compatibility is
// guaranteed across the homer + hepic-lake ecosystem.

// Package lineprotoreceiver implements an InfluxDB Line Protocol HTTP
// receiver that materialises every measurement as its own DuckLake
// table on the writer's primary shard.
//
// Routes (all enabled simultaneously):
//
//   - POST /write              — InfluxDB v1 write API
//   - POST /api/v2/write       — InfluxDB v2 write API
//   - POST /api/v3/write_lp    — gigapi-compatible alias for v2
//   - GET  /ping               — InfluxDB v1 health probe
//   - GET  /health             — InfluxDB v2 health probe
//
// Recognised query parameters: ?precision=ns|us|ms|s, ?db=<name>,
// ?bucket=<name> (synonym for db).
package lineprotoreceiver

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// LineProtoPoint is a single parsed InfluxDB Line Protocol data point.
//
// Fields store typed values (string / int64 / uint64 / float64 / bool).
// Timestamp is nanoseconds since Unix epoch; zero means "not provided —
// use wall-clock time at insertion".
type LineProtoPoint struct {
	Measurement string
	Tags        map[string]string
	Fields      map[string]interface{}
	TimestampNs int64
}

// LineProtoPrecision describes the precision of the timestamp in an LP payload.
// Valid values match InfluxDB 2.x / 3.x: "ns", "us" (or "µs"), "ms", "s".
type LineProtoPrecision string

const (
	PrecisionNanoseconds  LineProtoPrecision = "ns"
	PrecisionMicroseconds LineProtoPrecision = "us"
	PrecisionMilliseconds LineProtoPrecision = "ms"
	PrecisionSeconds      LineProtoPrecision = "s"
)

// ParsePrecision normalises the precision string (case/alias handling).
// Empty or unknown values default to nanoseconds.
func ParsePrecision(s string) LineProtoPrecision {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "n", "ns", "nanosecond", "nanoseconds":
		return PrecisionNanoseconds
	case "u", "us", "µs", "microsecond", "microseconds":
		return PrecisionMicroseconds
	case "ms", "millisecond", "milliseconds":
		return PrecisionMilliseconds
	case "s", "sec", "second", "seconds":
		return PrecisionSeconds
	}
	return PrecisionNanoseconds
}

// precisionMultiplier returns the factor to convert a raw timestamp integer
// into nanoseconds.
func precisionMultiplier(p LineProtoPrecision) int64 {
	switch p {
	case PrecisionSeconds:
		return int64(time.Second)
	case PrecisionMilliseconds:
		return int64(time.Millisecond)
	case PrecisionMicroseconds:
		return int64(time.Microsecond)
	}
	return 1
}

// ParseLineProtocol parses a block of text containing zero or more
// InfluxDB Line Protocol records. Empty lines and comment lines (starting
// with '#') are skipped. Each successfully parsed line becomes one point.
//
// On any parse error, returns the zero-based index of the offending input
// line, the partial slice of already-parsed points, and a descriptive error.
// Callers that want strict all-or-nothing semantics should discard the
// partial slice; callers that want best-effort ingestion can use it.
func ParseLineProtocol(data []byte, precision LineProtoPrecision) ([]LineProtoPoint, int, error) {
	mult := precisionMultiplier(precision)
	out := make([]LineProtoPoint, 0, 16)

	lineIdx := -1
	for _, raw := range splitLines(data) {
		lineIdx++
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		pt, err := parseSingleLine(line)
		if err != nil {
			return out, lineIdx, fmt.Errorf("line %d: %w", lineIdx+1, err)
		}
		if pt.TimestampNs != 0 {
			pt.TimestampNs *= mult
		}
		out = append(out, pt)
	}
	return out, -1, nil
}

func splitLines(data []byte) []string {
	s := string(data)
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

func parseSingleLine(line string) (LineProtoPoint, error) {
	var pt LineProtoPoint

	keyPart, rest, err := splitAtUnescapedSpace(line)
	if err != nil {
		return pt, err
	}
	if keyPart == "" {
		return pt, fmt.Errorf("empty measurement")
	}
	measurement, tags, err := parseMeasurementAndTags(keyPart)
	if err != nil {
		return pt, err
	}
	pt.Measurement = measurement
	pt.Tags = tags

	fieldPart, tsPart, err := splitAtUnescapedSpace(rest)
	if err != nil {
		return pt, err
	}
	if fieldPart == "" {
		return pt, fmt.Errorf("empty field set")
	}
	fields, err := parseFieldSet(fieldPart)
	if err != nil {
		return pt, err
	}
	if len(fields) == 0 {
		return pt, fmt.Errorf("no fields parsed")
	}
	pt.Fields = fields

	tsPart = strings.TrimSpace(tsPart)
	if tsPart != "" {
		ts, err := strconv.ParseInt(tsPart, 10, 64)
		if err != nil {
			return pt, fmt.Errorf("invalid timestamp %q: %w", tsPart, err)
		}
		pt.TimestampNs = ts
	}

	return pt, nil
}

// splitAtUnescapedSpace splits s into (before, after) at the first space
// that is not preceded by a backslash. An open quoted string (") swallows
// spaces.
func splitAtUnescapedSpace(s string) (string, string, error) {
	inQuote := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '\\':
			i++
		case '"':
			inQuote = !inQuote
		case ' ':
			if !inQuote {
				return s[:i], s[i+1:], nil
			}
		}
	}
	if inQuote {
		return "", "", fmt.Errorf("unterminated string literal")
	}
	return s, "", nil
}

func parseMeasurementAndTags(s string) (string, map[string]string, error) {
	parts, err := splitUnescaped(s, ',')
	if err != nil {
		return "", nil, err
	}
	if len(parts) == 0 || parts[0] == "" {
		return "", nil, fmt.Errorf("empty measurement")
	}
	measurement := unescapeLP(parts[0])

	if len(parts) == 1 {
		return measurement, nil, nil
	}
	tags := make(map[string]string, len(parts)-1)
	for _, raw := range parts[1:] {
		if raw == "" {
			continue
		}
		k, v, err := splitTagKV(raw)
		if err != nil {
			return "", nil, fmt.Errorf("tag %q: %w", raw, err)
		}
		if k == "" {
			return "", nil, fmt.Errorf("empty tag key in %q", raw)
		}
		tags[k] = v
	}
	return measurement, tags, nil
}

func splitTagKV(s string) (string, string, error) {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '\\' {
			i++
			continue
		}
		if c == '=' {
			return unescapeLP(s[:i]), unescapeLP(s[i+1:]), nil
		}
	}
	return "", "", fmt.Errorf("missing '=' in tag")
}

func parseFieldSet(s string) (map[string]interface{}, error) {
	parts, err := splitFieldSet(s)
	if err != nil {
		return nil, err
	}
	fields := make(map[string]interface{}, len(parts))
	for _, raw := range parts {
		if raw == "" {
			continue
		}
		k, v, err := splitFieldKV(raw)
		if err != nil {
			return nil, fmt.Errorf("field %q: %w", raw, err)
		}
		if k == "" {
			return nil, fmt.Errorf("empty field key in %q", raw)
		}
		val, err := parseFieldValue(v)
		if err != nil {
			return nil, fmt.Errorf("field %q: %w", k, err)
		}
		fields[k] = val
	}
	return fields, nil
}

// splitFieldSet splits a field set on top-level commas, respecting quoted
// string literals (which may contain commas).
func splitFieldSet(s string) ([]string, error) {
	var out []string
	var cur strings.Builder
	inQuote := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '\\':
			cur.WriteByte(c)
			if i+1 < len(s) {
				cur.WriteByte(s[i+1])
				i++
			}
		case '"':
			inQuote = !inQuote
			cur.WriteByte(c)
		case ',':
			if !inQuote {
				out = append(out, cur.String())
				cur.Reset()
				continue
			}
			cur.WriteByte(c)
		default:
			cur.WriteByte(c)
		}
	}
	if inQuote {
		return nil, fmt.Errorf("unterminated string literal in field set")
	}
	out = append(out, cur.String())
	return out, nil
}

func splitFieldKV(s string) (string, string, error) {
	inQuote := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '\\':
			i++
		case '"':
			inQuote = !inQuote
		case '=':
			if !inQuote {
				return unescapeLP(s[:i]), s[i+1:], nil
			}
		}
	}
	return "", "", fmt.Errorf("missing '=' in field")
}

// parseFieldValue converts a raw LP field value token into a typed Go value.
// Supported types:
//   - quoted string:           "foo"  → string
//   - integer with i suffix:   42i    → int64
//   - unsigned with u suffix:  42u    → int64 (DuckDB has no universal
//     unsigned type; we saturate at int64 max, matching what most
//     external InfluxDB integrations do)
//   - boolean:                 t|T|true|f|F|false → bool
//   - float:                   3.14   → float64
func parseFieldValue(s string) (interface{}, error) {
	if s == "" {
		return nil, fmt.Errorf("empty field value")
	}
	if s[0] == '"' {
		if len(s) < 2 || s[len(s)-1] != '"' {
			return nil, fmt.Errorf("unterminated string literal")
		}
		return unescapeQuoted(s[1 : len(s)-1]), nil
	}
	last := s[len(s)-1]
	switch last {
	case 'i', 'I':
		n, err := strconv.ParseInt(s[:len(s)-1], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid integer: %w", err)
		}
		return n, nil
	case 'u', 'U':
		n, err := strconv.ParseUint(s[:len(s)-1], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid unsigned integer: %w", err)
		}
		if n > 1<<63-1 {
			return int64(1<<63 - 1), nil
		}
		return int64(n), nil
	}
	switch s {
	case "t", "T", "true", "True", "TRUE":
		return true, nil
	case "f", "F", "false", "False", "FALSE":
		return false, nil
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid field value %q", s)
	}
	return f, nil
}

func splitUnescaped(s string, sep byte) ([]string, error) {
	var out []string
	var cur strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '\\' {
			cur.WriteByte(c)
			if i+1 < len(s) {
				cur.WriteByte(s[i+1])
				i++
			}
			continue
		}
		if c == sep {
			out = append(out, cur.String())
			cur.Reset()
			continue
		}
		cur.WriteByte(c)
	}
	out = append(out, cur.String())
	return out, nil
}

// unescapeLP removes backslash escapes valid in measurement, tag key,
// tag value, and field key tokens: "\\", "\,", "\=", "\ ".
func unescapeLP(s string) string {
	if !strings.ContainsRune(s, '\\') {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '\\' && i+1 < len(s) {
			n := s[i+1]
			if n == ',' || n == ' ' || n == '=' || n == '\\' {
				b.WriteByte(n)
				i++
				continue
			}
		}
		b.WriteByte(c)
	}
	return b.String()
}

// unescapeQuoted removes the two backslash escapes valid inside a quoted
// field value: "\\" and "\"".
func unescapeQuoted(s string) string {
	if !strings.ContainsRune(s, '\\') {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '\\' && i+1 < len(s) {
			n := s[i+1]
			if n == '"' || n == '\\' {
				b.WriteByte(n)
				i++
				continue
			}
		}
		b.WriteByte(c)
	}
	return b.String()
}
