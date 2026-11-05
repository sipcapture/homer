// Copyright (C) 2026 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package isup

import (
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"
)

// sampleIAM is an ITU-T IAM with a NUL in the forward-call indicators and a
// NUL end-of-optional marker. Those are the bytes toUTF8 used to drop.
func sampleIAM() []byte {
	return []byte{
		0x01,       // IAM
		0x00,       // nature of connection indicators
		0x60, 0x00, // forward call indicators (contains NUL)
		0x0a,                         // ordinary calling subscriber
		0x03,                         // 3.1 kHz audio
		0x02,                         // pointer to called party number
		0x09,                         // pointer to optional part
		0x07,                         // called party number length
		0x83,                         // odd, national
		0x90,                         // ISDN E.164
		0x36, 0x12, 0x70, 0x06, 0xf8, // 632107608
		0x0a,                                     // calling party number
		0x09,                                     // length
		0x82,                                     // odd, unknown
		0x13,                                     // ISDN, presentation allowed, network provided
		0x78, 0x08, 0x36, 0x37, 0x40, 0x74, 0xf3, // 8780637304473
		0x00, // end of optional parameters
	}
}

func sampleREL() []byte {
	return []byte{
		0x0c,       // REL
		0x02,       // pointer to cause
		0x00,       // no optional parameters
		0x02,       // cause length
		0x82, 0x90, // location = public network local, cause 16
	}
}

func TestDecodeIAM(t *testing.T) {
	msg := Decode(sampleIAM())
	if msg.Error != "" {
		t.Fatalf("error: %s", msg.Error)
	}
	if msg.Acronym != "IAM" {
		t.Fatalf("acronym %s", msg.Acronym)
	}
	if msg.CalledPartyNumber == nil || msg.CalledPartyNumber.Digits != "632107608" {
		t.Fatalf("called: %#v", msg.CalledPartyNumber)
	}
	if msg.CalledPartyNumber.NatureOfAddress != "national (significant) number" {
		t.Fatalf("called noa: %s", msg.CalledPartyNumber.NatureOfAddress)
	}
	if msg.CallingPartyNumber == nil || msg.CallingPartyNumber.Digits != "8780637304473" {
		t.Fatalf("calling: %#v", msg.CallingPartyNumber)
	}
	if msg.CallingPartyNumber.Presentation != "presentation allowed" || msg.CallingPartyNumber.Screening != "network provided" {
		t.Fatalf("calling flags: %#v", msg.CallingPartyNumber)
	}
	if msg.CallingPartyCategory != "ordinary calling subscriber" {
		t.Fatalf("cpc: %s", msg.CallingPartyCategory)
	}
	if msg.TransmissionMedium != "3.1 kHz audio" {
		t.Fatalf("tmr: %s", msg.TransmissionMedium)
	}
	if !strings.Contains(msg.Summary, "632107608") || !strings.Contains(msg.Summary, "8780637304473") {
		t.Fatalf("summary: %s", msg.Summary)
	}
}

func TestDecodeREL(t *testing.T) {
	msg := Decode(sampleREL())
	if msg.Error != "" {
		t.Fatalf("error: %s", msg.Error)
	}
	if msg.Acronym != "REL" || msg.Cause == nil || msg.Cause.Value != 16 {
		t.Fatalf("rel: %#v", msg)
	}
	if msg.Cause.Text != "Normal call clearing" {
		t.Fatalf("cause text: %s", msg.Cause.Text)
	}
	if !strings.Contains(msg.Cause.Location, "public network serving the local user") {
		t.Fatalf("location: %s", msg.Cause.Location)
	}
}

func TestPreserveRoundTripSinglePart(t *testing.T) {
	body := sampleIAM()
	original := sipInvite(body, "application/isup;version=itu-t92+")
	if !unsafeUTF8(original) {
		t.Fatal("sample should contain NUL")
	}
	stored := PreserveBinarySIPPayload(original)
	if unsafeUTF8(stored) || !utf8.ValidString(stored) {
		t.Fatal("stored payload is not valid UTF-8")
	}
	if strings.IndexByte(stored, 0) >= 0 {
		t.Fatal("stored payload still contains NUL")
	}
	if !strings.Contains(stored, "X-Homer-Binary-Body: hex") {
		t.Fatalf("marker missing:\n%s", stored)
	}
	if !strings.Contains(stored, "Content-Type: application/isup;version=itu-t92+") {
		t.Fatal("content type was rewritten")
	}
	if RestoreBinarySIPPayload(stored) != original {
		t.Fatalf("restore mismatch\noriginal:\n%q\nrestored:\n%q", original, RestoreBinarySIPPayload(stored))
	}
	got := DecodeSIP(stored)
	if got == nil || len(got.Messages) != 1 {
		t.Fatalf("decode: %#v", got)
	}
	if got.Version != "itu-t92+" {
		t.Fatalf("version %q", got.Version)
	}
	if got.Messages[0].CalledPartyNumber.Digits != "632107608" {
		t.Fatalf("digits: %#v", got.Messages[0].CalledPartyNumber)
	}
}

func TestPreserveRoundTripMultipart(t *testing.T) {
	iam := sampleIAM()
	// Trailing CRLF inside the ISUP body must stay part of the body, not the boundary.
	iam = append(iam, '\r', '\n')
	var body strings.Builder
	body.WriteString("--sipb\r\n")
	body.WriteString("Content-Type: application/sdp\r\n\r\n")
	body.WriteString("v=0\r\n")
	body.WriteString("\r\n--sipb\r\n")
	body.WriteString("Content-Type: application/isup;version=itu-t92+\r\n")
	body.WriteString("Content-Disposition: signal;handling=required\r\n\r\n")
	body.Write(iam)
	body.WriteString("\r\n--sipb--")
	original := sipInvite([]byte(body.String()), `multipart/mixed;boundary=sipb`)
	stored := PreserveBinarySIPPayload(original)
	if !strings.Contains(stored, "v=0") {
		t.Fatal("SDP part was encoded")
	}
	if strings.IndexByte(stored, 0) >= 0 || !utf8.ValidString(stored) {
		t.Fatal("multipart payload is not valid UTF-8")
	}
	if RestoreBinarySIPPayload(stored) != original {
		t.Fatalf("multipart restore mismatch\noriginal %q\nrestored %q", original, RestoreBinarySIPPayload(stored))
	}
	got := DecodeSIP(stored)
	if got == nil || len(got.Messages) != 1 || got.Messages[0].CallingPartyNumber == nil {
		t.Fatalf("multipart decode: %#v", got)
	}
	if got.Messages[0].CallingPartyNumber.Digits != "8780637304473" {
		t.Fatalf("calling %s", got.Messages[0].CallingPartyNumber.Digits)
	}
}

func TestPreserveLeavesTextSIP(t *testing.T) {
	msg := sipInvite([]byte("v=0\r\n"), "application/sdp")
	if PreserveBinarySIPPayload(msg) != msg {
		t.Fatal("text SIP was rewritten")
	}
	if DecodeSIP(msg) != nil {
		t.Fatal("SDP was treated as ISUP")
	}
	if RestoreBinarySIPPayload(msg) != msg {
		t.Fatal("text SIP restore changed the message")
	}
}

func TestDecodeSIPRawBinary(t *testing.T) {
	msg := sipInvite(sampleREL(), "application/isup;version=itu-t92+")
	got := DecodeSIP(msg)
	if got == nil || len(got.Messages) != 1 || got.Messages[0].Cause == nil || got.Messages[0].Cause.Value != 16 {
		t.Fatalf("raw: %#v", got)
	}
}

func TestDecodeANSINotDecoded(t *testing.T) {
	msg := sipInvite(sampleIAM(), "application/isup;version=ansi")
	got := DecodeSIP(msg)
	if got == nil || len(got.Messages) != 1 || got.Messages[0].Error == "" {
		t.Fatalf("ansi: %#v", got)
	}
}

func sipInvite(body []byte, contentType string) string {
	return "INVITE sip:100@example.com SIP/2.0\r\n" +
		"Via: SIP/2.0/UDP 10.0.0.1;branch=z9hG4bK1\r\n" +
		"From: <sip:a@example.com>;tag=1\r\n" +
		"To: <sip:b@example.com>\r\n" +
		"Call-ID: isup-call@example.com\r\n" +
		"CSeq: 1 INVITE\r\n" +
		"Content-Type: " + contentType + "\r\n" +
		"Content-Length: " + strconv.Itoa(len(body)) + "\r\n" +
		"\r\n" +
		string(body)
}
