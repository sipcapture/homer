// Copyright (C) 2025 Homer Server Contributors

package handlers

import (
	"strings"
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
	if got := m["codecs_offered"]; got != "PCMU, PCMA" {
		t.Fatalf("codecs_offered want PCMU, PCMA, got %v", got)
	}
	if got := m["codec_negotiated"]; got != "PCMU, PCMA" {
		t.Fatalf("codec_negotiated want PCMU, PCMA, got %v", got)
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

func TestExtractCodecsFromSDP_staticAndDynamic(t *testing.T) {
	payload := strings.Join([]string{
		"INVITE sip:bob@example SIP/2.0",
		"CSeq: 1 INVITE",
		"Content-Type: application/sdp",
		"",
		"v=0",
		"m=audio 4000 RTP/AVP 0 8 18 101 96",
		"a=rtpmap:101 telephone-event/8000",
		"a=rtpmap:96 opus/48000/2",
		"m=video 0 RTP/AVP 31",
		"m=video 4002 RTP/AVP 34",
	}, "\r\n")
	got := extractCodecsFromSDP(payload)
	want := "PCMU, PCMA, G729, telephone-event, opus, H263"
	if got != want {
		t.Fatalf("codecs got %q want %q", got, want)
	}
}

func TestExtractCodecsFromSDP_rtpmapOverridesStatic(t *testing.T) {
	payload := "m=audio 4000 RTP/AVP 18 97\r\nA=rtpmap:18 G729a/8000\r\n"
	got := extractCodecsFromSDP(payload)
	if got != "G729a" {
		t.Fatalf("got %q", got)
	}
}

func TestExtractCodecsFromSDP_noMedia(t *testing.T) {
	if got := extractCodecsFromSDP("INVITE sip:a SIP/2.0\r\nCSeq: 1 INVITE\r\n"); got != "" {
		t.Fatalf("got %q", got)
	}
}

func TestComputeSIPCallLeg_answerNarrowsCodecs(t *testing.T) {
	t0 := time.Date(2025, 3, 1, 12, 0, 0, 0, time.UTC)
	ms := func(d time.Duration) string { return t0.Add(d).UTC().Format(time.RFC3339Nano) }
	offer := "v=0\r\nm=audio 4000 RTP/AVP 0 8 18 101 96\r\na=rtpmap:101 telephone-event/8000\r\na=rtpmap:96 opus/48000/2\r\n"
	early := "v=0\r\nm=audio 4000 RTP/AVP 0 101\r\na=rtpmap:101 telephone-event/8000\r\n"
	answer := "v=0\r\nm=audio 4002 RTP/AVP 8 101\r\na=rtpmap:101 telephone-event/8000\r\nm=video 0 RTP/AVP 31\r\n"
	bogusAck := "m=audio 4000 RTP/AVP 4\r\n"

	rows := []map[string]interface{}{
		{"session_id": "cid", "timestamp": ms(0), "method": "INVITE", "response_code": "", "cseq_method": "INVITE", "payload": "INVITE sip:b SIP/2.0\r\nCSeq: 1 INVITE\r\n\r\n" + offer, "uuid": "1"},
		{"session_id": "cid", "timestamp": ms(time.Second), "method": "183", "response_code": "183", "cseq_method": "INVITE", "payload": "SIP/2.0 183 Session Progress\r\nCSeq: 1 INVITE\r\n\r\n" + early, "uuid": "2"},
		{"session_id": "cid", "timestamp": ms(2 * time.Second), "method": "200", "response_code": "200", "cseq_method": "INVITE", "payload": "SIP/2.0 200 OK\r\nCSeq: 1 INVITE\r\n\r\n" + answer, "uuid": "3"},
		// Retransmission must not reopen the transaction as a delayed offer,
		// otherwise the ACK body below would replace the negotiated codecs.
		{"session_id": "cid", "timestamp": ms(3 * time.Second), "method": "200", "response_code": "200", "cseq_method": "INVITE", "payload": "SIP/2.0 200 OK\r\nCSeq: 1 INVITE\r\n\r\n" + offer, "uuid": "4"},
		{"session_id": "cid", "timestamp": ms(4 * time.Second), "method": "ACK", "response_code": "", "cseq_method": "ACK", "payload": "ACK sip:b SIP/2.0\r\nCSeq: 1 ACK\r\n\r\n" + bogusAck, "uuid": "5"},
	}
	out := computeSIPCallInfoRows(rows)
	if len(out) != 1 {
		t.Fatalf("legs: %d", len(out))
	}
	m := out[0]
	if got, want := m["codecs_offered"], "PCMU, PCMA, G729, telephone-event, opus"; got != want {
		t.Fatalf("codecs_offered got %v want %s", got, want)
	}
	if got, want := m["codec_negotiated"], "PCMA, telephone-event"; got != want {
		t.Fatalf("codec_negotiated got %v want %s", got, want)
	}
	if m["codecs"] != m["codec_negotiated"] {
		t.Fatalf("codecs should follow the answer, got %v", m["codecs"])
	}
}

func TestComputeSIPCallLeg_reinviteReplacesNegotiated(t *testing.T) {
	t0 := time.Date(2025, 3, 1, 12, 0, 0, 0, time.UTC)
	ms := func(d time.Duration) string { return t0.Add(d).UTC().Format(time.RFC3339Nano) }
	offer := "m=audio 4000 RTP/AVP 0 8 18\r\n"
	answer := "m=audio 4000 RTP/AVP 8\r\n"
	reanswer := "m=audio 4002 RTP/AVP 18\r\n"
	rows := []map[string]interface{}{
		{"session_id": "cid", "timestamp": ms(0), "method": "INVITE", "cseq_method": "INVITE", "payload": "INVITE sip:b SIP/2.0\r\nCSeq: 1 INVITE\r\n\r\n" + offer, "uuid": "1"},
		{"session_id": "cid", "timestamp": ms(time.Second), "method": "200", "response_code": "200", "cseq_method": "INVITE", "payload": "SIP/2.0 200 OK\r\nCSeq: 1 INVITE\r\n\r\n" + answer, "uuid": "2"},
		{"session_id": "cid", "timestamp": ms(2 * time.Second), "method": "INVITE", "cseq_method": "INVITE", "payload": "INVITE sip:b SIP/2.0\r\nCSeq: 2 INVITE\r\n\r\n" + offer, "uuid": "3"},
		{"session_id": "cid", "timestamp": ms(3 * time.Second), "method": "200", "response_code": "200", "cseq_method": "INVITE", "payload": "SIP/2.0 200 OK\r\nCSeq: 2 INVITE\r\n\r\n" + reanswer, "uuid": "4"},
	}
	m := computeSIPCallInfoRows(rows)[0]
	if m["codecs_offered"] != "PCMU, PCMA, G729" {
		t.Fatalf("offered %v", m["codecs_offered"])
	}
	if m["codec_negotiated"] != "G729" || m["codecs"] != "G729" {
		t.Fatalf("negotiated %v codecs %v", m["codec_negotiated"], m["codecs"])
	}
}

func TestComputeSIPCallLeg_cseqOnlyOnOneSide(t *testing.T) {
	t0 := time.Date(2025, 3, 1, 12, 0, 0, 0, time.UTC)
	ms := func(d time.Duration) string { return t0.Add(d).UTC().Format(time.RFC3339Nano) }
	offer := "m=audio 4000 RTP/AVP 0 8 18\r\n"
	answer := "m=audio 4000 RTP/AVP 8\r\n"
	rows := []map[string]interface{}{
		{"session_id": "no-req", "timestamp": ms(0), "method": "INVITE", "cseq_method": "INVITE", "payload": "INVITE sip:b SIP/2.0\r\n\r\n" + offer, "uuid": "1"},
		{"session_id": "no-req", "timestamp": ms(time.Second), "method": "200", "response_code": "200", "cseq_method": "INVITE", "payload": "SIP/2.0 200 OK\r\nCSeq: 1 INVITE\r\n\r\n" + answer, "uuid": "2"},
		{"session_id": "no-res", "timestamp": ms(0), "method": "INVITE", "cseq_method": "INVITE", "payload": "INVITE sip:b SIP/2.0\r\nCSeq: 7 INVITE\r\n\r\n" + offer, "uuid": "3"},
		{"session_id": "no-res", "timestamp": ms(time.Second), "method": "200", "response_code": "200", "cseq_method": "INVITE", "payload": "SIP/2.0 200 OK\r\n\r\n" + answer, "uuid": "4"},
	}
	out := computeSIPCallInfoRows(rows)
	if len(out) != 2 {
		t.Fatalf("legs: %d", len(out))
	}
	for _, m := range out {
		if m["codecs_offered"] != "PCMU, PCMA, G729" || m["codec_negotiated"] != "PCMA" || m["codecs"] != "PCMA" {
			t.Fatalf("session %v offered %v negotiated %v codecs %v", m["session_id"], m["codecs_offered"], m["codec_negotiated"], m["codecs"])
		}
	}
}

func TestComputeSIPCallLeg_differentCSeqIsNotAnAnswer(t *testing.T) {
	t0 := time.Date(2025, 3, 1, 12, 0, 0, 0, time.UTC)
	ms := func(d time.Duration) string { return t0.Add(d).UTC().Format(time.RFC3339Nano) }
	rows := []map[string]interface{}{
		{"session_id": "cid", "timestamp": ms(0), "method": "INVITE", "cseq_method": "INVITE", "payload": "INVITE sip:b SIP/2.0\r\nCSeq: 1 INVITE\r\n\r\nm=audio 4000 RTP/AVP 0 8\r\n", "uuid": "1"},
		{"session_id": "cid", "timestamp": ms(time.Second), "method": "INVITE", "cseq_method": "INVITE", "payload": "INVITE sip:b SIP/2.0\r\nCSeq: 2 INVITE\r\n", "uuid": "2"},
		{"session_id": "cid", "timestamp": ms(2 * time.Second), "method": "200", "response_code": "200", "cseq_method": "INVITE", "payload": "SIP/2.0 200 OK\r\nCSeq: 2 INVITE\r\n\r\nm=audio 4000 RTP/AVP 8\r\n", "uuid": "3"},
		{"session_id": "cid", "timestamp": ms(3 * time.Second), "method": "ACK", "cseq_method": "ACK", "payload": "ACK sip:b SIP/2.0\r\nCSeq: 2 ACK\r\n\r\nm=audio 4000 RTP/AVP 18\r\n", "uuid": "4"},
	}
	m := computeSIPCallInfoRows(rows)[0]
	if m["codecs_offered"] != "PCMU, PCMA" {
		t.Fatalf("offered %v", m["codecs_offered"])
	}
	if m["codec_negotiated"] != "G729" || m["codecs"] != "G729" {
		t.Fatalf("negotiated %v codecs %v", m["codec_negotiated"], m["codecs"])
	}
}

func TestComputeSIPCallLeg_updateAnswer(t *testing.T) {
	t0 := time.Date(2025, 3, 1, 12, 0, 0, 0, time.UTC)
	ms := func(d time.Duration) string { return t0.Add(d).UTC().Format(time.RFC3339Nano) }
	offer := "m=audio 4000 RTP/AVP 0 8\r\n"
	answer := "m=audio 4000 RTP/AVP 8\r\n"
	updateAnswer := "m=audio 4000 RTP/AVP 0\r\n"
	rows := []map[string]interface{}{
		{"session_id": "cid", "timestamp": ms(0), "method": "INVITE", "cseq_method": "INVITE", "payload": "INVITE sip:b SIP/2.0\r\nCSeq: 1 INVITE\r\n\r\n" + offer, "uuid": "1"},
		{"session_id": "cid", "timestamp": ms(time.Second), "method": "200", "response_code": "200", "cseq_method": "INVITE", "payload": "SIP/2.0 200 OK\r\nCSeq: 1 INVITE\r\n\r\n" + answer, "uuid": "2"},
		{"session_id": "cid", "timestamp": ms(2 * time.Second), "method": "UPDATE", "cseq_method": "UPDATE", "payload": "UPDATE sip:b SIP/2.0\r\nCSeq: 2 UPDATE\r\n\r\n" + offer, "uuid": "3"},
		{"session_id": "cid", "timestamp": ms(3 * time.Second), "method": "200", "response_code": "200", "cseq_method": "UPDATE", "payload": "SIP/2.0 200 OK\r\nCSeq: 2 UPDATE\r\n\r\n" + updateAnswer, "uuid": "4"},
	}
	m := computeSIPCallInfoRows(rows)[0]
	if m["codecs_offered"] != "PCMU, PCMA" {
		t.Fatalf("offered %v", m["codecs_offered"])
	}
	if m["codec_negotiated"] != "PCMU" || m["codecs"] != "PCMU" {
		t.Fatalf("negotiated %v codecs %v", m["codec_negotiated"], m["codecs"])
	}
}

func TestComputeSIPCallLeg_delayedOffer(t *testing.T) {
	t0 := time.Date(2025, 3, 1, 12, 0, 0, 0, time.UTC)
	ms := func(d time.Duration) string { return t0.Add(d).UTC().Format(time.RFC3339Nano) }
	offer := "m=audio 4000 RTP/AVP 0 8 18\r\n"
	answer := "m=audio 4000 RTP/AVP 0\r\n"
	rows := []map[string]interface{}{
		{"session_id": "cid", "timestamp": ms(0), "method": "INVITE", "cseq_method": "INVITE", "payload": "INVITE sip:b SIP/2.0\r\nCSeq: 1 INVITE\r\n", "uuid": "1"},
		{"session_id": "cid", "timestamp": ms(time.Second), "method": "200", "response_code": "200", "cseq_method": "INVITE", "payload": "SIP/2.0 200 OK\r\nCSeq: 1 INVITE\r\n\r\n" + offer, "uuid": "2"},
		{"session_id": "cid", "timestamp": ms(2 * time.Second), "method": "200", "response_code": "200", "cseq_method": "INVITE", "payload": "SIP/2.0 200 OK\r\nCSeq: 1 INVITE\r\n\r\n" + "m=audio 4000 RTP/AVP 18\r\n", "uuid": "3"},
		{"session_id": "cid", "timestamp": ms(3 * time.Second), "method": "ACK", "cseq_method": "ACK", "payload": "ACK sip:b SIP/2.0\r\nCSeq: 1 ACK\r\n\r\n" + answer, "uuid": "4"},
	}
	m := computeSIPCallInfoRows(rows)[0]
	if m["codecs_offered"] != "PCMU, PCMA, G729" {
		t.Fatalf("offered %v", m["codecs_offered"])
	}
	if m["codec_negotiated"] != "PCMU" || m["codecs"] != "PCMU" {
		t.Fatalf("negotiated %v codecs %v", m["codec_negotiated"], m["codecs"])
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
