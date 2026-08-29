package blog

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestEval_CoverObjectsAcceptsStringOrArray(t *testing.T) {
	var asString writePlan
	if err := json.Unmarshal([]byte(`{"title":"T","cover_objects":"lighthouse and hull"}`), &asString); err != nil {
		t.Fatal(err)
	}
	if asString.CoverObjects.String() != "lighthouse and hull" {
		t.Fatalf("string: %q", asString.CoverObjects)
	}
	var asArray writePlan
	if err := json.Unmarshal([]byte(`{"title":"T","cover_objects":["lighthouse","hull silhouettes"]}`), &asArray); err != nil {
		t.Fatal(err)
	}
	if asArray.CoverObjects.String() != "lighthouse, hull silhouettes" {
		t.Fatalf("array: %q", asArray.CoverObjects)
	}
}

func TestEval_IllustrationRulesOnlyWhenEnabled(t *testing.T) {
	without := (&Runner{}).visualRules()
	if strings.Contains(without, "```illustration") {
		t.Fatal("disabled illustrator must not offer illustration fences")
	}
	with := testRunner(&fakeIllustrator{enabled: true}).visualRules()
	if !strings.Contains(with, "ILLUSTRATIONS:") {
		t.Fatal("enabled illustrator should offer illustrations")
	}
	if strings.Contains(with, "Most articles need none") {
		t.Fatal("prompt must not discourage body images")
	}
	if !strings.Contains(with, "ONE or TWO") {
		t.Fatal("prompt should ask for one or two body images")
	}
}

func TestEval_DraftWithoutFencesGetsBodyImages(t *testing.T) {
	md := "# Title\n\nIntro.\n\n## Control planes\n\nProse about reconciliation.\n\n## Observability\n\nMore prose.\n"
	out := ensureIllustrationFences(md, &writePlan{Sections: []string{"Control planes", "Observability"}})
	n := len(illustrationFence.FindAllString(out, -1))
	if n < 1 {
		t.Fatalf("expected at least one illustration fence, got %d:\n%s", n, out)
	}
	if n > maxInlineIllustrations {
		t.Fatalf("too many fences: %d", n)
	}

	fake := &fakeIllustrator{enabled: true}
	r := testRunner(fake)
	rendered, _ := r.illustrateBody(context.Background(), uuid.New(), out)
	if strings.Contains(rendered, "```illustration") {
		t.Fatal("fences should be replaced")
	}
	if !strings.Contains(rendered, "![") {
		t.Fatal("expected inline image markdown")
	}
}

func TestEval_DraftWithFencesStillReplaced(t *testing.T) {
	fake := &fakeIllustrator{enabled: true}
	r := testRunner(fake)
	md := "Intro.\n\n```illustration\nA control plane reconciling desired state\n```\n"
	out, _ := r.illustrateBody(context.Background(), uuid.New(), md)
	if strings.Contains(out, "```illustration") {
		t.Fatal("fence survived")
	}
	if !strings.Contains(out, "![A control plane reconciling desired state]") {
		t.Fatalf("image missing:\n%s", out)
	}
}

func TestEval_CoverPromptsShareHouseStyleAndDifferByMetaphor(t *testing.T) {
	a := coverPrompt("Gateway Changes", "kubernetes networking",
		"a harbour lighthouse guiding container ships", "lighthouse and hull silhouettes", "ice")
	b := coverPrompt("Postgres Vacuum", "database maintenance",
		"a clockwork broom sweeping parchment scrolls", "gears and a broom", "amber")

	for _, p := range []string{a, b} {
		for _, token := range []string{"charcoal navy", "LEFT", "teal", "coral", "Flat vector", "16:9"} {
			if !strings.Contains(p, token) {
				t.Errorf("house-style token %q missing from:\n%s", token, p)
			}
		}
	}
	if !strings.Contains(a, "lighthouse") || !strings.Contains(b, "broom") {
		t.Fatal("each cover should carry its own metaphor")
	}
	if strings.Contains(a, "broom") || strings.Contains(b, "lighthouse") {
		t.Fatal("metaphors leaked across topics")
	}
}

func TestEval_EnsureFencesIdempotentWhenAlreadyPresent(t *testing.T) {
	md := "# T\n\n```illustration\nAlready here\n```\n\n## Next\n"
	if got := ensureIllustrationFences(md, nil); got != md {
		t.Fatalf("should not rewrite an already-illustrated draft:\n%s", got)
	}
}
