package blog

import (
	"strings"
	"testing"
)

// A server that cannot draw must not tell the model it can. Otherwise the model
// requests illustrations, illustrateBody strips every block, and the article
// arrives referring to figures that were never generated.
func TestVisualRules_OffersIllustrationsOnlyWhenItCanDraw(t *testing.T) {
	withoutImages := (&Runner{}).visualRules()
	if strings.Contains(withoutImages, "```illustration") {
		t.Error("a runner that cannot draw must not offer illustrations")
	}
	if !strings.Contains(withoutImages, "DIAGRAMS:") {
		t.Error("diagram rules must be present regardless")
	}

	withImages := testRunner(&fakeIllustrator{enabled: true}).visualRules()
	if !strings.Contains(withImages, "ILLUSTRATIONS:") {
		t.Error("a runner that can draw should offer illustrations")
	}
	if !strings.Contains(withImages, "DIAGRAMS:") {
		t.Error("offering illustrations must not displace the diagram rules")
	}

	// A workstation-only illustrator can draw covers but cannot letter a
	// comparison table. The writer must not be offered fences it cannot fulfil.
	noLetters := testRunner(&noLetterIllustrator{fakeIllustrator{enabled: true}}).visualRules()
	if strings.Contains(noLetters, "```illustration") {
		t.Error("a path that cannot letter must not offer illustration fences")
	}
}

type noLetterIllustrator struct{ fakeIllustrator }

func (n *noLetterIllustrator) Letters() bool { return false }

// The fence the prompt teaches has to be the fence illustrateBody matches, or
// the feature is wired to nothing.
func TestIllustrationRules_TeachTheFenceThatIsActuallyParsed(t *testing.T) {
	rules := testRunner(&fakeIllustrator{enabled: true}).visualRules()

	// The example is shown indented inside the prompt, so it is matched as it
	// appears there rather than as a tidied-up copy — the point of this test is
	// that the two definitions cannot drift, and comparing against a rewritten
	// version would defeat it.
	const example = "  ```illustration comparison\n" +
		"  Polling vs webhooks: latency, operational cost, failure modes, and when each wins\n" +
		"  ```"
	if !strings.Contains(rules, example) {
		t.Fatalf("the prompt's example fence changed shape:\n%s", rules)
	}
	// The matcher must find it even indented, because that is how a model
	// copying the example will write it.
	if !illustrationFence.MatchString(example) {
		t.Error("the fence the prompt teaches is not the fence illustrateBody matches")
	}
	got := illustrationFence.FindStringSubmatch(example)
	if got == nil {
		t.Fatal("the taught fence produced no submatch")
	}
	if got[1] != "comparison" {
		t.Errorf("kind = %q, want comparison", got[1])
	}
	if !strings.Contains(got[2], "Polling vs webhooks") {
		t.Errorf("the description was not captured from the taught fence: %#v", got)
	}
}
