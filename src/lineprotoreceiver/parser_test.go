// Copyright (C) 2026 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later
//
// Adapted from hepic-lake/src/writer/lineproto_parser_test.go.

package lineprotoreceiver

import (
	"reflect"
	"testing"
)

func TestParseLineProtocol_Minimal(t *testing.T) {
	pts, bad, err := ParseLineProtocol([]byte("cpu value=0.64"), PrecisionNanoseconds)
	if err != nil {
		t.Fatalf("unexpected error: %v (bad=%d)", err, bad)
	}
	if len(pts) != 1 {
		t.Fatalf("expected 1 point, got %d", len(pts))
	}
	p := pts[0]
	if p.Measurement != "cpu" {
		t.Errorf("measurement: got %q, want cpu", p.Measurement)
	}
	if v, ok := p.Fields["value"].(float64); !ok || v != 0.64 {
		t.Errorf("value field: got %#v, want 0.64 (float64)", p.Fields["value"])
	}
	if p.TimestampNs != 0 {
		t.Errorf("expected no timestamp, got %d", p.TimestampNs)
	}
}

func TestParseLineProtocol_TagsFieldsTimestamp(t *testing.T) {
	in := []byte(`cpu,host=a,region=eu-west usage_user=42.5,usage_sys=3i,active=t 1700000000000000000`)
	pts, _, err := ParseLineProtocol(in, PrecisionNanoseconds)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	p := pts[0]
	wantTags := map[string]string{"host": "a", "region": "eu-west"}
	if !reflect.DeepEqual(p.Tags, wantTags) {
		t.Errorf("tags: got %v, want %v", p.Tags, wantTags)
	}
	if v := p.Fields["usage_user"].(float64); v != 42.5 {
		t.Errorf("usage_user: got %v, want 42.5", v)
	}
	if v := p.Fields["usage_sys"].(int64); v != 3 {
		t.Errorf("usage_sys: got %v, want 3", v)
	}
	if v := p.Fields["active"].(bool); !v {
		t.Errorf("active: got %v, want true", v)
	}
	if p.TimestampNs != 1700000000000000000 {
		t.Errorf("ts: got %d", p.TimestampNs)
	}
}

func TestParseLineProtocol_QuotedStringField(t *testing.T) {
	in := []byte(`events,src=web msg="hello, world \"quoted\"",level="info"`)
	pts, _, err := ParseLineProtocol(in, PrecisionNanoseconds)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	got := pts[0].Fields["msg"].(string)
	want := `hello, world "quoted"`
	if got != want {
		t.Errorf("msg: got %q, want %q", got, want)
	}
	if lvl, _ := pts[0].Fields["level"].(string); lvl != "info" {
		t.Errorf("level: got %v, want info", lvl)
	}
}

func TestParseLineProtocol_EscapedMeasurementAndTag(t *testing.T) {
	in := []byte(`my\,measure,host\=a=srv\ 1 value=1i`)
	pts, _, err := ParseLineProtocol(in, PrecisionNanoseconds)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	p := pts[0]
	if p.Measurement != "my,measure" {
		t.Errorf("measurement: got %q, want my,measure", p.Measurement)
	}
	if v, ok := p.Tags["host=a"]; !ok || v != "srv 1" {
		t.Errorf("tag host=a: got %q (tags=%v)", v, p.Tags)
	}
}

func TestParseLineProtocol_PrecisionSeconds(t *testing.T) {
	pts, _, err := ParseLineProtocol([]byte(`m v=1 1700000000`), PrecisionSeconds)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if pts[0].TimestampNs != 1700000000*1_000_000_000 {
		t.Errorf("ts: got %d, want %d", pts[0].TimestampNs, 1700000000*1_000_000_000)
	}
}

func TestParseLineProtocol_MultipleLinesSkipEmptyComments(t *testing.T) {
	pts, _, err := ParseLineProtocol([]byte("# comment\n\ncpu u=1\nmem u=2\n\n# trailing\n"), PrecisionNanoseconds)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(pts) != 2 {
		t.Fatalf("expected 2 points, got %d", len(pts))
	}
	if pts[0].Measurement != "cpu" || pts[1].Measurement != "mem" {
		t.Errorf("measurements: got %q, %q", pts[0].Measurement, pts[1].Measurement)
	}
}

func TestParseLineProtocol_ErrorCases(t *testing.T) {
	cases := []struct {
		name, in string
	}{
		{"missing field set", "cpu,host=a "},
		{"empty measurement", " v=1"},
		{"unterminated string", `e msg="oops`},
		{"malformed field", "cpu value=notanumber"},
		{"tag without =", "cpu,host v=1"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, _, err := ParseLineProtocol([]byte(c.in), PrecisionNanoseconds)
			if err == nil {
				t.Errorf("expected error for %q, got nil", c.in)
			}
		})
	}
}

func TestParseLineProtocol_UnsignedField(t *testing.T) {
	pts, _, err := ParseLineProtocol([]byte("sys counter=42u"), PrecisionNanoseconds)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if v := pts[0].Fields["counter"].(int64); v != 42 {
		t.Errorf("counter: got %v, want 42", v)
	}
}

func TestParseLineProtocol_BooleansAllForms(t *testing.T) {
	in := []byte("b a=t,b=T,c=true,d=True,e=TRUE,f=f,g=F,h=false,i=False,j=FALSE")
	pts, _, err := ParseLineProtocol(in, PrecisionNanoseconds)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	wantTrue := []string{"a", "b", "c", "d", "e"}
	wantFalse := []string{"f", "g", "h", "i", "j"}
	for _, k := range wantTrue {
		if v, ok := pts[0].Fields[k].(bool); !ok || !v {
			t.Errorf("%s: got %v, want true", k, pts[0].Fields[k])
		}
	}
	for _, k := range wantFalse {
		if v, ok := pts[0].Fields[k].(bool); !ok || v {
			t.Errorf("%s: got %v, want false", k, pts[0].Fields[k])
		}
	}
}

func TestParsePrecision(t *testing.T) {
	cases := map[string]LineProtoPrecision{
		"":            PrecisionNanoseconds,
		"ns":          PrecisionNanoseconds,
		"NS":          PrecisionNanoseconds,
		"us":          PrecisionMicroseconds,
		"µs":          PrecisionMicroseconds,
		"microsecond": PrecisionMicroseconds,
		"ms":          PrecisionMilliseconds,
		"s":           PrecisionSeconds,
		"second":      PrecisionSeconds,
		"bogus":       PrecisionNanoseconds,
	}
	for in, want := range cases {
		if got := ParsePrecision(in); got != want {
			t.Errorf("ParsePrecision(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSanitizeIdent(t *testing.T) {
	cases := map[string]string{
		"cpu":           "cpu",
		"cpu.usage":     "cpu_usage",
		"my-measure":    "my_measure",
		"123counter":    "_123counter",
		"":              "_",
		"with space":    "with_space",
		"http.req/2xx":  "http_req_2xx",
		"мой_измеритель": "______________",
	}
	for in, want := range cases {
		if got := SanitizeIdent(in); got != want {
			t.Errorf("SanitizeIdent(%q) = %q, want %q", in, got, want)
		}
	}
}
