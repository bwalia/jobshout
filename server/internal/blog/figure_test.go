package blog

import (
	"strings"
	"testing"
)

func TestCanonicalIllustrationKind(t *testing.T) {
	tests := []struct {
		in   string
		want illustrationKind
	}{
		{"flow", kindFlow},
		{"flowchart", kindFlow},
		{"comparison", kindComparison},
		{"table", kindComparison},
		{"architecture", kindArchitecture},
		{"process", kindProcess},
		{"steps", kindProcess},
		{"concept", kindConcept},
		{"", ""},
		{"scene", ""},
	}
	for _, tt := range tests {
		if got := canonicalIllustrationKind(tt.in); got != tt.want {
			t.Errorf("canonicalIllustrationKind(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestInferIllustrationKind(t *testing.T) {
	tests := []struct {
		in   string
		want illustrationKind
	}{
		{"Polling vs webhooks on latency", kindComparison},
		{"trade-offs between REST and gRPC", kindComparison},
		{"the control plane and its components", kindArchitecture},
		{"request path through the API gateway", kindFlow},
		{"first connect then authenticate finally query", kindProcess},
		{"how a lease is renewed", kindConcept},
	}
	for _, tt := range tests {
		if got := inferIllustrationKind(tt.in); got != tt.want {
			t.Errorf("inferIllustrationKind(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestParseIllustration_TypedAndUntyped(t *testing.T) {
	kind, desc := parseIllustration("comparison", "Polling vs webhooks: latency and cost")
	if kind != kindComparison {
		t.Errorf("typed kind = %q, want comparison", kind)
	}
	if !strings.Contains(desc, "Polling vs webhooks") {
		t.Errorf("lost the body: %q", desc)
	}

	kind, desc = parseIllustration("", "An agent ranks issuers then hands a shortlist to the desk")
	if kind != kindProcess {
		t.Errorf("untyped inference = %q, want process", kind)
	}
	if !strings.Contains(desc, "ranks issuers") {
		t.Errorf("lost the untyped body: %q", desc)
	}
}

func TestInlineImagePrompt_RequiresLabelsAndForbidsScenes(t *testing.T) {
	got := inlineImagePrompt(kindComparison, "Polling vs webhooks: latency, cost, failure modes")
	for _, need := range []string{
		"comparison table",
		"Polling vs webhooks",
		"labels",
		"No decorative characters",
		figurePromptStyle,
	} {
		if !strings.Contains(got, need) {
			t.Errorf("prompt missing %q:\n%s", need, got)
		}
	}
	if strings.Contains(got, "Strictly no text") {
		t.Fatal("prompt still bans text")
	}
}

func TestInlineSize_WidensDiagrams(t *testing.T) {
	w, h := inlineSize(kindComparison)
	if w != 1280 || h != 720 {
		t.Errorf("comparison size = %dx%d, want 1280x720", w, h)
	}
	w, h = inlineSize(kindConcept)
	if w != inlineWidth || h != inlineHeight {
		t.Errorf("concept size = %dx%d, want %dx%d", w, h, inlineWidth, inlineHeight)
	}
}

func TestPlannedFigure_MatchesSection(t *testing.T) {
	plan := &writePlan{Figures: []figureBrief{{
		Kind:    "comparison",
		Section: "Polling vs webhooks",
		Content: "Latency, operational cost, failure modes",
	}}}
	kind, content, ok := plannedFigure(plan, "Polling vs webhooks")
	if !ok {
		t.Fatal("expected a match")
	}
	if kind != kindComparison {
		t.Errorf("kind = %q", kind)
	}
	if !strings.Contains(content, "Latency") {
		t.Errorf("content = %q", content)
	}
	if _, _, ok := plannedFigure(plan, "Unrelated heading"); ok {
		t.Fatal("unrelated heading must not steal a planned figure")
	}
}

func TestSectionFigure_UsesProseAndPicksAKind(t *testing.T) {
	kind, desc := sectionFigure(
		"Control planes",
		"A control plane reconciles desired state against live positions.",
		"AI in finance",
	)
	if kind != kindArchitecture {
		t.Errorf("kind = %q, want architecture (control plane)", kind)
	}
	if !strings.Contains(desc, "reconciles desired state") {
		t.Errorf("lost the mechanism: %q", desc)
	}
	if strings.Contains(desc, "readable scene") {
		t.Errorf("decorative scene wording leaked: %q", desc)
	}
}

func TestFormatIllustrationFence_RoundTripsTheParser(t *testing.T) {
	raw := formatIllustrationFence(kindFlow, "Client → gateway → workers")
	if !illustrationFence.MatchString(raw) {
		t.Fatalf("formatter output is not parseable: %s", raw)
	}
	got := illustrationFence.FindStringSubmatch(raw)
	if got[1] != "flow" {
		t.Errorf("kind = %q", got[1])
	}
	if !strings.Contains(got[2], "Client") {
		t.Errorf("body = %q", got[2])
	}
}

func TestFigureWorthInserting_RejectsThinConcept(t *testing.T) {
	kind, _ := sectionFigure("Observability", "More prose.", "T")
	if figureWorthInserting(kind, "Observability", "More prose.", false) {
		t.Fatalf("thin concept should not be inserted: kind=%s words=%v", kind, contentWords("Observability More prose."))
	}
	kind, _ = sectionFigure("Control planes", "A control plane reconciles desired state against live positions.", "T")
	if !figureWorthInserting(kind, "Control planes", "A control plane reconciles desired state against live positions.", false) {
		t.Fatalf("a section with named parts should be inserted: kind=%s words=%v", kind, contentWords("Control planes A control plane reconciles desired state against live positions."))
	}
	if !figureWorthInserting(kindConcept, "X", "anything", true) {
		t.Fatal("a planned figure must not be dropped for thinness")
	}
}

func TestSalvageIllustration_DropsPureScenes(t *testing.T) {
	if _, _, ok := salvageIllustration(kindConcept, "A modern server room"); ok {
		t.Fatal("a stock scene with no facts must be dropped")
	}
	kind, desc, ok := salvageIllustration("", "An agent handing a ranked shortlist of issuers to a trader at a desk")
	if !ok {
		t.Fatal("a scene that still names the work should be rewritten, not dropped")
	}
	if kind == "" {
		t.Fatal("salvage must pick a kind")
	}
	if !strings.Contains(desc, "shortlist") || !strings.Contains(desc, "issuers") {
		t.Fatalf("salvage lost the article terms: %q", desc)
	}
	if !strings.Contains(desc, "Labeled") && !strings.Contains(desc, "Annotated") && !strings.Contains(desc, "Numbered") && !strings.Contains(desc, "Comparison") {
		t.Fatalf("salvage should wrap the scene as a figure spec: %q", desc)
	}
}

func TestImageRendersLabels(t *testing.T) {
	if imageRendersLabels("mflux", "z-image-turbo") {
		t.Fatal("workstation diffusion cannot letter")
	}
	if !imageRendersLabels("gemini", "gemini-3.1-flash-lite-image") {
		t.Fatal("Gemini should be treated as able to letter")
	}
}

func TestIllustrationFence_IgnoresExtraInfoLineWords(t *testing.T) {
	raw := "```illustration comparison table\nPolling vs webhooks: latency\n```"
	got := illustrationFence.FindStringSubmatch(raw)
	if got == nil {
		t.Fatal("extra info-line words must not break the fence")
	}
	if got[1] != "comparison" {
		t.Errorf("kind = %q, want comparison", got[1])
	}
	if !strings.Contains(got[2], "Polling vs webhooks") {
		t.Errorf("body = %q", got[2])
	}
}
