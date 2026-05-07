// Copyright (C) 2025 Homer Server Contributors

package remotelog

import "testing"

func TestValidateLokiCustomLabelKey(t *testing.T) {
	allow := LokiCustomLabelAllowlist([]string{"tenant", "trunk_id"})
	if err := ValidateLokiCustomLabelKey("tenant", allow); err != nil {
		t.Fatal(err)
	}
	if err := ValidateLokiCustomLabelKey("unknown", allow); err == nil {
		t.Fatal("expected error")
	}
	if err := ValidateLokiCustomLabelKey("job", allow); err == nil {
		t.Fatal("expected reserved error")
	}
	if err := ValidateLokiCustomLabelKey("__stream__", allow); err == nil {
		t.Fatal("expected __ error")
	}
	if err := ValidateLokiCustomLabelKey("bad-key", allow); err == nil {
		t.Fatal("expected syntax error")
	}
	if err := ValidateLokiCustomLabelKey("tenant", map[string]struct{}{}); err == nil {
		t.Fatal("expected empty allowlist error")
	}
}

func TestMergeCustomLokiLabelsIntoStream_builtinWins(t *testing.T) {
	dst := map[string]string{"job": "homer", "node": "n1"}
	MergeCustomLokiLabelsIntoStream(dst, map[string]string{"tenant": "acme", "job": "evil"})
	if dst["job"] != "homer" {
		t.Fatalf("job: %q", dst["job"])
	}
	if dst["tenant"] != "acme" {
		t.Fatalf("tenant: %q", dst["tenant"])
	}
}
