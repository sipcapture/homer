// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package hepstream

import (
	"net/url"
	"testing"
)

func TestFilterMatch(t *testing.T) {
	invite := Event{Proto: 1, SIP: &SIPMeta{Method: "INVITE"}}
	bye := Event{Proto: 1, SIP: &SIPMeta{Method: "BYE"}}
	resp := Event{Proto: 1, SIP: &SIPMeta{Method: "INVITE", RespCode: "200"}}
	rtcp := Event{Proto: 5}

	cases := []struct {
		name string
		f    Filter
		evt  Event
		want bool
	}{
		{"zero filter matches SIP", Filter{}, invite, true},
		{"zero filter matches RTCP", Filter{}, rtcp, true},
		{"proto=5 rejects SIP", Filter{Protos: []uint32{5}}, invite, false},
		{"proto=1,5 matches both", Filter{Protos: []uint32{1, 5}}, rtcp, true},
		{"method=INVITE matches INVITE", Filter{Methods: []string{"INVITE"}}, invite, true},
		{"method=INVITE rejects BYE", Filter{Methods: []string{"INVITE"}}, bye, false},
		{"method is case-insensitive", Filter{Methods: []string{"invite"}}, invite, true},
		{"method filter on RTCP is ignored", Filter{Methods: []string{"INVITE"}}, rtcp, true},
		{"only_requests drops response", Filter{OnlyRequests: true}, resp, false},
		{"only_requests keeps INVITE", Filter{OnlyRequests: true}, invite, true},
		{"proto+method combine as AND", Filter{Protos: []uint32{1}, Methods: []string{"BYE"}}, invite, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.f.Match(tc.evt); got != tc.want {
				t.Fatalf("Match=%v want %v", got, tc.want)
			}
		})
	}
}

func TestFilterFromQuery(t *testing.T) {
	q := url.Values{}
	q.Add("proto", "1")
	q.Add("proto", "5")
	q.Add("proto", "nope")
	q.Add("method", "INVITE")
	q.Add("method", "REGISTER")
	q.Set("only_requests", "true")
	q.Set("include_payload", "1")

	f := FilterFromQuery(q)
	if len(f.Protos) != 2 || f.Protos[0] != 1 || f.Protos[1] != 5 {
		t.Fatalf("protos=%v", f.Protos)
	}
	if len(f.Methods) != 2 {
		t.Fatalf("methods=%v", f.Methods)
	}
	if !f.OnlyRequests {
		t.Fatalf("OnlyRequests not parsed")
	}
	if !f.IncludePayload {
		t.Fatalf("IncludePayload not parsed")
	}
}

func TestFilterToQueryRoundtrip(t *testing.T) {
	orig := Filter{
		Protos:         []uint32{1, 5},
		Methods:        []string{"INVITE", "BYE"},
		OnlyRequests:   true,
		IncludePayload: true,
	}
	reparsed := FilterFromQuery(orig.ToQuery())
	if len(reparsed.Protos) != len(orig.Protos) {
		t.Fatalf("protos lost: %v vs %v", reparsed.Protos, orig.Protos)
	}
	if !reparsed.OnlyRequests || !reparsed.IncludePayload {
		t.Fatalf("flags lost: %+v", reparsed)
	}
}
