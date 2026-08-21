package sqlrewrite

import (
	"strings"
	"testing"
)

func TestRewriteTableDiscoveryIOX(t *testing.T) {
	r := &Rewriter{LakeName: "homer_lake", DB: nil}
	q, rule := r.Rewrite(`SELECT * FROM information_schema.tables WHERE table_schema = 'iox'`)
	if rule != "table discovery (iox → DuckLake catalog)" {
		t.Fatalf("rule: %s", rule)
	}
	if !strings.Contains(q, "table_catalog = 'homer_lake'") {
		t.Fatalf("unexpected: %s", q)
	}
}

func TestRewriteTimeColumnBare(t *testing.T) {
	r := &Rewriter{LakeName: "homer_lake", DB: nil}
	q, _ := r.Rewrite(`SELECT "time" FROM "hep_proto_1_call"`)
	if !strings.Contains(q, `"timestamp"`) {
		t.Fatalf("expected timestamp rewrite, got: %s", q)
	}
}

func TestRewriteColumnDiscoveryIOX(t *testing.T) {
	r := &Rewriter{LakeName: "homer_lake", DB: nil}
	q, rule := r.Rewrite(`SELECT * FROM information_schema.columns WHERE table_schema = 'iox' AND table_name = 'hep_proto_1_call'`)
	if rule != "column discovery (iox → DESCRIBE)" {
		t.Fatalf("rule: %s sql=%s", rule, q)
	}
	if !strings.Contains(q, `DESCRIBE "homer_lake".main."hep_proto_1_call"`) {
		t.Fatalf("unexpected: %s", q)
	}
}

func TestRewriteColumnDiscoveryRejectsUnsafeTableName(t *testing.T) {
	r := &Rewriter{LakeName: "homer_lake", DB: nil}
	orig := `SELECT * FROM information_schema.columns WHERE table_schema = 'iox' AND table_name = 'hep_proto_1_call; DROP TABLE x'`
	q, rule := r.Rewrite(orig)
	if rule == "column discovery (iox → DESCRIBE)" {
		t.Fatalf("unsafe table_name must not become DESCRIBE, got %s", q)
	}
	if strings.Contains(strings.ToUpper(q), "DROP") && strings.Contains(strings.ToUpper(q), "DESCRIBE") {
		t.Fatalf("injection reached DESCRIBE: %s", q)
	}
}

func TestValidSQLIdent(t *testing.T) {
	if !validSQLIdent("hep_proto_1_call") {
		t.Fatal("expected valid")
	}
	for _, s := range []string{"", "1abc", "a;b", "a b", "a-b", `a"b`} {
		if validSQLIdent(s) {
			t.Errorf("%q should be invalid", s)
		}
	}
}
