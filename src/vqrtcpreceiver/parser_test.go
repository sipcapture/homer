// Copyright (C) 2026 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package vqrtcpreceiver

import "testing"

func TestParseBody(t *testing.T) {
	body := `CallID: abc-123@host
VQIntervalReport: 
QualityEst:MOSLQ=4.20 MOSCQ=4.10
LocalAddr: 10.0.0.1 5060
RemoteAddr: 10.0.0.2 5070
JitterBuffer:JBA=0 JBR=0 JBN=10 JBM=0 JBX=0
PacketLoss:NLR=0.5 JDR=0.1
`
	r := ParseBody([]byte(body))
	if r.CallID != "abc-123@host" {
		t.Fatalf("callid: got %q", r.CallID)
	}
	if !r.HasIntervalReport {
		t.Fatal("expected interval report")
	}
	if r.MosLQ != 4.20 {
		t.Fatalf("moslq: got %v", r.MosLQ)
	}
	if r.MosCQ != 4.10 {
		t.Fatalf("moscq: got %v", r.MosCQ)
	}
	host, port := SplitHostPort(r.LocalAddr)
	if host != "10.0.0.1" || port != 5060 {
		t.Fatalf("local: %s %d", host, port)
	}
}

func TestParseBodyRFC6035(t *testing.T) {
	body := `CallID: 20910623@10.10.1.100
VQSessionReport:
QualityEst:RCQ=90 MOSLQ=4.2 MOSCQ=4.3
LocalAddr: IP=10.10.1.100 PORT=5000 SSRC=1a3b5c7d
RemoteAddr:IP=11.1.1.150 PORT=5002 SSRC=0x2468abcd
`
	r := ParseBody([]byte(body))
	if r.CallID != "20910623@10.10.1.100" {
		t.Fatalf("callid: got %q", r.CallID)
	}
	if !r.HasSessionReport {
		t.Fatal("expected session report")
	}
	host, port := SplitHostPort(r.LocalAddr)
	if host != "10.10.1.100" || port != 5000 {
		t.Fatalf("local: %s %d", host, port)
	}
	host, port = SplitHostPort(r.RemoteAddr)
	if host != "11.1.1.150" || port != 5002 {
		t.Fatalf("remote: %s %d", host, port)
	}
}

func TestParseBodyIgnoresSIPCallIDHeader(t *testing.T) {
	body := `Call-ID: publish-message-id@sbc
CallID: dialog-call-id@host
VQIntervalReport:
`
	r := ParseBody([]byte(body))
	if r.CallID != "dialog-call-id@host" {
		t.Fatalf("callid: got %q, want dialog CallID from RFC 6035 body", r.CallID)
	}
}

func TestSplitHostPort(t *testing.T) {
	tests := []struct {
		addr     string
		wantHost string
		wantPort uint16
	}{
		{"10.0.0.1 5060", "10.0.0.1", 5060},
		{"10.0.0.1:5070", "10.0.0.1", 5070},
		{"10.0.0.1 70000", "10.0.0.1 70000", 0},
		{"10.0.0.1:-1", "10.0.0.1:-1", 0},
		{"", "", 0},
		{"IP=10.10.1.100 PORT=5000 SSRC=1a3b5c7d", "10.10.1.100", 5000},
		{"IP=11.1.1.150 PORT=5002 SSRC=0x2468abcd", "11.1.1.150", 5002},
		{"IP=2001:db8::1 PORT=5000 SSRC=0x2468abcd", "2001:db8::1", 5000},
		{"IP=[2001:db8::1] PORT=5000 SSRC=0x2468abcd", "2001:db8::1", 5000},
		{"IP=10.10.1.100", "10.10.1.100", 0},
		{"ip=10.10.1.100 port=5000 ssrc=1a3b5c7d", "10.10.1.100", 5000},
	}
	for _, tt := range tests {
		host, port := SplitHostPort(tt.addr)
		if host != tt.wantHost || port != tt.wantPort {
			t.Errorf("SplitHostPort(%q) = (%q, %d), want (%q, %d)", tt.addr, host, port, tt.wantHost, tt.wantPort)
		}
	}
}
