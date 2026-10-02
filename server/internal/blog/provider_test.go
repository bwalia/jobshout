package blog

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/zap"

	"github.com/google/uuid"
	"github.com/jobshout/server/internal/llm"
	"github.com/jobshout/server/internal/research"
)

// refusingLLM stands in for the server's provider: any call reaching it means
// the run ignored the provider its agent was set to.
type refusingLLM struct{ calls int }

func (r *refusingLLM) ProviderName() string { return "ollama" }
func (r *refusingLLM) Generate(context.Context, llm.GenerateRequest) (*llm.GenerateResponse, error) {
	r.calls++
	return nil, errors.New("the server provider must not be called")
}

// ctxResearcher records the provider research was asked to use.
type ctxResearcher struct {
	fakeResearcher
	provider string
}

func (c *ctxResearcher) Research(ctx context.Context, org uuid.UUID, req research.Request, p research.ProgressFunc) (*research.Brief, error) {
	c.provider = llm.ProviderFrom(ctx)
	return c.fakeResearcher.Research(ctx, org, req, p)
}

// The Article Writer has no Gemini code: an agent set to Gemini reaches it
// through the routed client, research included, with no Ollama model names.
func TestGenerate_AgentProviderRoutesEveryCall(t *testing.T) {
	ollama := &refusingLLM{}
	gemini := &stubLLM{responses: writeScript("Kubernetes", "# Kubernetes\n\nBody.")}
	router := llm.NewTestRouter("ollama", map[string]llm.Client{"ollama": ollama, "gemini": gemini})
	researcher := &ctxResearcher{}

	r := NewRunner(Config{
		ContentDir: "content/blogs", AuthorName: "Test Writer",
		Provider: "ollama", Model: "qwen3:8b", ProseModel: "qwen3.8:latest",
	}, router.Routed("ollama"), nil, researcher, zap.NewNop())

	arts, err := r.Generate(context.Background(), GenerateRequest{
		Briefs:        briefsFor("Kubernetes debugging"),
		AgentProvider: "gemini",
	}, nil)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if len(arts) != 1 {
		t.Fatalf("want 1 article, got %d", len(arts))
	}
	if ollama.calls != 0 {
		t.Errorf("%d calls reached the server's provider", ollama.calls)
	}
	if len(gemini.calls) == 0 {
		t.Fatal("no calls reached gemini")
	}
	for _, c := range gemini.calls {
		if c.Model != "" {
			t.Errorf("gemini was sent model %q; want its own default", c.Model)
		}
	}
	if researcher.provider != "gemini" {
		t.Errorf("research ran on provider %q, want gemini", researcher.provider)
	}
}
