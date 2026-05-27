// Copyright (C) 2025 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package pcapwriter

import (
	"math"
	"testing"
)

func TestRowUint16_validAndOverflow(t *testing.T) {
	row := map[string]interface{}{
		"src_port": "5060",
		"bad":      "99999",
	}
	p, ok := RowUint16(row, "src_port")
	if !ok || p != 5060 {
		t.Fatalf("src_port: got %d ok=%v", p, ok)
	}
	_, ok = RowUint16(row, "bad")
	if ok {
		t.Fatal("expected overflow rejection")
	}
}

func TestParseUint16Decimal(t *testing.T) {
	p, ok := parseUint16Decimal("65535")
	if !ok || p != 65535 {
		t.Fatalf("got %d ok=%v", p, ok)
	}
	_, ok = parseUint16Decimal("not-a-port")
	if ok {
		t.Fatal("expected fail")
	}
	_, ok = parseUint16Decimal("65536")
	if ok {
		t.Fatal("expected overflow")
	}
	_ = math.MaxUint16
}
