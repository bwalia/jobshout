package course

import (
	"context"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/llm"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/research"
)

// recordingLLM wraps a scripted client and records each call's model.
type recordingLLM struct {
	inner  llm.Client
	mu     sync.Mutex
	models []string
}

func (r *recordingLLM) ProviderName() string { return r.inner.ProviderName() }
func (r *recordingLLM) Generate(ctx context.Context, req llm.GenerateRequest) (*llm.GenerateResponse, error) {
	r.mu.Lock()
	r.models = append(r.models, req.Model)
	r.mu.Unlock()
	return r.inner.Generate(ctx, req)
}

// ctxResearch records the provider and model research was asked to use.
type ctxResearch struct {
	stubResearch
	provider string
}

func (c *ctxResearch) Research(ctx context.Context, req research.Request, p research.ProgressFunc) (*research.Brief, error) {
	c.provider = llm.ProviderFrom(ctx)
	return c.stubResearch.Research(ctx, req, p)
}

// A brief naming Gemini sends every stage there through the routed client,
// and COURSE_MODEL (a model on the server's provider) is not sent along.
func TestGenerate_BriefProviderRoutesEveryCall(t *testing.T) {
	ollama := &recordingLLM{inner: &scriptLLM{}}
	gemini := &recordingLLM{inner: &scriptLLM{replies: goodScript()}}
	router := llm.NewTestRouter("ollama", map[string]llm.Client{"ollama": ollama, "gemini": gemini})
	rs := &ctxResearch{stubResearch: stubResearch{brief: usableBrief()}}

	g := NewGenerator(router.Routed("ollama"), rs, nil, Config{Provider: "ollama", Model: "qwen3:8b"}, nil)
	b := testBrief()
	b.Provider = "gemini"

	if err := g.Generate(context.Background(), Job{OrgID: uuid.New(), Brief: b}, Hooks{}); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if len(ollama.models) != 0 {
		t.Errorf("%d calls reached the server's provider", len(ollama.models))
	}
	if len(gemini.models) == 0 {
		t.Fatal("no calls reached gemini")
	}
	for _, m := range gemini.models {
		if m != "" {
			t.Errorf("gemini was sent model %q; want its own default", m)
		}
	}
	if rs.provider != "gemini" || rs.got.Model != "" {
		t.Errorf("research ran on %q with model %q", rs.provider, rs.got.Model)
	}
}

func TestModelForRespectsProvider(t *testing.T) {
	g := NewGenerator(&scriptLLM{}, nil, nil, Config{Provider: "ollama", Model: "qwen3:8b"}, nil)
	cases := []struct {
		brief model.CourseBrief
		want  string
	}{
		{model.CourseBrief{}, "qwen3:8b"},
		{model.CourseBrief{Provider: "ollama"}, "qwen3:8b"},
		{model.CourseBrief{Provider: "gemini"}, ""},
		{model.CourseBrief{Provider: "gemini", Model: "gemini-2.5-pro"}, "gemini-2.5-pro"},
		{model.CourseBrief{Model: "llama3.1:8b"}, "llama3.1:8b"},
	}
	for _, tc := range cases {
		if got := g.modelFor(tc.brief); got != tc.want {
			t.Errorf("modelFor(%+v) = %q, want %q", tc.brief, got, tc.want)
		}
	}
}

func TestBriefCarriesProvider(t *testing.T) {
	b := BriefFromValues(map[string]string{"topic": "Kubernetes networking", "provider": " Gemini "})
	n, err := NormalizeBrief(b, 8)
	if err != nil {
		t.Fatal(err)
	}
	if n.Provider != "gemini" {
		t.Errorf("provider = %q, want gemini", n.Provider)
	}
	n, _ = NormalizeBrief(model.CourseBrief{Topic: "x y z", Provider: "auto"}, 8)
	if n.Provider != "" {
		t.Errorf("auto must mean the server's provider, got %q", n.Provider)
	}
}
