// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

// Package hepstream exposes a tiny in-process publish/subscribe broker over
// decoded HEP messages. Producers (the ingest worker in src/server) call
// Broker.Publish; consumers subscribe via Broker.Subscribe and receive a
// stream of Event values until they cancel.
//
// Event is a pure-data type on purpose: it does not retain a reference to
// decoder.HEP so that the broker ring buffer cannot keep decoder pool
// objects alive, and so that serialising to JSON stays cheap and obvious.
package hepstream

import (
	"encoding/json"
	"time"

	"github.com/sipcapture/homer-core/src/decoder"
)

// SIPMeta captures the SIP-specific fields we care about for downstream
// consumers (Packet Defender, operator dashboards). It is only populated
// for ProtoType == 1 (SIP) when the decoder successfully parsed the
// message.
type SIPMeta struct {
	Method   string `json:"method,omitempty"`
	RespCode string `json:"resp_code,omitempty"`
	RespText string `json:"resp_text,omitempty"`
	CallID   string `json:"callid,omitempty"`
	CSeq     string `json:"cseq,omitempty"`
	FromUser string `json:"from_user,omitempty"`
	ToUser   string `json:"to_user,omitempty"`
	RURIUser string `json:"ruri_user,omitempty"`
}

// Event is the on-the-wire representation of a single HEP message emitted
// by the broker. Field names match the JSON frame format documented in
// docs/HEP_STREAM.md.
//
// Payload is populated only when the subscriber's filter requested it
// (IncludePayload=true) — the broker still fans out Event values with
// Payload set, and drops the field at serialisation time when the caller
// opted out. This keeps the ring buffer simple (one Event shape) at the
// cost of carrying the payload string in memory even when nobody asked
// for it; the publisher-side config (buffer_size) bounds that cost.
type Event struct {
	// TsMilli is the HEP capture timestamp in milliseconds since the
	// Unix epoch. We pick milliseconds because that is what the rest of
	// the v4 API already serves (see /api/v4/transactions/messages).
	TsMilli int64 `json:"ts"`

	// Proto mirrors decoder.HEP.ProtoType (e.g. 1 for SIP, 5 for RTCP).
	Proto uint32 `json:"proto"`

	SrcIP   string `json:"src_ip,omitempty"`
	SrcPort uint32 `json:"src_port,omitempty"`
	DstIP   string `json:"dst_ip,omitempty"`
	DstPort uint32 `json:"dst_port,omitempty"`

	// NodeID mirrors decoder.HEP.NodeID (the capture agent identifier).
	NodeID uint32 `json:"node_id,omitempty"`

	// SIP is omitted for non-SIP events or when SIP parsing failed.
	SIP *SIPMeta `json:"sip,omitempty"`

	// Payload is the raw text body of the HEP message. Subscribers
	// without IncludePayload see MarshalFor() strip this field.
	Payload string `json:"-"`
}

// FromHEP builds an Event from a decoded HEP message without holding any
// reference back to h. The decoder pool is free to recycle h immediately
// after FromHEP returns.
func FromHEP(h *decoder.HEP) Event {
	if h == nil {
		return Event{}
	}
	evt := Event{
		TsMilli: h.Timestamp.UnixMilli(),
		Proto:   h.ProtoType,
		SrcIP:   h.SrcIP,
		SrcPort: h.SrcPort,
		DstIP:   h.DstIP,
		DstPort: h.DstPort,
		NodeID:  h.NodeID,
		Payload: h.Payload,
	}
	// Fall back to wall-clock when the decoder left the HEP timestamp
	// at zero — otherwise the UI would show epoch 1970 events.
	if evt.TsMilli <= 0 {
		evt.TsMilli = time.Now().UnixMilli()
	}
	if h.ProtoType == 1 && h.SIP != nil {
		evt.SIP = &SIPMeta{
			Method:   h.SIP.CseqMethod,
			RespCode: h.SIP.FirstResp,
			RespText: h.SIP.FirstRespText,
			CallID:   h.SIP.CallID,
			CSeq:     h.SIP.CseqVal,
			FromUser: h.SIP.FromUser,
			ToUser:   h.SIP.ToUser,
			RURIUser: h.SIP.URIUser,
		}
		// For SIP requests the first line carries the method (INVITE,
		// REGISTER, …); CseqMethod mirrors that. For responses
		// CseqMethod still holds the underlying method (e.g. "INVITE"
		// for "100 Trying"), and the numeric status lives in
		// FirstResp. We leave both in place so the UI can colour
		// requests vs. responses as it sees fit.
		if evt.SIP.Method == "" {
			evt.SIP.Method = h.SIP.FirstMethod
		}
	}
	return evt
}

// MarshalFor serialises the event as a JSON object, honouring the
// subscriber's payload preference. Using a per-call helper avoids
// per-subscriber clones of the Event itself (which would defeat the
// point of passing Event by value through channels).
func (e Event) MarshalFor(includePayload bool) ([]byte, error) {
	if includePayload {
		return json.Marshal(frameWithPayload{
			TsMilli: e.TsMilli,
			Proto:   e.Proto,
			SrcIP:   e.SrcIP,
			SrcPort: e.SrcPort,
			DstIP:   e.DstIP,
			DstPort: e.DstPort,
			NodeID:  e.NodeID,
			SIP:     e.SIP,
			Payload: e.Payload,
		})
	}
	return json.Marshal(frameNoPayload{
		TsMilli: e.TsMilli,
		Proto:   e.Proto,
		SrcIP:   e.SrcIP,
		SrcPort: e.SrcPort,
		DstIP:   e.DstIP,
		DstPort: e.DstPort,
		NodeID:  e.NodeID,
		SIP:     e.SIP,
	})
}

// Twin frame structs so we can omit Payload cleanly without `json:"-"`
// leaking into the wire format. Keep them unexported — they are an
// implementation detail of MarshalFor.
type frameNoPayload struct {
	TsMilli int64    `json:"ts"`
	Proto   uint32   `json:"proto"`
	SrcIP   string   `json:"src_ip,omitempty"`
	SrcPort uint32   `json:"src_port,omitempty"`
	DstIP   string   `json:"dst_ip,omitempty"`
	DstPort uint32   `json:"dst_port,omitempty"`
	NodeID  uint32   `json:"node_id,omitempty"`
	SIP     *SIPMeta `json:"sip,omitempty"`
}

type frameWithPayload struct {
	TsMilli int64    `json:"ts"`
	Proto   uint32   `json:"proto"`
	SrcIP   string   `json:"src_ip,omitempty"`
	SrcPort uint32   `json:"src_port,omitempty"`
	DstIP   string   `json:"dst_ip,omitempty"`
	DstPort uint32   `json:"dst_port,omitempty"`
	NodeID  uint32   `json:"node_id,omitempty"`
	SIP     *SIPMeta `json:"sip,omitempty"`
	Payload string   `json:"payload,omitempty"`
}
