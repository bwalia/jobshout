package marketingemail

import (
	"context"
	"strings"
	"testing"

	"github.com/jobshout/server/internal/model"
)

func goodDraft() *model.MarketingEmailDraft {
	return &model.MarketingEmailDraft{
		Subject:   "The Workstation AI shop is live",
		Preheader: "DGX Spark, Mac Studio M5 and Trade membership with an AI engineer on call.",
		Title:     "AI shop launch",
		SlugHint:  "ai-shop-live",
		Hero:      model.MarketingEmailHero{Headline: "The AI shop is live", Subline: "Hardware that is ready to work", ImagePrompt: "a clean desk with a compact workstation, soft light"},
		Sections: []model.MarketingEmailSection{
			{Type: "text", Heading: "What is new", BodyMarkdown: "We now sell **AI workstations** online."},
			{Type: "cta", Heading: "See the range", BodyMarkdown: "Browse the shop.", CTALabel: "Shop now", CTAURL: "https://www.workstation.co.uk/shop"},
		},
	}
}

func hasProblem(problems []string, sub string) bool {
	for _, p := range problems {
		if strings.Contains(p, sub) {
			return true
		}
	}
	return false
}

func TestValidate_Good(t *testing.T) {
	if p := Validate(goodDraft()); len(p) != 0 {
		t.Fatalf("problems = %v", p)
	}
}

func TestValidate_Rules(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(d *model.MarketingEmailDraft)
		want   string
	}{
		{"subject too long", func(d *model.MarketingEmailDraft) { d.Subject = strings.Repeat("a", 71) }, "subject is 71"},
		{"subject at limit ok", func(d *model.MarketingEmailDraft) { d.Subject = strings.Repeat("a", 70) }, ""},
		{"subject empty", func(d *model.MarketingEmailDraft) { d.Subject = " " }, "subject is empty"},
		{"preheader short", func(d *model.MarketingEmailDraft) { d.Preheader = "too short" }, "preheader is 9"},
		{"preheader long", func(d *model.MarketingEmailDraft) { d.Preheader = strings.Repeat("p", 111) }, "preheader is 111"},
		{"preheader bounds ok", func(d *model.MarketingEmailDraft) { d.Preheader = strings.Repeat("p", 40) }, ""},
		{"one section", func(d *model.MarketingEmailDraft) { d.Sections = d.Sections[:1] }, "1 sections"},
		{"seven sections", func(d *model.MarketingEmailDraft) {
			for len(d.Sections) < 7 {
				d.Sections = append(d.Sections, d.Sections[0])
			}
		}, "7 sections"},
		{"http cta url", func(d *model.MarketingEmailDraft) { d.Sections[1].CTAURL = "http://example.com/x" }, "https"},
		{"javascript cta url", func(d *model.MarketingEmailDraft) { d.Sections[1].CTAURL = "javascript:alert(1)" }, "https"},
		{"cta without url", func(d *model.MarketingEmailDraft) { d.Sections[1].CTAURL = "" }, "needs a cta_url"},
		{"cta without label", func(d *model.MarketingEmailDraft) { d.Sections[1].CTALabel = "" }, "needs a cta_label"},
		{"http markdown link", func(d *model.MarketingEmailDraft) { d.Sections[0].BodyMarkdown = "see [this](http://x.example/a)" }, "every URL must be https"},
		{"merge tag", func(d *model.MarketingEmailDraft) { d.Sections[0].BodyMarkdown = "Hi {{first_name}}" }, "merge tag"},
		{"no hero headline", func(d *model.MarketingEmailDraft) { d.Hero.Headline = "" }, "hero.headline"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := goodDraft()
			tt.mutate(d)
			p := Validate(d)
			if tt.want == "" {
				if len(p) != 0 {
					t.Fatalf("problems = %v, want none", p)
				}
				return
			}
			if !hasProblem(p, tt.want) {
				t.Fatalf("problems = %v, want one containing %q", p, tt.want)
			}
		})
	}
}

// fakeCompleter replies from a queue and records prompts.
type fakeCompleter struct {
	replies []string
	prompts []string
}

func (f *fakeCompleter) Complete(_ context.Context, _, prompt string, _ int, _ bool) (string, error) {
	f.prompts = append(f.prompts, prompt)
	if len(f.replies) == 0 {
		return "", context.DeadlineExceeded
	}
	r := f.replies[0]
	f.replies = f.replies[1:]
	return r, nil
}

const goodJSON = `{
 "subject": "The Workstation AI shop is live",
 "preheader": "DGX Spark, Mac Studio M5 and Trade membership with an AI engineer on call.",
 "title": "AI shop launch", "slug_hint": "ai-shop-live",
 "hero": {"headline": "The AI shop is live", "subline": "Ready to work", "image_prompt": "a desk"},
 "sections": [
  {"type": "text", "heading": "What is new", "body_markdown": "We sell **AI workstations**."},
  {"type": "cta", "heading": "See the range", "body_markdown": "Browse.", "cta_label": "Shop now", "cta_url": "https://www.workstation.co.uk/shop"}
 ]}`

func TestDraft_RetriesOnceWithProblems(t *testing.T) {
	bad := strings.Replace(goodJSON, "The Workstation AI shop is live", strings.Repeat("x", 90), 1)
	f := &fakeCompleter{replies: []string{bad, goodJSON}}
	w := &Writer{LLM: f}
	d, err := w.Draft(context.Background(), model.MarketingEmailBrief{Brief: "AI shop launch announcement"})
	if err != nil {
		t.Fatal(err)
	}
	if d.Subject != "The Workstation AI shop is live" {
		t.Errorf("subject = %q", d.Subject)
	}
	if len(f.prompts) != 2 {
		t.Fatalf("prompts = %d, want 2", len(f.prompts))
	}
	if !strings.Contains(f.prompts[1], "subject is 90 characters") {
		t.Errorf("retry prompt does not name the problem:\n%s", f.prompts[1])
	}
}

func TestDraft_FailsAfterOneRetry(t *testing.T) {
	bad := strings.Replace(goodJSON, "https://www.workstation.co.uk/shop", "http://insecure.example", 1)
	f := &fakeCompleter{replies: []string{bad, bad, goodJSON}}
	_, err := (&Writer{LLM: f}).Draft(context.Background(), model.MarketingEmailBrief{Brief: "x"})
	var ve *ValidationError
	if err == nil || !errorsAs(err, &ve) {
		t.Fatalf("err = %v, want ValidationError", err)
	}
	if len(f.prompts) != 2 {
		t.Errorf("prompts = %d, want exactly one retry", len(f.prompts))
	}
}

func TestDraft_TolerantDecode(t *testing.T) {
	// Odd shapes: number subject-ish fields, list heading, items as objects or a string.
	odd := `{
 "subject": "The Workstation AI shop is live",
 "preheader": ["DGX Spark, Mac Studio M5 and Trade membership", "with an AI engineer on call."],
 "hero": {"headline": {"text": "The AI shop is live"}, "subline": 7, "image_prompt": null},
 "sections": [
  {"type": "Feature", "heading": "Points", "items": [{"title": "Fast"}, "Quiet", 3]},
  {"type": "banner", "body_markdown": "- one\n- two"},
  {"type": "cta", "heading": "Go", "cta_label": "Shop", "cta_url": "https://www.workstation.co.uk/shop"}
 ]}`
	f := &fakeCompleter{replies: []string{odd}}
	d, err := (&Writer{LLM: f}).Draft(context.Background(), model.MarketingEmailBrief{Brief: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if d.Sections[0].Type != "feature" || len(d.Sections[0].Items) != 3 {
		t.Errorf("section 0 = %+v", d.Sections[0])
	}
	if d.Sections[1].Type != "text" {
		t.Errorf("unknown type should become text, got %q", d.Sections[1].Type)
	}
	if d.Hero.Headline != "The AI shop is live" || d.Hero.Subline != "7" {
		t.Errorf("hero = %+v", d.Hero)
	}
}

func TestDraft_NoLLM(t *testing.T) {
	if _, err := (&Writer{}).Draft(context.Background(), model.MarketingEmailBrief{}); err == nil {
		t.Fatal("want an error with no LLM")
	}
}
