package blog

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jobshout/server/internal/integration/adapters/jobshoutcom"
	"github.com/jobshout/server/internal/model"
)

type fakeInsights struct {
	got    []jobshoutcom.SubmitInsightRequest
	failOn int // 1-based submission that fails; 0 = never
}

func (f *fakeInsights) SubmitInsight(_ context.Context, req jobshoutcom.SubmitInsightRequest) (*jobshoutcom.Insight, error) {
	f.got = append(f.got, req)
	if f.failOn == len(f.got) {
		return nil, errors.New("400 pick at least one topic")
	}
	return &jobshoutcom.Insight{ID: "id-" + req.Title, Slug: slugify(req.Title), Status: "pending_review"}, nil
}

func insightsRunner(p InsightsPublisher) *Runner {
	r := &Runner{cfg: Config{PublicBaseURL: "https://int.jobshout.co.uk/"}, logger: testLogger(),
		clock: func() time.Time { return time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC) }}
	return r.WithInsights(p)
}

func TestCanPublishInsights(t *testing.T) {
	if (&Runner{}).CanPublishInsights() {
		t.Error("no publisher: want false")
	}
	var nilClient *jobshoutcom.Client
	if (&Runner{}).WithInsights(nilClient).CanPublishInsights() {
		t.Error("typed-nil client: want false")
	}
	if !insightsRunner(&fakeInsights{}).CanPublishInsights() {
		t.Error("fake publisher: want true")
	}
}

func TestInsightsMarkdown(t *testing.T) {
	md := "# The Title\n\nIntro para.\n\n```mermaid\nflowchart LR\n  A-->B\n```\n\n" +
		"![Figure 1](/api/v1/images/file/fig.png)\n\n![Remote](https://cdn.example.com/x.png)\n\n## References\n\n1. Source"
	got := insightsMarkdown(md, "https://int.jobshout.co.uk")

	if strings.Contains(got, "# The Title") {
		t.Error("leading H1 should be removed; Insights shows the title itself")
	}
	if strings.Contains(got, "```mermaid") || !strings.Contains(got, "![Diagram](https://mermaid.ink/svg/") {
		t.Errorf("mermaid fence should become a mermaid.ink image:\n%s", got)
	}
	if !strings.Contains(got, "![Figure 1](https://int.jobshout.co.uk/api/v1/images/file/fig.png)") {
		t.Errorf("relative figure should be absolute:\n%s", got)
	}
	if !strings.Contains(got, "![Remote](https://cdn.example.com/x.png)") {
		t.Error("absolute images must be left alone")
	}
	if !strings.Contains(got, "## References") {
		t.Error("references section must survive")
	}

	// Without a public base a relative figure cannot be made loadable; leave
	// it rather than inventing a host.
	if got := insightsMarkdown("![f](/api/v1/images/file/a.png)", ""); got != "![f](/api/v1/images/file/a.png)" {
		t.Errorf("no base: got %q", got)
	}
}

func TestInsightTopics(t *testing.T) {
	cases := []struct {
		text string
		want []string
	}{
		{"How AI agents are changing hiring and interviews", []string{"hiring", "ai-trends"}},
		{"Salaries and layoffs: the AI job market in 2026, and which skills to learn", []string{"job-market", "skills-careers", "ai-trends"}},
		{"The EU AI Act and your compliance checklist", []string{"policy-regulation", "ai-trends"}},
		{"Gardening tips", []string{"ai-trends"}},
	}
	for _, c := range cases {
		got := insightTopics(c.text)
		if strings.Join(got, ",") != strings.Join(c.want, ",") {
			t.Errorf("insightTopics(%q) = %v, want %v", c.text, got, c.want)
		}
		if len(got) == 0 || len(got) > 3 {
			t.Errorf("insightTopics(%q) returned %d topics; want 1-3", c.text, len(got))
		}
	}
}

func TestInsightTopics_IgnoresURLsAndCode(t *testing.T) {
	md := "Hiring is changing.\n\n![fig](/api/v1/images/file/a.png)\n\n```go\nclient := sdk.New()\n```\n\n[source](https://example.com/benchmark)"
	got := insightTopics(proseOnly(md))
	for _, slug := range got {
		if slug == "tools-research" {
			t.Errorf("topics %v: a figure URL or code sample picked tools-research", got)
		}
	}
	if got[0] != "hiring" {
		t.Errorf("topics %v: want hiring first", got)
	}
}

func TestInsightFromArticle(t *testing.T) {
	a := GeneratedArticle{
		Topic:         "AI hiring",
		Title:         strings.Repeat("Long headline ", 20),
		Markdown:      "# H\n\nHiring with AI agents.",
		HTML:          "<p>" + strings.Repeat("Hiring with AI agents is changing recruitment. ", 12) + "</p>",
		CoverImageURL: "/api/v1/images/file/cover.png",
	}
	req := insightFromArticle(a, "https://int.jobshout.co.uk")

	if req.Kind != jobshoutcom.KindArticle || !req.Submit {
		t.Errorf("kind=%q submit=%v", req.Kind, req.Submit)
	}
	if n := len([]rune(req.Title)); n > insightsMaxTitle {
		t.Errorf("title is %d runes, limit %d", n, insightsMaxTitle)
	}
	if req.Summary == "" || len([]rune(req.Summary)) > insightsMaxSummary {
		t.Errorf("summary %q", req.Summary)
	}
	if req.CoverImageURL != "https://int.jobshout.co.uk/api/v1/images/file/cover.png" || req.CoverImageAlt == "" {
		t.Errorf("cover=%q alt=%q: Insights needs an absolute cover with alt text", req.CoverImageURL, req.CoverImageAlt)
	}
	if len(req.Topics) == 0 {
		t.Error("Insights rejects a submission with no topic")
	}

	a.CoverImageURL = ""
	if req := insightFromArticle(a, "https://x"); req.CoverImageURL != "" || req.CoverImageAlt != "" {
		t.Error("no cover: send neither URL nor alt")
	}
}

func TestInsightFromArticle_SummaryIgnoresStaleStoredHTML(t *testing.T) {
	// Stored before the underline fix: the HTML opens with the stray line.
	a := GeneratedArticle{
		Title:    "Hallucination-Free LLMs",
		Markdown: "# Hallucination-Free LLMs\n==========\n\n## Why\n\nObservability matters for AI agents.",
		HTML:     "<p>==========</p>\n<h2>Why</h2>\n<p>Observability matters for AI agents.</p>",
	}
	req := insightFromArticle(a, "https://x")
	if req.Summary != "Observability matters for AI agents." {
		t.Errorf("summary = %q; want the first paragraph, without the underline or the section title", req.Summary)
	}
	if strings.Contains(req.BodyMarkdown, "===") {
		t.Errorf("body kept the underline: %q", req.BodyMarkdown)
	}
}

func TestPublishInsights_FilesEachArticle(t *testing.T) {
	fake := &fakeInsights{}
	var steps []string
	res, err := insightsRunner(fake).PublishInsights(context.Background(), []GeneratedArticle{
		{Topic: "a", Slug: "a", Title: "First", Markdown: "# First\n\nAI body"},
		{Topic: "b", Slug: "b", Title: "Second", Markdown: "# Second\n\nAI body"},
	}, func(key, _, _ string) { steps = append(steps, key) })
	if err != nil {
		t.Fatal(err)
	}
	if len(fake.got) != 2 || len(res.Posts) != 2 {
		t.Fatalf("submitted %d, recorded %d; want 2 and 2", len(fake.got), len(res.Posts))
	}
	if res.Posts[1].Slug != "b" || res.Posts[1].ItemID != "id-Second" || res.Posts[1].Status != "pending_review" {
		t.Errorf("post = %+v", res.Posts[1])
	}
	if steps[len(steps)-1] != model.BlogStepInsightsSent {
		t.Errorf("last step = %q, want %q", steps[len(steps)-1], model.BlogStepInsightsSent)
	}
}

func TestPublishInsights_PartialFailureReturnsWhatWasFiled(t *testing.T) {
	fake := &fakeInsights{failOn: 2}
	res, err := insightsRunner(fake).PublishInsights(context.Background(), []GeneratedArticle{
		{Slug: "a", Title: "First", Markdown: "x"},
		{Slug: "b", Title: "Second", Markdown: "x"},
		{Slug: "c", Title: "Third", Markdown: "x"},
	}, nil)
	if err == nil || !strings.Contains(err.Error(), "insights 2/3") {
		t.Fatalf("err = %v", err)
	}
	// The first article was filed and must be recorded, or a retry files it twice.
	if res == nil || len(res.Posts) != 1 || res.Posts[0].Slug != "a" {
		t.Fatalf("result = %+v; want the one filed article", res)
	}
	if len(fake.got) != 2 {
		t.Errorf("stopped after the failure? submitted %d, want 2", len(fake.got))
	}
}

func TestPublishInsights_NotConfigured(t *testing.T) {
	if _, err := (&Runner{}).PublishInsights(context.Background(), []GeneratedArticle{{}}, nil); err == nil {
		t.Error("want an error when Insights is not configured")
	}
}
