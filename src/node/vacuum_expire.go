// Copyright (C) 2026 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package node

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// expireOlderThanPattern matches the documented /vacuum query form
// ("1 hour", "7 days"). The count is rendered as integer seconds so the
// value cannot close the SQL string (GHSA-q7c2-m23h-43g9).
var expireOlderThanPattern = regexp.MustCompile(`(?i)^(\d+)\s+(second|minute|hour|day|week|month|year)s?$`)

func parseExpireOlderThan(raw string) (label string, seconds int64, err error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "1 hour", 3600, nil
	}

	if d, perr := time.ParseDuration(raw); perr == nil {
		sec := int64(d.Seconds())
		if sec < 1 {
			return "", 0, fmt.Errorf("expire_older_than must be at least 1 second")
		}
		return raw, sec, nil
	}

	m := expireOlderThanPattern.FindStringSubmatch(raw)
	if m == nil {
		return "", 0, fmt.Errorf("invalid expire_older_than %q (use a duration such as \"1 hour\" or \"7 days\")", raw)
	}
	n, nerr := strconv.ParseInt(m[1], 10, 64)
	if nerr != nil || n < 1 {
		return "", 0, fmt.Errorf("invalid expire_older_than %q (use a duration such as \"1 hour\" or \"7 days\")", raw)
	}
	var mult int64
	switch strings.ToLower(m[2]) {
	case "second":
		mult = 1
	case "minute":
		mult = 60
	case "hour":
		mult = 3600
	case "day":
		mult = 86400
	case "week":
		mult = 7 * 86400
	case "month":
		mult = 30 * 86400
	case "year":
		mult = 365 * 86400
	default:
		return "", 0, fmt.Errorf("invalid expire_older_than %q (use a duration such as \"1 hour\" or \"7 days\")", raw)
	}
	if n > math.MaxInt64/mult {
		return "", 0, fmt.Errorf("expire_older_than is too large")
	}
	return raw, n * mult, nil
}

// expireSnapshotsSQL builds the DuckLake snapshot-expiry call. seconds is an
// integer, so it cannot break out of the INTERVAL literal.
func expireSnapshotsSQL(lakeName string, seconds int64) string {
	return fmt.Sprintf(
		"CALL ducklake_expire_snapshots('%s', older_than => CAST(NOW() - INTERVAL '%d seconds' AS TIMESTAMPTZ))",
		strings.ReplaceAll(lakeName, "'", "''"),
		seconds,
	)
}
