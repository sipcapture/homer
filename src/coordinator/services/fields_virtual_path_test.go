// Copyright (C) 2026 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package services

import (
	"encoding/json"
	"testing"
)

func TestVirtualRulesFromFieldsMapping_HyphenatedPath(t *testing.T) {
	raw := json.RawMessage(`[
		{"id": "x_icc_uuid", "virtual": {"kind": "data_extra_json", "path": "custom_headers.X-icc_uuid", "match": "equals"}}
	]`)
	rules, err := VirtualRulesFromFieldsMapping(raw)
	if err != nil {
		t.Fatal(err)
	}
	r, ok := rules["x_icc_uuid"]
	if !ok {
		t.Fatal("missing x_icc_uuid rule")
	}
	if r.Path != "custom_headers.X-icc_uuid" {
		t.Fatalf("path: got %q", r.Path)
	}
}

func TestDuckJSONPath_HyphenatedSegment(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"to_tag", "$.to_tag"},
		{"custom_headers.X-icc_uuid", `$.custom_headers."X-icc_uuid"`},
		{"custom_headers.P-Charging-Vector", `$.custom_headers."P-Charging-Vector"`},
		{"foo.bar", "$.foo.bar"},
	}
	for _, tt := range tests {
		if got := DuckJSONPath(tt.path); got != tt.want {
			t.Errorf("DuckJSONPath(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

func TestValidateVirtualJSONPath_RejectsUnsafeSegments(t *testing.T) {
	for _, path := range []string{
		`custom_headers.foo"bar`,
		`custom_headers.foo'bar`,
		`custom_headers..x`,
		`.leading`,
	} {
		if err := validateVirtualJSONPath(path); err == nil {
			t.Errorf("expected error for path %q", path)
		}
	}
}
