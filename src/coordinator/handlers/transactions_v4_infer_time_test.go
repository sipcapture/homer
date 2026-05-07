// Copyright (C) 2025 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package handlers

import "testing"

func TestInferTimeRange_lastTwoHoursEnglishWords(t *testing.T) {
	now := int64(1_700_000_000_000)
	from, to := inferTimeRange("find all INVITE and BYE from last two hours", now)
	if to != now {
		t.Fatalf("to: got %d want %d", to, now)
	}
	wantFrom := now - 2*60*60*1000
	if from != wantFrom {
		t.Fatalf("from: got %d want %d (2h window)", from, wantFrom)
	}
}
