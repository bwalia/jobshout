package imagegen

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/jobshout/server/internal/config"
)

// Router selects an image provider and answers "what can this platform draw
// with". It mirrors llm.Router deliberately: an operator who understands how a
// language model gets chosen should not have to learn a second scheme to
// understand how an image model gets chosen.
type Router struct {
	clients         map[string]Client
	defaultProvider string
	logger          *zap.Logger

	// cache memoises model discovery. Listing models on the local path is an
	// HTTP call to a machine that may be asleep, and a picker that hangs for
	// ten seconds every time it opens is a picker nobody opens.
	cache modelCache
}

// NewRouter builds a Router from configuration. Providers without enough
// configuration to work are skipped rather than registered and left to fail at
// call time.
func NewRouter(cfg *config.Config) *Router {
	r := &Router{
		clients:         make(map[string]Client),
		defaultProvider: cfg.ImageProvider,
	}

	// The workstation image service is registered whenever it has an address.
	// It needs no API key, and an unreachable service is a runtime failure with
	// a clear message rather than a reason to pretend the provider does not
	// exist.
	if cfg.ImageBaseURL != "" {
		r.clients[ProviderMFlux] = NewMFluxClient(
			cfg.ImageBaseURL, cfg.ImageDefaultModel, cfg.ImageJWTSecret, cfg.ImageTimeout,
		)
	}

	// Gemini sits in front of the workstation when a key is set. Unqualified
	// requests (the Image Generator agent, generate_image) try it first;
	// Generate falls back to mflux if the hosted call fails.
	if cfg.GeminiAPIKey != "" {
		r.clients[ProviderGemini] = NewGeminiClient(cfg.GeminiBaseURL, cfg.GeminiAPIKey, cfg.ImageGeminiModel)
	}

	// OpenAI images ride on the same key as OpenAI chat — one credential, one
	// place to rotate it.
	if cfg.OpenAIAPIKey != "" {
		r.clients[ProviderOpenAI] = NewOpenAIClient(cfg.OpenAIBaseURL, cfg.OpenAIAPIKey, cfg.ImageOpenAIModel)
	}

	// A default naming a provider that did not register would make every
	// unqualified request fail. Falling back to whichever provider does exist
	// keeps a partly-configured environment working.
	if _, ok := r.clients[r.defaultProvider]; !ok {
		r.defaultProvider = ""
		for _, name := range []string{ProviderGemini, ProviderMFlux, ProviderOpenAI} {
			if _, ok := r.clients[name]; ok {
				r.defaultProvider = name
				break
			}
		}
	} else if r.defaultProvider == ProviderMFlux {
		// IMAGE_PROVIDER still defaults to mflux in config and Helm. When
		// Gemini is configured, put it in front so the picker and the agent
		// agree on what "platform default" means.
		if _, ok := r.clients[ProviderGemini]; ok {
			r.defaultProvider = ProviderGemini
		}
	}

	return r
}

// WithLogger attaches a logger so a Gemini failure that falls back to the
// workstation is visible. Without this, a bad key looks like "mflux worked".
func (r *Router) WithLogger(logger *zap.Logger) *Router {
	if r != nil {
		r.logger = logger
	}
	return r
}

// NewTestRouter builds a Router over explicit clients, for tests.
func NewTestRouter(defaultProvider string, clients map[string]Client) *Router {
	return &Router{clients: clients, defaultProvider: defaultProvider}
}

// Enabled reports whether any provider is configured. Callers use it to leave
// image generation out of a run entirely rather than failing a step over it.
func (r *Router) Enabled() bool { return r != nil && len(r.clients) > 0 }

// DefaultProvider names the provider used when a request does not choose one.
func (r *Router) DefaultProvider() string {
	if r == nil {
		return ""
	}
	return r.defaultProvider
}

// For returns the client for provider, falling back to the default when the
// name is empty or unknown.
//
// Unknown falls back rather than failing because the name usually comes from a
// stored agent setting: a provider that was configured when the agent was
// created and has since been removed should not permanently break that agent.
func (r *Router) For(provider string) (Client, error) {
	if r == nil || len(r.clients) == 0 {
		return nil, fmt.Errorf("imagegen: no image provider is configured (set GEMINI_API_KEY, IMAGE_BASE_URL or OPENAI_API_KEY)")
	}
	if c, ok := r.clients[provider]; ok {
		return c, nil
	}
	if c, ok := r.clients[r.defaultProvider]; ok {
		return c, nil
	}
	return nil, fmt.Errorf("imagegen: no image provider named %q and no usable default", provider)
}

// Generate produces an image with the named provider.
//
// When Gemini is configured, an unqualified request or an explicit Gemini
// request tries Gemini first and falls back to the workstation (mflux) if
// that call fails. A cancelled context does not fall back — the caller has
// already given up. An explicit mflux or openai choice is honoured as-is,
// as is a request that names a workstation model such as z-image-turbo.
func (r *Router) Generate(ctx context.Context, provider string, req GenerateRequest) (*GenerateResponse, error) {
	if r == nil || len(r.clients) == 0 {
		return nil, fmt.Errorf("imagegen: no image provider is configured (set GEMINI_API_KEY, IMAGE_BASE_URL or OPENAI_API_KEY)")
	}
	if r.preferGemini(provider, req.Model) {
		resp, err := r.clients[ProviderGemini].Generate(ctx, req)
		if err == nil {
			return resp, nil
		}
		if ctx.Err() != nil {
			return nil, err
		}
		if req.NoFallback {
			return nil, err
		}
		if fb := r.workstationFallback(); fb != nil {
			if r.logger != nil {
				r.logger.Warn("gemini image generation failed; falling back to workstation",
					zap.Error(err), zap.String("fallback", fb.Provider()))
			}
			fbReq := req
			if isGeminiImageModel(fbReq.Model) {
				// mflux does not know Gemini model names; empty means its default.
				fbReq.Model = ""
			}
			resp, fbErr := fb.Generate(ctx, fbReq)
			if fbErr == nil {
				return resp, nil
			}
			return nil, fmt.Errorf("imagegen: gemini failed (%v); %s fallback failed: %w", err, fb.Provider(), fbErr)
		}
		return nil, err
	}

	if provider == "" && req.Model != "" && !isGeminiImageModel(req.Model) {
		if c, ok := r.clients[ProviderMFlux]; ok {
			return c.Generate(ctx, req)
		}
	}

	c, err := r.For(provider)
	if err != nil {
		return nil, err
	}
	return c.Generate(ctx, req)
}

// preferGemini reports whether this request should try the hosted Gemini
// model before the workstation.
func (r *Router) preferGemini(provider, model string) bool {
	if r == nil {
		return false
	}
	if _, ok := r.clients[ProviderGemini]; !ok {
		return false
	}
	switch provider {
	case ProviderGemini, "":
		return model == "" || isGeminiImageModel(model)
	default:
		return false
	}
}

// workstationFallback is the current local path — images.workstation / mflux.
func (r *Router) workstationFallback() Client {
	if r == nil {
		return nil
	}
	if c, ok := r.clients[ProviderMFlux]; ok {
		return c
	}
	return nil
}

// Providers lists the registered provider names, in a stable order.
func (r *Router) Providers() []string {
	if r == nil {
		return nil
	}
	out := make([]string, 0, len(r.clients))
	for name := range r.clients {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// modelCacheTTL is how long a discovered model list is trusted. Long enough
// that opening a picker is instant, short enough that a model downloaded on the
// workstation shows up without a restart.
const modelCacheTTL = 5 * time.Minute

type cachedModels struct {
	models  []ModelInfo
	fetched time.Time
}

type modelCache struct {
	mu sync.Mutex
	by map[string]cachedModels
}

func (m *modelCache) get(provider string) ([]ModelInfo, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	entry, ok := m.by[provider]
	if !ok || time.Since(entry.fetched) > modelCacheTTL {
		return nil, false
	}
	return entry.models, true
}

func (m *modelCache) put(provider string, models []ModelInfo) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.by == nil {
		m.by = make(map[string]cachedModels)
	}
	m.by[provider] = cachedModels{models: models, fetched: time.Now()}
}

// ListModels reports every model every registered provider can run.
//
// A provider that cannot be reached contributes nothing and does not fail the
// call: with the workstation asleep, the honest answer is "here is what the
// hosted provider can do", not an error page where the picker should be.
func (r *Router) ListModels(ctx context.Context) []ModelInfo {
	if r == nil {
		return nil
	}

	var out []ModelInfo
	for _, name := range r.Providers() {
		if cached, ok := r.cache.get(name); ok {
			out = append(out, cached...)
			continue
		}
		models, err := r.clients[name].ListModels(ctx)
		if err != nil {
			// Not cached: a provider that failed because it was momentarily
			// unreachable should be retried on the next open, not written off
			// for five minutes.
			continue
		}
		r.cache.put(name, models)
		out = append(out, models...)
	}
	return out
}

// Warm fetches every provider's model list into the cache.
//
// Called at startup so the first person to open the picker does not pay for
// discovery. Failures are ignored by design — this is an optimisation, and a
// workstation that is asleep at boot must not hold up the server.
func (r *Router) Warm(ctx context.Context) {
	if r == nil {
		return
	}
	r.ListModels(ctx)
}
