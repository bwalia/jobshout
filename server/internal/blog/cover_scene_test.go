package blog

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCleanCoverLabels_KeepsALabelThatFitsTheBox(t *testing.T) {
	got := cleanCoverLabels([]string{"You let a UK property", "You let a UK property to overseas tenants"})
	if len(got) != 1 || got[0] != "You let a UK property" {
		t.Errorf("got %q, want [\"You let a UK property\"]", got)
	}
}

// The art under a cover draws the plan's scene of the subject. Drawn from the
// labels as "a technology article", a UK property-tax cover came out as a
// server room.
func TestArtPrompt_DrawsThePlansPicture(t *testing.T) {
	var s coverScene
	if err := json.Unmarshal([]byte(`{"layout":"flow","labels":["Live abroad","Agent deducts tax"],"picture":"a London terraced house with a To Let sign, a passport on the doorstep."}`), &s); err != nil {
		t.Fatal(err)
	}
	got := artPrompt(s, "amber")
	if !strings.Contains(got, "illustration of a London terraced house with a To Let sign, a passport on the doorstep, shown as") {
		t.Errorf("prompt does not draw the picture: %s", got)
	}
	if strings.Contains(got, "technology") {
		t.Errorf("prompt still frames every article as technology: %s", got)
	}
	if fallback := artPrompt(coverScene{Labels: []string{"Live abroad"}}, "amber"); !strings.Contains(fallback, "article about live abroad") {
		t.Errorf("no picture should fall back to the labels: %s", fallback)
	}
}
