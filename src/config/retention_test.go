package config

import (
	"testing"
	"time"
)

func TestNormalizeRetentionUnit(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{"empty defaults to days", "", RetentionUnitDays, false},
		{"days", "days", RetentionUnitDays, false},
		{"Days mixed case", "Days", RetentionUnitDays, false},
		{"days with whitespace", "  DAYS  ", RetentionUnitDays, false},
		{"hours", "hours", RetentionUnitHours, false},
		{"Hours mixed case", "Hours", RetentionUnitHours, false},
		{"invalid unit", "weeks", "", true},
		{"invalid abbreviation", "d", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeRetentionUnit(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("NormalizeRetentionUnit(%q) = %q, want error", tt.input, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("NormalizeRetentionUnit(%q) unexpected error: %v", tt.input, err)
			}
			if got != tt.want {
				t.Fatalf("NormalizeRetentionUnit(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestRetentionCutoff(t *testing.T) {
	now := time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC)

	daysCutoff := RetentionCutoff(now, 30, RetentionUnitDays)
	wantDays := now.AddDate(0, 0, -30)
	if !daysCutoff.Equal(wantDays) {
		t.Fatalf("RetentionCutoff(days) = %v, want %v", daysCutoff, wantDays)
	}

	hoursCutoff := RetentionCutoff(now, 24, RetentionUnitHours)
	wantHours := now.Add(-24 * time.Hour)
	if !hoursCutoff.Equal(wantHours) {
		t.Fatalf("RetentionCutoff(hours) = %v, want %v", hoursCutoff, wantHours)
	}

	defaultCutoff := RetentionCutoff(now, 30, "")
	if !defaultCutoff.Equal(wantDays) {
		t.Fatalf("RetentionCutoff(\"\") = %v, want %v (days default)", defaultCutoff, wantDays)
	}
}
