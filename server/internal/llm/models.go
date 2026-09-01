package llm

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"
)

// Capability strings describing what a model can do. These mirror the values
// Ollama reports in /api/tags and are asserted by hand for providers that have
// no usable discovery. Lower-case and provider-neutral.
const (
	CapCompletion = "completion"
	CapTools      = "tools"
	CapVision     = "vision"
	CapThinking   = "thinking"
	CapEmbedding  = "embedding"
)

// ModelInfo is one model a provider can run.
//
// It carries only facts a provider reports about itself, or that we assert on
// its behalf — never judgements like "quality", which belong to modelselect.
// That split is what stops the two packages from duplicating a catalog.
type ModelInfo struct {
	Provider string `json:"provider"`
	// Name is the wire id, passed through as GenerateRequest.Model.
	Name string `json:"name"`
	// ContextTokens is the model's architectural context window. Zero means the
	// provider did not report one.
	//
	// NOTE: for Ollama this is the model's *limit*, not what a request is
	// granted — the server applies its own default unless num_ctx is set. See
	// OllamaClient.effectiveNumCtx.
	ContextTokens int `json:"context_tokens"`
	// ParameterSize is the provider's own description ("30.5B"). Empty when
	// unknown. Only modelselect interprets it.
	ParameterSize string `json:"parameter_size,omitempty"`
	// Quantization is the provider's own description ("Q4_K_M"). Empty when
	// unknown.
	Quantization string `json:"quantization,omitempty"`
	// Family is the provider's architecture label ("qwen3moe"). Empty when
	// unknown.
	Family string `json:"family,omitempty"`
	// Capabilities may be empty when a provider is too old to report them; an
	// empty slice means "unknown", which every caller must read as "assume chat,
	// assume no tools".
	Capabilities []string `json:"capabilities"`
}

// Has reports whether the model advertises the given capability.
func (m ModelInfo) Has(capability string) bool {
	for _, c := range m.Capabilities {
		if strings.EqualFold(c, capability) {
			return true
		}
	}
	return false
}

// SupportsTools reports whether the model itself can do native tool-calling.
// This is the model's capability only; whether our client implements the wire
// format is a separate question — see ToolCapableClient.
func (m ModelInfo) SupportsTools() bool { return m.Has(CapTools) }

// SupportsVision reports whether the model accepts images.
func (m ModelInfo) SupportsVision() bool { return m.Has(CapVision) }

// SupportsThinking reports whether the model has a reasoning phase that can be
// requested per call (GenerateRequest.Think).
func (m ModelInfo) SupportsThinking() bool { return m.Has(CapThinking) }

// IsEmbeddingOnly reports whether the model can only produce embeddings, and so
// must never appear in a chat-model picker.
func (m ModelInfo) IsEmbeddingOnly() bool {
	return m.Has(CapEmbedding) && !m.Has(CapCompletion)
}

// ModelLister is the OPTIONAL capability interface a Client implements when it
// can enumerate the models it is actually able to run. It is kept separate from
// Client — exactly as ToolCapableClient is — so existing implementations and
// test stubs satisfy Client unchanged and callers type-assert to discover it.
type ModelLister interface {
	ListModels(ctx context.Context) ([]ModelInfo, error)
}

// Discovery source labels, reported so the UI can distinguish a live answer from
// a fallback.
const (
	SourceDiscovered = "discovered"
	SourceStatic     = "static"
	SourceStale      = "stale"
)

// ProviderModels is one provider's slice of a discovery result.
type ProviderModels struct {
	Provider  string      `json:"provider"`
	IsDefault bool        `json:"is_default"`
	Source    string      `json:"source"`
	Models    []ModelInfo `json:"models"`
	// Error is set when a *registered* provider could not be probed. Providers
	// that are not registered are omitted entirely rather than reported here.
	Error string `json:"error,omitempty"`
}

const (
	// modelDiscoveryTTL bounds how stale a cached model list may be. The picker
	// is opened far more often than models are installed.
	modelDiscoveryTTL = 60 * time.Second
	// modelDiscoveryTimeout bounds a single provider probe. It is deliberately
	// much shorter than OllamaClient's generate timeout, which is minutes long
	// to allow cold model loads — a hung gateway must not stall an API request
	// for that long.
	modelDiscoveryTimeout = 5 * time.Second
)

type modelCacheEntry struct {
	models  []ModelInfo
	source  string
	err     error
	fetched time.Time
}

// modelCache memoises per-provider discovery.
//
// It is a value field on Router with a lazily-created map, NOT a pointer set up
// in NewRouter: NewTestRouter builds a Router as a composite literal, so any
// field it does not name is zero-valued. A pointer here would nil-panic in every
// test that uses that helper.
type modelCache struct {
	mu      sync.RWMutex
	entries map[string]modelCacheEntry
}

func (c *modelCache) get(provider string) (modelCacheEntry, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	e, ok := c.entries[provider]
	return e, ok
}

func (c *modelCache) put(provider string, e modelCacheEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entries == nil {
		c.entries = make(map[string]modelCacheEntry)
	}
	c.entries[provider] = e
}

// AvailableModels returns every registered provider's models, refreshing any
// entry older than the TTL.
//
// A provider whose probe fails degrades on its own — its last good result is
// served as "stale", otherwise the static list — and the error is reported in
// ProviderModels.Error. One broken provider never fails the whole call.
func (r *Router) AvailableModels(ctx context.Context) []ProviderModels {
	out := make([]ProviderModels, 0, len(r.clients))
	for name := range r.clients {
		out = append(out, r.modelsFor(ctx, name, false))
	}
	sort.Slice(out, func(i, j int) bool {
		// Default provider first, then alphabetical, so the picker's first
		// group is the one most users want.
		if out[i].IsDefault != out[j].IsDefault {
			return out[i].IsDefault
		}
		return out[i].Provider < out[j].Provider
	})
	return out
}

// CachedModels returns only what is already cached and performs no I/O.
//
// The selector uses this so auto-selection never blocks on a network call: it
// runs on the execution path, where a five-second stall per call would be far
// worse than a slightly stale model list.
func (r *Router) CachedModels() []ProviderModels {
	out := make([]ProviderModels, 0, len(r.clients))
	for name := range r.clients {
		e, ok := r.modelCache.get(name)
		if !ok {
			// Never probed yet — fall back to the static list rather than
			// reporting the provider as having no models at all.
			out = append(out, ProviderModels{
				Provider:  name,
				IsDefault: name == r.defaultProvider,
				Source:    SourceStatic,
				Models:    StaticModels(name),
			})
			continue
		}
		out = append(out, ProviderModels{
			Provider:  name,
			IsDefault: name == r.defaultProvider,
			Source:    e.source,
			Models:    e.models,
		})
	}
	return out
}

// RefreshModels forces a refresh of every registered provider, ignoring the TTL.
// Called once at startup and on a ticker so CachedModels is warm.
func (r *Router) RefreshModels(ctx context.Context) []ProviderModels {
	out := make([]ProviderModels, 0, len(r.clients))
	for name := range r.clients {
		out = append(out, r.modelsFor(ctx, name, true))
	}
	return out
}

// modelsFor resolves one provider, using the cache unless force is set.
func (r *Router) modelsFor(ctx context.Context, provider string, force bool) ProviderModels {
	res := ProviderModels{
		Provider:  provider,
		IsDefault: provider == r.defaultProvider,
	}

	cached, hasCached := r.modelCache.get(provider)
	if !force && hasCached && time.Since(cached.fetched) < modelDiscoveryTTL {
		res.Source = cached.source
		res.Models = cached.models
		if cached.err != nil {
			res.Error = cached.err.Error()
		}
		return res
	}

	client, err := r.For(provider)
	if err != nil {
		res.Source = SourceStatic
		res.Models = StaticModels(provider)
		return res
	}

	lister, ok := client.(ModelLister)
	if !ok {
		// No discovery for this provider (OpenAI and Claude both expose
		// /v1/models, but neither reports context window or tool capability —
		// so a live call would replace good data with worse).
		entry := modelCacheEntry{
			models:  StaticModels(provider),
			source:  SourceStatic,
			fetched: time.Now(),
		}
		r.modelCache.put(provider, entry)
		res.Source = entry.source
		res.Models = entry.models
		return res
	}

	probeCtx, cancel := context.WithTimeout(ctx, modelDiscoveryTimeout)
	defer cancel()

	models, err := lister.ListModels(probeCtx)
	if err != nil {
		// Degrade: last good result if we have one, else the static list.
		entry := modelCacheEntry{err: err, fetched: time.Now()}
		if hasCached && len(cached.models) > 0 {
			entry.models, entry.source = cached.models, SourceStale
		} else {
			entry.models, entry.source = StaticModels(provider), SourceStatic
		}
		r.modelCache.put(provider, entry)
		res.Source, res.Models, res.Error = entry.source, entry.models, err.Error()
		return res
	}

	entry := modelCacheEntry{models: models, source: SourceDiscovered, fetched: time.Now()}
	r.modelCache.put(provider, entry)
	res.Source, res.Models = entry.source, entry.models
	return res
}
