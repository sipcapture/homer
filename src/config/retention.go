package config

import (
	"fmt"
	"strings"
	"time"
)

// Retention unit values for CompactionConfig.RetentionUnit. "days" is the
// default and preserves legacy calendar-day semantics byte-for-byte.
const (
	RetentionUnitDays  = "days"
	RetentionUnitHours = "hours"
)

// NormalizeRetentionUnit validates and lower-cases a retention_unit value.
// "" is treated as RetentionUnitDays (the pre-existing default behavior).
func NormalizeRetentionUnit(unit string) (string, error) {
	u := strings.ToLower(strings.TrimSpace(unit))
	if u == "" {
		return RetentionUnitDays, nil
	}
	if u != RetentionUnitDays && u != RetentionUnitHours {
		return "", fmt.Errorf("invalid retention_unit %q: must be %q or %q", unit, RetentionUnitDays, RetentionUnitHours)
	}
	return u, nil
}

// RetentionCutoff returns the timestamp before which rows are eligible for
// deletion, given a raw retention value (as stored in retention_days /
// retention_days_by_table) and its unit. "days" (including "", defensively)
// uses calendar-day arithmetic (time.AddDate) to match the pre-existing
// behavior exactly (DST-correct, not a fixed 24h*N). "hours" uses a fixed
// time.Duration.
func RetentionCutoff(now time.Time, value int, unit string) time.Time {
	if strings.EqualFold(unit, RetentionUnitHours) {
		return now.Add(-time.Duration(value) * time.Hour)
	}
	return now.AddDate(0, 0, -value)
}
