// Copyright (C) 2026 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package sipparser

import (
	"strings"
	"testing"
)

// minimalSIP returns a minimal valid INVITE with extra header lines after CSeq.
func minimalSIP(extraLines string) []byte {
	const core = "INVITE sip:a@b SIP/2.0\r\n" +
		"Via: SIP/2.0/UDP 10.0.0.1;branch=z9hG4bK1\r\n" +
		"From: <sip:a@b>;tag=ft\r\n" +
		"To: <sip:c@d>\r\n" +
		"Call-ID: cid-one@host\r\n" +
		"CSeq: 1 INVITE\r\n"
	return []byte(core + extraLines + "\r\n")
}

func TestParseMsgZeroCopy_AlegIDs(t *testing.T) {
	raw := minimalSIP("X-Custom-Call: correl-xyz")
	opts := &ZeroCopyOpts{AlegIDs: []string{"X-Custom-Call"}}
	s := ParseMsgZeroCopy(raw, opts)
	if s.Error != nil {
		t.Fatalf("parse: %v", s.Error)
	}
	if s.XCallID != "correl-xyz" {
		t.Fatalf("XCallID: want correl-xyz, got %q", s.XCallID)
	}
	if s.CallID != "cid-one@host" {
		t.Fatalf("CallID: %q", s.CallID)
	}
}

func TestParseMsgZeroCopy_AlegIDsCaseInsensitive(t *testing.T) {
	raw := minimalSIP("x-custom-call: lower-val")
	opts := &ZeroCopyOpts{AlegIDs: []string{"X-Custom-Call"}}
	s := ParseMsgZeroCopy(raw, opts)
	if s.Error != nil {
		t.Fatalf("parse: %v", s.Error)
	}
	if s.XCallID != "lower-val" {
		t.Fatalf("XCallID: want lower-val, got %q", s.XCallID)
	}
}

func TestParseMsgZeroCopy_AlegIDsFirstWireHeaderWins(t *testing.T) {
	raw := minimalSIP("X-A: first\r\nX-B: second")
	opts := &ZeroCopyOpts{AlegIDs: []string{"X-B", "X-A"}}
	s := ParseMsgZeroCopy(raw, opts)
	if s.Error != nil {
		t.Fatalf("parse: %v", s.Error)
	}
	if s.XCallID != "first" {
		t.Fatalf("XCallID: want first (first matching line in message), got %q", s.XCallID)
	}
}

func TestParseMsgZeroCopy_CustomHeaders(t *testing.T) {
	raw := minimalSIP("X-Extra: hello")
	opts := &ZeroCopyOpts{CustomHeaders: []string{"X-Extra"}}
	s := ParseMsgZeroCopy(raw, opts)
	if s.Error != nil {
		t.Fatalf("parse: %v", s.Error)
	}
	if s.CustomHeader == nil || s.CustomHeader["X-Extra"] != "hello" {
		t.Fatalf("CustomHeader: %#v", s.CustomHeader)
	}
}

func TestParseMsgZeroCopy_CustomHeadersNonX(t *testing.T) {
	raw := minimalSIP("P-Charging-Vector: icid=abc")
	opts := &ZeroCopyOpts{CustomHeaders: []string{"P-Charging-Vector"}}
	s := ParseMsgZeroCopy(raw, opts)
	if s.Error != nil {
		t.Fatalf("parse: %v", s.Error)
	}
	if s.CustomHeader == nil || !strings.Contains(s.CustomHeader["P-Charging-Vector"], "icid=abc") {
		t.Fatalf("CustomHeader: %#v", s.CustomHeader)
	}
}

func TestParseMsgZeroCopy_NilOptsIgnoresExtraHeaders(t *testing.T) {
	raw := minimalSIP("X-Orphan: ignored")
	s := ParseMsgZeroCopy(raw, nil)
	if s.Error != nil {
		t.Fatalf("parse: %v", s.Error)
	}
	if s.XCallID != "" {
		t.Fatalf("XCallID: want empty, got %q", s.XCallID)
	}
	if len(s.CustomHeader) != 0 {
		t.Fatalf("CustomHeader: want empty, got %#v", s.CustomHeader)
	}
}

func TestParseMsgZeroCopyLegacy(t *testing.T) {
	raw := minimalSIP("X-Orphan: x")
	s := ParseMsgZeroCopyLegacy(raw)
	if s.Error != nil {
		t.Fatalf("parse: %v", s.Error)
	}
	if s.XCallID != "" {
		t.Fatalf("legacy wrapper: unexpected XCallID %q", s.XCallID)
	}
}

func TestParseMsgZeroCopy_TelToUser(t *testing.T) {
	raw := []byte("INVITE tel:+15551234567 SIP/2.0\r\n" +
		"From: <sip:alice@example.com>\r\n" +
		"To: <tel:+15551234567>\r\n" +
		"Call-ID: cid-tel@host\r\n" +
		"CSeq: 1 INVITE\r\n\r\n")
	s := ParseMsgZeroCopy(raw, nil)
	if s.Error != nil {
		t.Fatalf("parse: %v", s.Error)
	}
	if s.ToUser != "+15551234567" {
		t.Fatalf("ToUser: want +15551234567, got %q", s.ToUser)
	}
	legacy := ParseMsg(string(raw), nil, nil)
	if legacy.ToUser != s.ToUser {
		t.Fatalf("legacy ToUser %q != zero-copy ToUser %q", legacy.ToUser, s.ToUser)
	}
}

func TestParseMsgZeroCopy_XRTPStatPlusCustom(t *testing.T) {
	raw := minimalSIP("X-RTP-Stat: stats\r\nX-Extra: e")
	opts := &ZeroCopyOpts{CustomHeaders: []string{"X-Extra"}}
	s := ParseMsgZeroCopy(raw, opts)
	if s.Error != nil {
		t.Fatalf("parse: %v", s.Error)
	}
	if s.RTPStatVal != "stats" {
		t.Fatalf("RTPStatVal: %q", s.RTPStatVal)
	}
	if s.CustomHeader == nil || s.CustomHeader["X-Extra"] != "e" {
		t.Fatalf("CustomHeader: %#v", s.CustomHeader)
	}
}
