// Copyright (C) 2026 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package decoder

import (
	"testing"
	"time"
)

func TestSanitizeHEPTimestamp(t *testing.T) {
	// Fixed "receive" time matching the issue #909 sample window (2026-08-04).
	now := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	validUnix := uint32(time.Date(2026, 8, 3, 11, 33, 46, 0, time.UTC).Unix()) // 1785756826
	ntpAsUnix := uint32(3994745626)                                           // NTP MSW misread as Unix → 2096
	brokenHeader := uint32(2608623722)                                        // packet 490 tv_sec → 2052
	brokenUsec := uint32(1059783936)                                          // NTP frac in usec field

	tests := []struct {
		name       string
		tsec       uint32
		tmsec      uint32
		wantTsec   uint32
		wantTmsec  uint32
		wantNow    bool // expect receive time
		checkExact bool
	}{
		{
			name:       "normal unix preserved",
			tsec:       validUnix,
			tmsec:      123456,
			wantTsec:   validUnix,
			wantTmsec:  123456,
			checkExact: true,
		},
		{
			name:       "ntp-as-unix converted",
			tsec:       ntpAsUnix,
			tmsec:      0,
			wantTsec:   validUnix,
			wantTmsec:  0,
			checkExact: true,
		},
		{
			name:    "broken header falls back to receive time",
			tsec:    brokenHeader,
			tmsec:   brokenUsec,
			wantNow: true,
		},
		{
			name:    "zero timestamp uses receive time and syncs Tsec",
			tsec:    0,
			tmsec:   0,
			wantNow: true,
		},
		{
			name:       "valid usec under 1e6 preserved",
			tsec:       validUnix,
			tmsec:      999999,
			wantTsec:   validUnix,
			wantTmsec:  999999,
			checkExact: true,
		},
		{
			name:    "invalid usec clamped then out-of-range falls back",
			tsec:    brokenHeader,
			tmsec:   brokenUsec,
			wantNow: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &HEP{Tsec: tt.tsec, Tmsec: tt.tmsec, NodeID: 2001}
			h.sanitizeHEPTimestamp(now)

			if tt.wantNow {
				if h.Timestamp.UTC() != now {
					t.Fatalf("Timestamp = %v, want receive time %v", h.Timestamp, now)
				}
				if h.Tsec != uint32(now.Unix()) {
					t.Fatalf("Tsec = %d, want %d (synced receive time)", h.Tsec, now.Unix())
				}
				if h.Tmsec != uint32(now.Nanosecond()/1000) {
					t.Fatalf("Tmsec = %d, want %d", h.Tmsec, now.Nanosecond()/1000)
				}
				return
			}

			if !tt.checkExact {
				return
			}
			if h.Tsec != tt.wantTsec {
				t.Fatalf("Tsec = %d, want %d", h.Tsec, tt.wantTsec)
			}
			if h.Tmsec != tt.wantTmsec {
				t.Fatalf("Tmsec = %d, want %d", h.Tmsec, tt.wantTmsec)
			}
			if h.Timestamp.Unix() != int64(tt.wantTsec) {
				t.Fatalf("Timestamp.Unix() = %d, want %d", h.Timestamp.Unix(), tt.wantTsec)
			}
		})
	}
}

func TestHepTimestampInWindow(t *testing.T) {
	now := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)

	if !hepTimestampInWindow(now, now) {
		t.Fatal("now should be in window")
	}
	if !hepTimestampInWindow(now.Add(23*time.Hour), now) {
		t.Fatal("now+23h should be in window")
	}
	if hepTimestampInWindow(now.Add(25*time.Hour), now) {
		t.Fatal("now+25h should be out of window")
	}
	if hepTimestampInWindow(now.Add(-11*365*24*time.Hour), now) {
		t.Fatal("now-11y should be out of window")
	}
	if !hepTimestampInWindow(now.Add(-9*365*24*time.Hour), now) {
		t.Fatal("now-9y should be in window")
	}
}
