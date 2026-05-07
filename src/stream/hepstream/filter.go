// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package hepstream

import (
	"net/url"
	"strconv"
	"strings"
)

// Filter narrows the stream of events a subscriber sees.
//
// The zero value matches everything with payload stripped — that is the
// "safe default" for a fresh subscription. Filters are evaluated on the
// publisher side (so a picky filter does not even occupy a slot in the
// subscriber's queue when the event doesn't match).
//
// All slice-valued fields use OR semantics (event matches if any entry
// matches); combining multiple field constraints uses AND semantics
// (event must satisfy every populated field).
type Filter struct {
	// Protos limits the stream to the listed HEP proto_type values.
	// Empty means "any proto". Methods below is only applied when
	// Protos is empty or contains 1 (SIP), since Methods only make
	// sense for SIP.
	Protos []uint32

	// Methods limits SIP events to the listed SIP methods. Matching is
	// case-insensitive on the decoded CseqMethod string. Empty means
	// "any method". Non-SIP events are never filtered out by Methods.
	Methods []string

	// OnlyRequests drops SIP responses (first line = "SIP/2.0 …").
	// Useful for the Packet Defender use case where we want INVITE /
	// REGISTER to mix with the game's bad packets, not 100 Trying.
	OnlyRequests bool

	// IncludePayload passes the raw SIP/HEP payload through to the
	// subscriber's wire frame. Authorisation (admin-only) is enforced
	// by the HTTP handler before constructing the Filter — the broker
	// itself is oblivious to roles.
	IncludePayload bool
}

// Match reports whether evt satisfies every populated constraint of f.
// Match is hot-path code: it is called once per subscriber per event in
// Broker.Publish, so it deliberately avoids allocations.
func (f Filter) Match(evt Event) bool {
	if len(f.Protos) > 0 {
		ok := false
		for _, p := range f.Protos {
			if p == evt.Proto {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}

	// Method and OnlyRequests are only meaningful for SIP. Skip them
	// for other protocols so a non-SIP event isn't silently filtered
	// away when the caller passes `?method=INVITE`.
	if evt.Proto == 1 {
		if f.OnlyRequests && evt.SIP != nil && evt.SIP.RespCode != "" {
			return false
		}
		if len(f.Methods) > 0 {
			if evt.SIP == nil {
				return false
			}
			ok := false
			for _, m := range f.Methods {
				if strings.EqualFold(m, evt.SIP.Method) {
					ok = true
					break
				}
			}
			if !ok {
				return false
			}
		}
	}

	return true
}

// FilterFromQuery parses the subset of URL query parameters the HEP
// stream endpoint understands. It accepts:
//
//	proto=1&proto=5              repeated OR-filter on ProtoType
//	method=INVITE&method=BYE     repeated OR-filter on SIP method
//	only_requests=1              "1" / "true" drop responses
//	include_payload=1            only honoured by the caller if admin
//
// Unknown query keys are ignored; unparseable proto values are ignored
// as well (we do not want to 400 the whole subscription because one
// noisy client tacked on a typo).
func FilterFromQuery(q url.Values) Filter {
	f := Filter{}
	for _, raw := range q["proto"] {
		if n, err := strconv.ParseUint(raw, 10, 32); err == nil {
			f.Protos = append(f.Protos, uint32(n))
		}
	}
	for _, raw := range q["method"] {
		raw = strings.TrimSpace(raw)
		if raw != "" {
			f.Methods = append(f.Methods, raw)
		}
	}
	if isTrueish(q.Get("only_requests")) {
		f.OnlyRequests = true
	}
	if isTrueish(q.Get("include_payload")) {
		f.IncludePayload = true
	}
	return f
}

// ToQuery serialises the filter back into a URL query string suitable
// for forwarding the subscription from coordinator to node. Kept in
// lockstep with FilterFromQuery.
func (f Filter) ToQuery() url.Values {
	q := url.Values{}
	for _, p := range f.Protos {
		q.Add("proto", strconv.FormatUint(uint64(p), 10))
	}
	for _, m := range f.Methods {
		q.Add("method", m)
	}
	if f.OnlyRequests {
		q.Set("only_requests", "1")
	}
	if f.IncludePayload {
		q.Set("include_payload", "1")
	}
	return q
}

func isTrueish(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "t", "true", "y", "yes", "on":
		return true
	default:
		return false
	}
}
