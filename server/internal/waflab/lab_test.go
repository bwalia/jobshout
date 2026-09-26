package waflab

import "testing"

func TestMissingPolicyRules(t *testing.T) {
	pols := []map[string]any{{"waf_rules": []any{"a", "b", "c", "b"}}}
	present := []map[string]any{{"id": "a"}, {"id": "c"}}

	got := missingPolicyRules(pols, present)
	if len(got) != 1 || got[0] != "b" {
		t.Fatalf("want [b], got %v", got)
	}
	// Every reference resolving must not fail the run — this is the case that
	// regressed: a deployment whose rules were all present still aborted.
	if got := missingPolicyRules(pols, []map[string]any{
		{"id": "a"}, {"id": "b"}, {"id": "c"},
	}); got != nil {
		t.Fatalf("want nil when all present, got %v", got)
	}
	if got := missingPolicyRules(nil, nil); got != nil {
		t.Fatalf("want nil for no policies, got %v", got)
	}
}
