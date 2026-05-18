// Copyright (C) 2026 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package decoder

import "testing"

func TestIPv4BytesToString(t *testing.T) {
	got := ipv4BytesToString([]byte{10, 0, 0, 1})
	if got != "10.0.0.1" {
		t.Fatalf("got %q want 10.0.0.1", got)
	}
}
