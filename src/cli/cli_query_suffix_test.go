package cli

import "testing"

func TestParseCLIQueryLine(t *testing.T) {
	cases := []struct {
		in       string
		wantSQL  string
		vertical bool
	}{
		{"SELECT 1\\G", "SELECT 1", true},
		{"SELECT 1\\g", "SELECT 1", false},
		{"SELECT 1;", "SELECT 1", false},
		{"SELECT 1\\G;", "SELECT 1", true},
	}
	for _, c := range cases {
		sql, vert := parseCLIQueryLine(c.in)
		if sql != c.wantSQL || vert != c.vertical {
			t.Fatalf("parseCLIQueryLine(%q) = (%q, %v), want (%q, %v)",
				c.in, sql, vert, c.wantSQL, c.vertical)
		}
	}
}
