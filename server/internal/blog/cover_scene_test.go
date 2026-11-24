package blog

import "testing"

func TestCleanCoverLabels_KeepsALabelThatFitsTheBox(t *testing.T) {
	got := cleanCoverLabels([]string{"You let a UK property", "You let a UK property to overseas tenants"})
	if len(got) != 1 || got[0] != "You let a UK property" {
		t.Errorf("got %q, want [\"You let a UK property\"]", got)
	}
}
