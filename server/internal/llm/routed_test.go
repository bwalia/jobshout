package llm

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jobshout/server/internal/config"
)

// namedClient answers with its own name, so a test can see which provider a
// call reached.
type namedClient struct{ name string }

func (c namedClient) ProviderName() string { return c.name }
func (c namedClient) Generate(context.Context, GenerateRequest) (*GenerateResponse, error) {
	return &GenerateResponse{Content: c.name}, nil
}

func testRouter() *Router {
	return NewTestRouter("ollama", map[string]Client{
		"ollama": namedClient{"ollama"},
		"gemini": namedClient{"gemini"},
		"openai": namedClient{"openai"},
	})
}

func routedReply(t *testing.T, c Client, ctx context.Context) string {
	t.Helper()
	resp, err := c.Generate(ctx, GenerateRequest{})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	return resp.Content
}

func TestRoutedClientSelectsProvider(t *testing.T) {
	r := testRouter()
	bg := context.Background()
	cases := []struct {
		name     string
		fallback string
		ctx      context.Context
		want     string
	}{
		{"router default", "", bg, "ollama"},
		{"fallback", "openai", bg, "openai"},
		{"context beats fallback", "openai", WithProvider(bg, "gemini"), "gemini"},
		{"case and space", "", WithProvider(bg, "  Gemini "), "gemini"},
		{"auto means default", "openai", WithProvider(bg, "auto"), "openai"},
		{"blank means default", "", WithProvider(bg, ""), "ollama"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := routedReply(t, r.Routed(tc.fallback), tc.ctx); got != tc.want {
				t.Errorf("reached %q, want %q", got, tc.want)
			}
		})
	}
}

// An agent set to a hosted provider this ring has no key for must fail with a
// message that says what to configure.
func TestRoutedClientUnconfiguredProvider(t *testing.T) {
	r := NewTestRouter("ollama", map[string]Client{"ollama": namedClient{"ollama"}})
	_, err := r.Routed("").Generate(WithProvider(context.Background(), "gemini"), GenerateRequest{})
	if !errors.Is(err, ErrProviderNotConfigured) || !strings.Contains(err.Error(), "GEMINI_API_KEY") {
		t.Fatalf("err = %v", err)
	}
}

func TestNewRouterRegistersGeminiOnlyWithAKey(t *testing.T) {
	base := config.Config{LLMProvider: "ollama", OllamaBaseURL: "http://ollama.invalid"}

	if _, err := NewRouter(&base).For("gemini"); !errors.Is(err, ErrProviderNotConfigured) {
		t.Errorf("without a key: err = %v, want ErrProviderNotConfigured", err)
	}

	withKey := base
	withKey.GeminiAPIKey = testKey
	withKey.GeminiDefaultModel = "gemini-flash-latest"
	r := NewRouter(&withKey)
	c, err := r.For("gemini")
	if err != nil {
		t.Fatalf("For(gemini): %v", err)
	}
	g, ok := c.(*GeminiClient)
	if !ok || g.DefaultModel != "gemini-flash-latest" || g.BaseURL != GeminiDefaultBaseURL {
		t.Errorf("gemini client = %#v", c)
	}
	// Registering Gemini must not change the default for everything else.
	if d := r.Default(); d == nil || d.ProviderName() != "ollama" {
		t.Errorf("default provider changed: %v", d)
	}
}

// The existing providers are still registered exactly as before.
func TestNewRouterExistingProvidersUnchanged(t *testing.T) {
	cfg := config.Config{LLMProvider: "ollama", OllamaBaseURL: "http://ollama.invalid",
		OpenAIAPIKey: "k", OpenAIDefaultModel: "gpt-4o-mini", ClaudeAPIKey: "k"}
	r := NewRouter(&cfg)
	for _, name := range []string{"ollama", "openai", "claude"} {
		c, err := r.For(name)
		if err != nil || c.ProviderName() != name {
			t.Errorf("For(%q) = %v, %v", name, c, err)
		}
	}
	if _, err := r.For("gemini"); err == nil {
		t.Error("gemini must not register without GEMINI_API_KEY")
	}
}

func TestStaticModelsListsGemini(t *testing.T) {
	models := StaticModels("gemini")
	if len(models) == 0 || models[0].Name != GeminiDefaultModel {
		t.Fatalf("gemini static models = %+v", models)
	}
}
