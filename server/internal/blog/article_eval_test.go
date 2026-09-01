package blog

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestEval_EnsureFencesUsesPlannedFigure(t *testing.T) {
	md := "# Title\n\nIntro.\n\n## Polling vs webhooks\n\nProse about choosing a delivery model.\n"
	plan := &writePlan{
		Title:    "Title",
		Sections: []string{"Polling vs webhooks"},
		Figures: []figureBrief{{
			Kind:    "comparison",
			Section: "Polling vs webhooks",
			Content: "Latency, operational cost, failure modes, and when each wins",
		}},
	}
	out := ensureIllustrationFences(md, plan)
	if !strings.Contains(out, "```illustration comparison") {
		t.Fatalf("planned kind was not written:\n%s", out)
	}
	if !strings.Contains(out, "Latency, operational cost") {
		t.Fatalf("planned facts were not written:\n%s", out)
	}
}

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
	if !strings.Contains(with, "1–2") {
		t.Fatal("prompt should ask for 1–2 body images")
	}
	if !strings.Contains(with, fmt.Sprintf("up to %d", maxInlineIllustrations)) {
		t.Fatal("prompt budget must match maxInlineIllustrations")
	}
	if !strings.Contains(with, "Do not replace a") || !strings.Contains(with, "mermaid") {
		t.Fatal("prompt must not tell the writer to replace mermaid with a picture")
	}
	for _, kind := range []string{"flow", "comparison", "architecture", "process", "concept"} {
		if !strings.Contains(with, kind) {
			t.Fatalf("prompt must teach the %s figure kind", kind)
		}
	}
	if strings.Contains(with, "Do not ask for text, labels, charts or UI") {
		t.Fatal("prompt must not ban labels — figures need them")
	}
	if !strings.Contains(with, "TABLES:") {
		t.Fatal("comparison tables in prose must be offered even when images are on")
	}
}

func TestEval_DraftWithoutFencesGetsBodyImages(t *testing.T) {
	md := "# Title\n\nIntro.\n\n## Control planes\n\nProse about reconciliation.\n\n## Observability\n\nMore prose.\n"
	out := ensureIllustrationFences(md, &writePlan{Sections: []string{"Control planes", "Observability"}})
	n := len(illustrationFence.FindAllString(out, -1))
	if n < 1 || n > 2 {
		t.Fatalf("expected 1–2 illustration fences, got %d:\n%s", n, out)
	}
	if !strings.Contains(out, "reconciliation") {
		t.Fatalf("figure should use the section prose, not a stock heading photo:\n%s", out)
	}
	if !strings.Contains(out, "```illustration ") {
		t.Fatalf("auto-inserted fence should name a kind:\n%s", out)
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
	md := "# T\n\n```illustration\nAlready here\n```\n\n## Next\n\n```illustration\nSecond scene\n```\n"
	if got := ensureIllustrationFences(md, nil); got != md {
		t.Fatalf("should not rewrite a draft that already has 1–2 fences:\n%s", got)
	}
}

func TestEval_KeepsEveryMermaidDiagram(t *testing.T) {
	md := "# Title\n\n## Path\n\nAgents rank issuers then hand a shortlist to a trader.\n\n" +
		"```mermaid\nflowchart TD\n A --> B\n```\n\n" +
		"## Loop\n\nA control plane reconciles desired state against live positions.\n\n" +
		"```mermaid\nflowchart TD\n C --> D\n```\n"
	out := ensureIllustrationFences(md, &writePlan{Title: "AI Agents in Finance"})
	if n := len(mermaidFence.FindAllString(out, -1)); n != 2 {
		t.Fatalf("mermaid diagrams must stay, got %d:\n%s", n, out)
	}
	if strings.Contains(out, "photographed from a clear vantage") {
		t.Fatalf("stock heading photo leaked into the prompt:\n%s", out)
	}
}

func TestEval_ThinProseDoesNotForceAConceptPicture(t *testing.T) {
	md := "# Title\n\nIntro.\n\n## Observability\n\nMore prose.\n"
	out := ensureIllustrationFences(md, &writePlan{Title: "T", Sections: []string{"Observability"}})
	if n := len(illustrationFence.FindAllString(out, -1)); n != 0 {
		t.Fatalf("thin prose must not grow a decorative figure, got %d:\n%s", n, out)
	}
}

func TestEval_AlreadyIllustratedKeepsMermaid(t *testing.T) {
	md := "```illustration comparison\nPolling vs webhooks: latency and cost\n```\n\n" +
		"```mermaid\nflowchart TD\n A --> B\n```\n\n" +
		"```mermaid\nflowchart TD\n C --> D\n```\n"
	out := ensureIllustrationFences(md, nil)
	if n := len(mermaidFence.FindAllString(out, -1)); n != 2 {
		t.Fatalf("want both mermaids kept, got %d:\n%s", n, out)
	}
	if n := len(illustrationFence.FindAllString(out, -1)); n != 1 {
		t.Fatalf("want the existing illustration only, got %d:\n%s", n, out)
	}
	if !strings.Contains(out, "Polling vs webhooks: latency and cost") {
		t.Fatal("existing illustration was rewritten")
	}
}

func TestEval_SectionSceneUsesProseNotStockPhoto(t *testing.T) {
	got := sectionScene("Bond issuance", "An agent ranks issuers and hands a shortlist to the syndicate desk.", "AI in finance")
	if !strings.Contains(got, "ranks issuers") {
		t.Fatalf("lost the mechanism: %q", got)
	}
	if strings.Contains(got, "photographed from a clear vantage") {
		t.Fatalf("stock heading photo leaked: %q", got)
	}
	if strings.Contains(got, "No text, labels") || strings.Contains(got, "readable scene") {
		t.Fatalf("old decorative-scene wording leaked: %q", got)
	}
}

func TestEval_TablesOfferedWithoutAnIllustrator(t *testing.T) {
	rules := (&Runner{}).visualRules()
	if !strings.Contains(rules, "TABLES:") {
		t.Fatal("markdown tables must be offered even when images cannot be drawn")
	}
	if strings.Contains(rules, "```illustration") {
		t.Fatal("tables must not drag illustration fences in")
	}
}
