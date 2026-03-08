package llm

import (
	"fmt"

	"github.com/jobshout/server/internal/config"
)

// Router selects the appropriate LLM Client based on a provider name.
// The default provider is Ollama; per-agent overrides are applied at call time
// by passing the agent's ModelProvider value to For().
type Router struct {
	clients map[string]Client
	// defaultProvider is used when no per-agent override is present.
	defaultProvider string
}

// NewRouter builds a Router pre-populated with the configured LLM providers.
// Only providers with sufficient configuration are registered; missing optional
// providers are silently skipped.
func NewRouter(cfg *config.Config) *Router {
	r := &Router{
		clients:         make(map[string]Client),
		defaultProvider: cfg.LLMProvider,
	}

	// Ollama is always registered — it requires no API key.
	r.clients["ollama"] = NewOllamaClient(cfg.OllamaBaseURL, cfg.OllamaDefaultModel)

	// OpenAI (and compatible endpoints) are registered when an API key is set.
	if cfg.OpenAIAPIKey != "" {
		base := cfg.OpenAIBaseURL
		if base == "" {
			base = "https://api.openai.com"
		}
		r.clients["openai"] = NewOpenAIClient(base, cfg.OpenAIAPIKey, cfg.OpenAIDefaultModel)
	}

	return r
}

// For returns the Client for the given provider name, falling back to the
// default provider when providerName is empty.
func (r *Router) For(providerName string) (Client, error) {
	name := providerName
	if name == "" {
		name = r.defaultProvider
	}

	c, ok := r.clients[name]
	if !ok {
		return nil, fmt.Errorf("llm: unknown provider %q (registered: %v)", name, r.registeredNames())
	}
	return c, nil
}

// Default returns the default client.
func (r *Router) Default() Client {
	c, _ := r.For("")
	return c
}

func (r *Router) registeredNames() []string {
	names := make([]string, 0, len(r.clients))
	for n := range r.clients {
		names = append(names, n)
	}
	return names
}
