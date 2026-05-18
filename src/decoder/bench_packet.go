// Copyright (C) 2026 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

//go:build !vet
// +build !vet

package decoder

import (
	"encoding/binary"
	"net"
	"time"
)

const benchSIPInvite = "INVITE sip:bob@biloxi.example.com SIP/2.0\r\n" +
	"Via: SIP/2.0/UDP pc33.atlanta.example.com;branch=z9hG4bK776asdhds\r\n" +
	"Max-Forwards: 70\r\n" +
	"To: Bob <sip:bob@biloxi.example.com>\r\n" +
	"From: Alice <sip:alice@atlanta.example.com>;tag=1928301774\r\n" +
	"Call-ID: bench-callid-12345@atlanta.example.com\r\n" +
	"CSeq: 1 INVITE\r\n" +
	"Contact: <sip:alice@pc33.atlanta.example.com>\r\n" +
	"User-Agent: bench-ua/1.0\r\n" +
	"Content-Type: application/sdp\r\n" +
	"Content-Length: 150\r\n" +
	"\r\n" +
	"v=0\r\n" +
	"o=alice 53655765 2353687637 IN IP4 pc33.atlanta.example.com\r\n" +
	"s=-\r\n" +
	"c=IN IP4 pc33.atlanta.example.com\r\n" +
	"t=0 0\r\n" +
	"m=audio 3456 RTP/AVP 0 1 3 99\r\n" +
	"a=rtpmap:0 PCMU/8000\r\n"

// BenchHEP3SIPPacket returns a HEP3-encapsulated SIP INVITE for benchmarks.
func BenchHEP3SIPPacket() []byte {
	now := time.Now()
	chunks := make([]byte, 0, 256)
	chunks = benchAppendU8(chunks, 0x0001, 0x02)
	chunks = benchAppendU8(chunks, 0x0002, 0x11)
	chunks = benchAppendIP4(chunks, 0x0003, net.IPv4(10, 0, 0, 1))
	chunks = benchAppendIP4(chunks, 0x0004, net.IPv4(10, 0, 0, 2))
	chunks = benchAppendU16(chunks, 0x0007, 5060)
	chunks = benchAppendU16(chunks, 0x0008, 5060)
	chunks = benchAppendU32(chunks, 0x0009, uint32(now.Unix()))
	chunks = benchAppendU32(chunks, 0x000a, uint32(now.UnixMicro()%1_000_000))
	chunks = benchAppendU8(chunks, 0x000b, 1)
	chunks = benchAppendU32(chunks, 0x000c, 2001)
	chunks = benchAppendStr(chunks, 0x000e, "myHep")
	chunks = benchAppendStr(chunks, 0x000f, benchSIPInvite)
	chunks = benchAppendStr(chunks, 0x0011, "bench-callid-12345@atlanta.example.com")

	totalLen := 6 + len(chunks)
	packet := make([]byte, totalLen)
	copy(packet[0:4], []byte("HEP3"))
	binary.BigEndian.PutUint16(packet[4:6], uint16(totalLen))
	copy(packet[6:], chunks)
	return packet
}

func benchAppendChunkHeader(buf []byte, chunkType uint16, body uint16) []byte {
	var hdr [6]byte
	binary.BigEndian.PutUint16(hdr[0:2], 0)
	binary.BigEndian.PutUint16(hdr[2:4], chunkType)
	binary.BigEndian.PutUint16(hdr[4:6], 6+body)
	return append(buf, hdr[:]...)
}

func benchAppendU8(buf []byte, chunkType uint16, val byte) []byte {
	buf = benchAppendChunkHeader(buf, chunkType, 1)
	return append(buf, val)
}

func benchAppendU16(buf []byte, chunkType uint16, val uint16) []byte {
	buf = benchAppendChunkHeader(buf, chunkType, 2)
	var b [2]byte
	binary.BigEndian.PutUint16(b[:], val)
	return append(buf, b[:]...)
}

func benchAppendU32(buf []byte, chunkType uint16, val uint32) []byte {
	buf = benchAppendChunkHeader(buf, chunkType, 4)
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], val)
	return append(buf, b[:]...)
}

func benchAppendIP4(buf []byte, chunkType uint16, ip net.IP) []byte {
	v4 := ip.To4()
	buf = benchAppendChunkHeader(buf, chunkType, 4)
	return append(buf, v4...)
}

func benchAppendStr(buf []byte, chunkType uint16, s string) []byte {
	buf = benchAppendChunkHeader(buf, chunkType, uint16(len(s)))
	return append(buf, s...)
}
