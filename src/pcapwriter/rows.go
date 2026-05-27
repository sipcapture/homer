// Copyright (C) 2025 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package pcapwriter

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ErrNoSIPPacketsInRows is returned when no row had a non-empty payload field.
var ErrNoSIPPacketsInRows = errors.New("no SIP messages with payload to write (need payload, src_ip, dst_ip)")

// RowStr extracts a string field from a map row (API / search JSON).
func RowStr(row map[string]interface{}, key string) string {
	if v, ok := row[key]; ok && v != nil {
		return fmt.Sprintf("%v", v)
	}
	return ""
}

// RowInt extracts an integer-like field from a map row.
func RowInt(row map[string]interface{}, key string) int {
	if v, ok := row[key]; ok && v != nil {
		switch val := v.(type) {
		case int:
			return val
		case int32:
			return int(val)
		case int64:
			return int(val)
		case float64:
			return int(val)
		case string:
			if u, ok := parseUint32Decimal(val); ok {
				return int(u)
			}
		}
	}
	return 0
}

// RowUint16 extracts a port-like field (0–65535) from a map row.
func RowUint16(row map[string]interface{}, key string) (uint16, bool) {
	if v, ok := row[key]; ok && v != nil {
		switch val := v.(type) {
		case uint16:
			return val, true
		case uint32:
			if val <= 65535 {
				return uint16(val), true
			}
			return 0, false
		case int:
			if val >= 0 && val <= 65535 {
				return uint16(val), true
			}
			return 0, false
		case int32:
			if val >= 0 && val <= 65535 {
				return uint16(val), true
			}
			return 0, false
		case int64:
			if val >= 0 && val <= 65535 {
				return uint16(val), true
			}
			return 0, false
		case float64:
			if val >= 0 && val <= 65535 && val == float64(uint16(val)) {
				return uint16(val), true
			}
			return 0, false
		case string:
			return parseUint16Decimal(val)
		}
	}
	return 0, false
}

func parseUint16Decimal(s string) (uint16, bool) {
	u, err := strconv.ParseUint(strings.TrimSpace(s), 10, 16)
	if err != nil {
		return 0, false
	}
	return uint16(u), true
}

func parseUint32Decimal(s string) (uint32, bool) {
	u, err := strconv.ParseUint(strings.TrimSpace(s), 10, 32)
	if err != nil {
		return 0, false
	}
	return uint32(u), true
}

// RowTime parses a timestamp field from a map row.
// Accepts time.Time, string (RFC3339/RFC3339Nano), or numeric (unix seconds/ms/µs/ns).
func RowTime(row map[string]interface{}, key string) time.Time {
	if t, ok := RowTimeOptional(row, key); ok {
		return t
	}
	return time.Now().UTC()
}

// RowTimeOptional parses row[key] into UTC time. Missing or unparseable values return (_, false).
func RowTimeOptional(row map[string]interface{}, key string) (time.Time, bool) {
	v, ok := row[key]
	if !ok || v == nil {
		return time.Time{}, false
	}
	switch val := v.(type) {
	case time.Time:
		return val.UTC(), true
	case string:
		val = strings.TrimSpace(val)
		for _, layout := range []string{
			time.RFC3339Nano,
			time.RFC3339,
			"2006-01-02 15:04:05.999999999",
			"2006-01-02 15:04:05",
		} {
			if t, err := time.Parse(layout, val); err == nil {
				return t.UTC(), true
			}
		}
		return time.Time{}, false
	case float64:
		return unixToTime(int64(val)), true
	case int64:
		return unixToTime(val), true
	}
	return time.Time{}, false
}

func unixToTime(n int64) time.Time {
	switch {
	case n > 1e18:
		return time.Unix(0, n).UTC()
	case n > 1e15:
		return time.Unix(0, n*1000).UTC()
	case n > 1e12:
		return time.Unix(n/1000, (n%1000)*1e6).UTC()
	default:
		return time.Unix(n, 0).UTC()
	}
}

// SIPSearchRowsToPCAP builds a PCAP from search/export rows (same shape as POST …/export/pcap).
// Each row uses timestamp, src_ip, dst_ip, src_port, dst_port, payload; empty payload rows are skipped.
// Default SIP ports 5060 when src_port/dst_port are 0.
func SIPSearchRowsToPCAP(rows []map[string]interface{}) ([]byte, error) {
	pw, err := NewPCAPWriter()
	if err != nil {
		return nil, err
	}
	written := 0
	for _, row := range rows {
		payload := RowStr(row, "payload")
		if payload == "" {
			continue
		}
		ts := RowTime(row, "timestamp")
		srcIP := RowStr(row, "src_ip")
		dstIP := RowStr(row, "dst_ip")
		srcPort, _ := RowUint16(row, "src_port")
		dstPort, _ := RowUint16(row, "dst_port")
		if srcPort == 0 {
			srcPort = 5060
		}
		if dstPort == 0 {
			dstPort = 5060
		}
		if err := pw.WritePacket(ts, srcIP, dstIP, srcPort, dstPort, []byte(payload)); err != nil {
			return nil, err
		}
		written++
	}
	if written == 0 {
		return nil, ErrNoSIPPacketsInRows
	}
	return pw.Bytes(), nil
}

// SIPSearchRowsPacketCount returns how many rows would produce PCAP packets (non-empty payload).
func SIPSearchRowsPacketCount(rows []map[string]interface{}) int {
	n := 0
	for _, row := range rows {
		if RowStr(row, "payload") != "" {
			n++
		}
	}
	return n
}
