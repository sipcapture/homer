// Copyright (C) 2026 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package pcapwriter_test

import (
	"bytes"
	"encoding/binary"
	"testing"
	"time"

	"github.com/sipcapture/homer-core/src/pcapwriter"
)

func TestSIPSearchRowsToPCAP_MinimalInvite(t *testing.T) {
	ts := time.Date(2020, 5, 1, 12, 0, 0, 0, time.UTC)
	msg := "INVITE sip:a@b SIP/2.0\r\nContent-Length: 0\r\n\r\n"
	rows := []map[string]interface{}{
		{
			"timestamp": ts.Format(time.RFC3339Nano),
			"src_ip":    "10.0.0.1",
			"dst_ip":    "10.0.0.2",
			"src_port":  float64(5060),
			"dst_port":  float64(5060),
			"payload":   msg,
		},
	}
	raw, err := pcapwriter.SIPSearchRowsToPCAP(rows)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) < 24+16 {
		t.Fatalf("pcap too short: %d", len(raw))
	}
	magic := binary.LittleEndian.Uint32(raw[0:4])
	if magic != 0xA1B2C3D4 {
		t.Fatalf("bad pcap magic: %#x", magic)
	}
	if pcapwriter.SIPSearchRowsPacketCount(rows) != 1 {
		t.Fatal("packet count")
	}
}

func TestSIPSearchRowsToPCAP_EmptyPayloadError(t *testing.T) {
	rows := []map[string]interface{}{
		{"timestamp": "2020-01-01T00:00:00Z", "src_ip": "1.1.1.1", "dst_ip": "2.2.2.2", "payload": ""},
	}
	_, err := pcapwriter.SIPSearchRowsToPCAP(rows)
	if err != pcapwriter.ErrNoSIPPacketsInRows {
		t.Fatalf("want ErrNoSIPPacketsInRows, got %v", err)
	}
}

func TestSIPSearchRowsToPCAP_DefaultPorts5060(t *testing.T) {
	ts := time.Unix(1000, 0).UTC()
	rows := []map[string]interface{}{
		{
			"timestamp": ts,
			"src_ip":    "10.0.0.1",
			"dst_ip":    "10.0.0.2",
			"payload":   "OPTIONS sip:x SIP/2.0\r\n\r\n",
		},
	}
	raw, err := pcapwriter.SIPSearchRowsToPCAP(rows)
	if err != nil {
		t.Fatal(err)
	}
	// Find UDP header inside first frame: after eth (14) + IPv4 (20) -> UDP src/dst ports big-endian
	off := 24 + 16 // skip global hdr + record hdr
	if len(raw) < off+14+20+8 {
		t.Fatalf("short buffer len=%d", len(raw))
	}
	udpBase := off + 14 + 20
	src := binary.BigEndian.Uint16(raw[udpBase : udpBase+2])
	dst := binary.BigEndian.Uint16(raw[udpBase+2 : udpBase+4])
	if src != 5060 || dst != 5060 {
		t.Fatalf("expected default ports 5060/5060, got %d/%d", src, dst)
	}
}

func TestNewPCAPWriter_WritePacket_RoundTripHeader(t *testing.T) {
	w, err := pcapwriter.NewPCAPWriter()
	if err != nil {
		t.Fatal(err)
	}
	if err := w.WritePacket(time.Unix(1, 0), "10.0.0.1", "10.0.0.2", 5060, 5060, []byte("x")); err != nil {
		t.Fatal(err)
	}
	b := w.Bytes()
	if !bytes.HasPrefix(b, []byte{0xd4, 0xc3, 0xb2, 0xa1}) {
		t.Fatalf("bad LE magic prefix")
	}
}

func TestRowTimeOptional(t *testing.T) {
	t.Run("RFC3339", func(t *testing.T) {
		row := map[string]interface{}{"timestamp": "2020-05-01T12:00:00Z"}
		got, ok := pcapwriter.RowTimeOptional(row, "timestamp")
		if !ok {
			t.Fatal("expected ok")
		}
		if got.Year() != 2020 || got.Month() != 5 || got.Day() != 1 {
			t.Fatalf("time = %v", got)
		}
	})
	t.Run("missing", func(t *testing.T) {
		_, ok := pcapwriter.RowTimeOptional(map[string]interface{}{}, "timestamp")
		if ok {
			t.Fatal("expected !ok")
		}
	})
	t.Run("garbage", func(t *testing.T) {
		_, ok := pcapwriter.RowTimeOptional(map[string]interface{}{"timestamp": "not-a-date"}, "timestamp")
		if ok {
			t.Fatal("expected !ok")
		}
	})
}
