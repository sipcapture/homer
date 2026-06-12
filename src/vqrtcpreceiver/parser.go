// Copyright (C) 2026 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package vqrtcpreceiver

import (
	"strconv"
	"strings"
)

// Report holds parsed fields from an application/vq-rtcpxr body.
type Report struct {
	CallID            string
	HasSessionReport  bool
	HasIntervalReport bool
	MosLQ             float64
	MosCQ             float64
	LocalAddr         string
	RemoteAddr        string
	JitterBuffer      string
	PacketLoss        string
	Raw               string
}

// ParseBody parses a VQ-RTCPXR document (line-oriented key: value format).
func ParseBody(body []byte) Report {
	r := Report{Raw: string(body)}
	current := body
	for len(current) > 0 {
		lineEnd := -1
		for i := 0; i < len(current); i++ {
			if current[i] == '\r' && i+1 < len(current) && current[i+1] == '\n' {
				lineEnd = i
				break
			}
			if current[i] == '\n' {
				lineEnd = i
				break
			}
		}
		var line []byte
		if lineEnd < 0 {
			line = current
			current = nil
		} else {
			line = current[:lineEnd]
			current = current[lineEnd:]
			if len(current) > 0 && current[0] == '\r' {
				current = current[1:]
			}
			if len(current) > 0 && current[0] == '\n' {
				current = current[1:]
			}
		}
		if len(line) < 4 {
			continue
		}
		s := string(line)
		switch {
		case strings.HasPrefix(s, "Call-ID:"):
			r.CallID = strings.TrimSpace(s[len("Call-ID:"):])
		case strings.HasPrefix(s, "VQSessionReport:"):
			r.HasSessionReport = true
		case strings.HasPrefix(s, "VQIntervalReport:"):
			r.HasIntervalReport = true
		case strings.HasPrefix(s, "QualityEst:"):
			parseQualityEst(s[len("QualityEst:"):], &r)
		case strings.HasPrefix(s, "LocalAddr:"):
			r.LocalAddr = strings.TrimSpace(s[len("LocalAddr:"):])
		case strings.HasPrefix(s, "RemoteAddr:"):
			r.RemoteAddr = strings.TrimSpace(s[len("RemoteAddr:"):])
		case strings.HasPrefix(s, "JitterBuffer:"):
			r.JitterBuffer = strings.TrimSpace(s[len("JitterBuffer:"):])
		case strings.HasPrefix(s, "PacketLoss:"):
			r.PacketLoss = strings.TrimSpace(s[len("PacketLoss:"):])
		}
	}
	return r
}

func parseQualityEst(val string, r *Report) {
	val = strings.TrimSpace(val)
	for _, part := range strings.Fields(val) {
		k, v, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			continue
		}
		switch strings.ToUpper(k) {
		case "MOSLQ":
			r.MosLQ = f
		case "MOSCQ":
			r.MosCQ = f
		}
	}
}

// EventType returns VQSessionReport or VQIntervalReport label.
func (r Report) EventType() string {
	if r.HasSessionReport {
		return "VQSessionReport"
	}
	if r.HasIntervalReport {
		return "VQIntervalReport"
	}
	return ""
}

func parsePort(s string) (uint16, bool) {
	p, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil || p < 0 || p > 65535 {
		return 0, false
	}
	return uint16(p), true
}

// SplitHostPort parses "IP PORT" or "IP:PORT" style LocalAddr/RemoteAddr.
func SplitHostPort(addr string) (host string, port uint16) {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return "", 0
	}
	if i := strings.LastIndex(addr, ":"); i > 0 && strings.Count(addr, ":") == 1 {
		host = addr[:i]
		if p, ok := parsePort(addr[i+1:]); ok {
			return host, p
		}
	}
	fields := strings.Fields(addr)
	if len(fields) >= 2 {
		if p, ok := parsePort(fields[1]); ok {
			return fields[0], p
		}
	}
	return addr, 0
}
