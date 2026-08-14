package blog

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/research"
)

// The title is the agent's, chosen from what the research found — not the topic
// echoed back. This is the whole point of treating input as a subject.
func TestWrite_TitleComesFromThePlanNotTheTopic(t *testing.T) {
	r := newTestRunner(nil, writeScript(
		"Gateway API Is GA and Ingress Is Done",
		"# Gateway API Is GA and Ingress Is Done\n\nBody [1].",
	)...)

	arts, err := r.Generate(context.Background(), GenerateRequest{
		Briefs: briefsFor("kubernetes ingress"),
	}, nil)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if arts[0].Title != "Gateway API Is GA and Ingress Is Done" {
		t.Errorf("Title = %q, want the agent's chosen title", arts[0].Title)
	}
	if arts[0].Topic != "kubernetes ingress" {
		t.Errorf("Topic = %q, want the original subject preserved", arts[0].Topic)
	}
	// The slug follows the title, since that is what the article is about.
	if !strings.HasPrefix(arts[0].Slug, "gateway-api-is-ga") {
		t.Errorf("Slug = %q, want it derived from the title", arts[0].Slug)
	}
}

// The requester's context is half the instruction. It must reach both the
// research and the writing, or supplying it changes nothing.
func TestWrite_ContextReachesResearchAndDraft(t *testing.T) {
	researcher := &fakeResearcher{}
	llmStub := &stubLLM{responses: writeScript("T", "# T\n\nBody [1].")}
	r := NewRunner(Config{ContentDir: "content/blogs"}, llmStub, nil, researcher, testLogger())

	_, err := r.Generate(context.Background(), GenerateRequest{
		Briefs: []model.BlogBrief{{
			Topic:   "Gateway API",
			Context: "Assume the reader already runs Ingress in production.",
		}},
	}, nil)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	if len(researcher.requests) != 1 {
		t.Fatalf("researcher called %d times, want 1", len(researcher.requests))
	}
	if !strings.Contains(researcher.requests[0].Context, "already runs Ingress") {
		t.Errorf("research request lost the context: %+v", researcher.requests[0])
	}

	var sawInDraft bool
	for _, c := range llmStub.calls {
		if strings.Contains(c.Messages[0].Content, "already runs Ingress") {
			sawInDraft = true
		}
	}
	if !sawInDraft {
		t.Error("the requester's context never reached the writing prompts")
	}
}

// Citations are resolved against sources that were actually retrieved. A
// citation to a number that was never offered is removed rather than left to
// imply a reference that cannot be printed.
func TestWrite_DropsCitationsToSourcesThatDoNotExist(t *testing.T) {
	r := newTestRunner(nil, writeScript("T",
		"# T\n\nReal claim [1]. Invented claim [9].\n",
	)...)

	arts, err := r.Generate(context.Background(), GenerateRequest{Briefs: briefsFor("t")}, nil)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	md := arts[0].Markdown

	if strings.Contains(md, "[9]") {
		t.Errorf("a citation to a non-existent source survived:\n%s", md)
	}
	if !strings.Contains(md, "[1]") {
		t.Errorf("the valid citation was lost:\n%s", md)
	}
	if len(arts[0].References) != 1 {
		t.Errorf("got %d references, want 1", len(arts[0].References))
	}
}

// The reference list is what the article cites, not everything the researcher
// read. A source nothing cites does not belong in it.
func TestWrite_ReferencesContainOnlyCitedSources(t *testing.T) {
	// The brief carries two sources; the draft cites only the first.
	r := newTestRunner(nil, writeScript("T", "# T\n\nOnly the first source is used [1].")...)

	arts, err := r.Generate(context.Background(), GenerateRequest{Briefs: briefsFor("t")}, nil)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	if len(arts[0].References) != 1 {
		t.Fatalf("got %d references, want only the cited one: %+v", len(arts[0].References), arts[0].References)
	}
	if arts[0].References[0].URL != "https://kubernetes.io/blog/ga" {
		t.Errorf("wrong source in the reference list: %+v", arts[0].References[0])
	}
	if !strings.Contains(arts[0].Markdown, "## References") {
		t.Error("no reference list was appended to the article")
	}
	if !strings.Contains(arts[0].Markdown, "https://kubernetes.io/blog/ga") {
		t.Error("the reference list does not contain the cited URL")
	}
}

// Research failing must fail the article. Writing anyway would produce exactly
// the unsourced content this pipeline exists to avoid.
func TestWrite_ResearchFailureFailsTheArticle(t *testing.T) {
	r := newTestRunnerWith(nil,
		&fakeResearcher{err: fmt.Errorf("no sources found")},
		writeScript("T", "# T\n\nBody.")...)

	_, err := r.Generate(context.Background(), GenerateRequest{Briefs: briefsFor("t")}, nil)
	if err == nil {
		t.Fatal("Generate succeeded despite research failing")
	}
	if !strings.Contains(err.Error(), "no sources") {
		t.Errorf("error %q does not explain that research failed", err)
	}
}

// An empty brief is not a licence to write from nothing.
func TestWrite_UnusableBriefFailsTheArticle(t *testing.T) {
	r := newTestRunnerWith(nil,
		&fakeResearcher{brief: &research.Brief{Topic: "t"}}, // no findings, no sources
		writeScript("T", "# T\n\nBody.")...)

	_, err := r.Generate(context.Background(), GenerateRequest{Briefs: briefsFor("t")}, nil)
	if err == nil {
		t.Fatal("Generate succeeded on a brief with no verified findings")
	}
}

// The agent revises when its own review finds problems.
func TestWrite_RevisesWhenReviewFindsIssues(t *testing.T) {
	responses := []scriptedResponse{
		{trigger: promptPlan, content: `{"title":"T","angle":"a","sections":["One"]}`},
		{trigger: promptDraft, content: "# T\n\nFirst draft, uncited claim."},
		{trigger: promptReview, content: `{"issues":["The claim in paragraph 1 has no citation."]}`},
		{trigger: promptRevise, content: "# T\n\nRevised draft with a citation [1]."},
	}
	r := newTestRunner(nil, responses...)

	var steps []string
	arts, err := r.Generate(context.Background(), GenerateRequest{Briefs: briefsFor("t")},
		func(key, _, _ string) { steps = append(steps, key) })
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	if !strings.Contains(arts[0].Markdown, "Revised draft") {
		t.Errorf("the revision was not kept:\n%s", arts[0].Markdown)
	}
	var revised bool
	for _, s := range steps {
		if s == model.BlogStepRevising {
			revised = true
		}
	}
	if !revised {
		t.Errorf("the revising step was never reported: %v", steps)
	}
}

// A clean draft is not rewritten for the sake of it.
func TestWrite_SkipsRevisionWhenReviewIsClean(t *testing.T) {
	r := newTestRunner(nil, writeScript("T", "# T\n\nA clean draft [1].")...)

	var steps []string
	arts, err := r.Generate(context.Background(), GenerateRequest{Briefs: briefsFor("t")},
		func(key, _, _ string) { steps = append(steps, key) })
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	if !strings.Contains(arts[0].Markdown, "A clean draft") {
		t.Error("the clean draft was replaced")
	}
	for _, s := range steps {
		if s == model.BlogStepRevising {
			t.Errorf("revised a draft the reviewer had no complaints about: %v", steps)
		}
	}
}

// A broken reviewer costs the revision pass, not the article — the draft is
// already written from verified sources.
func TestWrite_SurvivesReviewFailure(t *testing.T) {
	llmStub := &stubLLM{
		failOn:    promptReview,
		responses: writeScript("T", "# T\n\nBody [1]."),
	}
	r := NewRunner(Config{ContentDir: "content/blogs"}, llmStub, nil, &fakeResearcher{}, testLogger())

	arts, err := r.Generate(context.Background(), GenerateRequest{Briefs: briefsFor("t")}, nil)
	if err != nil {
		t.Fatalf("Generate failed when only the reviewer was broken: %v", err)
	}
	if !strings.Contains(arts[0].Markdown, "Body") {
		t.Error("the unrevised draft was discarded")
	}
}

// A Runner with no researcher cannot do its job, and should say so plainly
// rather than producing an unsourced article.
func TestWrite_RefusesWithoutAResearcher(t *testing.T) {
	r := NewRunner(Config{}, &stubLLM{}, nil, nil, testLogger())

	_, err := r.Generate(context.Background(), GenerateRequest{Briefs: briefsFor("t")}, nil)
	if err == nil {
		t.Fatal("Generate succeeded with no researcher configured")
	}
	if !strings.Contains(err.Error(), "research") {
		t.Errorf("error %q does not name the missing dependency", err)
	}
}

// longBody returns markdown of roughly n words, for exercising the length guard.
func longBody(title string, n int) string {
	return "# " + title + "\n\n" + strings.TrimSpace(strings.Repeat("substantive sentence about the subject [1]. ", n/6))
}

// Asking for a word count is not getting one: a live run returned 382 words
// against a 900-word instruction. The floor is checked, not trusted.
func TestWrite_ExpandsAnArticleThatCameInShort(t *testing.T) {
	responses := []scriptedResponse{
		{trigger: promptPlan, content: `{"title":"T","angle":"a","sections":["One"]}`},
		{trigger: promptDraft, content: "# T\n\nFar too short [1]."},
		{trigger: promptReview, content: `{"issues":[]}`},
		{trigger: promptExpand, content: longBody("T", MinArticleWords+120)},
	}
	r := newTestRunner(nil, responses...)

	var steps []string
	arts, err := r.Generate(context.Background(), GenerateRequest{Briefs: briefsFor("t")},
		func(key, _, _ string) { steps = append(steps, key) })
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	if arts[0].WordCount < MinArticleWords {
		t.Errorf("got %d words, want at least %d after expansion", arts[0].WordCount, MinArticleWords)
	}
	var expanded bool
	for _, s := range steps {
		if s == model.BlogStepExpanding {
			expanded = true
		}
	}
	if !expanded {
		t.Errorf("the expanding step was never reported: %v", steps)
	}
}

// An article already at length is left alone — expansion is a repair, not a
// routine pass.
func TestWrite_SkipsExpansionWhenLongEnough(t *testing.T) {
	responses := []scriptedResponse{
		{trigger: promptPlan, content: `{"title":"T","angle":"a","sections":["One"]}`},
		{trigger: promptDraft, content: longBody("T", MinArticleWords+200)},
		{trigger: promptReview, content: `{"issues":[]}`},
	}
	r := newTestRunner(nil, responses...)

	var steps []string
	if _, err := r.Generate(context.Background(), GenerateRequest{Briefs: briefsFor("t")},
		func(key, _, _ string) { steps = append(steps, key) }); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	for _, s := range steps {
		if s == model.BlogStepExpanding {
			t.Errorf("expanded an article that was already long enough: %v", steps)
		}
	}
}

// A model that "expands" to something no longer than the original has usually
// rewritten it shorter and blander. Keep what we had.
func TestWrite_KeepsOriginalWhenExpansionDoesNotLengthen(t *testing.T) {
	responses := []scriptedResponse{
		{trigger: promptPlan, content: `{"title":"T","angle":"a","sections":["One"]}`},
		{trigger: promptDraft, content: "# T\n\nThe original short draft with its own distinctive wording [1]."},
		{trigger: promptReview, content: `{"issues":[]}`},
		{trigger: promptExpand, content: "# T\n\nShorter [1]."},
	}
	r := newTestRunner(nil, responses...)

	arts, err := r.Generate(context.Background(), GenerateRequest{Briefs: briefsFor("t")}, nil)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if !strings.Contains(arts[0].Markdown, "distinctive wording") {
		t.Errorf("the original was replaced by a shorter expansion:\n%s", arts[0].Markdown)
	}
}

// A failed expansion costs length, not the article.
func TestWrite_SurvivesExpansionFailure(t *testing.T) {
	llmStub := &stubLLM{
		failOn:    promptExpand,
		responses: writeScript("T", "# T\n\nShort but real [1]."),
	}
	r := NewRunner(Config{ContentDir: "content/blogs"}, llmStub, nil, &fakeResearcher{}, testLogger())

	arts, err := r.Generate(context.Background(), GenerateRequest{Briefs: briefsFor("t")}, nil)
	if err != nil {
		t.Fatalf("Generate failed when only expansion was broken: %v", err)
	}
	if !strings.Contains(arts[0].Markdown, "Short but real") {
		t.Error("the short draft was discarded")
	}
}

// Each step records which agent performed it, so the trace shows the handover
// rather than presenting the run as one anonymous process.
func TestWrite_AttributesStepsToTheRightAgent(t *testing.T) {
	r := newTestRunner(nil, writeScript("T", "# T\n\nBody [1].")...)

	got := map[string]string{}
	if _, err := r.Generate(context.Background(), GenerateRequest{Briefs: briefsFor("t")},
		func(key, _, agent string) { got[key] = agent }); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	want := map[string]string{
		model.BlogStepResearching: model.AgentNameResearcher,
		model.BlogStepOutlining:   model.AgentNameArticleWriter,
		model.BlogStepGenerating:  model.AgentNameArticleWriter,
		model.BlogStepReviewing:   model.AgentNameArticleWriter,
		model.BlogStepConverting:  model.AgentNameArticleWriter,
		model.BlogStepGenerated:   model.AgentNameArticleWriter,
	}
	for key, wantAgent := range want {
		if got[key] != wantAgent {
			t.Errorf("step %q attributed to %q, want %q", key, got[key], wantAgent)
		}
	}
}

// The research agent's own sub-phases stay attributed to the Research Agent,
// not to whoever commissioned the research.
func TestWrite_ResearchSubPhasesStayWithTheResearcher(t *testing.T) {
	r := newTestRunner(nil, writeScript("T", "# T\n\nBody [1].")...)

	var researchAgents []string
	if _, err := r.Generate(context.Background(), GenerateRequest{Briefs: briefsFor("t")},
		func(key, _, agent string) {
			if key == model.BlogStepResearching {
				researchAgents = append(researchAgents, agent)
			}
		}); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	if len(researchAgents) < 2 {
		t.Fatalf("expected the researcher's own phases to be reported, got %v", researchAgents)
	}
	for i, a := range researchAgents {
		if a != model.AgentNameResearcher {
			t.Errorf("research report %d attributed to %q, want %q", i, a, model.AgentNameResearcher)
		}
	}
}

func TestResolveCitations(t *testing.T) {
	brief := defaultBrief()

	tests := []struct {
		name     string
		markdown string
		wantRefs int
		wantHas  []string
		wantGone []string
	}{
		{
			name:     "renumbers in order of appearance",
			markdown: "Second source first [2]. Then the first [1].",
			wantRefs: 2,
			wantHas:  []string{"[1]", "[2]"},
		},
		{
			name:     "repeated source keeps one number",
			markdown: "Claim [1]. Another claim [1].",
			wantRefs: 1,
			wantHas:  []string{"[1]"},
		},
		{
			name:     "out of range citations are dropped",
			markdown: "Real [1]. Invented [7].",
			wantRefs: 1,
			wantGone: []string{"[7]"},
		},
		{
			name:     "markdown links are not citations",
			markdown: "See [1](https://example.com) for details.",
			wantRefs: 0,
			wantHas:  []string{"[1](https://example.com)"},
		},
		{
			name:     "no citations means no references",
			markdown: "An article that cites nothing at all.",
			wantRefs: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, refs := resolveCitations(tt.markdown, brief)

			if len(refs) != tt.wantRefs {
				t.Errorf("got %d references, want %d: %+v", len(refs), tt.wantRefs, refs)
			}
			for _, want := range tt.wantHas {
				if !strings.Contains(got, want) {
					t.Errorf("output %q is missing %q", got, want)
				}
			}
			for _, gone := range tt.wantGone {
				if strings.Contains(got, gone) {
					t.Errorf("output %q still contains %q", got, gone)
				}
			}
		})
	}
}

// Removing a citation must not leave "the API was released ." behind.
func TestResolveCitations_TidiesSpacingAfterRemoval(t *testing.T) {
	got, _ := resolveCitations("The API shipped [9]. Next sentence.", defaultBrief())

	if strings.Contains(got, " .") {
		t.Errorf("a space was left before the full stop: %q", got)
	}
	if !strings.Contains(got, "shipped. Next") {
		t.Errorf("spacing was not repaired: %q", got)
	}
}

func TestResolveCitations_NilBriefStripsMarkers(t *testing.T) {
	got, refs := resolveCitations("A claim [1] and another [2].", nil)

	if refs != nil {
		t.Errorf("got references from a nil brief: %+v", refs)
	}
	if strings.Contains(got, "[1]") || strings.Contains(got, "[2]") {
		t.Errorf("citation markers survived with no brief to resolve them: %q", got)
	}
}

func TestReferencesMarkdown(t *testing.T) {
	refs := []model.BlogReference{
		{URL: "https://a.com/1", Title: "First", Site: "a.com"},
		{URL: "https://b.com/2", Title: "", Site: "b.com"},
	}

	got := referencesMarkdown(refs)

	if !strings.Contains(got, "## References") {
		t.Error("no References heading")
	}
	if !strings.Contains(got, "1. [First](https://a.com/1)") {
		t.Errorf("first reference not rendered as a link: %q", got)
	}
	// A source with no title falls back to its URL rather than rendering an
	// empty link label.
	if !strings.Contains(got, "[https://b.com/2](https://b.com/2)") {
		t.Errorf("untitled reference did not fall back to its URL: %q", got)
	}
}

func TestReferencesMarkdown_EmptyIsEmpty(t *testing.T) {
	if got := referencesMarkdown(nil); got != "" {
		t.Errorf("got %q, want an empty string for no references", got)
	}
}

func TestCountCitations(t *testing.T) {
	tests := []struct {
		name     string
		markdown string
		want     int
	}{
		{"none", "no citations here", 0},
		{"two distinct", "a [1] b [2]", 2},
		{"repeats count separately", "a [1] b [1]", 2},
		{"markdown link is not a citation", "[1](https://x.com)", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := countCitations(tt.markdown); got != tt.want {
				t.Errorf("countCitations(%q) = %d, want %d", tt.markdown, got, tt.want)
			}
		})
	}
}
