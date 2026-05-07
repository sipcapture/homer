// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package hepstream

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/sipcapture/homer-core/src/decoder"
	"github.com/sipcapture/homer-core/src/sipparser"
)

func TestFromHEPBasic(t *testing.T) {
	h := &decoder.HEP{
		ProtoType: 1,
		SrcIP:     "10.0.0.1", SrcPort: 5060,
		DstIP: "10.0.0.2", DstPort: 5060,
		NodeID:    42,
		Payload:   "INVITE sip:...\r\n...",
		Timestamp: time.Unix(1700000000, 123*1000*1000),
		SIP: &sipparser.SipMsg{
			CseqMethod: "INVITE",
			CseqVal:    "1 INVITE",
			CallID:     "abc@host",
			FromUser:   "alice",
			ToUser:     "bob",
			URIUser:    "bob",
		},
	}
	e := FromHEP(h)
	if e.Proto != 1 || e.SrcIP != "10.0.0.1" || e.NodeID != 42 {
		t.Fatalf("bad event: %+v", e)
	}
	if e.TsMilli != 1700000000123 {
		t.Fatalf("ts=%d want 1700000000123", e.TsMilli)
	}
	if e.SIP == nil || e.SIP.Method != "INVITE" || e.SIP.CallID != "abc@host" {
		t.Fatalf("sip meta wrong: %+v", e.SIP)
	}
	if e.Payload == "" {
		t.Fatalf("payload should be carried in Event")
	}
}

func TestFromHEPNonSIPOmitsSIPMeta(t *testing.T) {
	h := &decoder.HEP{
		ProtoType: 5, // RTCP
		SrcIP:     "10.0.0.1",
		Timestamp: time.Now(),
	}
	e := FromHEP(h)
	if e.SIP != nil {
		t.Fatalf("non-SIP event must not carry SIP meta: %+v", e.SIP)
	}
}

func TestFromHEPZeroTimestampFallsBackToNow(t *testing.T) {
	h := &decoder.HEP{ProtoType: 1}
	before := time.Now().UnixMilli() - 1
	e := FromHEP(h)
	after := time.Now().UnixMilli() + 1
	if e.TsMilli < before || e.TsMilli > after {
		t.Fatalf("fallback timestamp %d not in [%d,%d]", e.TsMilli, before, after)
	}
}

func TestMarshalForStripsPayloadWhenNotIncluded(t *testing.T) {
	e := Event{
		TsMilli: 1700000000000,
		Proto:   1,
		SIP:     &SIPMeta{Method: "INVITE"},
		Payload: "INVITE sip:...\r\n",
	}
	buf, err := e.MarshalFor(false)
	if err != nil {
		t.Fatalf("MarshalFor: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(buf, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, has := got["payload"]; has {
		t.Fatalf("payload must not be in frame: %s", buf)
	}
	if got["proto"].(float64) != 1 {
		t.Fatalf("proto missing: %s", buf)
	}
}

func TestMarshalForIncludesPayloadWhenAsked(t *testing.T) {
	e := Event{
		TsMilli: 1700000000000,
		Proto:   1,
		Payload: "PAYLOAD",
	}
	buf, err := e.MarshalFor(true)
	if err != nil {
		t.Fatalf("MarshalFor: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(buf, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got["payload"] != "PAYLOAD" {
		t.Fatalf("payload missing: %s", buf)
	}
}
