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
		"## How the pipeline runs\n\nEvery change goes through the same steps.\n\nMore detail.\n"}

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
	old := "## How it runs\n\nSteps.\n\n![old](" + imagestore.URLPrefix + "o/old.png)\n"
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
	md := "Argo CD syncs the staging cluster. Deploys got 68% faster."
	s := groundScene(coverScene{Layout: layoutBars, Labels: []string{"Argo CD", "staging", "quantum ledger"}, Stat: "-68%"}, md)
	if len(s.Labels) != 2 || s.Stat != "-68%" {
		t.Fatalf("got %+v", s)
	}
	s = groundScene(coverScene{Layout: layoutHub, Labels: []string{"quantum ledger", "Argo CD"}}, md)
	if s.Layout != "" {
		t.Fatalf("one grounded label must drop the scene: %+v", s)
	}
	if s := groundScene(coverScene{Layout: layoutBars, Labels: []string{"Argo CD", "staging"}, Stat: "90%"}, md); s.Stat != "" {
		t.Fatalf("a stat the article does not state must go: %+v", s)
	}
}
