package handlers

import "testing"

func TestNormalizeTransactionSessionIDs(t *testing.T) {
	got := normalizeTransactionSessionIDs([]string{"  a ", "b", " a ", "", "b"})
	want := []string{"a", "b"}
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v want %v", got, want)
		}
	}
}

func TestNormalizeTransactionSessionIDsCap(t *testing.T) {
	in := make([]string, 60)
	for i := range in {
		in[i] = string(rune('a' + i))
	}
	got := normalizeTransactionSessionIDs(in)
	if len(got) != maxTransactionSessionIDs {
		t.Fatalf("len got %d want %d", len(got), maxTransactionSessionIDs)
	}
}

func TestBuildSessionIDOrWhere(t *testing.T) {
	where := buildSessionIDOrWhere([]string{"abc", "def"})
	if where != "(session_id = 'abc' OR session_id = 'def')" {
		t.Fatalf("unexpected where: %q", where)
	}
}
