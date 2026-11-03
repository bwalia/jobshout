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
	if len(notes) != 1 || !strings.Contains(notes[0], "too few labels to draw it as a diagram") {
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
	}
	for _, c := range cases {
		s, ok := figureScene(c.kind, c.desc)
		if ok != (c.want != "") || s.Layout != c.want {
			t.Errorf("figureScene(%s, %q) = %+v %v, want %q", c.kind, c.desc, s, ok, c.want)
		}
	}
}
