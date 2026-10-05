package llm

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/zap"
)

// failing answers with a ProviderError of the given kind, so the fallback
// decision can be exercised without a real provider.
type failing struct {
	name string
	kind error
}

func (c failing) ProviderName() string { return c.name }
func (c failing) Generate(context.Context, GenerateRequest) (*GenerateResponse, error) {
	return nil, &ProviderError{Provider: c.name, Kind: c.kind}
}

func fallbackRouter(primary Client) *Router {
	r := NewTestRouter("ollama", map[string]Client{
		"ollama": namedClient{"ollama"},
		"gemini": primary,
	})
	r.EnableOllamaFallback("", zap.NewNop())
	return r
}

func TestOllamaFallback_OnRateLimit(t *testing.T) {
	r := fallbackRouter(failing{"gemini", ErrProviderRateLimited})
	got := routedReply(t, r.Routed("gemini"), context.Background())
	if got != "ollama" {
		t.Fatalf("a rate-limited gemini call should fall back to ollama, reached %q", got)
	}
}

func TestOllamaFallback_OnUnavailable(t *testing.T) {
	r := fallbackRouter(failing{"gemini", ErrProviderUnavailable})
	if got := routedReply(t, r.Routed("gemini"), context.Background()); got != "ollama" {
		t.Fatalf("unavailable provider should fall back, reached %q", got)
	}
}

func TestOllamaFallback_NotOnAuthError(t *testing.T) {
	// An auth failure would fail on Ollama too — do not fall back, surface it.
	r := fallbackRouter(failing{"gemini", ErrProviderAuth})
	_, err := r.Routed("gemini").Generate(context.Background(), GenerateRequest{})
	if err == nil {
		t.Fatal("auth error should not fall back; it should surface")
	}
	if !errors.Is(err, ErrProviderAuth) {
		t.Fatalf("want the original auth error, got %v", err)
	}
}

func TestOllamaFallback_Disabled(t *testing.T) {
	r := NewTestRouter("ollama", map[string]Client{
		"ollama": namedClient{"ollama"},
		"gemini": failing{"gemini", ErrProviderRateLimited},
	})
	// EnableOllamaFallback not called → no fallback.
	_, err := r.Routed("gemini").Generate(context.Background(), GenerateRequest{})
	if err == nil {
		t.Fatal("with fallback disabled the error should surface")
	}
}

func TestOllamaFallback_NoSelfFallback(t *testing.T) {
	// An Ollama call that is itself rate-limited must not loop back to Ollama.
	r := NewTestRouter("ollama", map[string]Client{
		"ollama": failing{"ollama", ErrProviderRateLimited},
	})
	r.EnableOllamaFallback("", zap.NewNop())
	_, err := r.Routed("ollama").Generate(context.Background(), GenerateRequest{})
	if err == nil {
		t.Fatal("ollama failing should surface, not self-fall-back")
	}
}

func TestForWithFallback(t *testing.T) {
	r := fallbackRouter(failing{"gemini", ErrProviderRateLimited})
	c, err := r.ForWithFallback("gemini")
	if err != nil {
		t.Fatalf("ForWithFallback: %v", err)
	}
	resp, err := c.Generate(context.Background(), GenerateRequest{})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if resp.Content != "ollama" {
		t.Fatalf("ForWithFallback client should fall back to ollama, got %q", resp.Content)
	}
}
