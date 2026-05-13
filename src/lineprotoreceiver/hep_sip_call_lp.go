// Copyright (C) 2026 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package lineprotoreceiver

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/sipcapture/homer-core/src/storage/ducklake"
)

// lineProtoPointToSIPCallRow maps one LP point (tags + fields) into a
// hep_proto_1_call row slice matching ducklake.SIPCallInsertColumnNames order.
func lineProtoPointToSIPCallRow(p *LineProtoPoint) ([]interface{}, error) {
	if p == nil {
		return nil, fmt.Errorf("nil point")
	}
	m := make(map[string]interface{}, len(p.Tags)+len(p.Fields))
	for k, v := range p.Tags {
		m[SanitizeIdent(k)] = v
	}
	for k, v := range p.Fields {
		m[SanitizeIdent(k)] = v
	}

	ts, err := resolveSIPCallTimestamp(p, m)
	if err != nil {
		return nil, err
	}

	dateStr, err := resolveSIPCallDate(m, ts)
	if err != nil {
		return nil, err
	}

	uuidStr := stringField(m, "uuid")
	if uuidStr == "" {
		uuidStr = uuid.NewString()
	}

	extra, err := resolveDataExtra(m)
	if err != nil {
		return nil, err
	}

	cols := ducklake.SIPCallInsertColumnNames()
	if len(cols) == 0 {
		return nil, fmt.Errorf("sip call column list unavailable")
	}

	vals := map[string]interface{}{
		"uuid":          uuidStr,
		"date":          dateStr,
		"timestamp":     ts,
		"session_id":    stringField(m, "session_id"),
		"caller":        stringField(m, "caller"),
		"callee":        stringField(m, "callee"),
		"src_ip":        stringField(m, "src_ip"),
		"dst_ip":        stringField(m, "dst_ip"),
		"src_port":      uint32Field(m, "src_port"),
		"dst_port":      uint32Field(m, "dst_port"),
		"method":        stringField(m, "method"),
		"response_code": stringField(m, "response_code"),
		"cseq_method":   stringField(m, "cseq_method"),
		"protocol":      uint32Field(m, "protocol"),
		"node_id":       stringField(m, "node_id"),
		"cid":           stringField(m, "cid"),
		"payload":       stringField(m, "payload"),
		"data_extra":    extra,
	}

	row := make([]interface{}, len(cols))
	for i, col := range cols {
		v, ok := vals[col]
		if !ok {
			return nil, fmt.Errorf("internal: missing binding for column %q", col)
		}
		row[i] = v
	}
	return row, nil
}

func stringField(m map[string]interface{}, key string) string {
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	switch x := v.(type) {
	case string:
		return x
	case []byte:
		return string(x)
	default:
		return fmt.Sprint(x)
	}
}

func uint32Field(m map[string]interface{}, key string) uint32 {
	v, ok := m[key]
	if !ok || v == nil {
		return 0
	}
	switch x := v.(type) {
	case uint32:
		return x
	case uint64:
		if x > math.MaxUint32 {
			return math.MaxUint32
		}
		return uint32(x)
	case int:
		if x < 0 {
			return 0
		}
		if x > math.MaxUint32 {
			return math.MaxUint32
		}
		return uint32(x)
	case int64:
		if x < 0 {
			return 0
		}
		if x > math.MaxUint32 {
			return math.MaxUint32
		}
		return uint32(x)
	case float64:
		if x < 0 || math.IsNaN(x) || math.IsInf(x, 0) {
			return 0
		}
		if x > float64(math.MaxUint32) {
			return math.MaxUint32
		}
		return uint32(x)
	case string:
		n, err := strconv.ParseUint(strings.TrimSpace(x), 10, 32)
		if err != nil {
			return 0
		}
		return uint32(n)
	default:
		return 0
	}
}

func resolveSIPCallTimestamp(p *LineProtoPoint, m map[string]interface{}) (time.Time, error) {
	if _, ok := m["timestamp"]; ok {
		v := m["timestamp"]
		if t, ok := parseFlexibleTime(v); ok {
			return t.UTC(), nil
		}
		return time.Time{}, fmt.Errorf("timestamp: could not parse value of type %T", v)
	}

	tsNs := p.TimestampNs
	if tsNs == 0 {
		tsNs = time.Now().UnixNano()
	}
	return time.Unix(0, tsNs).UTC(), nil
}

func resolveSIPCallDate(m map[string]interface{}, ts time.Time) (string, error) {
	if _, ok := m["date"]; ok {
		s := strings.TrimSpace(stringField(m, "date"))
		if s != "" {
			if _, err := time.Parse("2006-01-02", s); err != nil {
				return "", fmt.Errorf("date: expected YYYY-MM-DD, got %q", s)
			}
			return s, nil
		}
	}
	return ts.UTC().Format("2006-01-02"), nil
}

func resolveDataExtra(m map[string]interface{}) (string, error) {
	raw := strings.TrimSpace(stringField(m, "data_extra"))
	if raw == "" {
		return "{}", nil
	}
	if !json.Valid([]byte(raw)) {
		return "", fmt.Errorf("data_extra: invalid JSON")
	}
	var probe interface{}
	if err := json.Unmarshal([]byte(raw), &probe); err != nil {
		return "", fmt.Errorf("data_extra: invalid JSON: %w", err)
	}
	if _, ok := probe.(map[string]interface{}); !ok {
		return "", fmt.Errorf("data_extra: must be a JSON object")
	}
	return raw, nil
}

func parseFlexibleTime(v interface{}) (time.Time, bool) {
	switch x := v.(type) {
	case time.Time:
		return x, true
	case string:
		s := strings.TrimSpace(x)
		if s == "" {
			return time.Time{}, false
		}
		layouts := []string{
			time.RFC3339Nano,
			time.RFC3339,
			"2006-01-02 15:04:05.999999999",
			"2006-01-02 15:04:05.999999",
			"2006-01-02 15:04:05",
			"2006-01-02T15:04:05",
		}
		for _, ly := range layouts {
			if t, err := time.Parse(ly, s); err == nil {
				return t, true
			}
		}
		if n, err := strconv.ParseInt(s, 10, 64); err == nil {
			return unixIntToTime(n), true
		}
		return time.Time{}, false
	case int:
		return unixIntToTime(int64(x)), true
	case int64:
		return unixIntToTime(x), true
	case uint64:
		if x > math.MaxInt64 {
			return time.Unix(0, int64(math.MaxInt64)).UTC(), true
		}
		return unixIntToTime(int64(x)), true
	case float64:
		if math.IsNaN(x) || math.IsInf(x, 0) {
			return time.Time{}, false
		}
		sec, frac := math.Modf(x)
		return time.Unix(int64(sec), int64(frac*1e9)).UTC(), true
	default:
		return time.Time{}, false
	}
}

func unixIntToTime(n int64) time.Time {
	abs := n
	if n < 0 {
		abs = -n
	}
	switch {
	case abs >= 1_000_000_000_000_000_000: // ns
		return time.Unix(0, n).UTC()
	case abs >= 1_000_000_000_000: // µs
		return time.Unix(0, n*1000).UTC()
	case abs >= 1_000_000_000: // ms
		return time.Unix(0, n*1_000_000).UTC()
	default:
		return time.Unix(n, 0).UTC()
	}
}
