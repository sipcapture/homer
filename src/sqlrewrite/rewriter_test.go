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
