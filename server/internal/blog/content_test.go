package blog

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jobshout/server/internal/llm"
)

// stubLLM returns a canned response for each Generate call, in order.
type stubLLM struct {
	responses []string
	calls     []llm.GenerateRequest
	idx       int
}

func (s *stubLLM) Generate(ctx context.Context, req llm.GenerateRequest) (*llm.GenerateResponse, error) {
	s.calls = append(s.calls, req)
	if s.idx >= len(s.responses) {
		return nil, fmt.Errorf("stubLLM: out of responses")
	}
	out := s.responses[s.idx]
	s.idx++
	return &llm.GenerateResponse{Content: out}, nil
}

func (s *stubLLM) ProviderName() string { return "stub" }

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

func TestGenerateArticles_Success(t *testing.T) {
	llm := &stubLLM{responses: []string{
		"# Kubernetes\n\nBody 1.",
		"```\n# AI Agents\n\nBody 2.\n```",
	}}
	now := time.Date(2026, 4, 29, 10, 0, 0, 0, time.UTC)

	arts, err := generateArticles(context.Background(), llm, "llama3", "content/blogs",
		[]string{"Kubernetes debugging", "AI agents"}, now)
	if err != nil {
		t.Fatalf("generateArticles: %v", err)
	}
	if len(arts) != 2 {
		t.Fatalf("want 2 articles, got %d", len(arts))
	}
	if !strings.HasPrefix(arts[0].Path, "content/blogs/2026-04-29-kubernetes-debugging") {
		t.Errorf("unexpected path[0]: %q", arts[0].Path)
	}
	if strings.HasPrefix(arts[1].Markdown, "```") {
		t.Errorf("outer fence leaked into article[1]: %q", arts[1].Markdown[:20])
	}

	// Prompt should contain the topic verbatim so the LLM sees it.
	if !strings.Contains(llm.calls[0].Messages[0].Content, "Kubernetes debugging") {
		t.Error("prompt missing topic")
	}
	if llm.calls[0].Model != "llama3" {
		t.Errorf("model override not applied: %q", llm.calls[0].Model)
	}
}

func TestGenerateArticles_EmptyTopic(t *testing.T) {
	l := &stubLLM{responses: []string{"# x"}}
	_, err := generateArticles(context.Background(), l, "", "content/blogs",
		[]string{"  "}, time.Now())
	if err == nil {
		t.Fatal("expected error for empty topic")
	}
}

func TestGenerateArticles_DuplicateTopics(t *testing.T) {
	l := &stubLLM{responses: []string{"# a", "# b"}}
	arts, err := generateArticles(context.Background(), l, "", "content/blogs",
		[]string{"Kubernetes debugging", "Kubernetes debugging"}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if arts[0].Path == arts[1].Path {
		t.Errorf("duplicate topics produced duplicate paths: %q", arts[0].Path)
	}
}
