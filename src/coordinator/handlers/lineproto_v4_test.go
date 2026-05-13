// Copyright (C) 2026 Homer Server Contributors
// SPDX-License-Identifier: AGPL-3.0-or-later

package handlers

import (
	"testing"
)

func TestIsSafeIdent(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"", false},
		{"lp_cpu", true},
		{"_under", true},
		{"a1", true},
		{"1abc", false},
		{"with space", false},
		{"semi;colon", false},
		{"quoted'literal", false},
		{"main", true},
	}
	for _, c := range cases {
		if got := isSafeIdent(c.in); got != c.want {
			t.Errorf("isSafeIdent(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestIsSafePrefix(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"", true}, // empty: exclude hep_proto_/otlp_/mem_hep_ (matches default table_prefix)
		{"lp_", true},
		{"123_", true}, // prefixes may start with digits
		{"a-b", false},
		{"a%", false}, // explicit guard against accidental wildcards
	}
	for _, c := range cases {
		if got := isSafePrefix(c.in); got != c.want {
			t.Errorf("isSafePrefix(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestFQNOf(t *testing.T) {
	cases := []struct {
		cat, sch, name, want string
	}{
		{"", "", "lp_cpu", "lp_cpu"},
		{"", "main", "lp_cpu", "main.lp_cpu"},
		{"hepic_lake", "main", "lp_cpu", "hepic_lake.main.lp_cpu"},
	}
	for _, c := range cases {
		if got := fqnOf(c.cat, c.sch, c.name); got != c.want {
			t.Errorf("fqnOf(%q,%q,%q) = %q, want %q", c.cat, c.sch, c.name, got, c.want)
		}
	}
}

func TestStringField(t *testing.T) {
	row := map[string]interface{}{
		"name": "  lp_cpu  ",
		"nil":  nil,
	}
	if got := stringField(row, "name"); got != "lp_cpu" {
		t.Errorf("stringField name = %q, want %q", got, "lp_cpu")
	}
	if got := stringField(row, "missing"); got != "" {
		t.Errorf("stringField missing = %q, want empty", got)
	}
	if got := stringField(row, "nil"); got != "" {
		t.Errorf("stringField nil = %q, want empty", got)
	}
}

func TestIntField(t *testing.T) {
	row := map[string]interface{}{
		"i":   7,
		"i32": int32(8),
		"i64": int64(9),
		"f":   12.0,
		"s":   "42",
		"nil": nil,
	}
	for _, k := range []string{"i", "i32", "i64", "f", "s"} {
		if got := intField(row, k); got == 0 {
			t.Errorf("intField %q = 0, want non-zero", k)
		}
	}
	if got := intField(row, "missing"); got != 0 {
		t.Errorf("intField missing = %d, want 0", got)
	}
	if got := intField(row, "nil"); got != 0 {
		t.Errorf("intField nil = %d, want 0", got)
	}
}

// TestNewLineProtoHandlerPrefixPassthrough ensures the constructor keeps
// the default prefix string (including empty) for ?prefix= resolution.
func TestNewLineProtoHandlerPrefixPassthrough(t *testing.T) {
	h := NewLineProtoHandler(nil, "")
	if h == nil {
		t.Fatal("constructor returned nil")
	}
	if h.defaultPrefix != "" {
		t.Errorf("defaultPrefix = %q, want empty", h.defaultPrefix)
	}

	h2 := NewLineProtoHandler(nil, "metric_")
	if h2.defaultPrefix != "metric_" {
		t.Errorf("defaultPrefix = %q, want %q", h2.defaultPrefix, "metric_")
	}
}
