package blog

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/llm"
	"github.com/jobshout/server/internal/research"
)

// stubLLM answers each prompt with the first canned response whose trigger
// appears in it.
//
// Keyed on content rather than call order because the writer now makes several
// different calls per article — plan, draft, review, sometimes revise — and an
// ordered list breaks every time a phase is added or skipped.
type stubLLM struct {
	responses []scriptedResponse
	calls     []llm.GenerateRequest
	// failOn makes any prompt containing this substring return an error.
	failOn string
}

type scriptedResponse struct {
	trigger string
	content string
}

func (s *stubLLM) Generate(ctx context.Context, req llm.GenerateRequest) (*llm.GenerateResponse, error) {
	s.calls = append(s.calls, req)
	prompt := req.Messages[len(req.Messages)-1].Content

	if s.failOn != "" && strings.Contains(prompt, s.failOn) {
		return nil, fmt.Errorf("stubLLM: scripted failure")
	}
	for _, r := range s.responses {
		if strings.Contains(prompt, r.trigger) {
			return &llm.GenerateResponse{Content: r.content}, nil
		}
	}
	return nil, fmt.Errorf("stubLLM: no canned response matched prompt")
}

func (s *stubLLM) ProviderName() string { return "stub" }

// Prompt fragments that identify each phase, so a test can script one phase
// without knowing the wording of the others.
const (
	promptPlan   = "planning a technical article"
	promptDraft  = "writing a technical article"
	promptReview = "reviewing a draft technical article"
	promptRevise = "revising a technical article"
	promptExpand = "This article is too short"
)

// writeScript builds the canned responses for a single clean article.
func writeScript(title, body string) []scriptedResponse {
	return []scriptedResponse{
		{trigger: promptPlan, content: fmt.Sprintf(`{"title":%q,"angle":"why it matters","sections":["One","Two"]}`, title)},
		{trigger: promptDraft, content: body},
		{trigger: promptReview, content: `{"issues":[]}`},
	}
}

// testDoc is the source text the fake researcher's findings quote from.
const testDoc = "Kubernetes 1.31 promoted the Gateway API to general availability after an extended beta."

// fakeResearcher returns a canned brief without touching the network or an LLM.
type fakeResearcher struct {
	brief *research.Brief
	err   error
	// requests records what the writer asked to have researched.
	requests []research.Request
}

func (f *fakeResearcher) Research(
	_ context.Context, _ uuid.UUID, req research.Request, progress research.ProgressFunc,
) (*research.Brief, error) {
	f.requests = append(f.requests, req)
	if progress != nil {
		progress(research.PhaseSearching, "searching")
	}
	if f.err != nil {
		return nil, f.err
	}
	if f.brief != nil {
		return f.brief, nil
	}
	return defaultBrief(), nil
}

// defaultBrief is a usable two-source brief.
func defaultBrief() *research.Brief {
	return &research.Brief{
		Topic:   "Gateway API",
		Summary: "Gateway API is now GA.",
		Findings: []research.Finding{
			{Claim: "Gateway API reached GA in 1.31.", SourceURL: "https://kubernetes.io/blog/ga", Quote: testDoc},
			{Claim: "A migration tool exists.", SourceURL: "https://kubernetes.io/blog/migrate", Quote: testDoc},
		},
		Sources: []research.Source{
			{URL: "https://kubernetes.io/blog/ga", Title: "Gateway API is GA", Site: "kubernetes.io"},
			{URL: "https://kubernetes.io/blog/migrate", Title: "Migrating from Ingress", Site: "kubernetes.io"},
		},
	}
}

func TestSlugify(t *testing.T) {
	cases := map[string]string{
		"Kubernetes debugging tips":  "kubernetes-debugging-tips",
		"AI agent architectures!":    "ai-agent-architectures",
		"  multiple   spaces  ":      "multiple-spaces",
		"Spëcial Chärs & symbols #1": "sp-cial-ch-rs-symbols-1",
		"":                           "untitled",
		"---only-hyphens---":         "only-hyphens",
	}
	for in, want := range cases {
		if got := slugify(in); got != want {
			t.Errorf("slugify(%q) = %q, want %q", in, got, want)
		}
	}

	long := strings.Repeat("topic-", 20) // 120 chars
	if got := slugify(long); len(got) > 60 {
		t.Errorf("slugify long: len %d > 60", len(got))
	}
}

func TestStripOuterFence(t *testing.T) {
	wrapped := "```markdown\n# Title\n\nBody text.\n```"
	got := stripOuterFence(wrapped)
	if strings.HasPrefix(got, "```") || strings.HasSuffix(got, "```") {
		t.Errorf("stripOuterFence left fence: %q", got)
	}
	if !strings.HasPrefix(got, "# Title") {
		t.Errorf("stripOuterFence lost content: %q", got)
	}

	// Plain markdown should be unchanged.
	plain := "# Title\n\nBody."
	if got := stripOuterFence(plain); got != plain {
		t.Errorf("stripOuterFence mutated plain markdown: %q", got)
	}
}

// testLogger is a no-op logger for tests that construct a Runner directly.
func testLogger() *zap.Logger { return zap.NewNop() }
