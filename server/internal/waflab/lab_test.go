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

func TestSummarizeErrorBody(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"structured", `{"error":{"message":"WAF rule not found","code":"NOT_FOUND"}}`, "WAF rule not found (NOT_FOUND)"},
		{"plain message", `{"message":"Missing token"}`, "Missing token"},
		{"empty", "   ", "(empty response body)"},
		{"non json", "Missing token", "Missing token"},
	}
	for _, c := range cases {
		if got := summarizeErrorBody(c.in); got != c.want {
			t.Errorf("%s: got %q want %q", c.name, got, c.want)
		}
	}

	// The actual regression: a multi-KB HTML error page must not reach the UI.
	html := `<!DOCTYPE html><html><head><title>Server Error | WSL Proxy</title>` +
		`<style>` + string(make([]byte, 5000)) + `</style></head><body>500</body></html>`
	got := summarizeErrorBody(html)
	if len(got) > 200 {
		t.Fatalf("HTML page not summarized: %d chars", len(got))
	}
	if want := "Server Error | WSL Proxy"; !contains(got, want) {
		t.Fatalf("summary %q lost the page title %q", got, want)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
