// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package handlers

import "testing"

func TestIsAllEventType(t *testing.T) {
	for _, v := range []string{"all", "ALL", " All ", "*"} {
		if !isAllEventType(v) {
			t.Fatalf("expected %q to be recognized as all-event-types", v)
		}
	}
	for _, v := range []string{"", "call", "registration", "default", "everything"} {
		if isAllEventType(v) {
			t.Fatalf("did not expect %q to be recognized as all-event-types", v)
		}
	}
}

func TestResolveSearchTables(t *testing.T) {
	got := resolveSearchTables("homer_lake", 1, "all")
	want := []string{
		"homer_lake.main.hep_proto_1_call",
		"homer_lake.main.hep_proto_1_registration",
		"homer_lake.main.hep_proto_1_default",
	}
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v want %v", got, want)
		}
	}

	// Non-SIP proto types have no call/registration/default split — "all"
	// must not fan out for them.
	single := resolveSearchTables("homer_lake", 5, "all")
	if len(single) != 1 {
		t.Fatalf("expected single table for non-SIP proto, got %v", single)
	}

	// A concrete event_type always resolves to a single table.
	single = resolveSearchTables("homer_lake", 1, "call")
	if len(single) != 1 || single[0] != "homer_lake.main.hep_proto_1_call" {
		t.Fatalf("unexpected single-table resolution: %v", single)
	}
}

func TestTransactionSearchWantsAllEventTypes(t *testing.T) {
	base := func() *SearchObjectV4 {
		req := &SearchObjectV4{}
		req.Filter.ProtoType = 1
		req.Filter.EventType = "all"
		return req
	}

	if !transactionSearchWantsAllEventTypes(base()) {
		t.Fatal("plain SIP top-N search with event_type=all must fan out")
	}

	req := base()
	req.Filter.ProtoType = 5 // RTCP — no call/registration/default split
	if transactionSearchWantsAllEventTypes(req) {
		t.Fatal("non-SIP proto_type must not fan out")
	}

	req = base()
	req.Filter.EventType = "call"
	if transactionSearchWantsAllEventTypes(req) {
		t.Fatal("a concrete event_type must not fan out")
	}

	req = base()
	req.Param.GroupBy = "method"
	if transactionSearchWantsAllEventTypes(req) {
		t.Fatal("aggregation queries cannot be merged across tables in Go and must not fan out")
	}

	req = base()
	req.Param.Select = "method, count(*) as cnt"
	if transactionSearchWantsAllEventTypes(req) {
		t.Fatal("custom projection queries must not fan out")
	}

	req = base()
	req.Param.OrderBy = "response_code ASC"
	if transactionSearchWantsAllEventTypes(req) {
		t.Fatal("non-default ordering must not fan out (merge only supports the default newest-first shape)")
	}
}

func TestGetTableNameAllEventTypeFallsBackToDefaultTable(t *testing.T) {
	// getTableName/normalizeSIPTransactionType do not know about "all" —
	// callers that support the merge (transactionSearchWantsAllEventTypes,
	// resolveSearchTables) must intercept it first. Any caller that still
	// passes "all" straight through (e.g. aggregation searches) keeps the
	// historical fallback: the single "default" table.
	got := getTableName("homer_lake", 1, "all")
	want := "homer_lake.main.hep_proto_1_default"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}
