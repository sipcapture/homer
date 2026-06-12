// Copyright (C) 2026 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package vqrtcpreceiver

import "testing"

func TestParseBody(t *testing.T) {
	body := `Call-ID: abc-123@host
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
