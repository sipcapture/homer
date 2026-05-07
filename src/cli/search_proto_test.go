package cli

import "testing"

func TestParseSearchProto(t *testing.T) {
	tests := []struct {
		in   string
		want int
	}{
		{"sip", 1},
		{"SIP", 1},
		{"1", 1},
		{"rtcp", 5},
		{"5", 5},
		{"rtp", 34},
		{"dns", 35},
		{"log", 100},
		{"logs", 100},
		{"otlp_traces", 200},
		{"OTLP_TRACES", 200},
		{"otlp-traces", 200},
		{"otlp traces", 200},
		{"otlp_metrics", 201},
		{"otlp_logs", 202},
		{"lp", 300},
		{"line_protocol", 300},
		{"42", 42},
	}
	for _, tt := range tests {
		got, err := ParseSearchProto(tt.in)
		if err != nil {
			t.Fatalf("ParseSearchProto(%q): %v", tt.in, err)
		}
		if got != tt.want {
			t.Fatalf("ParseSearchProto(%q) = %d, want %d", tt.in, got, tt.want)
		}
	}
}

func TestParseSearchProto_errors(t *testing.T) {
	for _, in := range []string{"", "nosuch", "otlp"} {
		if _, err := ParseSearchProto(in); err == nil {
			t.Fatalf("ParseSearchProto(%q): want error", in)
		}
	}
}

func TestFormatSearchProtoDisplay(t *testing.T) {
	if g := FormatSearchProtoDisplay(200); g != "otlp_traces" {
		t.Fatalf("got %q", g)
	}
	if g := FormatSearchProtoDisplay(99); g != "99" {
		t.Fatalf("got %q", g)
	}
}
