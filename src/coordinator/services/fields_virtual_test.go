// Copyright (C) 2026 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package services_test

import (
	"encoding/json"
	"testing"

	"github.com/sipcapture/homer-core/src/coordinator/services"
)

func TestVirtualRulesFromFieldsMapping(t *testing.T) {
	raw := json.RawMessage(`[
		{"id": "call_id", "name": "Call-ID"},
		{"id": "to_tag", "name": "To tag", "virtual": {"kind": "data_extra_json", "path": "to_tag", "match": "like"}}
	]`)
	rules, err := services.VirtualRulesFromFieldsMapping(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(rules))
	}
	r, ok := rules["to_tag"]
	if !ok {
		t.Fatal("missing to_tag rule")
	}
	if r.Kind != services.VirtualKindDataExtraJSON || r.Path != "to_tag" || r.Match != services.VirtualMatchLike {
		t.Fatalf("unexpected rule: %+v", r)
	}
}

func TestVirtualRulesFromFieldsMapping_InvalidKind(t *testing.T) {
	raw := json.RawMessage(`[{"id": "x", "virtual": {"kind": "drop_table", "path": "a"}}]`)
	_, err := services.VirtualRulesFromFieldsMapping(raw)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestVirtualRulesAbsentPresent(t *testing.T) {
	raw := json.RawMessage(`[
		{"id": "no_to_tag", "virtual": {"kind": "data_extra_json", "path": "to_tag", "match": "absent"}},
		{"id": "has_to_tag", "virtual": {"kind": "data_extra_json", "path": "to_tag", "match": "present"}}
	]`)
	rules, err := services.VirtualRulesFromFieldsMapping(raw)
	if err != nil {
		t.Fatal(err)
	}
	if rules["no_to_tag"].Match != services.VirtualMatchAbsent {
		t.Fatalf("no_to_tag: %+v", rules["no_to_tag"])
	}
	if rules["has_to_tag"].Match != services.VirtualMatchPresent {
		t.Fatalf("has_to_tag: %+v", rules["has_to_tag"])
	}
}
