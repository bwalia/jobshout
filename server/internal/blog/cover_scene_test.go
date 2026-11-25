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

func TestWordlessPicture_CutsBeforeWritingObjects(t *testing.T) {
	for in, want := range map[string]string{
		"A letting agent at a desk in a high-street office, holding a rental payment slip and a calculator, with a calendar showing tax quarter dates on the wall behind.": "A letting agent at a desk in a high-street office",
		"A radiologist reviewing 3D images on a computer screen in a hospital radiology department.":                                                                       "A radiologist reviewing 3D images",
		"A delivery van loading wooden trays of bread outside a small bakery at dawn":                                                                                      "A delivery van loading wooden trays of bread outside a small bakery at dawn",
		"A calendar on a kitchen wall": "",
	} {
		if got := wordlessPicture(in); got != want {
			t.Errorf("wordlessPicture(%q) = %q, want %q", in, got, want)
		}
	}
}
