package waflab

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestCatalogueCountAndBOLA(t *testing.T) {
	all := Catalogue()
	if len(all) < 30 {
		t.Fatalf("catalogue too small: %d", len(all))
	}
	var bola *Attack
	for i := range all {
		if all[i].ID == "bola-idor-business-logic" {
			bola = &all[i]
			break
		}
	}
	if bola == nil {
		t.Fatal("missing BOLA case")
	}
	if bola.ExpectBlock {
		t.Fatal("BOLA must have ExpectBlock=false by design")
	}
	if bola.Method != "GET" || bola.Path != "/api/accounts/9999" {
		t.Fatalf("BOLA shape: %+v", bola)
	}

	benign := 0
	for _, a := range all {
		if a.Category == "benign" && !a.ExpectBlock {
			benign++
		}
	}
	if benign < 2 {
		t.Fatalf("need benign false-positive probes, got %d", benign)
	}

	full := FilterBySet("full")
	if len(full) != len(all) {
		t.Fatalf("full filter %d != catalogue %d", len(full), len(all))
	}
	owasp := FilterBySet("owasp_core")
	if len(owasp) >= len(all) {
		t.Fatalf("owasp_core should be smaller than full")
	}
	// Controls always present
	hasBOLA := false
	for _, a := range owasp {
		if a.ID == "bola-idor-business-logic" {
			hasBOLA = true
		}
	}
	if !hasBOLA {
		t.Fatal("filters must retain BOLA control")
	}
}

func TestCatalogueDriftAgainstDemo(t *testing.T) {
	root := strings.TrimSpace(os.Getenv("WSLPROXY_DEMO_ROOT"))
	if root == "" {
		t.Skip("WSLPROXY_DEMO_ROOT unset — skip catalogue drift check")
	}
	py := filepath.Join(root, "test_waf_live.py")
	b, err := os.ReadFile(py)
	if err != nil {
		t.Fatalf("read %s: %v", py, err)
	}
	re := regexp.MustCompile(`Case\(\s*"([^"]+)"`)
	matches := re.FindAllStringSubmatch(string(b), -1)
	if len(matches) == 0 {
		t.Fatal("no Case() names found in test_waf_live.py")
	}
	want := map[string]bool{}
	for _, m := range matches {
		want[m[1]] = true
	}
	// Catalogue intentionally omits monitor-only / staged-allow cases that are
	// not part of the efficacy score set — but core IDs must be present.
	required := []string{
		"sqli-union", "xss-script", "lfi-dotdot", "log4shell-jndi", "ssrf-metadata",
		"nosqli", "xxe", "jwt-alg-none", "proto-pollution", "graphql-introspection",
		"mass-assignment", "http-request-smuggling", "scanner-ua", "v2-method",
		"bola-idor-business-logic", "benign-products",
	}
	have := map[string]bool{}
	for _, id := range CatalogueIDs() {
		have[id] = true
	}
	for _, id := range required {
		if !want[id] {
			t.Errorf("demo missing Case %q (demo file drifted?)", id)
		}
		if !have[id] {
			t.Errorf("catalogue missing %q present in demo", id)
		}
	}
}
