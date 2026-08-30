package blog

import (
	"strings"
	"testing"
)

// A recognised accent override pins that hue; an unknown one and the empty
// string both fall back to the per-topic default, so a bad value degrades to
// normal behaviour rather than erroring or blanking the accent.
func TestCoverPromptWithAccent_OverridesOrFallsBack(t *testing.T) {
	const topic = "kubernetes networking"

	def := coverPrompt("A Title", topic)
	if want := "amber and coral"; !strings.Contains(coverPromptWithAccent("A Title", topic, want), want) {
		t.Errorf("override %q did not appear in the prompt", want)
	}

	// Empty override is exactly the default prompt.
	if coverPromptWithAccent("A Title", topic, "") != def {
		t.Error(`empty override changed the prompt; it must equal coverPrompt`)
	}

	// An unknown accent is ignored, leaving the default prompt untouched.
	if coverPromptWithAccent("A Title", topic, "chartreuse and puce") != def {
		t.Error("unknown accent was not ignored")
	}
}

func TestNormalizeAccent(t *testing.T) {
	if got := normalizeAccent("  TEAL and Cyan "); got != "teal and cyan" {
		t.Errorf("normalizeAccent case/space = %q, want canonical %q", got, "teal and cyan")
	}
	if got := normalizeAccent("not a real accent"); got != "" {
		t.Errorf("normalizeAccent unknown = %q, want \"\"", got)
	}
	if got := normalizeAccent(""); got != "" {
		t.Errorf("normalizeAccent empty = %q, want \"\"", got)
	}
}

// Turning in-body illustrations off must remove any fence the writer emitted,
// so it never reaches the reader as a raw ```illustration code block.
func TestStripIllustrationFences(t *testing.T) {
	md := "Intro.\n\n```illustration\nA machinist at a lathe\n```\n\nMore body."
	out := stripIllustrationFences(md)
	if strings.Contains(out, "illustration") || strings.Contains(out, "```") {
		t.Errorf("fence survived stripping: %q", out)
	}
	if !strings.Contains(out, "Intro.") || !strings.Contains(out, "More body.") {
		t.Errorf("surrounding prose was lost: %q", out)
	}
}

// nil and true draw; a pointer to false suppresses.
func TestWantsIllustrations(t *testing.T) {
	yes, no := true, false
	if !(GenerateRequest{}).wantsIllustrations() {
		t.Error("nil Illustrations must default to drawing")
	}
	if !(GenerateRequest{Illustrations: &yes}).wantsIllustrations() {
		t.Error("explicit true must draw")
	}
	if (GenerateRequest{Illustrations: &no}).wantsIllustrations() {
		t.Error("explicit false must suppress")
	}
}
