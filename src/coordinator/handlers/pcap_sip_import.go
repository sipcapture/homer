// Copyright (C) 2026 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package handlers

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcapgo"
	"github.com/sipcapture/homer-core/src/decoder"
	"github.com/sipcapture/homer-core/src/coordinator/services"
	"github.com/sipcapture/homer-core/src/storage/ducklake"
)

const (
	maxPcapImportBytes = 64 << 20
	pcapInsertBatch    = 80
)

type pcapImportOptions struct {
	OverrideToCurrentTime bool
	// ForceSIPSubtype, if non-empty, must be ducklake.SIPTypeCall / SIPTypeRegistration / SIPTypeDefault
	ForceSIPSubtype string
}

// parseForceSIPTable maps multipart values to a SIP DuckLake subtype (empty = auto).
// Accepts table nicknames such as hep_proto_1_call or 1_call.
func parseForceSIPTable(v string) (string, error) {
	s := strings.TrimSpace(strings.ToLower(v))
	if s == "" {
		return "", nil
	}
	switch s {
	case "call", "sip_call", "1_call", "hep_proto_1_call":
		return ducklake.SIPTypeCall, nil
	case "registration", "reg", "register", "1_registration", "hep_proto_1_registration":
		return ducklake.SIPTypeRegistration, nil
	case "default", "1_default", "hep_proto_1_default":
		return ducklake.SIPTypeDefault, nil
	default:
		return "", fmt.Errorf("force_sip_table: unknown value %q (use call, registration, or default)", v)
	}
}

func sipPayloadLooksLikeSIP(payload string) bool {
	s := strings.TrimSpace(payload)
	if strings.HasPrefix(s, "SIP/2.0") {
		return true
	}
	for _, m := range []string{
		"INVITE ", "ACK ", "BYE ", "CANCEL ", "REGISTER ", "OPTIONS ", "PRACK ",
		"UPDATE ", "INFO ", "SUBSCRIBE ", "NOTIFY ", "REFER ", "PUBLISH ", "MESSAGE ",
	} {
		if strings.HasPrefix(s, m) {
			return true
		}
	}
	return false
}

func sipPayloadLooksComplete(payload string) bool {
	return strings.Contains(payload, "\r\n\r\n") || strings.Contains(payload, "\n\n")
}

func extractTransport(pkt gopacket.Packet) (srcIP, dstIP string, srcPort, dstPort uint16, ipProto uint32, payload []byte, ok bool) {
	nl := pkt.NetworkLayer()
	if nl == nil {
		return "", "", 0, 0, 0, nil, false
	}
	switch v := nl.(type) {
	case *layers.IPv4:
		srcIP, dstIP = v.SrcIP.String(), v.DstIP.String()
	case *layers.IPv6:
		srcIP, dstIP = v.SrcIP.String(), v.DstIP.String()
	default:
		return "", "", 0, 0, 0, nil, false
	}

	if tl := pkt.TransportLayer(); tl != nil {
		switch v := tl.(type) {
		case *layers.UDP:
			return srcIP, dstIP, uint16(v.SrcPort), uint16(v.DstPort), 17, v.Payload, true
		case *layers.TCP:
			p := v.Payload
			if len(p) == 0 {
				return "", "", 0, 0, 0, nil, false
			}
			return srcIP, dstIP, uint16(v.SrcPort), uint16(v.DstPort), 6, p, true
		default:
			return "", "", 0, 0, 0, nil, false
		}
	}
	return "", "", 0, 0, 0, nil, false
}

func newPcapIterator(buf []byte) (next func() ([]byte, gopacket.CaptureInfo, error), linkType layers.LinkType, err error) {
	if len(buf) < 24 {
		return nil, 0, fmt.Errorf("pcap buffer too small")
	}
	// PCAPNG section header block type
	if binary.LittleEndian.Uint32(buf[0:4]) == 0x0A0D0D0A {
		r, err := pcapgo.NewNgReader(bytes.NewReader(buf), pcapgo.DefaultNgReaderOptions)
		if err != nil {
			return nil, 0, err
		}
		lt := r.LinkType()
		return func() ([]byte, gopacket.CaptureInfo, error) { return r.ReadPacketData() }, lt, nil
	}
	r, err := pcapgo.NewReader(bytes.NewReader(buf))
	if err != nil {
		return nil, 0, err
	}
	lt := r.LinkType()
	return func() ([]byte, gopacket.CaptureInfo, error) { return r.ReadPacketData() }, lt, nil
}

func importPcapSIP(
	ctx context.Context,
	flight *services.FlightService,
	lakeName string,
	raw []byte,
	opts pcapImportOptions,
) (inserted, rejected int, err error) {
	if len(raw) == 0 {
		return 0, 0, fmt.Errorf("empty pcap")
	}
	if len(raw) > maxPcapImportBytes {
		return 0, 0, fmt.Errorf("pcap exceeds max size (%d bytes)", maxPcapImportBytes)
	}

	next, lt, err := newPcapIterator(raw)
	if err != nil {
		return 0, 0, fmt.Errorf("pcap open: %w", err)
	}

	dec := decoder.NewDecoder(&decoder.DecoderConfig{})
	var firstCap time.Time
	var nowBase time.Time
	remap := func(ts time.Time) time.Time {
		if !opts.OverrideToCurrentTime {
			return ts
		}
		if firstCap.IsZero() {
			firstCap = ts.UTC()
			nowBase = time.Now().UTC()
			return nowBase
		}
		return nowBase.Add(ts.Sub(firstCap))
	}

	batches := make(map[ducklake.TableKey][][]interface{})

	flushKey := func(key ducklake.TableKey) error {
		rows := batches[key]
		if len(rows) == 0 {
			return nil
		}
		sql, err := ducklake.BuildInsertMultiValues(lakeName, key, rows)
		if err != nil {
			return err
		}
		if err := flight.ExecFirstConnected(ctx, sql); err != nil {
			return err
		}
		inserted += len(rows)
		batches[key] = batches[key][:0]
		return nil
	}

	for {
		data, ci, err := next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return inserted, rejected, fmt.Errorf("read packet: %w", err)
		}
		pkt := gopacket.NewPacket(data, lt, gopacket.Default)
		srcIP, dstIP, sp, dp, ipProto, pay, ok := extractTransport(pkt)
		if !ok || len(pay) == 0 {
			rejected++
			continue
		}
		pl := string(pay)
		// Many traces use LF-only; sipparser expects CRLF framing.
		if !strings.Contains(pl, "\r\n") {
			pl = strings.ReplaceAll(pl, "\n", "\r\n")
		}
		if !sipPayloadLooksLikeSIP(pl) || !sipPayloadLooksComplete(pl) {
			rejected++
			continue
		}

		ts := remap(ci.Timestamp)
		h := &decoder.HEP{}
		if err := decoder.ApplyCapturedSIP(dec, h, srcIP, dstIP, uint32(sp), uint32(dp), ipProto, pl, ts, 0); err != nil {
			rejected++
			continue
		}

		var key ducklake.TableKey
		var vals []interface{}
		var convErr error
		if opts.ForceSIPSubtype != "" {
			key, vals, convErr = ducklake.ConvertHEPToLakeRowSIPForced(h, opts.ForceSIPSubtype)
		} else {
			key, vals, convErr = ducklake.ConvertHEPToLakeRow(h)
		}
		if convErr != nil {
			rejected++
			continue
		}

		batches[key] = append(batches[key], vals)
		if len(batches[key]) >= pcapInsertBatch {
			if err := flushKey(key); err != nil {
				return inserted, rejected, err
			}
		}
	}

	for k := range batches {
		if err := flushKey(k); err != nil {
			return inserted, rejected, err
		}
	}

	return inserted, rejected, nil
}
