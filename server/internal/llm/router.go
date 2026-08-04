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

	// embedders holds registered embedding providers, keyed by provider name.
	embedders map[string]Embedder
	// defaultEmbeddingProvider is used when no explicit provider is requested.
	defaultEmbeddingProvider string
}

// NewRouter builds a Router pre-populated with the configured LLM providers.
// Only providers with sufficient configuration are registered; missing optional
// providers are silently skipped.
func NewRouter(cfg *config.Config) *Router {
	r := &Router{
		clients:                  make(map[string]Client),
		defaultProvider:          cfg.LLMProvider,
		embedders:                make(map[string]Embedder),
		defaultEmbeddingProvider: cfg.EmbeddingProvider,
	}

	// Ollama is always registered — it requires no API key.
	r.clients["ollama"] = NewOllamaClient(cfg.OllamaBaseURL, cfg.OllamaDefaultModel)

	// OpenAI (and compatible endpoints) are registered when an API key is set.
	openAIBase := cfg.OpenAIBaseURL
	if openAIBase == "" {
		openAIBase = "https://api.openai.com"
	}
	if cfg.OpenAIAPIKey != "" {
		r.clients["openai"] = NewOpenAIClient(openAIBase, cfg.OpenAIAPIKey, cfg.OpenAIDefaultModel)
	}

	// Claude / Anthropic is registered when an API key is set.
	if cfg.ClaudeAPIKey != "" {
		r.clients["claude"] = NewClaudeClient(cfg.ClaudeBaseURL, cfg.ClaudeAPIKey, cfg.ClaudeDefaultModel)
	}

	// ─── Embedders ───────────────────────────────────────────────────────────
	dims := cfg.EmbeddingDimensions
	if dims <= 0 {
		dims = 1536
	}
	// Resolve the per-provider embedding model, honouring EMBEDDING_MODEL only
	// for the configured default embedding provider.
	openAIEmbModel := "text-embedding-3-small"
	ollamaEmbModel := "nomic-embed-text"
	switch cfg.EmbeddingProvider {
	case "openai":
		if cfg.EmbeddingModel != "" {
			openAIEmbModel = cfg.EmbeddingModel
		}
	case "ollama":
		if cfg.EmbeddingModel != "" {
			ollamaEmbModel = cfg.EmbeddingModel
		}
	}

	// Ollama embedder requires no API key, so it is always available.
	r.embedders["ollama"] = NewOllamaEmbedder(cfg.OllamaBaseURL, ollamaEmbModel, dims)
	// OpenAI embedder is registered only when an API key is set.
	if cfg.OpenAIAPIKey != "" {
		r.embedders["openai"] = NewOpenAIEmbedder(openAIBase, cfg.OpenAIAPIKey, openAIEmbModel, dims)
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

// EmbedderFor returns the Embedder for the given provider name, falling back to
// the default embedding provider when providerName is empty. An error is
// returned when the requested provider is not registered (e.g. OpenAI without
// an API key) so callers can degrade gracefully.
func (r *Router) EmbedderFor(providerName string) (Embedder, error) {
	name := providerName
	if name == "" {
		name = r.defaultEmbeddingProvider
	}

	e, ok := r.embedders[name]
	if !ok {
		return nil, fmt.Errorf("llm: unknown or unconfigured embedding provider %q (registered: %v)", name, r.embedderNames())
	}
	return e, nil
}

// Embedder returns the default embedder, or an error when the configured
// default embedding provider is not registered.
func (r *Router) Embedder() (Embedder, error) {
	return r.EmbedderFor("")
}

func (r *Router) embedderNames() []string {
	names := make([]string, 0, len(r.embedders))
	for n := range r.embedders {
		names = append(names, n)
	}
	return names
}

func (r *Router) registeredNames() []string {
	names := make([]string, 0, len(r.clients))
	for n := range r.clients {
		names = append(names, n)
	}
	return names
}
