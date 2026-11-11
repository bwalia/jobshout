package blog

import (
	"bytes"
	"context"
	"errors"
	"image/png"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/imagegen"
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
	md := "Intro. Each commit builds an image and runs the tests before we deploy to staging; polling and webhooks differ on latency and cost.\n\n" +
		"```illustration process\nCommit → Build image → Run tests → Deploy to staging\n```\n\n" +
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
	out, _ := r.illustrateBody(context.Background(), uuid.New(), nil, "We plan, build and ship.\n\n```illustration flow\nPlan → Build → Ship\n```\n")
	if !strings.Contains(out, "](https://jobshout.co.uk"+imagestore.URLPrefix) {
		t.Fatalf("figure link is not absolute: %s", out)
	}
}

// A lettering model that fails is asked once more for the figure's simpler
// scene, and the vector diagram still leaves the article its figure.
func TestIllustrateBody_FallsBackToDiagram(t *testing.T) {
	gpu := &fakeIllustrator{enabled: true, failAfter: -1}
	r := testRunner(gpu).WithCoverStore(newMemCovers())
	out, notes := r.illustrateBody(context.Background(), uuid.New(), nil,
		"The API gateway fronts the auth, orders and billing services, backed by Postgres and a Redis cache.\n\n"+
			"```illustration architecture\nAPI gateway: auth service, orders service, billing service, Postgres, Redis cache\n```\n")
	if len(gpu.calls) != 2 || gpu.calls[0].Text != imagegen.TextLabels || gpu.calls[1].Text != imagegen.TextCaptions ||
		len(notes) != 0 || !strings.Contains(out, "]("+imagestore.URLPrefix) {
		t.Fatalf("calls=%d notes=%v out=%s", len(gpu.calls), notes, out)
	}
}

// The workstation on ERNIE draws a flow or hub figure as it draws a cover: the
// scene's labels, each once, with no figure text to repeat or misspell. A
// comparison has no scene and is left to a model that letters; a picture
// from a model that cannot caption is not used.
func TestIllustrateBody_ErnieDrawsFigureScenes(t *testing.T) {
	ernie := &captionIllustrator{fakeIllustrator{enabled: true, provider: "mflux", model: "ernie-image-turbo"}}
	r := testRunner(ernie).WithCoverStore(newMemCovers())
	md := "A client reaches the service pod through the API gateway and the ingress controller; polling and webhooks differ on latency and cost.\n\n" +
		"```illustration architecture\nClient → API Gateway → Ingress Controller → Service Pod\n```\n\n" +
		"```illustration comparison\nPolling vs webhooks: latency, cost\n```\n"
	out, notes := r.illustrateBody(context.Background(), uuid.New(), nil, md)
	if len(ernie.calls) != 1 {
		t.Fatalf("calls = %d, want one (the flow's scene)", len(ernie.calls))
	}
	call := ernie.calls[0]
	if call.Text != imagegen.TextCaptions || !strings.HasPrefix(call.Prompt, figureOpening) ||
		!strings.Contains(call.Prompt, "Exactly 4 labelled parts") || strings.Count(call.Prompt, "Ingress Controller") != 1 ||
		strings.Join(call.ExpectText, "|") != "Client|API Gateway|Ingress Controller|Service Pod" {
		t.Fatalf("figure scene prompt:\n%s", call.Prompt)
	}
	// ERNIE's picture, not the vector fallback, and only one figure.
	if !strings.Contains(out, "/api/v1/images/file/img-1.png") || strings.Count(out, "![") != 1 || len(notes) != 1 {
		t.Fatalf("want ERNIE's flow and the comparison dropped: notes=%v\n%s", notes, out)
	}

	// The same scene from z-image-turbo would be lettering it cannot do.
	zimage := &captionIllustrator{fakeIllustrator{enabled: true, provider: "mflux", model: "z-image-turbo"}}
	out, _ = testRunner(zimage).WithCoverStore(newMemCovers()).illustrateBody(context.Background(), uuid.New(), nil, md)
	if strings.Contains(out, "img-1.png") || !strings.Contains(out, "]("+imagestore.URLPrefix) {
		t.Fatalf("z-image's figure was used, or no vector figure replaced it:\n%s", out)
	}
}

// The two figures int drew on 2026-11-11 that said nothing about the article:
// the prompt's own example, and key: value pairs drawn as a hub around
// "Distribution".
func TestIllustrateBody_DropsFiguresTheArticleDoesNotSupport(t *testing.T) {
	r := testRunner(&noLetterIllustrator{fakeIllustrator{enabled: true}}).WithCoverStore(newMemCovers())
	md := "Tenants who create Ingress objects can inject snippets; CVE-2021-25742 let them exfiltrate secrets past the API gateway.\n\n" +
		"```illustration architecture\nAPI gateway: auth service, orders service, billing service, Redis cache\n```\n\n" +
		"```illustration concept\nDistribution: Ingress objects, Score: tenant-supplied snippets, Alert: secret exfiltration, Record: CVE-2021-25742\n```\n"
	out, notes := r.illustrateBody(context.Background(), uuid.New(), nil, md)
	if strings.Contains(out, "![") || len(notes) != 2 || !strings.Contains(notes[0], "orders service") || !strings.Contains(notes[1], "key: value") {
		t.Fatalf("notes=%v\n%s", notes, out)
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
		// int's redraw, 2026-11-11: the planner's kinds did not match the lists.
		{kindArchitecture, "Client → API Gateway → Ingress Controller → Service Pod", layoutFlow},
		{kindConcept, "Tenant creates Ingress object → Ingress controller injects snippet → Secret exfiltration", layoutFlow},
		{kindComparison, "API Gateway: auth service, orders service, billing service, Redis cache", layoutHub},
		{kindComparison, "Polling vs webhooks: latency, cost, complexity, failure modes", ""},
	}
	for _, c := range cases {
		s, ok := figureScene(c.kind, c.desc)
		if ok != (c.want != "") || s.Layout != c.want {
			t.Errorf("figureScene(%s, %q) = %+v %v, want %q", c.kind, c.desc, s, ok, c.want)
		}
	}
}

// Figures are always asked for as lists, so a lettering model that fails (int's
// Gemini has no image quota) still leaves something the diagram path can draw.
func TestFigureKinds(t *testing.T) {
	kinds, format := testRunner(&fakeIllustrator{enabled: true}).figureKinds()
	if !strings.Contains(kinds, "comparison") || !strings.Contains(format, "Mix dough → Proof") || !strings.Contains(format, "vs") {
		t.Fatalf("with a lettering model: %q %q", kinds, format)
	}
	kinds, format = testRunner(&noLetterIllustrator{fakeIllustrator{enabled: true}}).WithCoverStore(newMemCovers()).figureKinds()
	if strings.Contains(kinds, "comparison") || !strings.Contains(format, "Bakery: oven") {
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

type fullStore struct{ memCovers }

func (*fullStore) PutFile(context.Context, uuid.UUID, []byte, string, string) (string, error) {
	return "", errors.New("Storage backend has reached its minimum free drive threshold")
}

// int, v1.0.209: the planner wrote sentences, cut to fit their boxes as
// "Tenant injects custom" and "API gateway handles load".
func TestFigureMisfit_LabelsTooLongToFit(t *testing.T) {
	text := "A tenant creates an Ingress object and injects a custom snippet; the snippet obtains secrets. " +
		"The API gateway handles load balancing, security and rate limiting."
	for _, d := range []string{
		"Tenant creates Ingress object → Tenant injects custom snippet → Snippet obtains secrets",
		"Gateway: API gateway handles load balancing, API gateway handles security, API gateway handles rate limiting",
	} {
		if why := figureMisfit(d, text); !strings.Contains(why, "cut short") {
			t.Errorf("%q: %q", d, why)
		}
	}
	if why := figureMisfit("Tenant → Ingress object → Custom snippet → Secrets", text); why != "" {
		t.Errorf("short labels refused: %q", why)
	}
}

// A flow draws five steps and highlights the last; a sixth used to push the
// destination ("proxy to pod") off the cover.
func TestCoverSceneFor_FlowKeepsItsDestination(t *testing.T) {
	a := &GeneratedArticle{Title: "T", CoverScene: coverScene{Layout: layoutFlow,
		Labels: []string{"HTTP request", "auth", "rate limit", "routing policy", "forward request", "proxy to pod"}}}
	if got := coverSceneFor(a).Labels; len(got) != 5 || got[3] != "routing policy" || got[4] != "proxy to pod" {
		t.Fatalf("labels = %v", got)
	}
}

// int, v1.0.210: five vendors with neither arrows nor a centre were drawn as a
// five-step flow.
func TestFigureMisfit_BareListHasNoShape(t *testing.T) {
	text := "Kong Inc., HAProxy, Shieldfy, Contour and Armosec all sell API gateways; each gateway checks requests."
	if why := figureMisfit("Kong Inc., HAProxy, Shieldfy, Contour, Armosec", text); !strings.Contains(why, "bare list") {
		t.Fatalf("got %q", why)
	}
	for _, ok := range []string{"API gateway: Kong Inc., HAProxy, Shieldfy, Contour", "Kong Inc. → HAProxy → Contour"} {
		if why := figureMisfit(ok, text); why != "" {
			t.Errorf("%q refused: %q", ok, why)
		}
	}
}
