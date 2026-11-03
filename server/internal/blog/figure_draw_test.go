package blog

import (
	"bytes"
	"context"
	"image/png"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/imagestore"
)

// Prod's case: the workstation draws covers but cannot letter a figure, and
// there is no hosted image model. Body figures are drawn as diagrams instead
// of being left out of every article, and the GPU is never asked.
func TestIllustrateBody_DrawsDiagramsWithoutALetteringModel(t *testing.T) {
	gpu := &noLetterIllustrator{fakeIllustrator{enabled: true}}
	covers := newMemCovers()
	r := testRunner(gpu).WithCoverStore(covers)

	if !strings.Contains(r.visualRules(), "```illustration") {
		t.Fatal("a runner that can draw diagrams must offer figures")
	}
	md := "Intro.\n\n```illustration process\nCommit → Build image → Run tests → Deploy to staging\n```\n\n" +
		"```illustration comparison\nPolling vs webhooks: latency, cost\n```\n\nEnd.\n"
	out, notes := r.illustrateBody(context.Background(), uuid.New(), nil, md)

	if len(gpu.calls) != 0 {
		t.Fatalf("asked the GPU %d times for figures it cannot letter", len(gpu.calls))
	}
	if !strings.Contains(out, "![Commit → Build image → Run tests → Deploy to staging]("+imagestore.URLPrefix) {
		t.Fatalf("process figure not drawn:\n%s", out)
	}
	if strings.Contains(out, "```illustration") || !strings.Contains(out, "Intro.") || !strings.Contains(out, "End.") {
		t.Fatalf("fences survived or prose damaged:\n%s", out)
	}
	if len(notes) != 1 || !strings.Contains(notes[0], "not a list of steps or parts") {
		t.Fatalf("the comparison should be dropped with a reason: %v", notes)
	}
	if len(covers.objects) != 1 {
		t.Fatalf("stored %d images, want 1", len(covers.objects))
	}
	for _, data := range covers.objects {
		img, err := png.Decode(bytes.NewReader(data))
		if err != nil || img.Bounds().Dx() != int(designW) {
			t.Fatalf("stored figure is not a diagram PNG: %v", err)
		}
	}
}

// The CMS posts the body to another site, so a figure link must be absolute.
func TestIllustrateBody_FigureLinksAreAbsolute(t *testing.T) {
	r := testRunner(&noLetterIllustrator{fakeIllustrator{enabled: true}}).WithCoverStore(newMemCovers())
	r.cfg.PublicBaseURL = "https://jobshout.co.uk/"
	out, _ := r.illustrateBody(context.Background(), uuid.New(), nil, "```illustration flow\nPlan → Build → Ship\n```\n")
	if !strings.Contains(out, "](https://jobshout.co.uk"+imagestore.URLPrefix) {
		t.Fatalf("figure link is not absolute: %s", out)
	}
}

// A lettering model that fails still leaves the article its figure.
func TestIllustrateBody_FallsBackToDiagram(t *testing.T) {
	gpu := &fakeIllustrator{enabled: true, failAfter: -1}
	r := testRunner(gpu).WithCoverStore(newMemCovers())
	out, notes := r.illustrateBody(context.Background(), uuid.New(), nil,
		"```illustration architecture\nAPI gateway: auth service, orders service, billing service, Postgres, Redis cache\n```\n")
	if len(gpu.calls) != 1 || len(notes) != 0 || !strings.Contains(out, "]("+imagestore.URLPrefix) {
		t.Fatalf("calls=%d notes=%v out=%s", len(gpu.calls), notes, out)
	}
}

func TestFigureScene(t *testing.T) {
	cases := []struct {
		kind illustrationKind
		desc string
		want coverLayout
	}{
		{kindFlow, "Commit → Build → Test → Deploy", layoutFlow},
		{kindArchitecture, "API gateway: auth service, orders service, billing service, Postgres, Redis cache", layoutHub},
		{kindProcess, "Plan → Ship", ""},                     // two steps would be padded with "output"
		{kindConcept, "Gateway: auth service, Postgres", ""}, // a hub needs four real spokes
		{kindComparison, "Polling vs webhooks vs queues vs streams", ""},
		{kindFlow, "A control plane reconciles the desired state of every workload continuously", ""}, // prose, not a list
		{kindProcess, "Steps: plan, build, and ship", layoutFlow},                                     // the "Steps:" title is not a step
	}
	for _, c := range cases {
		s, ok := figureScene(c.kind, c.desc)
		if ok != (c.want != "") || s.Layout != c.want {
			t.Errorf("figureScene(%s, %q) = %+v %v, want %q", c.kind, c.desc, s, ok, c.want)
		}
	}
}

// Redrawing a finished article: its old figure goes, the model picks the
// section and the steps, and the new figure is drawn and rendered into HTML.
func TestRedrawFigures(t *testing.T) {
	r := testRunner(&noLetterIllustrator{fakeIllustrator{enabled: true}}).WithCoverStore(newMemCovers())
	r.llm = &stubLLM{responses: []scriptedResponse{{
		trigger: "Choose the one or two sections",
		content: `{"figures":[{"kind":"process","section":"How the pipeline runs","content":"Commit → Build image → Run tests → Deploy"}]}`,
	}}}
	a := &GeneratedArticle{Title: "CI in practice", Markdown: "# CI in practice\n\nIntro.\n\n" +
		"## Why it matters\n\nTeams ship faster.\n\n![old figure](https://jobshout.co.uk" + imagestore.URLPrefix + "org/2026/10/old.png)\n\n" +
		"## How the pipeline runs\n\nEvery change goes through the same steps.\n\nMore detail.\n"}

	notes, err := r.RedrawFigures(context.Background(), uuid.New(), a)
	if err != nil || len(notes) != 0 {
		t.Fatalf("err=%v notes=%v", err, notes)
	}
	if strings.Contains(a.Markdown, "old.png") {
		t.Fatalf("the old figure survived:\n%s", a.Markdown)
	}
	section := a.Markdown[strings.Index(a.Markdown, "## How the pipeline runs"):]
	if !strings.Contains(section, "![Commit → Build image → Run tests → Deploy]("+imagestore.URLPrefix) {
		t.Fatalf("figure not drawn under its section:\n%s", a.Markdown)
	}
	if !strings.Contains(a.HTML, "<img") || strings.Contains(a.HTML, "old.png") {
		t.Fatalf("HTML not re-rendered:\n%s", a.HTML)
	}
}

// Figures are always asked for as lists, so a lettering model that fails (int's
// Gemini has no image quota) still leaves something the diagram path can draw.
func TestFigureKinds(t *testing.T) {
	kinds, format := testRunner(&fakeIllustrator{enabled: true}).figureKinds()
	if !strings.Contains(kinds, "comparison") || !strings.Contains(format, "Commit → Build image") || !strings.Contains(format, "vs") {
		t.Fatalf("with a lettering model: %q %q", kinds, format)
	}
	kinds, format = testRunner(&noLetterIllustrator{fakeIllustrator{enabled: true}}).WithCoverStore(newMemCovers()).figureKinds()
	if strings.Contains(kinds, "comparison") || !strings.Contains(format, "API gateway: auth service") {
		t.Fatalf("diagrams only: %q %q", kinds, format)
	}
}

// int drew a hub labelled "core" when the model copied the format example.
func TestDropTemplateLabel(t *testing.T) {
	got := dropTemplateLabel("core: Agnost, Kusk Gateway, OpenAPI definition, Ingress controller, application")
	s, ok := figureScene(kindArchitecture, got)
	if !ok || s.Labels[0] != "Agnost" || len(s.Labels) != 5 {
		t.Fatalf("got %q -> %+v %v", got, s, ok)
	}
	if dropTemplateLabel("API gateway: auth, orders") != "API gateway: auth, orders" {
		t.Fatal("a real centre must stay")
	}
}
