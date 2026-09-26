package audience

import (
	"strings"
	"testing"
)

// The default has to be the developer profile, because every article written
// before this package existed was one and nothing recorded that it was.
func TestDefaultIsTheDeveloperProfile(t *testing.T) {
	for _, key := range []string{"", "  ", "developer", "DEVELOPER", "nonsense"} {
		if got := For(key).Key; got != DefaultKey {
			t.Errorf("For(%q).Key = %q; want %q", key, got, DefaultKey)
		}
	}
}

// Normalize stores the default as empty so an old row and a row written with
// the default chosen are the same row.
func TestNormalizeCollapsesTheDefaultToEmpty(t *testing.T) {
	cases := map[string]string{
		"":          "",
		"developer": "",
		"DEVELOPER": "",
		" business": "business",
		"Technote":  "technote",
	}
	for in, want := range cases {
		if got := Normalize(in); got != want {
			t.Errorf("Normalize(%q) = %q; want %q", in, got, want)
		}
	}
}

// An unknown key survives normalization so that Validate — which runs after it
// on both the HTTP path and the scheduler path — still has a typo to reject.
// Erasing it here would hand a schedule that asked for managers a developer
// article and no error, which is the failure this whole field exists to avoid.
func TestNormalizeKeepsAnUnknownKeyForValidateToReject(t *testing.T) {
	if got := Normalize("  Manger "); got != "manger" {
		t.Errorf("Normalize dropped an unknown key: %q", got)
	}
	if Known(Normalize("Manger")) {
		t.Error("an unknown key normalized into a known one")
	}
	// And it still cannot reach a prompt: For falls back to the default.
	if For("manger").Key != DefaultKey {
		t.Error("an unknown key resolved to something other than the default")
	}
}

// Known is what Validate rejects an unknown audience on, so it must accept
// empty (the default) and refuse a typo.
func TestKnown(t *testing.T) {
	for _, ok := range []string{"", "developer", "technote", "business", "BUSINESS"} {
		if !Known(ok) {
			t.Errorf("Known(%q) = false; want true", ok)
		}
	}
	for _, bad := range []string{"manager", "devloper", "exec"} {
		if Known(bad) {
			t.Errorf("Known(%q) = true; want false", bad)
		}
	}
}

// Every profile has to fill the slots the prompts splice it into. A reader
// added with a blank Reader or a zero word range produces a prompt with a hole
// in it, and the article that comes back is the first anyone hears about it.
func TestEveryProfileFillsItsPromptSlots(t *testing.T) {
	for _, p := range All() {
		t.Run(p.Key, func(t *testing.T) {
			if p.Key == "" || p.Label == "" || p.Hint == "" {
				t.Error("a profile needs a key, a label and a hint for the picker")
			}
			if p.Piece == "" || p.Reader == "" {
				t.Error("Piece and Reader open every writing prompt")
			}
			if p.MinWords <= 0 || p.MaxWords <= p.MinWords {
				t.Errorf("word range %d-%d is not a range", p.MinWords, p.MaxWords)
			}
			if p.Sections == "" || p.TitleStyle == "" {
				t.Error("the planner asks for a section count and a title style")
			}
			if p.Code == "" || p.Diagrams == "" {
				t.Error("the draft needs a stated code and diagram policy, even if it is 'none'")
			}
			if len(p.Expand) == 0 {
				t.Error("the expansion pass needs something to add that is not padding")
			}
			if p.Remit == "" || p.TopicExample == "" {
				t.Error("topic discovery needs a remit and an event-versus-topic example")
			}
			if len(p.Prefer) == 0 || len(p.Reject) == 0 {
				t.Error("topic discovery needs candidate filters")
			}
			if len(p.Tags) == 0 {
				t.Error("CMS drafts are tagged by reader so an editor can tell them apart")
			}
			// Every reader needs a stated position on code in the review, or
			// the critic applies the last reader's. The developer check moved
			// here from the shared list when the checklist became per-reader,
			// and a profile added without one silently loses it.
			if len(p.ReviewChecks) == 0 {
				t.Error("a reader needs at least one review check of its own")
			}
		})
	}
}

// The rendered fragments have to read as English, because they are spliced
// into a sentence rather than shown as fields.
func TestPromptLinesReadAsSentences(t *testing.T) {
	dev := For("developer")
	if got := dev.WritingLine(); got != "You are writing a technical article for a developer audience." {
		t.Errorf("developer WritingLine = %q", got)
	}
	// "an" rather than "a" in front of a vowel.
	vowel := Profile{Piece: "explainer", Reader: "someone"}
	if got := vowel.IndefinitePiece(); got != "an explainer" {
		t.Errorf("IndefinitePiece = %q; want %q", got, "an explainer")
	}
}

// The business profile's whole point is that it changes the rules, not the
// wording — so the draft must ban code and the review must hunt for jargon.
func TestBusinessProfileChangesTheRulesNotJustTheWording(t *testing.T) {
	biz := For("business")

	rules := biz.DraftRules()
	if !strings.Contains(rules, "Do not include code") {
		t.Errorf("business draft rules permit code:\n%s", rules)
	}
	if !strings.Contains(rules, "plain-English gloss") {
		t.Errorf("business draft rules do not require a gloss for jargon:\n%s", rules)
	}

	checks := biz.ReviewExtras()
	if !strings.Contains(checks, "UNEXPLAINED JARGON") {
		t.Errorf("business review does not check for jargon:\n%s", checks)
	}

	dev := For("developer")
	if strings.Contains(dev.DraftRules(), "Do not include code") {
		t.Error("the developer profile picked up the business code ban")
	}
}

// A technote that runs to 1400 words has stopped being a technote.
func TestTechnoteIsShorterThanADeepDive(t *testing.T) {
	note, deep := For("technote"), For("developer")
	if note.MaxWords >= deep.MinWords {
		t.Errorf("technote max %d does not sit below deep-dive min %d",
			note.MaxWords, deep.MinWords)
	}
}

// Industry framing is absent rather than empty when nobody asked for one, so
// the prompts do not carry a heading with nothing under it.
func TestIndustryBriefIsAbsentWhenUnset(t *testing.T) {
	for _, none := range []string{"", "   "} {
		if got := IndustryBrief(none); got != "" {
			t.Errorf("IndustryBrief(%q) = %q; want empty", none, got)
		}
		if got := IndustryDiscoveryBrief(none); got != "" {
			t.Errorf("IndustryDiscoveryBrief(%q) = %q; want empty", none, got)
		}
	}
	if got := IndustryBrief(" NHS trusts "); !strings.Contains(got, "NHS trusts") {
		t.Errorf("IndustryBrief dropped the sector: %q", got)
	}
}

// Options is what the schema picker is built from, so it must cover the
// registry rather than a hand-maintained subset.
func TestOptionsCoverEveryProfile(t *testing.T) {
	opts, all := Options(), All()
	if len(opts) != len(all) {
		t.Fatalf("Options has %d entries for %d profiles", len(opts), len(all))
	}
	for i, o := range opts {
		if o.Value != all[i].Key || o.Label != all[i].Label {
			t.Errorf("option %d = %+v; want key %q label %q", i, o, all[i].Key, all[i].Label)
		}
	}
}
