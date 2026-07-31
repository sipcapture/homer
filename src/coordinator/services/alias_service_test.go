// Copyright (C) 2026 Homer Server Contributors
// SPDX-License-Identifier: AGPL-3.0-or-later

package services

import "testing"

// TestToBoolValue_AllNumericTypes regresses the canonical
// "active_rows=N loaded_prefixes=0" bug shipped in 11.0.122: the
// duckdb-go driver returns INTEGER columns as int32, but the original
// switch only covered int / int64 / float64, so every alias.status=1
// row ended up looking inactive after mapRowToAlias.
//
// Cover every scalar shape the driver might emit so a future
// regression — e.g. "we upgraded the driver and now it returns
// uint32" — fails this test instead of silently disabling the
// IPAliasMap LPM table in production.
func TestToBoolValue_AllNumericTypes(t *testing.T) {
	type tc struct {
		name string
		in   any
		want bool
	}
	cases := []tc{
		// nil / unknown
		{"nil", nil, false},
		{"struct (default)", struct{ X int }{1}, false},

		// bool
		{"bool true", true, true},
		{"bool false", false, false},

		// signed ints
		{"int(0)", int(0), false},
		{"int(1)", int(1), true},
		{"int8(1)", int8(1), true},
		{"int16(1)", int16(1), true},
		{"int32(1) [duckdb INTEGER]", int32(1), true},
		{"int32(0)", int32(0), false},
		{"int64(1)", int64(1), true},

		// unsigned ints
		{"uint(1)", uint(1), true},
		{"uint8(1)", uint8(1), true},
		{"uint16(1)", uint16(1), true},
		{"uint32(1)", uint32(1), true},
		{"uint64(1)", uint64(1), true},
		{"uint64(0)", uint64(0), false},

		// floats
		{"float32(1.0)", float32(1), true},
		{"float64(0.0)", float64(0), false},

		// strings (canonical truthy/falsy forms)
		{"string '1'", "1", true},
		{"string 'true'", "true", true},
		{"string 'TRUE'", "TRUE", true},
		{"string '  yes  '", "  yes  ", true},
		{"string 'on'", "on", true},
		{"string 'false'", "false", false},
		{"string '0'", "0", false},
		{"string ''", "", false},
		{"string 'banana'", "banana", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := toBoolValue(c.in); got != c.want {
				t.Fatalf("toBoolValue(%v) = %v, want %v", c.in, got, c.want)
			}
		})
	}
}

// TestMapRowToAlias_StatusFromInt32 is the end-to-end regression for
// the "active_rows=1 loaded_prefixes=0" report: an active alias row
// (status=1 in DuckDB) must round-trip through mapRowToAlias as
// Status=true, regardless of which integer width the driver hands us.
func TestMapRowToAlias_StatusFromInt32(t *testing.T) {
	row := map[string]interface{}{
		"guid":       "abc",
		"alias":      "edge-1",
		"ip":         "10.1.68.0",
		"port":       int32(0),
		"mask":       int32(24),
		"capture_id": "",
		"status":     int32(1), // exact shape duckdb-go returns
	}
	got := mapRowToAlias(row)
	if !got.Status {
		t.Fatalf("Status=false from int32(1); upstream filter would skip a live alias. row=%+v", got)
	}
	if got.Mask != 24 || got.IP != "10.1.68.0" || got.Alias != "edge-1" {
		t.Fatalf("mapRowToAlias dropped fields: %+v", got)
	}
}

// TestRowGetStringCI_NilSQLNull regresses the broken <img src="<nil>"> /
// "<nil>" tag-chip bug: a present map key with a nil value (SQL NULL from
// DuckDB) must return ("", true), never fmt.Sprint(nil) == "<nil>".
func TestRowGetStringCI_NilSQLNull(t *testing.T) {
	row := map[string]interface{}{
		"custom_image": nil,
		"tag1":         nil,
		"alias":        "gw",
	}
	got, ok := rowGetStringCI(row, "custom_image")
	if !ok || got != "" {
		t.Fatalf("custom_image nil: got (%q, %v), want (\"\", true)", got, ok)
	}
	got, ok = rowGetStringCI(row, "tag1")
	if !ok || got != "" {
		t.Fatalf("tag1 nil: got (%q, %v), want (\"\", true)", got, ok)
	}
	got, ok = rowGetStringCI(row, "alias")
	if !ok || got != "gw" {
		t.Fatalf("alias string: got (%q, %v), want (\"gw\", true)", got, ok)
	}
	got, ok = rowGetStringCI(row, "missing")
	if ok || got != "" {
		t.Fatalf("missing key: got (%q, %v), want (\"\", false)", got, ok)
	}
}

func TestMapRowToAlias_NullOptionalFields(t *testing.T) {
	row := map[string]interface{}{
		"guid":         "g1",
		"alias":        "edge-1",
		"ip":           "10.0.0.1",
		"port":         int32(5060),
		"mask":         int32(32),
		"status":       int32(1),
		"custom_image": nil,
		"tag1":         nil,
		"tag2":         nil,
		"tag3":         nil,
		"tag4":         nil,
	}
	got := mapRowToAlias(row)
	if got.CustomImage != "" || got.Tag1 != "" || got.Tag2 != "" || got.Tag3 != "" || got.Tag4 != "" {
		t.Fatalf("NULL optional fields must map to empty strings, got %+v", got)
	}
	if got.CustomImage == "<nil>" || got.Tag1 == "<nil>" {
		t.Fatal("must not serialize SQL NULL as the literal \"<nil>\"")
	}
}
