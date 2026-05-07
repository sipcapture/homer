// Copyright (C) 2026 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package decoder

import (
	"time"
)

// ApplyCapturedSIP fills h with link metadata and SIP payload, then runs SIP parsing.
// Use for PCAP / offline ingest (not a HEP wire frame). d may be nil (DefaultDecoder).
func ApplyCapturedSIP(d *Decoder, h *HEP, srcIP, dstIP string, srcPort, dstPort, ipProto uint32, payload string, ts time.Time, nodeID uint32) error {
	if d == nil {
		d = DefaultDecoder
	}
	h.decoder = d
	h.Version = 2
	h.Protocol = ipProto
	h.SrcIP = srcIP
	h.DstIP = dstIP
	h.SrcPort = srcPort
	h.DstPort = dstPort
	t := ts.UTC()
	nano := t.UnixNano()
	sec := nano / 1e9
	if sec < 0 {
		sec = 0
	}
	h.Tsec = uint32(sec)
	h.Tmsec = uint32((nano % 1e9) / 1000)
	h.ProtoType = 1
	h.NodeID = nodeID
	h.Payload = payload
	return h.parseSIP()
}
