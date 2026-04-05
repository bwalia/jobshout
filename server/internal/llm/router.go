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

	// Claude / Anthropic is registered when an API key is set.
	if cfg.ClaudeAPIKey != "" {
		r.clients["claude"] = NewClaudeClient(cfg.ClaudeBaseURL, cfg.ClaudeAPIKey, cfg.ClaudeDefaultModel)
	}

	return r
}

// RegisteredProviders returns a list of all registered provider names and
// whether each is the default. Useful for the /llm-providers API endpoint.
func (r *Router) RegisteredProviders() []ProviderInfo {
	infos := make([]ProviderInfo, 0, len(r.clients))
	for name := range r.clients {
		infos = append(infos, ProviderInfo{
			Name:      name,
			IsDefault: name == r.defaultProvider,
		})
	}
	return infos
}

// ProviderInfo describes a registered LLM provider.
type ProviderInfo struct {
	Name      string `json:"name"`
	IsDefault bool   `json:"is_default"`
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
