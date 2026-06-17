// Copyright (C) 2026 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package sipparser

import "testing"

func TestCalleeUser_RuriFallbackTo(t *testing.T) {
	raw := []byte("INVITE sip:gateway.example.com SIP/2.0\r\n" +
		"From: <sip:alice@example.com>\r\n" +
		"To: <sip:bob@example.com>\r\n" +
		"Call-ID: cid-ruri@host\r\n" +
		"CSeq: 1 INVITE\r\n\r\n")
	s := ParseMsgZeroCopy(raw, nil)
	if s.Error != nil {
		t.Fatalf("parse: %v", s.Error)
	}
	if s.URIUser != "" {
		t.Fatalf("URIUser: want empty, got %q", s.URIUser)
	}
	if s.ToUser != "bob" {
		t.Fatalf("ToUser: want bob, got %q", s.ToUser)
	}
	if got := s.CalleeUser(); got != "bob" {
		t.Fatalf("CalleeUser: want bob, got %q", got)
	}
}

func TestCalleeUser_PrefersRuri(t *testing.T) {
	raw := []byte("INVITE sip:ruri@example.com SIP/2.0\r\n" +
		"From: <sip:alice@example.com>\r\n" +
		"To: <sip:to@example.com>\r\n" +
		"Call-ID: cid-pref@host\r\n" +
		"CSeq: 1 INVITE\r\n\r\n")
	s := ParseMsgZeroCopy(raw, nil)
	if s.Error != nil {
		t.Fatalf("parse: %v", s.Error)
	}
	if got := s.CalleeUser(); got != "ruri" {
		t.Fatalf("CalleeUser: want ruri, got %q", got)
	}
}

func TestCalleeUser_ResponseUsesTo(t *testing.T) {
	raw := []byte("SIP/2.0 200 OK\r\n" +
		"From: <sip:alice@example.com>\r\n" +
		"To: <sip:bob@example.com>;tag=abc\r\n" +
		"Call-ID: cid-resp@host\r\n" +
		"CSeq: 1 INVITE\r\n\r\n")
	s := ParseMsgZeroCopy(raw, nil)
	if s.Error != nil {
		t.Fatalf("parse: %v", s.Error)
	}
	if got := s.CalleeUser(); got != "bob" {
		t.Fatalf("CalleeUser: want bob, got %q", got)
	}
}
