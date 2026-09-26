package mail

import (
	"context"
	"encoding/json"
	"fmt"
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
	d := NewDrafter(fake, "", nil)
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
	d := NewDrafter(fake, "", nil)
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

func TestBuildDraftPromptIncludesKnowledgeNotes(t *testing.T) {
	p := BuildDraftPrompt(fixtureSupportQuestion(), ClassifyResult{
		Intent: "question", SuggestedAction: "reply", Reason: "asks for a price",
	}, nil, DraftOptions{
		KnowledgeNotes: "Mac Studio M5 Max: $2,499\nRefunds within 30 days.",
	})
	for _, want := range []string{
		"Operator-provided knowledge (use only these facts):",
		"Mac Studio M5 Max: $2,499",
		"Refunds within 30 days.",
		"ONLY if it appears in the provided knowledge",
		"Never fill the gap from memory",
		"never tell the sender to visit a website",
		"Never claim this reply has been sent",
	} {
		if !strings.Contains(p, want) {
			t.Errorf("prompt missing %q", want)
		}
	}
}

func TestBuildDraftPromptCombinesNotesAndPinnedFindings(t *testing.T) {
	p := BuildDraftPrompt(fixtureSupportQuestion(), ClassifyResult{Intent: "question"},
		&research.Brief{Summary: "The pinned page lists the M5 Ultra at $5,499."},
		DraftOptions{
			KnowledgeNotes:  "M5 Max: $2,499",
			PinnedKnowledge: true,
		})
	for _, want := range []string{
		"Operator-provided knowledge (use only these facts):",
		"M5 Max: $2,499",
		"Research findings (use only these facts):",
		"$5,499",
		"ONLY if it appears in the provided knowledge or the research findings",
	} {
		if !strings.Contains(p, want) {
			t.Errorf("prompt missing %q", want)
		}
	}
}

func TestDraftQuotesPricesFromKnowledgeNotes(t *testing.T) {
	fake := &scriptedLLM{replies: []string{
		draftJSON("The M5 Max starts at $2,499 and the M5 Ultra at $5,499."),
	}}
	d := NewDrafter(fake, "", nil)
	got, err := d.Draft(context.Background(), fixtureSupportQuestion(),
		ClassifyResult{Intent: "question"}, nil, DraftOptions{
			KnowledgeNotes: "M5 Max: $2,499. M5 Ultra: $5,499.",
		})
	if err != nil {
		t.Fatal(err)
	}
	if fake.calls != 1 {
		t.Errorf("calls = %d, amounts from the notes should not trigger a redraft", fake.calls)
	}
	if !strings.Contains(got.Body, "$2,499") || !strings.Contains(got.Body, "$5,499") {
		t.Errorf("prices from notes lost: %s", got.Body)
	}
}

func TestDraftRedraftsWhenAmountNotInKnowledgeNotes(t *testing.T) {
	fake := &scriptedLLM{replies: []string{
		draftJSON("The Mac Studio is $1,999."),
	}}
	d := NewDrafter(fake, "", nil)
	got, err := d.Draft(context.Background(), fixtureSupportQuestion(),
		ClassifyResult{Intent: "question"}, nil, DraftOptions{
			KnowledgeNotes: "We sell the Mac Studio. Ask sales for a quote.",
		})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got.Body, "$") {
		t.Errorf("invented price survived the notes guard: %s", got.Body)
	}
	if !strings.Contains(got.Body, "follow up") {
		t.Errorf("fallback should promise a follow-up: %s", got.Body)
	}
}

// capturingLLM records each GenerateRequest and replies with a fixed draft.
type capturingLLM struct {
	reqs []llm.GenerateRequest
}

func (c *capturingLLM) Generate(ctx context.Context, req llm.GenerateRequest) (*llm.GenerateResponse, error) {
	c.reqs = append(c.reqs, req)
	return &llm.GenerateResponse{Content: draftJSON("Thanks, we will reply shortly.")}, nil
}

func (c *capturingLLM) ProviderName() string { return "capturing" }

func TestDraftRequestsConfiguredModelAndThinking(t *testing.T) {
	fake := &capturingLLM{}
	d := NewDrafter(fake, "muse-glimmer:latest", nil)
	if _, err := d.Draft(context.Background(), fixtureSupportQuestion(),
		ClassifyResult{Intent: "question"}, nil, DraftOptions{}); err != nil {
		t.Fatal(err)
	}
	if len(fake.reqs) != 1 {
		t.Fatalf("calls = %d", len(fake.reqs))
	}
	req := fake.reqs[0]
	if req.Model != "muse-glimmer:latest" {
		t.Errorf("model = %q", req.Model)
	}
	if !req.Think {
		t.Error("draft request should ask for thinking")
	}
	if req.MaxTokens < 2000 {
		t.Errorf("MaxTokens = %d — thinking counts against the budget, so it must be generous", req.MaxTokens)
	}
}

// onlyThinkingLLM fails thinking requests the way Ollama does when the model
// spends the whole budget reasoning, and answers once thinking is off.
type onlyThinkingLLM struct {
	calls int
}

func (o *onlyThinkingLLM) Generate(ctx context.Context, req llm.GenerateRequest) (*llm.GenerateResponse, error) {
	o.calls++
	if req.Think {
		return nil, fmt.Errorf("ollama: model %q %w", req.Model, llm.ErrOnlyThinking)
	}
	return &llm.GenerateResponse{Content: draftJSON("Thanks, happy to help.")}, nil
}

func (o *onlyThinkingLLM) ProviderName() string { return "only-thinking" }

func TestDraftRetriesWithoutThinkingWhenBudgetExhausted(t *testing.T) {
	fake := &onlyThinkingLLM{}
	d := NewDrafter(fake, "muse-glimmer:latest", nil)
	got, err := d.Draft(context.Background(), fixtureSupportQuestion(),
		ClassifyResult{Intent: "question"}, nil, DraftOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if fake.calls != 2 {
		t.Errorf("calls = %d, want thinking attempt then plain retry", fake.calls)
	}
	if !strings.Contains(got.Body, "happy to help") {
		t.Errorf("retry draft lost: %s", got.Body)
	}
}

func TestDraftKeepsSourcedAmountsWithoutRedraft(t *testing.T) {
	fake := &scriptedLLM{replies: []string{
		draftJSON("The M5 Max starts at $2,499 and the M5 Ultra at $5,499."),
	}}
	brief := &research.Brief{Summary: "M5 Max starts at $2,499; M5 Ultra starts at $5,499."}
	d := NewDrafter(fake, "", nil)
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
