package blog

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"go.uber.org/zap"

	"github.com/jobshout/server/internal/integration/adapters/opsapi"
	"github.com/jobshout/server/internal/model"
)

// fakeCMS records what would have been sent to opsapi and answers with the
// shape a real create returns.
type fakeCMS struct {
	posts []opsapi.CreatePostRequest
	err   error
}

func (f *fakeCMS) Namespace() string { return "acme" }

func (f *fakeCMS) CreatePost(_ context.Context, req opsapi.CreatePostRequest) (*opsapi.Post, error) {
	if f.err != nil {
		return nil, f.err
	}
	f.posts = append(f.posts, req)
	return &opsapi.Post{
		UUID:   fmt.Sprintf("post-%d", len(f.posts)),
		Title:  req.Title,
		Slug:   req.Slug,
		Status: req.Status,
	}, nil
}

// newTestRunner builds a Runner with the given CMS. A nil CMS is the
// interesting case: generation must still work, publishing must not.
func newTestRunner(cms CMSPublisher, responses ...scriptedResponse) *Runner {
	return newTestRunnerWith(cms, &fakeResearcher{}, responses...)
}

// newTestRunnerWith also substitutes the researcher, for tests about what
// happens when research fails or comes back thin.
func newTestRunnerWith(cms CMSPublisher, researcher Researcher, responses ...scriptedResponse) *Runner {
	return NewRunner(Config{
		ContentDir: "content/blogs",
		AuthorName: "Test Writer",
	}, &stubLLM{responses: responses}, cms, researcher, zap.NewNop())
}

// briefsFor is shorthand for a request over plain topics.
func briefsFor(topics ...string) []model.BlogBrief {
	out := make([]model.BlogBrief, 0, len(topics))
	for _, t := range topics {
		out = append(out, model.BlogBrief{Topic: t})
	}
	return out
}

func TestCanPublish(t *testing.T) {
	if newTestRunner(nil).CanPublish() {
		t.Error("CanPublish() = true with no CMS, want false")
	}
	if !newTestRunner(&fakeCMS{}).CanPublish() {
		t.Error("CanPublish() = false with a CMS, want true")
	}
}

// A nil *opsapi.Client held in a non-nil interface is what main.go hands over
// when opsapi is unconfigured. It must not read as "publishing works".
func TestCanPublish_TypedNilClient(t *testing.T) {
	var client *opsapi.Client // NewClient returns this when config is incomplete
	if newTestRunner(client).CanPublish() {
		t.Error("CanPublish() = true for a typed-nil opsapi client, want false")
	}
}

// Generation must not require CMS credentials — the whole point of the split is
// that an article can be written and read without anything leaving the system.
func TestGenerate_WithoutCMS(t *testing.T) {
	r := newTestRunner(nil, writeScript("Kubernetes", "# Kubernetes\n\nBody.")...)

	arts, err := r.Generate(context.Background(), GenerateRequest{
		Briefs: briefsFor("Kubernetes debugging"),
	}, nil)
	if err != nil {
		t.Fatalf("Generate without CMS: %v", err)
	}
	if len(arts) != 1 {
		t.Fatalf("want 1 article, got %d", len(arts))
	}
	if !strings.Contains(arts[0].Markdown, "# Kubernetes") {
		t.Errorf("markdown not returned: %q", arts[0].Markdown)
	}
}

// Every generated article carries its HTML: conversion is part of the pipeline,
// not something publishing does on the way out.
func TestGenerate_RendersHTML(t *testing.T) {
	r := newTestRunner(nil, writeScript("Title", "# Title\n\nHello **world**.")...)

	arts, err := r.Generate(context.Background(), GenerateRequest{Briefs: briefsFor("t")}, nil)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	got := arts[0]
	if got.Title != "Title" {
		t.Errorf("Title = %q, want %q", got.Title, "Title")
	}
	if !strings.Contains(got.HTML, "<strong>world</strong>") {
		t.Errorf("HTML not rendered: %q", got.HTML)
	}
	if strings.Contains(got.HTML, "<h1>") {
		t.Errorf("leading H1 should be dropped, the CMS renders the title: %q", got.HTML)
	}
	if got.Excerpt == "" {
		t.Error("Excerpt should be derived during generation")
	}
}

func TestPublish_WithoutCMSIsRefused(t *testing.T) {
	r := newTestRunner(nil)
	_, err := r.Publish(context.Background(), []GeneratedArticle{{Topic: "x", Markdown: "# x"}}, nil)
	if err == nil {
		t.Fatal("expected Publish to be refused without a CMS")
	}
	if !strings.Contains(err.Error(), "OPSAPI_BASE_URL") {
		t.Errorf("error should name the missing config, got: %v", err)
	}
}

func TestPublish_NothingToPublish(t *testing.T) {
	r := newTestRunner(&fakeCMS{})
	if _, err := r.Publish(context.Background(), nil, nil); err == nil {
		t.Fatal("expected an error when publishing zero articles")
	}
}

// Everything this pipeline sends is a draft. A run that could publish straight
// to a live site would make review optional, which is the opposite of the point.
func TestPublish_AlwaysCreatesDrafts(t *testing.T) {
	cms := &fakeCMS{}
	r := newTestRunner(cms, writeScript("One", "# One\n\nBody one.")...)

	arts, err := r.Generate(context.Background(), GenerateRequest{Briefs: briefsFor("one", "two")}, nil)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	result, err := r.Publish(context.Background(), arts, nil)
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}

	if len(cms.posts) != 2 {
		t.Fatalf("sent %d posts, want 2", len(cms.posts))
	}
	for i, p := range cms.posts {
		if p.Status != opsapi.StatusDraft {
			t.Errorf("post %d status = %q, want %q", i, p.Status, opsapi.StatusDraft)
		}
		if p.ContentHTML == "" {
			t.Errorf("post %d has no HTML body", i)
		}
		if p.AuthorName != "Test Writer" {
			t.Errorf("post %d author = %q, want the configured byline", i, p.AuthorName)
		}
	}
	if len(result.Posts) != 2 {
		t.Fatalf("result has %d posts, want 2", len(result.Posts))
	}
	if result.Namespace != "acme" {
		t.Errorf("result namespace = %q, want %q", result.Namespace, "acme")
	}
	// The slug is how the service matches a result back to a stored article.
	if result.Posts[0].Slug != arts[0].Slug {
		t.Errorf("result slug = %q, want %q", result.Posts[0].Slug, arts[0].Slug)
	}
	if result.Posts[0].PostUUID == "" {
		t.Error("result should carry the CMS post UUID")
	}
}

// Articles stored before HTML rendering existed have markdown and nothing else.
// Publishing them must work rather than refusing on a missing field.
func TestPublish_RendersArticlesWithoutHTML(t *testing.T) {
	cms := &fakeCMS{}
	r := newTestRunner(cms)

	_, err := r.Publish(context.Background(), []GeneratedArticle{{
		Topic: "legacy", Slug: "legacy", Markdown: "# Legacy\n\nStored before HTML existed.",
	}}, nil)
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if len(cms.posts) != 1 {
		t.Fatalf("sent %d posts, want 1", len(cms.posts))
	}
	if !strings.Contains(cms.posts[0].ContentHTML, "<p>") {
		t.Errorf("body was not rendered on the way out: %q", cms.posts[0].ContentHTML)
	}
	if cms.posts[0].Title != "Legacy" {
		t.Errorf("title = %q, want the markdown H1", cms.posts[0].Title)
	}
}

func TestGenerate_NoTopics(t *testing.T) {
	r := newTestRunner(nil)
	if _, err := r.Generate(context.Background(), GenerateRequest{Briefs: briefsFor(" ", "")}, nil); err == nil {
		t.Fatal("expected an error when every topic is blank")
	}
}

// The batch is capped regardless of what the caller asks for, so a typo cannot
// produce 25 drafts in the CMS.
func TestGenerate_CapsTopics(t *testing.T) {
	topics := make([]string, HardMaxArticles+5)
	for i := range topics {
		topics[i] = "topic " + string(rune('a'+i))
	}

	r := newTestRunner(nil, writeScript("T", "# T\n\nbody")...)
	arts, err := r.Generate(context.Background(), GenerateRequest{Briefs: briefsFor(topics...)}, nil)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if len(arts) != HardMaxArticles {
		t.Errorf("got %d articles, want the hard cap of %d", len(arts), HardMaxArticles)
	}
}

// Generation reports its steps in order so the UI can show "ready" rather than
// leaving the last per-topic label running forever.
func TestGenerate_ReportsSteps(t *testing.T) {
	var keys []string
	r := newTestRunner(nil, writeScript("A", "# a\n\nbody")...)
	if _, err := r.Generate(context.Background(), GenerateRequest{Briefs: briefsFor("a")},
		func(key, _ string) { keys = append(keys, key) }); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	if len(keys) < 3 {
		t.Fatalf("want the writing steps plus converting and generated, got %v", keys)
	}
	// Research comes first now: nothing is written before there are sources.
	if keys[0] != model.BlogStepResearching {
		t.Errorf("first step = %q, want %q", keys[0], model.BlogStepResearching)
	}
	if keys[len(keys)-2] != model.BlogStepConverting {
		t.Errorf("second-to-last step = %q, want %q", keys[len(keys)-2], model.BlogStepConverting)
	}
	if keys[len(keys)-1] != model.BlogStepGenerated {
		t.Errorf("last step = %q, want %q", keys[len(keys)-1], model.BlogStepGenerated)
	}
}

// Publishing loads articles back from the database, where only markdown, HTML
// and the identifying fields are stored — Title and Excerpt are derived and do
// not survive the round trip. Publish must recompute them, because opsapi
// rejects a post with no title outright.
func TestPublish_RecomputesDerivedFieldsFromStoredArticle(t *testing.T) {
	cms := &fakeCMS{}
	r := newTestRunner(cms)

	// Exactly what service.Publish builds from a blog_articles row.
	stored := GeneratedArticle{
		Topic:    "kubernetes debugging",
		Slug:     "kubernetes-debugging",
		Path:     "content/blogs/2026-08-12-kubernetes-debugging.md",
		Markdown: "# Debugging Kubernetes\n\nStart with the events.",
		HTML:     "<p>Start with the events.</p>",
	}

	if _, err := r.Publish(context.Background(), []GeneratedArticle{stored}, nil); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if len(cms.posts) != 1 {
		t.Fatalf("sent %d posts, want 1", len(cms.posts))
	}
	got := cms.posts[0]
	if got.Title != "Debugging Kubernetes" {
		t.Errorf("Title = %q, want the markdown H1 — opsapi 400s on an empty title", got.Title)
	}
	if got.SEOTitle == "" {
		t.Error("SEOTitle is empty")
	}
	if got.Excerpt == "" {
		t.Error("Excerpt is empty — the draft would have no summary")
	}
	// The stored HTML is what was reviewed, so it must be sent verbatim.
	if got.ContentHTML != stored.HTML {
		t.Errorf("ContentHTML = %q, want the stored HTML %q", got.ContentHTML, stored.HTML)
	}
}
