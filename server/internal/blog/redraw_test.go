package blog

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/imagestore"
)

// Redrawing a finished article: the model reads it once, the old figure goes,
// the new one is drawn under the section it chose, and the cover is the layout
// and labels it chose from the article — not its headings.
func TestRedrawImages(t *testing.T) {
	r := testRunner(&noLetterIllustrator{fakeIllustrator{enabled: true}}).WithCoverStore(newMemCovers())
	r.llm = &stubLLM{responses: []scriptedResponse{{
		trigger: "Read this finished article",
		content: `{"cover_scene":{"layout":"hub","labels":["Argo CD","Git repository","staging cluster","Helm chart","quantum ledger"],"eyebrow":"GitOps"},` +
			`"figures":[{"kind":"process","section":"How the pipeline runs","content":"Commit → Build image → Run tests → Deploy"}]}`,
	}}}
	a := &GeneratedArticle{Title: "CI in practice", Markdown: "# CI in practice\n\n" +
		"Argo CD syncs the staging cluster from a Git repository and a Helm chart.\n\n" +
		"## Why it matters\n\nTeams ship faster.\n\n![old figure](https://jobshout.co.uk" + imagestore.URLPrefix + "org/2026/10/old.png)\n\n" +
		"## How the pipeline runs\n\nEvery commit builds an image, runs the tests, then we deploy.\n\nMore detail.\n"}

	res, err := r.RedrawImages(context.Background(), uuid.New(), a)
	if err != nil || res.Figures != 1 || !res.Cover {
		t.Fatalf("err=%v res=%+v", err, res)
	}
	if strings.Contains(a.Markdown, "old.png") || !strings.Contains(a.HTML, "<img") {
		t.Fatalf("old figure kept or HTML not re-rendered:\n%s", a.Markdown)
	}
	section := a.Markdown[strings.Index(a.Markdown, "## How the pipeline runs"):]
	if !strings.Contains(section, "![Commit → Build image → Run tests → Deploy]("+imagestore.URLPrefix) {
		t.Fatalf("figure not drawn under its section:\n%s", a.Markdown)
	}
	if a.CoverImageModel != "scene:hub" || strings.Contains(a.CoverImagePrompt, "quantum ledger") ||
		!strings.Contains(a.CoverImagePrompt, "Argo CD") {
		t.Fatalf("cover = %s %s, want the model's hub, held to the article's words", a.CoverImageModel, a.CoverImagePrompt)
	}
}

// int, 2026-11-11: Gemini planned the prompt's own example for an API-gateway
// article. The plan is asked again, told why, and the second answer is drawn.
func TestRedrawImages_ReplansFiguresTheArticleDoesNotSupport(t *testing.T) {
	r := testRunner(&noLetterIllustrator{fakeIllustrator{enabled: true}}).WithCoverStore(newMemCovers())
	stub := &stubLLM{responses: []scriptedResponse{
		{trigger: "Read this finished article", content: `{"figures":[{"kind":"architecture","section":"The gateway",` +
			`"content":"API gateway: auth service, orders service, billing service, Redis cache"}]}`},
		{trigger: "The figures planned for this article cannot be drawn", content: `{"figures":[{"kind":"flow",` +
			`"section":"The gateway","content":"Client → API gateway → Ingress controller → Service pod"}]}`},
	}}
	r.llm = stub
	a := &GeneratedArticle{Title: "Ingress security", Markdown: "# Ingress security\n\n## The gateway\n\n" +
		"A client request passes the API gateway, then the ingress controller, then a service pod.\n"}

	res, err := r.RedrawImages(context.Background(), uuid.New(), a)
	if err != nil || res.Figures != 1 || !strings.Contains(a.Markdown, "![Client → API gateway → Ingress controller → Service pod](") {
		t.Fatalf("err=%v res=%+v\n%s", err, res, a.Markdown)
	}
	retry := stub.calls[1].Messages[len(stub.calls[1].Messages)-1].Content
	if !strings.Contains(retry, "orders service") || !strings.Contains(retry, "names what the article does not") {
		t.Fatalf("the retry should say what was wrong:\n%s", retry)
	}
}

// When nothing new can be drawn, the old figures stay — except those the
// article does not support, like int's copied example and key: value hub.
func TestRedrawImages_PrunesOldFiguresTheArticleDoesNotSupport(t *testing.T) {
	r := testRunner(&noLetterIllustrator{fakeIllustrator{enabled: true}}).WithCoverStore(&fullStore{*newMemCovers()})
	r.llm = &stubLLM{responses: []scriptedResponse{{trigger: "Read this finished article", content: `{"figures":[]}`}}}
	img := func(alt, name string) string {
		return "![" + alt + "](" + imagestore.URLPrefix + "o/" + name + ".png)\n\n"
	}
	a := &GeneratedArticle{Title: "T", Markdown: "## The gateway\n\nA client passes the API gateway and the ingress controller to a service pod; " +
		"tenants inject snippets to exfiltrate secrets (CVE-2021-25742).\n\n" +
		img("Client → API Gateway → Ingress Controller → Service Pod", "good") +
		img("API Gateway: auth service, orders service, billing service, Redis cache", "copied") +
		img("Distribution: Ingress objects, Score: tenant-supplied snippets, Alert: secret exfiltration", "pairs")}

	res, err := r.RedrawImages(context.Background(), uuid.New(), a)
	if err != nil || !strings.Contains(a.Markdown, "good.png") || strings.Contains(a.Markdown, "copied.png") || strings.Contains(a.Markdown, "pairs.png") {
		t.Fatalf("err=%v notes=%v\n%s", err, res.Notes, a.Markdown)
	}
	if !strings.Contains(strings.Join(res.Notes, " | "), "removed 2 old figure(s)") {
		t.Fatalf("notes = %v", res.Notes)
	}
}

// int's image store filled up mid-redraw: the article keeps the figure and the
// cover it had, and the result says nothing was redrawn.
func TestRedrawImages_KeepsWhatItCannotReplace(t *testing.T) {
	store := newMemCovers()
	coverURL, _ := store.PutFile(context.Background(), uuid.New(), []byte("png"), "png", "image/png")
	r := testRunner(&noLetterIllustrator{fakeIllustrator{enabled: true}}).WithCoverStore(&fullStore{*store})
	r.llm = &stubLLM{responses: []scriptedResponse{{
		trigger: "Read this finished article",
		content: `{"figures":[{"kind":"process","section":"How it runs","content":"Commit → Build → Test → Deploy"}]}`,
	}}}
	old := "## How it runs\n\nCommit, build, test, deploy.\n\n![old](" + imagestore.URLPrefix + "o/old.png)\n"
	a := &GeneratedArticle{Title: "T", Markdown: old, HTML: "<p>kept</p>", CoverImageURL: coverURL}

	res, err := r.RedrawImages(context.Background(), uuid.New(), a)
	if err != nil || res.Figures != 0 || res.Cover || a.Markdown != old || a.HTML != "<p>kept</p>" || a.CoverImageURL != coverURL {
		t.Fatalf("err=%v res=%+v md=%q cover=%q", err, res, a.Markdown, a.CoverImageURL)
	}
	notes := strings.Join(res.Notes, " | ")
	if !strings.Contains(notes, "kept the article's figures") || !strings.Contains(notes, "kept the cover") {
		t.Fatalf("notes = %v", res.Notes)
	}
}

func TestGroundScene(t *testing.T) {
	md := "Argo CD syncs the staging cluster from a Helm chart. Deploys got 68% faster."
	s, rejected, ok := groundScene(coverScene{Layout: layoutBars, Labels: []string{"Argo CD", "staging", "Helm chart", "quantum ledger"}, Stat: "-68%"}, md)
	if !ok || len(s.Labels) != 3 || s.Stat != "-68%" || len(rejected) != 1 {
		t.Fatalf("got %+v %v %v", s, rejected, ok)
	}
	// int drew a hub with "teams" and "data": two real labels are not enough.
	if _, _, ok := groundScene(coverScene{Layout: layoutHub, Labels: []string{"Argo CD", "staging"}}, md); ok {
		t.Fatal("a hub needs a centre and three real parts")
	}
	if s, _, _ := groundScene(coverScene{Layout: layoutVersus, Labels: []string{"Argo CD", "staging"}, Stat: "90%"}, md); s.Stat != "" {
		t.Fatalf("a stat the article does not state must go: %+v", s)
	}
}

// A scene too thin to draw is planned once more, with the reason.
func TestGroundedCover_RetriesWithTheReason(t *testing.T) {
	r := testRunner(&noLetterIllustrator{fakeIllustrator{enabled: true}}).WithCoverStore(newMemCovers())
	stub := &stubLLM{responses: []scriptedResponse{{
		trigger: "cannot be drawn",
		content: `{"layout":"hub","labels":["GitOps","Argo CD","staging cluster","Helm chart"],"eyebrow":"DevOps"}`,
	}}}
	r.llm = stub
	md := "GitOps with Argo CD syncs the staging cluster from a Helm chart."
	s := r.groundedCover(context.Background(), "T", md, coverScene{Layout: layoutHub, Labels: []string{"GitOps", "teams", "data"}})
	if s.Layout != layoutHub || len(s.Labels) != 4 {
		t.Fatalf("scene = %+v", s)
	}
	prompt := stub.calls[0].Messages[len(stub.calls[0].Messages)-1].Content
	if !strings.Contains(prompt, "needs at least 4 labels") || !strings.Contains(prompt, "not in the article: teams, data") {
		t.Fatalf("retry prompt does not say why:\n%s", prompt[:300])
	}
}

func TestArticleTerms(t *testing.T) {
	md := "# Title\n\n## Intro\n\nHowever, Agnost runs on Kubernetes. Install Agnost with one command, then use Kusk Gateway.\n" +
		"Create a ReplicaSet for each version; the ReplicaSet keeps old pods. Kusk Gateway reads your OpenAPI file.\n" +
		"Most APIs need an API key. Kubernetes schedules the pods.\n\n```bash\nkubectl apply -f Secret.yaml\n```\n\n" +
		"## References\n\n1. [Helm Docs](https://helm.sh) — Helm\n"
	got := strings.Join(articleTerms(md, 10), ", ")
	for _, want := range []string{"Agnost", "Kubernetes", "Kusk Gateway", "ReplicaSet", "OpenAPI", "API"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
	for _, bad := range []string{"However", "Create", "Install", "Most", "APIs", "Helm", "Secret", "Title", "Intro"} {
		if strings.Contains(", "+got+",", ", "+bad+",") {
			t.Errorf("unexpected %q in %s", bad, got)
		}
	}
}

// Both model scenes unusable: the cover is a hub of what the article names,
// centred on the term its title names — not a flow of its headings.
func TestGroundedCover_FallsBackToTheArticlesTerms(t *testing.T) {
	r := testRunner(&noLetterIllustrator{fakeIllustrator{enabled: true}}).WithCoverStore(newMemCovers())
	stub := &stubLLM{responses: []scriptedResponse{{trigger: "cannot be drawn", content: `{"layout":"hub","labels":["Argo CD","Flux"]}`}}}
	r.llm = stub
	md := "Agnost runs on Kubernetes. Install Agnost, then use Kusk Gateway. Create a ReplicaSet. Kusk Gateway reads OpenAPI. Kubernetes schedules pods."
	s := r.groundedCover(context.Background(), "Shipping with Kusk Gateway", md, coverScene{Layout: layoutHub, Labels: []string{"Argo CD"}})
	if s.Layout != layoutHub || len(s.Labels) < 4 || s.Labels[0] != "Kusk Gateway" {
		t.Fatalf("scene = %+v", s)
	}
	if prompt := stub.calls[0].Messages[len(stub.calls[0].Messages)-1].Content; !strings.Contains(prompt, "THINGS THE ARTICLE NAMES: ") {
		t.Fatal("the retry must offer the article's terms")
	}
}
