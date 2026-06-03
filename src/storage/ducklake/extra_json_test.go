// Copyright (C) 2026 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package ducklake

import (
	"testing"

	"github.com/sipcapture/homer-core/src/decoder"
)

func TestBuildExtraJSONCell_PooledBuffer(t *testing.T) {
	pkt := decoder.BenchHEP3SIPPacket()
	hep, err := decoder.DecodeHEP(pkt)
	if err != nil {
		t.Fatal(err)
	}
	defer decoder.ReleaseHEP(hep)

	cell := buildExtraJSONCell(hep)
	bp, ok := cell.(*[]byte)
	if !ok {
		t.Fatalf("expected *[]byte, got %T", cell)
	}
	if len(*bp) == 0 || (*bp)[0] != '{' {
		t.Fatalf("unexpected json: %q", *bp)
	}
	releaseExtraJSONCell(cell)
}

func TestCellToDriverValue_pooledJSONIsString(t *testing.T) {
	b := []byte(`{"version":3}`)
	cell := any(&b)
	dv := cellToDriverValue(cell)
	s, ok := dv.(string)
	if !ok {
		t.Fatalf("expected string, got %T", dv)
	}
	if s != string(b) {
		t.Fatalf("got %q", s)
	}
}

func TestCachedSIPVersionOnlyJSON(t *testing.T) {
	a := cachedSIPVersionOnlyJSON(7)
	b := cachedSIPVersionOnlyJSON(7)
	if a != b {
		t.Fatalf("cache miss: %q vs %q", a, b)
	}
	if a != `{"version":7}` {
		t.Fatalf("got %q", a)
	}
}
