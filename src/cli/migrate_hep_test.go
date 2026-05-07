// Copyright (C) 2026 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package cli

import (
	"bytes"
	"encoding/binary"
	"net"
	"testing"
	"time"
)

// parseHEP3 is a tiny deserialiser used only by tests — it walks chunks and
// returns a map of {typeID → payload}. It rejects malformed packets.
func parseHEP3(t *testing.T, pkt []byte) map[uint16][]byte {
	t.Helper()
	if len(pkt) < 6 {
		t.Fatalf("packet too short: %d", len(pkt))
	}
	if !bytes.Equal(pkt[:4], []byte("HEP3")) {
		t.Fatalf("magic not HEP3: % x", pkt[:4])
	}
	total := binary.BigEndian.Uint16(pkt[4:6])
	if int(total) != len(pkt) {
		t.Fatalf("total length mismatch: header=%d actual=%d", total, len(pkt))
	}
	out := map[uint16][]byte{}
	off := 6
	for off < len(pkt) {
		if off+6 > len(pkt) {
			t.Fatalf("truncated chunk header at %d", off)
		}
		vendor := binary.BigEndian.Uint16(pkt[off : off+2])
		typeID := binary.BigEndian.Uint16(pkt[off+2 : off+4])
		clen := binary.BigEndian.Uint16(pkt[off+4 : off+6])
		if vendor != 0 {
			t.Fatalf("unexpected vendor=%#x at %d", vendor, off)
		}
		if int(clen) < 6 || off+int(clen) > len(pkt) {
			t.Fatalf("bad chunk length=%d at %d", clen, off)
		}
		out[typeID] = append([]byte(nil), pkt[off+6:off+int(clen)]...)
		off += int(clen)
	}
	return out
}

func TestReconstructHEP3_AllChunks(t *testing.T) {
	created := time.Date(2025, 4, 1, 12, 30, 45, 250_000_000, time.UTC)
	hdr := hep3ProtoHeader{
		ProtocolFamily: 2,
		Protocol:       17,
		SrcIP:          "192.0.2.10",
		DstIP:          "192.0.2.20",
		SrcPort:        5060,
		DstPort:        5061,
		TimeSec:        created.Unix(),
		TimeUsec:       250_000,
		PayloadType:    1,
		CaptureID:      0xCAFEBABE,
		CapturePass:    "secret",
		CorrelationID:  "abc-123",
		NodeName:       "edge-1",
	}
	payload := "INVITE sip:bob@example.com SIP/2.0\r\nCall-ID: abc-123\r\n\r\n"

	pkt, err := reconstructHEP3(payload, "abc-123", hdr, created, 0)
	if err != nil {
		t.Fatalf("reconstruct: %v", err)
	}
	chunks := parseHEP3(t, pkt)

	if got := chunks[0x0001]; len(got) != 1 || got[0] != 2 {
		t.Errorf("family chunk = %v, want [2]", got)
	}
	if got := chunks[0x0002]; len(got) != 1 || got[0] != 17 {
		t.Errorf("protocol chunk = %v, want [17]", got)
	}
	if got, want := chunks[0x0003], net.ParseIP("192.0.2.10").To4(); !bytes.Equal(got, want) {
		t.Errorf("src ip chunk = %v, want %v", got, want)
	}
	if got, want := chunks[0x0004], net.ParseIP("192.0.2.20").To4(); !bytes.Equal(got, want) {
		t.Errorf("dst ip chunk = %v, want %v", got, want)
	}
	if got := binary.BigEndian.Uint16(chunks[0x0007]); got != 5060 {
		t.Errorf("src port = %d, want 5060", got)
	}
	if got := binary.BigEndian.Uint16(chunks[0x0008]); got != 5061 {
		t.Errorf("dst port = %d, want 5061", got)
	}
	if got := binary.BigEndian.Uint32(chunks[0x0009]); int64(got) != created.Unix() {
		t.Errorf("time sec = %d, want %d", got, created.Unix())
	}
	if got := binary.BigEndian.Uint32(chunks[0x000a]); got != 250_000 {
		t.Errorf("time usec = %d, want 250000", got)
	}
	if got := chunks[0x000b]; len(got) != 1 || got[0] != 1 {
		t.Errorf("payload type chunk = %v, want [1]", got)
	}
	if got := binary.BigEndian.Uint32(chunks[0x000c]); got != 0xCAFEBABE {
		t.Errorf("capture id = %#x, want 0xCAFEBABE", got)
	}
	if got := string(chunks[0x000e]); got != "secret" {
		t.Errorf("capture pass = %q, want secret", got)
	}
	if got := string(chunks[0x000f]); got != payload {
		t.Errorf("payload chunk mismatch")
	}
	if got := string(chunks[0x0011]); got != "abc-123" {
		t.Errorf("correlation id = %q, want abc-123", got)
	}
	if got := string(chunks[0x0013]); got != "edge-1" {
		t.Errorf("node name = %q, want edge-1", got)
	}
}

func TestReconstructHEP3_DefaultsAndCaptureOverride(t *testing.T) {
	hdr := hep3ProtoHeader{}                                              // mostly empty header
	created := time.Date(2024, 1, 1, 0, 0, 0, 12_000, time.UTC)
	payload := "RTCP-XR data"

	pkt, err := reconstructHEP3(payload, "", hdr, created, 42)
	if err != nil {
		t.Fatalf("reconstruct: %v", err)
	}
	chunks := parseHEP3(t, pkt)

	if got := chunks[0x0001]; got[0] != 2 {
		t.Errorf("default family = %v, want IPv4 (2)", got)
	}
	if got := chunks[0x0002]; got[0] != 17 {
		t.Errorf("default protocol = %v, want UDP (17)", got)
	}
	if got := binary.BigEndian.Uint32(chunks[0x000c]); got != 42 {
		t.Errorf("forced capture id = %d, want 42", got)
	}
	if got := binary.BigEndian.Uint32(chunks[0x0009]); int64(got) != created.Unix() {
		t.Errorf("fallback time sec = %d, want %d", got, created.Unix())
	}
	if got := binary.BigEndian.Uint32(chunks[0x000a]); got != 12 {
		t.Errorf("fallback time usec = %d, want 12", got)
	}
	if _, ok := chunks[0x0011]; ok {
		t.Errorf("correlation id chunk should be absent when both header and call-id are empty")
	}
}

func TestReconstructHEP3_FallbackToCallID(t *testing.T) {
	hdr := hep3ProtoHeader{ProtocolFamily: 2, Protocol: 17, SrcIP: "127.0.0.1", DstIP: "127.0.0.1"}
	pkt, err := reconstructHEP3("payload", "call-from-sid", hdr, time.Now(), 0)
	if err != nil {
		t.Fatalf("reconstruct: %v", err)
	}
	chunks := parseHEP3(t, pkt)
	if got := string(chunks[0x0011]); got != "call-from-sid" {
		t.Errorf("correlation id = %q, want call-from-sid", got)
	}
}

func TestReconstructHEP3_IPv6(t *testing.T) {
	hdr := hep3ProtoHeader{ProtocolFamily: 10, Protocol: 6, SrcIP: "2001:db8::1", DstIP: "2001:db8::2"}
	pkt, err := reconstructHEP3("hi", "", hdr, time.Now(), 1)
	if err != nil {
		t.Fatalf("reconstruct: %v", err)
	}
	chunks := parseHEP3(t, pkt)
	if got, want := chunks[0x0003], net.ParseIP("2001:db8::1").To16(); !bytes.Equal(got, want) {
		t.Errorf("ipv6 src = %v, want %v", got, want)
	}
	if got := chunks[0x0001][0]; got != 10 {
		t.Errorf("family = %d, want 10", got)
	}
	if got := chunks[0x0002][0]; got != 6 {
		t.Errorf("proto = %d, want 6 (TCP)", got)
	}
}

func TestReconstructHEP3_EmptyPayloadFails(t *testing.T) {
	_, err := reconstructHEP3("", "", hep3ProtoHeader{}, time.Now(), 0)
	if err == nil {
		t.Fatal("expected error for empty payload, got nil")
	}
}

func TestIsSafeTableName(t *testing.T) {
	cases := map[string]bool{
		"hep_proto_1_call":              true,
		"hep_proto_5_default":           true,
		"hep_proto_100_default":         true,
		"":                              false,
		"hep_proto_1; DROP TABLE users": false,
		"hep-proto-1":                   false,
		"hep_proto_1.call":              false,
		"hep_proto_1\nfoo":              false,
	}
	for in, want := range cases {
		if got := isSafeTableName(in); got != want {
			t.Errorf("isSafeTableName(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestSplitNonEmpty(t *testing.T) {
	got := splitNonEmpty("a,b , ,c,, b ")
	want := []string{"a", "b", "b", "c"}
	if len(got) != len(want) {
		t.Fatalf("len mismatch: got=%v want=%v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("index %d: got=%q want=%q", i, got[i], want[i])
		}
	}
}
