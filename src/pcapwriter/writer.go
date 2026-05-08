// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

// Package pcapwriter provides a minimal libpcap file writer (Ethernet + UDP + payload)
// used by the coordinator export API and the homer CLI.
package pcapwriter

import (
	"bytes"
	"encoding/binary"
	"net"
	"time"
)

const (
	pcapMagic     = 0xA1B2C3D4 // microsecond timestamps
	pcapMajor     = 2
	pcapMinor     = 4
	pcapSnapLen   = 65535
	pcapLinkEther = 1 // DLT_EN10MB
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
	binary.BigEndian.PutUint16(udp[6:8], 0)

	var ipHdr []byte
	etherType := []byte{0x08, 0x00} // IPv4
	if isV6 {
		etherType = []byte{0x86, 0xDD} // IPv6
		ipHdr = buildIPv6Header(src, dst, udpPayloadLen)
	} else {
		ipHdr = buildIPv4Header(src.To4(), dst.To4(), udpPayloadLen)
	}

	eth := []byte{
		0x06, 0x3d, 0x20, 0x12, 0x10, 0x20,
		0x02, 0x5d, 0x69, 0x74, 0x20, 0x12,
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
	hdr[0] = 0x45
	hdr[1] = 0x00
	binary.BigEndian.PutUint16(hdr[2:4], totalLen)
	binary.BigEndian.PutUint16(hdr[4:6], 0x0000)
	binary.BigEndian.PutUint16(hdr[6:8], 0x0000)
	hdr[8] = 64
	hdr[9] = 17
	copy(hdr[12:16], src.To4())
	copy(hdr[16:20], dst.To4())
	return hdr
}

func buildIPv6Header(src, dst net.IP, udpTotalLen uint16) []byte {
	hdr := make([]byte, 40)
	hdr[0] = 0x60
	binary.BigEndian.PutUint16(hdr[4:6], udpTotalLen)
	hdr[6] = 17
	hdr[7] = 64
	copy(hdr[8:24], src.To16())
	copy(hdr[24:40], dst.To16())
	return hdr
}
