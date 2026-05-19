package cli

import "testing"

func TestApplyCLISelectLimit(t *testing.T) {
	cases := []struct {
		in      string
		limited bool
		want    string
	}{
		{"SELECT * FROM t", true, "SELECT * FROM t LIMIT 10000"},
		{"SELECT * FROM t LIMIT 5", false, "SELECT * FROM t LIMIT 5"},
		{"SHOW TABLES", false, "SHOW TABLES"},
		{"WITH c AS (SELECT 1) SELECT * FROM c", true, "WITH c AS (SELECT 1) SELECT * FROM c LIMIT 10000"},
	}
	for _, c := range cases {
		q := c.in
		got := applyCLISelectLimit(&q)
		if got != c.limited {
			t.Fatalf("applyCLISelectLimit(%q) limited=%v want %v", c.in, got, c.limited)
		}
		if q != c.want {
			t.Fatalf("query %q => %q, want %q", c.in, q, c.want)
		}
	}
}
