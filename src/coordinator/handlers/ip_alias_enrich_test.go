// Copyright (C) 2025 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package handlers

import (
	"context"
	"testing"
)

func TestFlattenRowCustomHeaders_stringDataExtra(t *testing.T) {
	row := map[string]interface{}{
		"method":     "INVITE",
		"data_extra": `{"user_agent":"UA","custom_headers":{"X-Call-Trace":"trace-1","X-Session-Id":"sess-9"}}`,
	}
	flattenRowCustomHeaders(row)
	if row["X-Call-Trace"] != "trace-1" {
		t.Fatalf("X-Call-Trace: got %#v", row["X-Call-Trace"])
	}
	if row["X-Session-Id"] != "sess-9" {
		t.Fatalf("X-Session-Id: got %#v", row["X-Session-Id"])
	}
	if row["method"] != "INVITE" {
		t.Fatalf("built-in method must stay unchanged: %#v", row["method"])
	}
}

func TestFlattenRowCustomHeaders_mapDataExtra(t *testing.T) {
	row := map[string]interface{}{
		"src_ip": "10.0.0.1",
		"data_extra": map[string]interface{}{
			"custom_headers": map[string]interface{}{
				"P-Charging-Vector": "icid-value=abc",
			},
		},
	}
	flattenRowCustomHeaders(row)
	if row["P-Charging-Vector"] != "icid-value=abc" {
		t.Fatalf("P-Charging-Vector: got %#v", row["P-Charging-Vector"])
	}
}

func TestFlattenRowCustomHeaders_doesNotOverwriteExisting(t *testing.T) {
	row := map[string]interface{}{
		"method": "INVITE",
		"data_extra": map[string]interface{}{
			"custom_headers": map[string]interface{}{
				"method":        "should-not-win",
				"X-Custom-Only": "ok",
			},
		},
	}
	flattenRowCustomHeaders(row)
	if row["method"] != "INVITE" {
		t.Fatalf("existing top-level key must not be overwritten: %#v", row["method"])
	}
	if row["X-Custom-Only"] != "ok" {
		t.Fatalf("non-colliding header should flatten: %#v", row["X-Custom-Only"])
	}
}

func TestFlattenRowCustomHeaders_skipsEmptyName(t *testing.T) {
	row := map[string]interface{}{
		"data_extra": map[string]interface{}{
			"custom_headers": map[string]interface{}{
				"":        "nope",
				"X-Valid": "yes",
			},
		},
	}
	flattenRowCustomHeaders(row)
	if _, has := row[""]; has {
		t.Fatal("empty header name must not create a top-level key")
	}
	if row["X-Valid"] != "yes" {
		t.Fatalf("X-Valid: got %#v", row["X-Valid"])
	}
}

func TestFlattenRowCustomHeaders_noopCases(t *testing.T) {
	flattenRowCustomHeaders(nil) // must not panic

	row := map[string]interface{}{"method": "BYE"}
	flattenRowCustomHeaders(row)
	if len(row) != 1 || row["method"] != "BYE" {
		t.Fatalf("missing data_extra must leave row unchanged: %#v", row)
	}

	row = map[string]interface{}{
		"data_extra": `{"user_agent":"UA"}`,
	}
	flattenRowCustomHeaders(row)
	if _, has := row["user_agent"]; has {
		t.Fatal("non-custom_headers fields must not be flattened")
	}

	row = map[string]interface{}{
		"data_extra": map[string]interface{}{
			"custom_headers": "not-a-map",
		},
	}
	flattenRowCustomHeaders(row)
	if len(row) != 1 {
		t.Fatalf("invalid custom_headers type must be ignored: %#v", row)
	}
}

func TestEnrichRowsWithIPAliases_flattensWithoutAliasService(t *testing.T) {
	h := &SearchHandler{} // aliasService nil
	rows := []map[string]interface{}{
		{
			"uuid":       "1",
			"data_extra": `{"custom_headers":{"X-icc_uuid":"icc-42"}}`,
		},
	}
	h.enrichRowsWithIPAliases(context.Background(), rows)
	if rows[0]["X-icc_uuid"] != "icc-42" {
		t.Fatalf("custom header flatten must run without alias service: %#v", rows[0]["X-icc_uuid"])
	}
}

func TestGetColumns_unionsKeysAcrossRows(t *testing.T) {
	if got := getColumns(nil); got != nil {
		t.Fatalf("empty input: got %#v", got)
	}
	if got := getColumns([]map[string]interface{}{}); got != nil {
		t.Fatalf("zero rows: got %#v", got)
	}

	results := []map[string]interface{}{
		{"uuid": "1", "method": "INVITE"},
		{"uuid": "2", "method": "200", "aliasSrc": "gw", "X-Call-Trace": "t1"},
		{"uuid": "3", "aliasDst": "peer"},
	}
	cols := getColumns(results)
	want := map[string]bool{
		"uuid": true, "method": true, "aliasSrc": true, "aliasDst": true, "X-Call-Trace": true,
	}
	if len(cols) != len(want) {
		t.Fatalf("column count: got %d (%v), want %d", len(cols), cols, len(want))
	}
	seen := make(map[string]bool, len(cols))
	for _, c := range cols {
		if !want[c] {
			t.Fatalf("unexpected column %q in %v", c, cols)
		}
		if seen[c] {
			t.Fatalf("duplicate column %q in %v", c, cols)
		}
		seen[c] = true
	}
}
