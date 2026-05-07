// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package handlers

// Pure-Go minimal libpcap writer and SIP-over-UDP/TCP packet builder.
// No external dependencies required (no gopacket).

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
)

// ── PCAP global header constants (little-endian, microsecond resolution) ─────

const (
	pcapMagic      = 0xA1B2C3D4 // microsecond timestamps
	pcapMajor      = 2
	pcapMinor      = 4
	pcapSnapLen    = 65535
	pcapLinkEther  = 1 // DLT_EN10MB
)

// PCAPWriter writes a libpcap file into an in-memory buffer.
type PCAPWriter struct {
	buf bytes.Buffer
}

// NewPCAPWriter creates a writer and writes the global PCAP header.
func NewPCAPWriter() (*PCAPWriter, error) {
	w := &PCAPWriter{}
	hdr := make([]byte, 24)
	binary.LittleEndian.PutUint32(hdr[0:4], pcapMagic)
	binary.LittleEndian.PutUint16(hdr[4:6], pcapMajor)
	binary.LittleEndian.PutUint16(hdr[6:8], pcapMinor)
	// timezone offset (unused), accuracy (unused)
	binary.LittleEndian.PutUint32(hdr[8:12], 0)
	binary.LittleEndian.PutUint32(hdr[12:16], 0)
	binary.LittleEndian.PutUint32(hdr[16:20], pcapSnapLen)
	binary.LittleEndian.PutUint32(hdr[20:24], pcapLinkEther)
	_, err := w.buf.Write(hdr)
	return w, err
}

// WritePacket encapsulates payload into Ethernet+IPv4/IPv6+UDP and appends to the buffer.
func (w *PCAPWriter) WritePacket(ts time.Time, srcIP, dstIP string, srcPort, dstPort uint16, payload []byte) error {
	pkt, err := buildUDPPacket(srcIP, dstIP, srcPort, dstPort, payload)
	if err != nil {
		return err
	}

	sec := uint32(ts.Unix())
	usec := uint32(ts.Nanosecond() / 1000)
	capLen := uint32(len(pkt))

	recHdr := make([]byte, 16)
	binary.LittleEndian.PutUint32(recHdr[0:4], sec)
	binary.LittleEndian.PutUint32(recHdr[4:8], usec)
	binary.LittleEndian.PutUint32(recHdr[8:12], capLen)
	binary.LittleEndian.PutUint32(recHdr[12:16], capLen)
	if _, err := w.buf.Write(recHdr); err != nil {
		return err
	}
	_, err = w.buf.Write(pkt)
	return err
}

// Bytes returns the complete PCAP file contents.
func (w *PCAPWriter) Bytes() []byte { return w.buf.Bytes() }

// ── Packet builder ────────────────────────────────────────────────────────────

func buildUDPPacket(srcIP, dstIP string, srcPort, dstPort uint16, payload []byte) ([]byte, error) {
	src := net.ParseIP(srcIP)
	dst := net.ParseIP(dstIP)
	if src == nil {
		src = net.ParseIP("127.0.0.1")
	}
	if dst == nil {
		dst = net.ParseIP("127.0.0.2")
	}

	isV6 := src.To4() == nil && src.To16() != nil

	udpPayloadLen := uint16(8 + len(payload))

	var udp [8]byte
	binary.BigEndian.PutUint16(udp[0:2], srcPort)
	binary.BigEndian.PutUint16(udp[2:4], dstPort)
	binary.BigEndian.PutUint16(udp[4:6], udpPayloadLen)
	// checksum: 0 (optional for UDP)
	binary.BigEndian.PutUint16(udp[6:8], 0)

	var ipHdr []byte
	etherType := []byte{0x08, 0x00} // IPv4
	if isV6 {
		etherType = []byte{0x86, 0xDD} // IPv6
		ipHdr = buildIPv6Header(src, dst, udpPayloadLen)
	} else {
		ipHdr = buildIPv4Header(src.To4(), dst.To4(), udpPayloadLen)
	}

	// Ethernet header: dst MAC (6) + src MAC (6) + ethertype (2)
	eth := []byte{
		0x06, 0x3d, 0x20, 0x12, 0x10, 0x20, // dst
		0x02, 0x5d, 0x69, 0x74, 0x20, 0x12, // src
	}
	eth = append(eth, etherType...)

	var pkt []byte
	pkt = append(pkt, eth...)
	pkt = append(pkt, ipHdr...)
	pkt = append(pkt, udp[:]...)
	pkt = append(pkt, payload...)
	return pkt, nil
}

func buildIPv4Header(src, dst net.IP, udpTotalLen uint16) []byte {
	totalLen := uint16(20) + udpTotalLen
	hdr := make([]byte, 20)
	hdr[0] = 0x45 // version=4, IHL=5
	hdr[1] = 0x00 // DSCP/ECN
	binary.BigEndian.PutUint16(hdr[2:4], totalLen)
	binary.BigEndian.PutUint16(hdr[4:6], 0x0000) // ID
	binary.BigEndian.PutUint16(hdr[6:8], 0x0000) // flags/offset
	hdr[8] = 64                                   // TTL
	hdr[9] = 17                                   // protocol = UDP
	// checksum bytes 10:12 stay 0 (wireshark recalculates)
	copy(hdr[12:16], src.To4())
	copy(hdr[16:20], dst.To4())
	return hdr
}

func buildIPv6Header(src, dst net.IP, udpTotalLen uint16) []byte {
	hdr := make([]byte, 40)
	hdr[0] = 0x60 // version=6
	binary.BigEndian.PutUint16(hdr[4:6], udpTotalLen) // payload length
	hdr[6] = 17                                        // next header = UDP
	hdr[7] = 64                                        // hop limit
	copy(hdr[8:24], src.To16())
	copy(hdr[24:40], dst.To16())
	return hdr
}

// ── Text formatter ────────────────────────────────────────────────────────────

// FormatTextLine returns the header line for a SIP message in homer style.
func FormatTextLine(srcIP, dstIP string, srcPort, dstPort int, ts time.Time, proto string) string {
	return fmt.Sprintf("proto:%s %s  %s:%d ---> %s:%d\r\n\r\n",
		proto,
		ts.UTC().Format(time.RFC3339Nano),
		srcIP, srcPort,
		dstIP, dstPort,
	)
}

// ── Row helpers ───────────────────────────────────────────────────────────────

// rowStr extracts a string field from a map[string]interface{} row.
func rowStr(row map[string]interface{}, key string) string {
	if v, ok := row[key]; ok && v != nil {
		return fmt.Sprintf("%v", v)
	}
	return ""
}

// rowInt extracts an integer-like field from a map row.
func rowInt(row map[string]interface{}, key string) int {
	if v, ok := row[key]; ok && v != nil {
		switch val := v.(type) {
		case int:
			return val
		case int32:
			return int(val)
		case int64:
			return int(val)
		case float64:
			return int(val)
		case string:
			n, _ := strconv.Atoi(val)
			return n
		}
	}
	return 0
}

// rowTime parses a timestamp field from a map row.
// Accepts time.Time, string (RFC3339/RFC3339Nano), or numeric (unix seconds/ms/µs/ns).
func rowTime(row map[string]interface{}, key string) time.Time {
	v, ok := row[key]
	if !ok || v == nil {
		return time.Now().UTC()
	}
	switch val := v.(type) {
	case time.Time:
		return val.UTC()
	case string:
		val = strings.TrimSpace(val)
		for _, layout := range []string{
			time.RFC3339Nano,
			time.RFC3339,
			"2006-01-02 15:04:05.999999999",
			"2006-01-02 15:04:05",
		} {
			if t, err := time.Parse(layout, val); err == nil {
				return t.UTC()
			}
		}
		return time.Now().UTC()
	case float64:
		return unixToTime(int64(val))
	case int64:
		return unixToTime(val)
	}
	return time.Now().UTC()
}

func unixToTime(n int64) time.Time {
	switch {
	case n > 1e18:
		return time.Unix(0, n).UTC()
	case n > 1e15:
		return time.Unix(0, n*1000).UTC()
	case n > 1e12:
		return time.Unix(n/1000, (n%1000)*1e6).UTC()
	default:
		return time.Unix(n, 0).UTC()
	}
}
