// Copyright (C) 2026 Homer Server Contributors
// SPDX-License-Identifier: AGPL-3.0-or-later

package services

import (
	"encoding/json"
	"testing"
)

func TestBuildLPFieldsMapping_Empty(t *testing.T) {
	got, err := BuildLPFieldsMapping(nil)
	if err != nil {
		t.Fatalf("BuildLPFieldsMapping(nil): %v", err)
	}
	if string(got) != "[]" {
		t.Fatalf("expected empty array, got %q", string(got))
	}
}

func TestBuildLPFieldsMapping_TypeRouting(t *testing.T) {
	cols := []LPColumn{
		{Name: "time", DataType: "TIMESTAMP", Position: 1},
		{Name: "host", DataType: "VARCHAR", Position: 2},
		{Name: "usage_idle", DataType: "DOUBLE", Position: 3},
		{Name: "is_up", DataType: "BOOLEAN", Position: 4},
		{Name: "samples", DataType: "BIGINT", Position: 5},
		{Name: "raw", DataType: "JSON", Position: 6},
	}
	out, err := BuildLPFieldsMapping(cols)
	if err != nil {
		t.Fatalf("BuildLPFieldsMapping: %v", err)
	}
	var parsed []map[string]any
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, out)
	}
	if len(parsed) != 6 {
		t.Fatalf("expected 6 entries, got %d", len(parsed))
	}

	// time: must be primary + sid_type, datetime form, label "Time"
	timeRow := parsed[0]
	if timeRow["id"] != "time" || timeRow["form_type"] != "datetime" || timeRow["sid_type"] != true {
		t.Fatalf("time row wrong: %+v", timeRow)
	}
	if timeRow["index"] != "primary" {
		t.Fatalf("time row should be primary index, got %+v", timeRow["index"])
	}

	// host (VARCHAR) → string/input, not hidden
	if parsed[1]["id"] != "host" || parsed[1]["form_type"] != "input" || parsed[1]["type"] != "string" {
		t.Fatalf("host row wrong: %+v", parsed[1])
	}
	// usage_idle (DOUBLE) → number/input
	if parsed[2]["form_type"] != "input" || parsed[2]["type"] != "number" {
		t.Fatalf("usage_idle row wrong: %+v", parsed[2])
	}
	// is_up (BOOLEAN) → boolean/switch
	if parsed[3]["form_type"] != "switch" || parsed[3]["type"] != "boolean" {
		t.Fatalf("is_up row wrong: %+v", parsed[3])
	}
	// samples (BIGINT) → number
	if parsed[4]["type"] != "number" {
		t.Fatalf("samples row wrong: %+v", parsed[4])
	}
	// raw column must be hidden by default to keep the picker clean
	if parsed[5]["id"] != "raw" || parsed[5]["hide"] != true {
		t.Fatalf("raw row should be hidden, got %+v", parsed[5])
	}
}

func TestBuildLPFieldsMapping_PrettifiesNames(t *testing.T) {
	cols := []LPColumn{{Name: "user_id", DataType: "VARCHAR", Position: 1}}
	out, _ := BuildLPFieldsMapping(cols)
	var parsed []map[string]any
	_ = json.Unmarshal(out, &parsed)
	if got := parsed[0]["name"]; got != "User id" {
		t.Fatalf("user_id should be prettified to 'User id', got %v", got)
	}
}

func TestBuildLPFieldsMapping_SkipsBlankColumnNames(t *testing.T) {
	cols := []LPColumn{
		{Name: "", DataType: "VARCHAR", Position: 1},
		{Name: "value", DataType: "DOUBLE", Position: 2},
	}
	out, _ := BuildLPFieldsMapping(cols)
	var parsed []map[string]any
	_ = json.Unmarshal(out, &parsed)
	if len(parsed) != 1 || parsed[0]["id"] != "value" {
		t.Fatalf("expected only 'value' to survive, got %+v", parsed)
	}
	// position must be re-numbered starting at 1, not preserve the
	// original gap that came from the dropped row
	if int(parsed[0]["position"].(float64)) != 1 {
		t.Fatalf("position not renumbered: %+v", parsed[0])
	}
}

func TestNormaliseDuckType(t *testing.T) {
	cases := map[string]string{
		"TIMESTAMP":              "TIMESTAMP",
		"timestamp":              "TIMESTAMP",
		"TIMESTAMP WITH TIME ZONE": "TIMESTAMP",
		"DECIMAL(18,4)":          "DECIMAL",
		"VARCHAR[]":              "VARCHAR",
		"BOOL":                   "BOOLEAN",
		"INT4":                   "INTEGER",
	}
	for in, want := range cases {
		if got := normaliseDuckType(in); got != want {
			t.Errorf("normaliseDuckType(%q) = %q, want %q", in, got, want)
		}
	}
}
