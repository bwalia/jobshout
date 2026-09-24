package blog

import (
	"context"
	"strings"
	"testing"

	"github.com/jobshout/server/internal/model"
)

// promptFor returns the prompt the stub was given for one phase, so a test can
// assert what the writer was actually told rather than what we meant to tell it.
func promptFor(t *testing.T, stub *stubLLM, phase string) string {
	t.Helper()
	for _, c := range stub.calls {
		p := c.Messages[len(c.Messages)-1].Content
		if strings.Contains(p, phase) {
			return p
		}
	}
	t.Fatalf("no call matched %q; made %d call(s)", phase, len(stub.calls))
	return ""
}

// businessScript matches the business profile's own phase wording, which is
// deliberately different from the developer profile's.
func businessScript(body string) []scriptedResponse {
	return []scriptedResponse{
		{trigger: "planning a briefing", content: `{"title":"T","angle":"a","sections":["One"]}`},
		{trigger: "writing a briefing", content: body},
		{trigger: "reviewing a draft briefing", content: `{"issues":[]}`},
	}
}

// The reader has to reach the prompts. This is the assertion the whole feature
// rests on: if the audience is stored and displayed but never spliced in, every
// article comes out as a developer deep dive and nothing says so.
func TestAudienceReachesTheWritingPrompts(t *testing.T) {
	stub := &stubLLM{responses: businessScript("# T\n\nA plain-English body [1].")}
	r := newRunnerWithStub(stub)

	_, err := r.Generate(context.Background(), GenerateRequest{
		Briefs: []model.BlogBrief{{Topic: "Gateway API", Audience: "business"}},
	}, nil)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	draft := promptFor(t, stub, "writing a briefing")
	if !strings.Contains(draft, "a business manager who is accountable") {
		t.Errorf("the draft prompt does not name the reader:\n%s", draft)
	}
	if !strings.Contains(draft, "Do not include code") {
		t.Errorf("the draft prompt does not carry the business code ban:\n%s", draft)
	}
	if strings.Contains(draft, "900-1400 words") {
		t.Errorf("the draft prompt kept the developer word range:\n%s", draft)
	}

	review := promptFor(t, stub, "reviewing a draft briefing")
	if !strings.Contains(review, "UNEXPLAINED JARGON") {
		t.Errorf("the review prompt does not carry the business checks:\n%s", review)
	}
}

// Research is where the difference starts, so the reader has to be in the
// brief the Research Agent is handed, not only in the writing prompts.
func TestAudienceReachesTheResearchAgent(t *testing.T) {
	researcher := &fakeResearcher{}
	r := newRunnerWithResearcher(researcher, businessScript("# T\n\nBody [1]."))

	_, err := r.Generate(context.Background(), GenerateRequest{
		Briefs: []model.BlogBrief{{
			Topic:    "Gateway API",
			Context:  "keep it under ten minutes to read",
			Audience: "business",
			Industry: "NHS trusts",
		}},
	}, nil)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	if len(researcher.requests) != 1 {
		t.Fatalf("research called %d time(s); want 1", len(researcher.requests))
	}
	got := researcher.requests[0].Context
	for _, want := range []string{
		"keep it under ten minutes to read", // the requester's own guidance survives
		"a business manager who is accountable",
		"NHS trusts",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("research context is missing %q:\n%s", want, got)
		}
	}
}

// An industry frames the piece. Nobody naming one should have to read the
// article to find out whether it was applied.
func TestIndustryFramesTheDraft(t *testing.T) {
	stub := &stubLLM{responses: writeScript("T", "# T\n\nBody [1].")}
	r := newRunnerWithStub(stub)

	_, err := r.Generate(context.Background(), GenerateRequest{
		Briefs: []model.BlogBrief{{Topic: "Gateway API", Industry: "3PL logistics"}},
	}, nil)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	draft := promptFor(t, stub, promptDraft)
	if !strings.Contains(draft, "3PL logistics") {
		t.Errorf("the draft prompt was not framed for the sector:\n%s", draft)
	}
}

// No industry means no heading, not an empty one. A prompt carrying
// "INDUSTRY —" with nothing under it invites the model to invent a sector.
func TestNoIndustryLeavesNoHeading(t *testing.T) {
	stub := &stubLLM{responses: writeScript("T", "# T\n\nBody [1].")}
	r := newRunnerWithStub(stub)

	if _, err := r.Generate(context.Background(), GenerateRequest{
		Briefs: briefsFor("Gateway API"),
	}, nil); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if draft := promptFor(t, stub, promptDraft); strings.Contains(draft, "INDUSTRY") {
		t.Errorf("an empty industry heading reached the prompt:\n%s", draft)
	}
}

// The length floor is the reader's. A technote finished at 500 words is
// finished, and expanding it to 900 would be the pipeline undoing the choice.
func TestTechnoteIsNotExpandedToDeepDiveLength(t *testing.T) {
	body := "# T\n\n" + strings.Repeat("word ", 600) + "[1]."
	stub := &stubLLM{responses: []scriptedResponse{
		{trigger: "planning a technote", content: `{"title":"T","angle":"a","sections":["One"]}`},
		{trigger: "writing a technote", content: body},
		{trigger: "reviewing a draft technote", content: `{"issues":[]}`},
	}}
	r := newRunnerWithStub(stub)

	var steps []string
	_, err := r.Generate(context.Background(), GenerateRequest{
		Briefs: []model.BlogBrief{{Topic: "Rotating a Postgres primary key", Audience: "technote"}},
	}, func(key, _, _ string) { steps = append(steps, key) })
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	for _, s := range steps {
		if s == model.BlogStepExpanding {
			t.Errorf("a 600-word technote was expanded toward the deep-dive floor: %v", steps)
		}
	}
}

// The reader travels out of the pipeline on the article, so the caller can
// store it and the CMS draft can be tagged without re-deriving it.
func TestArticleCarriesItsReaderAndTags(t *testing.T) {
	script := append(businessScript("# T\n\nBody [1]."),
		scriptedResponse{trigger: promptExpand, content: "# T\n\n" + strings.Repeat("word ", 800) + "[1]."})
	r := newRunnerWithStub(&stubLLM{responses: script})

	arts, err := r.Generate(context.Background(), GenerateRequest{
		Briefs: []model.BlogBrief{{Topic: "t", Audience: "business", Industry: "NHS Trusts"}},
	}, nil)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if arts[0].Audience != "business" || arts[0].Industry != "NHS Trusts" {
		t.Errorf("article carries audience %q industry %q", arts[0].Audience, arts[0].Industry)
	}

	tags := articleTags(arts[0])
	if !containsString(tags, "business") || !containsString(tags, "nhs trusts") {
		t.Errorf("CMS tags = %v; want the reader and the sector", tags)
	}
	if containsString(articleTags(GeneratedArticle{}), "business") {
		t.Error("a default article was tagged as a business piece")
	}
}

func containsString(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}
