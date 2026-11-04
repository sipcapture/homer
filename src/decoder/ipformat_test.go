// Copyright (C) 2026 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package decoder

import (
	"encoding/binary"
	"net"
	"testing"
	"time"
)

func TestIPv4BytesToString(t *testing.T) {
	got := ipv4BytesToString([]byte{10, 0, 0, 1})
	if got != "10.0.0.1" {
		t.Fatalf("got %q want 10.0.0.1", got)
	}
}

// ipv6BytesToString must give the RFC 5952 text that net.IP.String gave
// before the formatter was added: search filters compare this text.
func TestIPv6BytesToStringMatchesNetIP(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"::", "::"},
		{"::1", "::1"},
		{"2001:db8::10", "2001:db8::10"},
		{"2001:0db8:0000:0000:0000:0000:0000:0010", "2001:db8::10"},
		{"2001:db8:0:1:0:0:0:1", "2001:db8:0:1::1"},      // the longest zero run is compressed
		{"2001:db8:0:0:1:0:0:1", "2001:db8::1:0:0:1"},    // on a tie, the first run
		{"2001:db8:0:1:1:1:1:1", "2001:db8:0:1:1:1:1:1"}, // a single zero group is not compressed
		{"2001:db8:aaaa:bbbb:cccc:dddd:eeee:ffff", "2001:db8:aaaa:bbbb:cccc:dddd:eeee:ffff"},
		{"FE80::1", "fe80::1"},
		{"::ffff:192.0.2.1", "192.0.2.1"}, // IPv4-mapped prints as a dotted-quad
		{"::192.0.2.1", "::c000:201"},
		{"1::", "1::"},
	} {
		ip := net.ParseIP(tc.in)
		if ip == nil {
			t.Fatalf("bad test input %q", tc.in)
		}
		b := []byte(ip.To16())
		if old := ip.To16().String(); old != tc.want {
			t.Fatalf("test table: net.IP gives %q for %s, table says %q", old, tc.in, tc.want)
		}
		if got := ipv6BytesToString(b); got != tc.want {
			t.Errorf("ipv6BytesToString(%s) = %q, want %q", tc.in, got, tc.want)
		}
	}
	if got := ipv6BytesToString(make([]byte, 15)); got != "" {
		t.Errorf("15 bytes: got %q, want empty", got)
	}
}

func hep3WithIPv6(src, dst net.IP) []byte {
	chunks := make([]byte, 0, 256)
	chunks = benchAppendU8(chunks, 0x0001, 0x0a)
	chunks = benchAppendU8(chunks, 0x0002, 0x11)
	chunks = benchAppendChunkHeader(chunks, 0x0005, 16)
	chunks = append(chunks, src.To16()...)
	chunks = benchAppendChunkHeader(chunks, 0x0006, 16)
	chunks = append(chunks, dst.To16()...)
	chunks = benchAppendU16(chunks, 0x0007, 5060)
	chunks = benchAppendU16(chunks, 0x0008, 5060)
	chunks = benchAppendU32(chunks, 0x0009, uint32(time.Now().Unix()))
	chunks = benchAppendU32(chunks, 0x000a, 0)
	chunks = benchAppendU8(chunks, 0x000b, 1)
	chunks = benchAppendU32(chunks, 0x000c, 2001)
	chunks = benchAppendStr(chunks, 0x000f, benchSIPInvite)
	packet := make([]byte, 6+len(chunks))
	copy(packet, "HEP3")
	binary.BigEndian.PutUint16(packet[4:6], uint16(len(packet)))
	copy(packet[6:], chunks)
	return packet
}

func TestDecodeHEP3IPv6AddressesAreRFC5952(t *testing.T) {
	h, err := DecodeHEP(hep3WithIPv6(net.ParseIP("2001:db8::10"), net.ParseIP("2001:db8::1")))
	if err != nil {
		t.Fatal(err)
	}
	defer ReleaseHEP(h)
	if h.SrcIP != "2001:db8::10" || h.DstIP != "2001:db8::1" {
		t.Fatalf("SrcIP %q, DstIP %q; want 2001:db8::10 and 2001:db8::1", h.SrcIP, h.DstIP)
	}
}

func TestParseHEP2IPv6AddressesAreRFC5952(t *testing.T) {
	packet := make([]byte, 0, 64)
	packet = append(packet, 1, 0, 10, 17) // version 1, length, AF_INET6, UDP
	packet = binary.BigEndian.AppendUint16(packet, 5060)
	packet = binary.BigEndian.AppendUint16(packet, 5060)
	packet = append(packet, net.ParseIP("2001:db8::10").To16()...)
	packet = append(packet, net.ParseIP("2001:db8::1").To16()...)
	packet = append(packet, "INVITE sip:bob@example.com SIP/2.0\r\n\r\n"...)
	var h HEP
	if err := h.parseHEP2(packet); err != nil {
		t.Fatal(err)
	}
	if h.SrcIP != "2001:db8::10" || h.DstIP != "2001:db8::1" {
		t.Fatalf("SrcIP %q, DstIP %q; want 2001:db8::10 and 2001:db8::1", h.SrcIP, h.DstIP)
	}
}
