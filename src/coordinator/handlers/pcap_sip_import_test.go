// Copyright (C) 2026 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package handlers

import (
	"strings"
	"testing"
	"time"

	"github.com/google/gopacket"
	"github.com/sipcapture/homer-core/src/coordinator/sqlvalidator"
	"github.com/sipcapture/homer-core/src/decoder"
	"github.com/sipcapture/homer-core/src/pcapwriter"
	"github.com/sipcapture/homer-core/src/storage/ducklake"
)

func minimalInvite() string {
	// CRLF-terminated SIP (required by sipparser)
	return "INVITE sip:bob@biloxi.example.com SIP/2.0\r\n" +
		"Via: SIP/2.0/UDP pc33.atlanta.example.com;branch=z9hG4bK776asdhds\r\n" +
		"Max-Forwards: 70\r\n" +
		"To: Bob <sip:bob@biloxi.example.com>\r\n" +
		"From: Alice <sip:alice@atlanta.example.com>;tag=1928301774\r\n" +
		"Call-ID: a84b4c76e66710@pc33.atlanta.example.com\r\n" +
		"CSeq: 314159 INVITE\r\n" +
		"Contact: <sip:alice@pc33.atlanta.example.com>\r\n" +
		"Content-Length: 0\r\n" +
		"\r\n"
}

func TestPcapSyntheticInvitePipeline(t *testing.T) {
	w, err := pcapwriter.NewPCAPWriter()
	if err != nil {
		t.Fatal(err)
	}
	msg := minimalInvite()
	ts := time.Date(2020, 5, 1, 12, 0, 0, 0, time.UTC)
	if err := w.WritePacket(ts, "10.0.0.1", "10.0.0.2", 5060, 5060, []byte(msg)); err != nil {
		t.Fatal(err)
	}
	raw := w.Bytes()
	next, lt, err := newPcapIterator(raw)
	if err != nil {
		t.Fatal(err)
	}
	data, _, err := next()
	if err != nil {
		t.Fatal(err)
	}
	pkt := gopacket.NewPacket(data, lt, gopacket.Default)
	srcIP, dstIP, sp, dp, ipProto, pay, ok := extractTransport(pkt)
	if !ok {
		t.Fatal("extractTransport failed")
	}
	pl := string(pay)
	dec := decoder.NewDecoder(&decoder.DecoderConfig{})
	h := &decoder.HEP{}
	if err := decoder.ApplyCapturedSIP(dec, h, srcIP, dstIP, uint32(sp), uint32(dp), ipProto, pl, ts, 0); err != nil {
		t.Fatal(err)
	}
	if h.SIP == nil || h.SIP.FirstMethod != "INVITE" {
		t.Fatalf("expected INVITE, got %#v", h.SIP)
	}
	key, vals, err := ducklake.ConvertHEPToLakeRow(h)
	if err != nil {
		t.Fatal(err)
	}
	if key.SubType != ducklake.SIPTypeCall {
		t.Fatalf("expected call table, got %q", key.SubType)
	}
	sql, err := ducklake.BuildInsertMultiValues("homer_lake", key, [][]interface{}{vals})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sql, "INSERT INTO homer_lake.main.hep_proto_1_call") {
		t.Fatalf("unexpected sql: %s", sql)
	}
	if !strings.Contains(sql, "314159") {
		t.Fatalf("cseq not in sql: %s", sql)
	}
	if err := sqlvalidator.ValidateWriteSQL(sql); err != nil {
		t.Fatalf("generated INSERT must pass write validator: %v\n%s", err, sql)
	}
}

func TestPcapForceInviteIntoRegistrationTable(t *testing.T) {
	w, err := pcapwriter.NewPCAPWriter()
	if err != nil {
		t.Fatal(err)
	}
	msg := minimalInvite()
	ts := time.Date(2020, 5, 1, 12, 0, 0, 0, time.UTC)
	if err := w.WritePacket(ts, "10.0.0.1", "10.0.0.2", 5060, 5060, []byte(msg)); err != nil {
		t.Fatal(err)
	}
	next, lt, err := newPcapIterator(w.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	data, _, err := next()
	if err != nil {
		t.Fatal(err)
	}
	pkt := gopacket.NewPacket(data, lt, gopacket.Default)
	srcIP, dstIP, sp, dp, ipProto, pay, ok := extractTransport(pkt)
	if !ok {
		t.Fatal("extractTransport")
	}
	pl := string(pay)
	dec := decoder.NewDecoder(&decoder.DecoderConfig{})
	h := &decoder.HEP{}
	if err := decoder.ApplyCapturedSIP(dec, h, srcIP, dstIP, uint32(sp), uint32(dp), ipProto, pl, ts, 0); err != nil {
		t.Fatal(err)
	}
	key, vals, err := ducklake.ConvertHEPToLakeRowSIPForced(h, ducklake.SIPTypeRegistration)
	if err != nil {
		t.Fatal(err)
	}
	if key.SubType != ducklake.SIPTypeRegistration {
		t.Fatalf("expected forced registration, got %q", key.SubType)
	}
	sql, err := ducklake.BuildInsertMultiValues("homer_lake", key, [][]interface{}{vals})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sql, "INSERT INTO homer_lake.main.hep_proto_1_registration") {
		t.Fatalf("expected registration table, got: %s", sql)
	}
}

func TestPcapSyntheticRegisterRouting(t *testing.T) {
	w, err := pcapwriter.NewPCAPWriter()
	if err != nil {
		t.Fatal(err)
	}
	msg := "REGISTER sip:registrar.biloxi.example.com SIP/2.0\r\n" +
		"Via: SIP/2.0/UDP bobspc.biloxi.example.com;branch=z9hG4bKnashds7\r\n" +
		"Max-Forwards: 70\r\n" +
		"To: Bob <sip:bob@biloxi.example.com>\r\n" +
		"From: Bob <sip:bob@biloxi.example.com>;tag=456383\r\n" +
		"Call-ID: 84317@bobspc.biloxi.example.com\r\n" +
		"CSeq: 1826 REGISTER\r\n" +
		"Contact: <sip:bob@192.0.2.4>\r\n" +
		"Expires: 7200\r\n" +
		"Content-Length: 0\r\n" +
		"\r\n"
	ts := time.Date(2021, 1, 2, 3, 4, 5, 0, time.UTC)
	if err := w.WritePacket(ts, "192.0.2.4", "192.0.2.1", 5060, 5060, []byte(msg)); err != nil {
		t.Fatal(err)
	}
	next, lt, err := newPcapIterator(w.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	data, _, err := next()
	if err != nil {
		t.Fatal(err)
	}
	pkt := gopacket.NewPacket(data, lt, gopacket.Default)
	srcIP, dstIP, sp, dp, ipProto, pay, ok := extractTransport(pkt)
	if !ok {
		t.Fatal("extractTransport")
	}
	dec := decoder.NewDecoder(&decoder.DecoderConfig{})
	h := &decoder.HEP{}
	if err := decoder.ApplyCapturedSIP(dec, h, srcIP, dstIP, uint32(sp), uint32(dp), ipProto, string(pay), ts, 0); err != nil {
		t.Fatal(err)
	}
	key, _, err := ducklake.ConvertHEPToLakeRow(h)
	if err != nil {
		t.Fatal(err)
	}
	if key.SubType != ducklake.SIPTypeRegistration {
		t.Fatalf("expected registration, got %q", key.SubType)
	}
}
