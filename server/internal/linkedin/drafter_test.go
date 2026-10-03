package linkedin

import (
	"context"
	"strings"
	"testing"

	"github.com/jobshout/server/internal/llm"
	"github.com/jobshout/server/internal/model"
)

type fakeLLM struct {
	reply string
	reqs  []llm.GenerateRequest
	stage []string
}

func (f *fakeLLM) Generate(ctx context.Context, req llm.GenerateRequest) (*llm.GenerateResponse, error) {
	f.reqs = append(f.reqs, req)
	f.stage = append(f.stage, llm.StageFrom(ctx))
	return &llm.GenerateResponse{Content: f.reply}, nil
}

func (f *fakeLLM) ProviderName() string { return "fake" }

func TestCleanPost(t *testing.T) {
	in := "<think>plan the post</think>\n```text\n**Your deploys fail at 2am.**\n\n\n\n## Why\nBecause.\n```"
	want := "Your deploys fail at 2am.\n\nWhy\nBecause."
	if got := CleanPost(in); got != want {
		t.Errorf("CleanPost = %q, want %q", got, want)
	}
	if got := CleanPost(`"Quoted post."`); got != "Quoted post." {
		t.Errorf("wrapping quotes kept: %q", got)
	}
	if got := CleanPost(`He said "hi" and "bye"`); got != `He said "hi" and "bye"` {
		t.Errorf("inner quotes damaged: %q", got)
	}
}

func TestFitCommentaryCutsAtParagraph(t *testing.T) {
	para := strings.Repeat("word ", 150) // 750 chars
	s := strings.TrimSpace(strings.Repeat(para+"\n\n", 6))
	got := FitCommentary(s)
	if n := len([]rune(EscapeCommentary(got))); n > model.LinkedInCommentaryMax {
		t.Fatalf("still %d chars", n)
	}
	if strings.HasSuffix(got, "\n") || !strings.HasSuffix(got, "word") {
		t.Errorf("not cut at a paragraph: ...%q", got[len(got)-20:])
	}
	if short := "short post"; FitCommentary(short) != short {
		t.Error("short post changed")
	}
}

func TestDraftUsesVariantBriefStageAndModel(t *testing.T) {
	f := &fakeLLM{reply: "Post body.\n\n#Go"}
	d := &Drafter{LLM: f, Cfg: Config{Model: "qwen3:8b"}}
	got, err := d.Draft(context.Background(), DraftRequest{
		Article: model.LinkedInArticle{Title: "Fixing flaky deploys", Markdown: "# Fixing\nBody"},
		Variant: model.LinkedInVariantBusiness,
		Notes:   "lead with on-call cost",
		HasLink: true,
	})
	if err != nil || got != "Post body.\n\n#Go" {
		t.Fatalf("Draft = %q, %v", got, err)
	}
	if f.stage[0] != "linkedin_business" || f.reqs[0].Model != "qwen3:8b" {
		t.Errorf("stage %q model %q", f.stage[0], f.reqs[0].Model)
	}
	prompt := f.reqs[0].Messages[1].Content
	for _, want := range []string{"founders, managers", "lead with on-call cost", "Fixing flaky deploys", "link card"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt is missing %q", want)
		}
	}
	if _, err := d.Draft(context.Background(), DraftRequest{Variant: "poem"}); err == nil {
		t.Error("unknown variant accepted")
	}
}

func TestDraftWithoutLinkForbidsURL(t *testing.T) {
	f := &fakeLLM{reply: "x"}
	d := &Drafter{LLM: f}
	if _, err := d.Draft(context.Background(), DraftRequest{Variant: model.LinkedInVariantTechnical}); err != nil {
		t.Fatal(err)
	}
	p := f.reqs[0].Messages[1].Content
	if !strings.Contains(p, "software engineers") || !strings.Contains(p, "Do not include a URL") {
		t.Errorf("prompt: %s", p)
	}
}

func TestDraftRejectsEmptyReply(t *testing.T) {
	d := &Drafter{LLM: &fakeLLM{reply: "<think>only thinking</think>"}}
	if _, err := d.Draft(context.Background(), DraftRequest{Variant: model.LinkedInVariantTechnical}); err == nil {
		t.Error("empty post accepted")
	}
}
