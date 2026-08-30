package mail

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/jobshout/server/internal/llm"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/research"
)

func TestBuildDraftPromptIncludesResearchAndForbidsSent(t *testing.T) {
	brief := &research.Brief{
		Summary: "Kubernetes 1.31 is the current release.",
		Findings: []research.Finding{{
			Claim:     "1.31 is current",
			SourceURL: "https://kubernetes.io/blog/",
			Quote:     "released 1.31",
		}},
	}
	p := BuildDraftPrompt(fixtureSupportQuestion(), ClassifyResult{
		Intent: "question", SuggestedAction: "reply", Reason: "needs facts",
	}, brief, DraftOptions{})
	for _, want := range []string{
		"Kubernetes 1.31",
		"https://kubernetes.io/blog/",
		"Do not claim the reply has been sent",
		"alex@customer.com",
	} {
		if !strings.Contains(p, want) {
			t.Errorf("prompt missing %q", want)
		}
	}
}

func TestHeuristicDraftNeverClaimsSent(t *testing.T) {
	d := HeuristicDraft(fixtureSupportQuestion(), nil)
	if d.Status != model.MailDraftDraft {
		t.Errorf("status %q", d.Status)
	}
	lower := strings.ToLower(d.Body)
	if strings.Contains(lower, "i have sent") || strings.Contains(lower, "has been sent") {
		t.Errorf("heuristic draft claimed send: %s", d.Body)
	}
	if !strings.HasPrefix(strings.ToLower(d.Subject), "re:") {
		t.Errorf("subject %q", d.Subject)
	}
}

func TestBuildDraftPromptIncludesReplyInstructionsAndPinnedGuidance(t *testing.T) {
	p := BuildDraftPrompt(fixtureSupportQuestion(), ClassifyResult{
		Intent: "question", SuggestedAction: "reply", Reason: "needs facts",
	}, &research.Brief{Summary: "Price is £40/user from the pricing page."}, DraftOptions{
		ReplyInstructions: "Be warm, under 80 words, never mention competitors.",
		PinnedKnowledge:   true,
	})
	for _, want := range []string{
		"REPLY INSTRUCTIONS FROM THE OPERATOR",
		"Be warm, under 80 words, never mention competitors.",
		"pinned knowledge pages",
		"ONLY if it appears in the findings",
		"Never fill the gap from memory",
		"never tell the sender to visit a website",
		"Never claim this reply has been sent",
	} {
		if !strings.Contains(p, want) {
			t.Errorf("prompt missing %q", want)
		}
	}
}

func TestHeuristicDraftFoldsResearchSummary(t *testing.T) {
	d := HeuristicDraft(fixtureSupportQuestion(), &research.Brief{Summary: "Fact from research."})
	if !strings.Contains(d.Body, "Fact from research.") {
		t.Errorf("research summary missing: %s", d.Body)
	}
}

func TestUnsupportedAmounts(t *testing.T) {
	findings := "The M5 Max starts at $2,499. The M5 Ultra starts at $5499."
	sender := "Can you do it for £500?"
	cases := []struct {
		name string
		body string
		want []string
	}{
		{"all sourced", "It starts at $2,499; the Ultra is $5,499.", nil},
		{"sender's own figure echoed", "Regarding your £500 offer…", nil},
		{"invented price", "Prices are $1,999 (base) and $2,499.", []string{"$1,999"}},
		{"no amounts at all", "The price is not listed on that page.", nil},
	}
	for _, tc := range cases {
		got := unsupportedAmounts(tc.body, findings, sender)
		if len(got) != len(tc.want) {
			t.Errorf("%s: got %v want %v", tc.name, got, tc.want)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("%s: got %v want %v", tc.name, got, tc.want)
			}
		}
	}
}

// scriptedLLM returns each reply in order, repeating the last one.
type scriptedLLM struct {
	replies []string
	calls   int
}

func (s *scriptedLLM) Generate(ctx context.Context, req llm.GenerateRequest) (*llm.GenerateResponse, error) {
	i := s.calls
	if i >= len(s.replies) {
		i = len(s.replies) - 1
	}
	s.calls++
	return &llm.GenerateResponse{Content: s.replies[i]}, nil
}

func (s *scriptedLLM) ProviderName() string { return "scripted" }

func draftJSON(body string) string {
	raw, _ := json.Marshal(map[string]string{"subject": "Re: pricing", "body": body})
	return string(raw)
}

func TestDraftRedraftsWhenAmountNotInFindings(t *testing.T) {
	fake := &scriptedLLM{replies: []string{
		draftJSON("The Mac Studio is $1,999 (base) and $2,499 (fully configured)."),
		draftJSON("The M5 Max model starts at $2,499."),
	}}
	brief := &research.Brief{Findings: []research.Finding{{
		Claim: "M5 Max starts at $2,499", SourceURL: "https://www.apple.com/mac-studio/",
	}}}
	d := NewDrafter(fake, nil)
	got, err := d.Draft(context.Background(), fixtureSupportQuestion(),
		ClassifyResult{Intent: "question"}, brief, DraftOptions{PinnedKnowledge: true})
	if err != nil {
		t.Fatal(err)
	}
	if fake.calls != 2 {
		t.Errorf("calls = %d, want a redraft", fake.calls)
	}
	if strings.Contains(got.Body, "$1,999") {
		t.Errorf("invented price survived: %s", got.Body)
	}
	if !strings.Contains(got.Body, "$2,499") {
		t.Errorf("sourced price lost: %s", got.Body)
	}
}

func TestDraftFallsBackWhenModelKeepsInventingAmounts(t *testing.T) {
	fake := &scriptedLLM{replies: []string{
		draftJSON("The Mac Studio is $1,999."),
	}}
	brief := &research.Brief{Summary: "The pinned page lists no prices."}
	d := NewDrafter(fake, nil)
	got, err := d.Draft(context.Background(), fixtureSupportQuestion(),
		ClassifyResult{Intent: "question"}, brief, DraftOptions{PinnedKnowledge: true})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got.Body, "$") {
		t.Errorf("fallback still quotes a figure: %s", got.Body)
	}
	if !strings.Contains(got.Body, "follow up") {
		t.Errorf("fallback should promise a follow-up: %s", got.Body)
	}
	if got.To != "alex@customer.com" {
		t.Errorf("to = %q", got.To)
	}
}

func TestDraftKeepsSourcedAmountsWithoutRedraft(t *testing.T) {
	fake := &scriptedLLM{replies: []string{
		draftJSON("The M5 Max starts at $2,499 and the M5 Ultra at $5,499."),
	}}
	brief := &research.Brief{Summary: "M5 Max starts at $2,499; M5 Ultra starts at $5,499."}
	d := NewDrafter(fake, nil)
	got, err := d.Draft(context.Background(), fixtureSupportQuestion(),
		ClassifyResult{Intent: "question"}, brief, DraftOptions{PinnedKnowledge: true})
	if err != nil {
		t.Fatal(err)
	}
	if fake.calls != 1 {
		t.Errorf("calls = %d, sourced amounts should not trigger a redraft", fake.calls)
	}
	if !strings.Contains(got.Body, "$2,499") || !strings.Contains(got.Body, "$5,499") {
		t.Errorf("sourced prices lost: %s", got.Body)
	}
}
