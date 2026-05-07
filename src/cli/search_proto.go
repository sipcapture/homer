// Copyright (C) 2026 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package cli

import (
	"fmt"
	"strconv"
	"strings"
)

// ParseSearchProto resolves a --proto flag value to coordinator proto_type (int).
// Accepts decimal numbers or aliases (case-insensitive; "-" and spaces become "_").
func ParseSearchProto(s string) (int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty proto")
	}
	if n, err := strconv.Atoi(s); err == nil {
		if n < 0 {
			return 0, fmt.Errorf("proto must be non-negative: %d", n)
		}
		return n, nil
	}
	key := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(s, "-", "_"), " ", "_"))
	switch key {
	case "sip", "hep_sip":
		return 1, nil
	case "rtcp":
		return 5, nil
	case "rtp", "rtp_agent", "rtpagent":
		return 34, nil
	case "dns":
		return 35, nil
	case "log", "logs", "hep_log", "heplogs":
		return 100, nil
	case "otlp_traces", "otlp_trace", "otlptraces":
		return 200, nil
	case "otlp_metrics", "otlp_metric", "otlpmetrics":
		return 201, nil
	case "otlp_logs", "otlp_log", "otlplogs":
		return 202, nil
	case "lp", "line_protocol", "lineprotocol", "line_proto":
		return 300, nil
	default:
		return 0, fmt.Errorf("unknown proto %q: use sip, rtcp, rtp, dns, log, otlp_traces, otlp_metrics, otlp_logs, lp, or a number", s)
	}
}

// FormatSearchProtoDisplay returns a canonical alias for known proto_type values,
// otherwise the decimal string (for TUI / defaults).
func FormatSearchProtoDisplay(p int) string {
	switch p {
	case 1:
		return "sip"
	case 5:
		return "rtcp"
	case 34:
		return "rtp"
	case 35:
		return "dns"
	case 100:
		return "log"
	case 200:
		return "otlp_traces"
	case 201:
		return "otlp_metrics"
	case 202:
		return "otlp_logs"
	case 300:
		return "lp"
	default:
		return strconv.Itoa(p)
	}
}

// searchProtoFlag implements flag.Value for --proto (number or alias).
type searchProtoFlag struct {
	v   int
	set bool
}

func (p *searchProtoFlag) Set(s string) error {
	n, err := ParseSearchProto(s)
	if err != nil {
		return err
	}
	p.v = n
	p.set = true
	return nil
}

func (p *searchProtoFlag) String() string {
	if !p.set {
		return "sip"
	}
	return FormatSearchProtoDisplay(p.v)
}

func (p *searchProtoFlag) Int() int {
	if !p.set {
		return 1
	}
	return p.v
}

func (p *searchProtoFlag) IsSet() bool { return p.set }
