package research

import (
	"context"
	"fmt"
	"strings"

	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/jobshout/server/internal/llm"
)

// slowExtractLLM answers extraction prompts after a delay that is longest for
// the first document, so completion order is the reverse of document order.
// It records the peak number of calls in flight and whether each call asked
// for JSON mode.
type slowExtractLLM struct {
	inFlight atomic.Int32
	peak     atomic.Int32
	calls    atomic.Int32
	notJSON  atomic.Int32
	docs     int
}

func (s *slowExtractLLM) ProviderName() string { return "slow" }

func (s *slowExtractLLM) Generate(ctx context.Context, req llm.GenerateRequest) (*llm.GenerateResponse, error) {
	s.calls.Add(1)
	if !req.JSON {
		s.notJSON.Add(1)
	}
	n := s.inFlight.Add(1)
	defer s.inFlight.Add(-1)
	for {
		p := s.peak.Load()
		if n <= p || s.peak.CompareAndSwap(p, n) {
			break
		}
	}

	prompt := req.Messages[len(req.Messages)-1].Content
	var idx int
	if _, err := fmt.Sscanf(prompt[strings.Index(prompt, "SOURCE URL: ")+len("SOURCE URL: "):], "https://doc.example/%d", &idx); err != nil {
		return nil, err
	}
	select {
	case <-time.After(time.Duration(s.docs-idx) * 15 * time.Millisecond):
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return &llm.GenerateResponse{Content: fmt.Sprintf(`{"findings":[{"claim":"claim %d","quote":"quote %d"}]}`, idx, idx)}, nil
}

func extractDocs(n int) []Document {
	docs := make([]Document, n)
	for i := range docs {
		docs[i] = Document{Source: Source{URL: fmt.Sprintf("https://doc.example/%d", i), Title: "t"}, Text: "text"}
	}
	return docs
}

func TestExtractAll_BoundedConcurrencyAndStableOrder(t *testing.T) {
	const n = 7
	model := &slowExtractLLM{docs: n}
	agent := NewAgent(nil, model, DefaultAgentConfig(), zap.NewNop())

	findings := agent.extractAll(context.Background(), Request{Topic: "x"}, extractDocs(n), &Brief{})

	if got := model.peak.Load(); got != 3 {
		t.Errorf("peak concurrent extractions = %d, want 3", got)
	}
	if got := model.calls.Load(); got != n {
		t.Errorf("extraction calls = %d, want %d (one per document, no retries)", got, n)
	}
	if got := model.notJSON.Load(); got != 0 {
		t.Errorf("%d extraction calls did not request JSON mode", got)
	}
	if len(findings) != n {
		t.Fatalf("findings = %d, want %d", len(findings), n)
	}
	for i, f := range findings {
		if want := fmt.Sprintf("https://doc.example/%d", i); f.SourceURL != want {
			t.Errorf("findings[%d] from %s, want %s — document order not preserved", i, f.SourceURL, want)
		}
	}
}

func TestExtractAll_StopsStartingWorkWhenCancelled(t *testing.T) {
	const n = 7
	model := &slowExtractLLM{docs: n}
	agent := NewAgent(nil, model, DefaultAgentConfig(), zap.NewNop())

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	brief := &Brief{}
	findings := agent.extractAll(ctx, Request{Topic: "x"}, extractDocs(n), brief)

	if len(findings) != 0 {
		t.Errorf("findings = %d after cancel, want 0", len(findings))
	}
	if got := model.calls.Load(); got != 0 {
		t.Errorf("%d extractions started on a cancelled context", got)
	}
	if len(brief.Warnings) != n {
		t.Errorf("warnings = %d, want one per unextracted document (%d)", len(brief.Warnings), n)
	}
}

// Malformed JSON gets exactly one retry, then the document is a warning.
func TestExtractAll_MalformedJSONRetriesOnce(t *testing.T) {
	var calls atomic.Int32
	model := llmFunc(func(context.Context, llm.GenerateRequest) (*llm.GenerateResponse, error) {
		calls.Add(1)
		return &llm.GenerateResponse{Content: "Okay, let me tackle this query. [0] looks relevant."}, nil
	})
	agent := NewAgent(nil, model, DefaultAgentConfig(), zap.NewNop())
	brief := &Brief{}

	done := make(chan []Finding, 1)
	go func() { done <- agent.extractAll(context.Background(), Request{Topic: "x"}, extractDocs(1), brief) }()
	select {
	case f := <-done:
		if len(f) != 0 {
			t.Errorf("findings = %d from malformed replies, want 0", len(f))
		}
	case <-time.After(5 * time.Second):
		t.Fatal("extraction did not return — retry loop?")
	}
	if got := calls.Load(); got != 2 {
		t.Errorf("calls = %d, want 2 (first try + one retry)", got)
	}
	if len(brief.Warnings) != 1 || !strings.Contains(brief.Warnings[0], "extraction: parse response") {
		t.Errorf("warnings = %q, want one parse failure", brief.Warnings)
	}
}

type llmFunc func(context.Context, llm.GenerateRequest) (*llm.GenerateResponse, error)

func (f llmFunc) Generate(ctx context.Context, req llm.GenerateRequest) (*llm.GenerateResponse, error) {
	return f(ctx, req)
}
func (llmFunc) ProviderName() string { return "func" }
