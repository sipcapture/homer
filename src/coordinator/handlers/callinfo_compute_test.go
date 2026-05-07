// Copyright (C) 2025 Homer Server Contributors

package handlers

import (
	"testing"
	"time"
)

func TestComputeSIPCallLeg_basicFlow(t *testing.T) {
	t0 := time.Date(2025, 3, 1, 12, 0, 0, 0, time.UTC)
	ms := func(d time.Duration) string {
		return t0.Add(d).UTC().Format(time.RFC3339Nano)
	}
	sdp := "Content-Type: application/sdp\n\nm=audio 49170 RTP/AVP 0 8\na=rtpmap:0 PCMU/8000\na=rtpmap:8 PCMA/8000\n"

	rows := []map[string]interface{}{
		{"session_id": "cid-1", "timestamp": ms(0), "method": "INVITE", "response_code": "", "cseq_method": "INVITE", "caller": "alice", "callee": "bob", "src_ip": "10.0.0.1", "src_port": "5060", "dst_ip": "10.0.0.2", "dst_port": "5060", "payload": "INVITE sip:bob\r\nUser-Agent: UAC-Test\r\n\r\n" + sdp, "data_extra": `{"user_agent":"UAC-Extra","from_host":"alice.net","to_host":"bob.net","request_uri":"sip:bob@bob.net"}`, "uuid": "1"},
		{"session_id": "cid-1", "timestamp": ms(2 * time.Second), "method": "180", "response_code": "180", "cseq_method": "INVITE", "payload": "SIP/2.0 180 Ringing\r\n", "uuid": "2"},
		{"session_id": "cid-1", "timestamp": ms(5 * time.Second), "method": "200", "response_code": "200", "cseq_method": "INVITE", "payload": "SIP/2.0 200 OK\r\nServer: UAS-Test\r\n\r\n" + sdp, "uuid": "3"},
		{"session_id": "cid-1", "timestamp": ms(65 * time.Second), "method": "BYE", "response_code": "", "cseq_method": "BYE", "payload": "BYE sip:...\r\n", "uuid": "4"},
	}

	out := computeSIPCallInfoRows(append([]map[string]interface{}{}, rows...))
	if len(out) != 1 {
		t.Fatalf("expected 1 leg, got %d", len(out))
	}
	m := out[0]
	if m["session_id"] != "cid-1" {
		t.Fatalf("session_id: %v", m["session_id"])
	}
	if m["ringing_seconds"] != 3.0 {
		t.Fatalf("ringing_seconds want 3, got %v", m["ringing_seconds"])
	}
	if m["call_duration_seconds"] != 60.0 {
		t.Fatalf("call_duration_seconds want 60, got %v", m["call_duration_seconds"])
	}
	if got := m["codecs"]; got != "PCMU, PCMA" {
		t.Fatalf("codecs want PCMU, PCMA, got %v", got)
	}
	if m["from_party"] != "alice@alice.net" {
		t.Fatalf("from_party: %v", m["from_party"])
	}
	if m["ruri_party"] != "sip:bob@bob.net" {
		t.Fatalf("ruri_party: %v", m["ruri_party"])
	}
	dist, ok := m["methods_distribution"].([]map[string]interface{})
	if !ok || len(dist) < 4 {
		t.Fatalf("methods_distribution: %#v", m["methods_distribution"])
	}
}

func TestComputeSIPCallLeg_sortMultiSession(t *testing.T) {
	t0 := time.Date(2025, 3, 1, 12, 0, 0, 0, time.UTC)
	ms := func(d time.Duration) string { return t0.Add(d).UTC().Format(time.RFC3339Nano) }
	rows := []map[string]interface{}{
		{"session_id": "b", "timestamp": ms(0), "method": "INVITE", "response_code": "", "cseq_method": "INVITE", "uuid": "1"},
		{"session_id": "a", "timestamp": ms(0), "method": "INVITE", "response_code": "", "cseq_method": "INVITE", "uuid": "2"},
	}
	out := computeSIPCallInfoRows(rows)
	if len(out) != 2 {
		t.Fatalf("legs: %d", len(out))
	}
	if out[0]["session_id"] != "a" || out[1]["session_id"] != "b" {
		t.Fatalf("order: %#v, %#v", out[0]["session_id"], out[1]["session_id"])
	}
}
