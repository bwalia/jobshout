package chatagent

import (
	"context"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/jobshout/server/internal/llm"
	"github.com/jobshout/server/internal/model"
)

// summaryLLM returns a canned compression and records what it was asked to
// compress, so a test can assert the early facts reached the compressor.
type summaryLLM struct {
	reply  string
	prompt string
	err    error
	calls  int
}

func (s *summaryLLM) ProviderName() string { return "summary-fake" }

func (s *summaryLLM) Generate(_ context.Context, req llm.GenerateRequest) (*llm.GenerateResponse, error) {
	s.calls++
	s.prompt = req.Messages[len(req.Messages)-1].Content
	if s.err != nil {
		return nil, s.err
	}
	return &llm.GenerateResponse{Content: s.reply}, nil
}

func turns(n int, prefix string) []model.ChatMessage {
	out := make([]model.ChatMessage, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, model.ChatMessage{Role: model.ChatRoleUser, Content: prefix})
	}
	return out
}

// The defect this fixes: a fact stated early survived only until the summary
// hit its cap, at which point truncation kept the newest content and discarded
// the oldest — which is exactly backwards, because recent turns are still in
// the live window.
func TestRollSummary_KeepsEarlyFactsWhenTheBudgetIsExceeded(t *testing.T) {
	fake := &summaryLLM{reply: "- user is working on repo bwalia/jobshout\n- reviewing PR 42"}
	a := New(fake, nil, nil, nil, nil)

	early := []model.ChatMessage{{
		Role:    model.ChatRoleUser,
		Content: "We are working on repo bwalia/jobshout, and I only care about PR 42.",
	}}
	summary := a.rollSummary(context.Background(), "", early)

	// Now flood it well past the compression threshold.
	noise := turns(80, strings.Repeat("some incidental chatter about nothing in particular ", 3))
	summary = a.rollSummary(context.Background(), summary, noise)

	if fake.calls == 0 {
		t.Fatal("expected the model to be asked to compress an oversized summary")
	}
	if !strings.Contains(fake.prompt, "bwalia/jobshout") {
		t.Error("the early fact never reached the compressor, so it could not survive")
	}
	if !strings.Contains(summary, "bwalia/jobshout") {
		t.Errorf("early repo fact was lost: %q", summary)
	}
	if len(summary) > summaryBudget {
		t.Errorf("summary is %d bytes, over the %d budget", len(summary), summaryBudget)
	}
}

func TestRollSummary_AppendsWithoutCallingTheModelWhenSmall(t *testing.T) {
	fake := &summaryLLM{reply: "should not be used"}
	a := New(fake, nil, nil, nil, nil)

	got := a.rollSummary(context.Background(), "", []model.ChatMessage{
		{Role: model.ChatRoleUser, Content: "review PR 42 on bwalia/jobshout"},
	})
	if fake.calls != 0 {
		t.Error("a small summary should be appended, not compressed — that is a wasted model call")
	}
	if !strings.Contains(got, "PR 42") {
		t.Errorf("summary lost the turn: %q", got)
	}
}

// A failed compression must degrade, not fail the turn.
func TestRollSummary_FallsBackWhenCompressionFails(t *testing.T) {
	fake := &summaryLLM{err: context.DeadlineExceeded}
	a := New(fake, nil, nil, nil, nil)

	summary := a.rollSummary(context.Background(), "",
		turns(60, strings.Repeat("padding that pushes this over the compression threshold ", 3)))

	if summary == "" {
		t.Fatal("a failed compression should still leave a usable summary")
	}
	if len(summary) > summaryBudget {
		t.Errorf("fallback ignored the budget: %d bytes", len(summary))
	}
}

// The old implementation sliced bytes, which split multi-byte characters and
// put a replacement character into the system prompt.
func TestTrimSummary_NeverSplitsARune(t *testing.T) {
	long := strings.Repeat("é日本語 ", 4000)
	got := trimSummary(long)

	if len(got) > summaryBudget {
		t.Errorf("trim ignored the budget: %d bytes", len(got))
	}
	if !utf8.ValidString(got) {
		t.Error("trim produced invalid UTF-8")
	}
	if strings.ContainsRune(got, '�') {
		t.Error("trim produced a replacement character")
	}
}

// Trimming keeps the beginning, because that is where standing facts are.
func TestTrimSummary_KeepsTheOldestContent(t *testing.T) {
	head := "user: the repo is bwalia/jobshout\n"
	got := trimSummary(head + strings.Repeat("user: chatter\n", 1000))

	if !strings.HasPrefix(got, head) {
		t.Errorf("trim discarded the oldest content, which is the fact worth keeping: %q",
			got[:min(120, len(got))])
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
