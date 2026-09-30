package blog

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/jobshout/server/internal/model"
)

// A live Insights draft ran into the token ceiling and ended "…is critical,
// GPT-" under its last heading.
func TestTrimIncompleteTail_LiveInsightsDraft(t *testing.T) {
	raw, err := os.ReadFile("testdata/live_insights_truncated.md")
	if err != nil {
		t.Fatal(err)
	}
	// The fixture is the stored article; what the model returned is everything
	// before the pipeline's own reference list.
	draft := string(raw)
	draft = strings.TrimSpace(draft[:strings.LastIndex(draft, "## References")])
	if !strings.HasSuffix(draft, "GPT-") {
		t.Fatalf("fixture no longer ends mid-sentence: %q", draft[len(draft)-40:])
	}

	out := trimIncompleteTail(draft)

	if strings.HasSuffix(out, "GPT-") || strings.Contains(out, "It is important to acknowledge the limitations") {
		t.Errorf("the unfinished paragraph survived: …%q", out[len(out)-80:])
	}
	if strings.Contains(out, "## Where the Technology Does Not Fit") {
		t.Error("the heading of the dropped paragraph survived with nothing under it")
	}
	if !strings.HasSuffix(out, "Robust testing and fallback mechanisms are essential.") {
		t.Errorf("trimmed too far or not enough; now ends: …%q", out[len(out)-80:])
	}
}

func TestTrimIncompleteTail(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"finished text is untouched", "# T\n\nOne.\n\nTwo [2].", "# T\n\nOne.\n\nTwo [2]."},
		{"unfinished paragraph", "# T\n\nOne.\n\nTwo is a", "# T\n\nOne."},
		{"unclosed code fence", "# T\n\nOne.\n\n```go\nfunc main() {", "# T\n\nOne."},
		{"half a table row", "# T\n\nOne.\n\n| a | b |\n|---|---|\n| 1 | 2", "# T\n\nOne."},
		{"a list is kept", "# T\n\nOne.\n\n- first\n- second", "# T\n\nOne.\n\n- first\n- second"},
		{"closed fence is kept", "# T\n\n```go\nx := 1\n```", "# T\n\n```go\nx := 1\n```"},
	}
	for _, tc := range cases {
		if got := trimIncompleteTail(tc.in); got != tc.want {
			t.Errorf("%s:\n got %q\nwant %q", tc.name, got, tc.want)
		}
	}
}

// A draft cut at the ceiling reaches the reader trimmed, not mid-sentence.
func TestGenerate_DraftCutAtTheCeilingIsTrimmed(t *testing.T) {
	script := writeScript("T", "# T\n\nA finished paragraph [1].\n\n## Next\n\nThis one stops half-way thr")
	for i := range script {
		if script[i].trigger == promptDraft {
			script[i].finish = "length"
		}
	}
	r := newRunnerWithStub(&stubLLM{responses: script})

	got, err := r.Generate(context.Background(), GenerateRequest{
		Briefs: []model.BlogBrief{{Topic: "Gateway API"}},
	}, nil)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if strings.Contains(got[0].Markdown, "half-way thr") || strings.Contains(got[0].Markdown, "## Next") {
		t.Errorf("the unfinished ending was published:\n%s", got[0].Markdown)
	}
	if !strings.Contains(got[0].Markdown, "A finished paragraph") {
		t.Errorf("the finished part was lost:\n%s", got[0].Markdown)
	}
}

// The Insights reader's longest piece does not fit the default ceiling.
func TestArticleTokens_ScalesWithTheReader(t *testing.T) {
	if got := articleTokens(model.BlogBrief{}); got != maxArticleTokens {
		t.Errorf("default reader = %d, want %d", got, maxArticleTokens)
	}
	if got := articleTokens(model.BlogBrief{Audience: "insights"}); got <= maxArticleTokens {
		t.Errorf("insights reader = %d, want more than %d", got, maxArticleTokens)
	}
}

// Running out of time during review is not a critic being unavailable: the
// unreviewed draft must not be returned as a finished article.
func TestGenerate_DeadlineDuringReviewDoesNotShipTheDraft(t *testing.T) {
	ctx, expire := context.WithCancel(context.Background())
	defer expire()
	stub := &stubLLM{responses: writeScript("T", "# T\n\nBody [1]."), deadlineOn: promptReview, expire: expire}
	r := newRunnerWithStub(stub)

	stored := 0
	got, err := r.Generate(ctx, GenerateRequest{
		Briefs:    []model.BlogBrief{{Topic: "Gateway API"}},
		OnArticle: func(GeneratedArticle) error { stored++; return nil },
	}, nil)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want the context's", err)
	}
	if len(got) != 0 || stored != 0 {
		t.Fatalf("an unreviewed draft was returned (%d) or stored (%d)", len(got), stored)
	}
}

// A reviewer that is simply unavailable still costs only the revision pass.
func TestGenerate_FailedReviewStillShipsTheDraft(t *testing.T) {
	stub := &stubLLM{responses: writeScript("T", "# T\n\nBody [1]."), failOn: promptReview}
	got, err := newRunnerWithStub(stub).Generate(context.Background(), GenerateRequest{
		Briefs: []model.BlogBrief{{Topic: "Gateway API"}},
	}, nil)
	if err != nil || len(got) != 1 {
		t.Fatalf("Generate = %d article(s), %v; want the draft kept", len(got), err)
	}
}

func TestCollapseRepeatedCitations(t *testing.T) {
	cases := map[string]string{
		"cost per task [1][1].":           "cost per task [1].",
		"benchmarks [2][2][2] exist":      "benchmarks [2] exist",
		"sources [1], [1], and more":      "sources [1], and more",
		"two sources [1][2] agree":        "two sources [1][2] agree",
		"mixed [2][1][2] run":             "mixed [2][1] run",
		"apart [1] and later [1] is fine": "apart [1] and later [1] is fine",
	}
	for in, want := range cases {
		if got := collapseRepeatedCitations(in); got != want {
			t.Errorf("%q → %q, want %q", in, got, want)
		}
	}
}

// The larger window is for the calls that write article text. The JSON stages
// run on the worker model, which research is using at the default window;
// asking for a different one there would make Ollama reload it.
func TestProseNumCtx_OnlyOnProseCalls(t *testing.T) {
	stub := &stubLLM{responses: writeScript("T", "# T\n\nBody [1].")}
	r := NewRunner(Config{ProseNumCtx: 16384}, stub, nil, &fakeResearcher{}, testLogger())

	if _, err := r.Generate(context.Background(), GenerateRequest{
		Briefs: []model.BlogBrief{{Topic: "Gateway API"}},
	}, nil); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	prose, structured := 0, 0
	for _, call := range stub.calls {
		prompt := call.Messages[len(call.Messages)-1].Content
		isProse := strings.Contains(prompt, promptDraft) || strings.Contains(prompt, promptRevise) || strings.Contains(prompt, promptExpand)
		switch {
		case isProse && call.NumCtx == 16384:
			prose++
		case isProse:
			t.Errorf("a prose call asked for num_ctx %d, want 16384", call.NumCtx)
		case call.NumCtx != 0:
			t.Errorf("a JSON stage asked for num_ctx %d, want the client default", call.NumCtx)
		default:
			structured++
		}
	}
	if prose == 0 || structured == 0 {
		t.Fatalf("saw %d prose and %d structured calls; the test needs both", prose, structured)
	}
}

// The outline and the review must come back as JSON, so they ask for JSON mode;
// the calls that write the article must not, or the prose would be forced into
// a JSON value.
func TestJSONMode_OnStructuredStagesOnly(t *testing.T) {
	stub := &stubLLM{responses: writeScript("T", "# T\n\nBody [1].")}
	if _, err := newRunnerWithStub(stub).Generate(context.Background(), GenerateRequest{
		Briefs: []model.BlogBrief{{Topic: "Gateway API"}},
	}, nil); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	sawPlan, sawReview, sawDraft := false, false, false
	for _, call := range stub.calls {
		prompt := call.Messages[len(call.Messages)-1].Content
		switch {
		case strings.Contains(prompt, promptReview):
			sawReview = true
			if !call.JSON {
				t.Error("the review did not ask for JSON mode")
			}
		case strings.Contains(prompt, promptPlan):
			sawPlan = true
			if !call.JSON {
				t.Error("the outline did not ask for JSON mode")
			}
		case strings.Contains(prompt, promptDraft):
			sawDraft = true
			if call.JSON {
				t.Error("the draft asked for JSON mode")
			}
		}
	}
	if !sawPlan || !sawReview || !sawDraft {
		t.Fatalf("plan=%v review=%v draft=%v; the test needs all three", sawPlan, sawReview, sawDraft)
	}
}
