package cli

import (
	"strings"
	"testing"
)

func TestSQLCLICompleter_keywords(t *testing.T) {
	c := newSQLCLICompleter("homer_lake", "/data/homer/parquet", func() []string {
		return []string{"homer_lake.hep_proto_1_call"}
	})
	line := []rune("SEL")
	sugs, off := c.Do(line, len(line))
	if off != 3 {
		t.Fatalf("offset=%d want 3", off)
	}
	found := false
	for _, s := range sugs {
		if string(s) == "ECT" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected SELECT completion, got %v", runesToStrings(sugs))
	}
}

func TestSQLCLICompleter_fromTable(t *testing.T) {
	c := newSQLCLICompleter("homer_lake", "/data/p", func() []string {
		return []string{"homer_lake.hep_proto_1_call"}
	})
	line := []rune("SELECT * FROM hep_proto_1")
	sugs, _ := c.Do(line, len(line))
	joined := strings.Join(runesToStrings(sugs), ",")
	if !strings.Contains(joined, "_call") {
		t.Fatalf("expected table completion, got %q", joined)
	}
}

func TestSQLCLICompleter_pathInQuotes(t *testing.T) {
	c := newSQLCLICompleter("homer_lake", "/data/homer/parquet", func() []string {
		return []string{"homer_lake.hep_proto_1_call"}
	})
	line := []rune("SELECT * FROM read_parquet('/data/homer/par")
	pos := len(line)
	sugs, _ := c.Do(line, pos)
	if len(sugs) == 0 {
		t.Fatal("expected path completions")
	}
}

func TestTokenizeSQL(t *testing.T) {
	toks := tokenizeSQL("SELECT a FROM homer_lake.hep_proto_1_call WHERE x = 1")
	if len(toks) < 5 {
		t.Fatalf("tokens: %v", toks)
	}
}

func runesToStrings(in [][]rune) []string {
	out := make([]string, len(in))
	for i, r := range in {
		out[i] = string(r)
	}
	return out
}
