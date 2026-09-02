package career

import "testing"

func TestBuiltinPortalsUniqueAndComplete(t *testing.T) {
	list := BuiltinPortals()
	if len(list) != len(builtinPortals) {
		t.Fatalf("copy mutated length")
	}
	seen := map[string]bool{}
	boards := map[string]int{}
	for _, p := range list {
		key := p.Board + ":" + p.Slug
		if seen[key] {
			t.Fatalf("duplicate %s", key)
		}
		seen[key] = true
		boards[p.Board]++
	}
	if boards["greenhouse"] == 0 || boards["ashby"] == 0 || boards["lever"] == 0 {
		t.Fatalf("expected all three ATS boards, got %v", boards)
	}
}
