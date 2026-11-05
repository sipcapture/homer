// Copyright (C) 2026 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package decoder

import (
	"encoding/binary"
	"strings"
	"testing"
	"time"

	"github.com/sipcapture/homer-core/src/isup"
)

func TestDecodeHEPPreservesISUPBody(t *testing.T) {
	body := []byte{
		0x01, 0x00, 0x60, 0x00, 0x0a, 0x03, 0x02, 0x00,
		0x07, 0x83, 0x90, 0x36, 0x12, 0x70, 0x06, 0xf8,
	}
	sip := "INVITE sip:100@example.com SIP/2.0\r\n" +
		"Via: SIP/2.0/UDP 10.0.0.1;branch=z9hG4bK1\r\n" +
		"From: <sip:a@example.com>;tag=1\r\n" +
		"To: <sip:b@example.com>\r\n" +
		"Call-ID: isup-call@example.com\r\n" +
		"CSeq: 1 INVITE\r\n" +
		"Content-Type: application/isup;version=itu-t92+\r\n" +
		"Content-Length: 16\r\n" +
		"\r\n" +
		string(body)

	h, err := DecodeHEP(hep3SIP(sip))
	if err != nil {
		t.Fatal(err)
	}
	defer ReleaseHEP(h)
	if h.SIP.CallID != "isup-call@example.com" {
		t.Fatalf("call-id %q", h.SIP.CallID)
	}
	if strings.IndexByte(h.Payload, 0) >= 0 {
		t.Fatal("stored payload still contains NUL")
	}
	got := isup.DecodeSIP(h.Payload)
	if got == nil || len(got.Messages) != 1 || got.Messages[0].CalledPartyNumber == nil {
		t.Fatalf("decode %#v", got)
	}
	if got.Messages[0].CalledPartyNumber.Digits != "632107608" {
		t.Fatalf("digits %s", got.Messages[0].CalledPartyNumber.Digits)
	}
	if isup.RestoreBinarySIPPayload(h.Payload) != sip {
		t.Fatal("stored payload does not restore to the captured SIP message")
	}
}

func hep3SIP(payload string) []byte {
	now := time.Now()
	var chunks []byte
	chunks = hepAppendU8(chunks, 0x000b, 1)
	chunks = hepAppendU32(chunks, 0x0009, uint32(now.Unix()))
	chunks = hepAppendU32(chunks, 0x000a, uint32(now.Nanosecond()/1000))
	chunks = hepAppendBytes(chunks, 0x000f, []byte(payload))
	total := 6 + len(chunks)
	packet := make([]byte, total)
	copy(packet[0:4], []byte("HEP3"))
	binary.BigEndian.PutUint16(packet[4:6], uint16(total))
	copy(packet[6:], chunks)
	return packet
}

func hepAppendU8(buf []byte, chunkType uint16, val byte) []byte {
	return hepAppendBytes(buf, chunkType, []byte{val})
}

func hepAppendU32(buf []byte, chunkType uint16, val uint32) []byte {
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], val)
	return hepAppendBytes(buf, chunkType, b[:])
}

func hepAppendBytes(buf []byte, chunkType uint16, body []byte) []byte {
	var hdr [6]byte
	binary.BigEndian.PutUint16(hdr[2:4], chunkType)
	binary.BigEndian.PutUint16(hdr[4:6], uint16(6+len(body)))
	buf = append(buf, hdr[:]...)
	return append(buf, body...)
}
