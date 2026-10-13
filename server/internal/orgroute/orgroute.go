// Package orgroute routes an org's LLM calls through the org's own provider
// configs (llm_provider_configs) with an ordered fallback chain, instead of the
// server's env providers.
//
// It is installed as the outermost llm.Router client wrapper, so every caller
// that resolves a client from the router — the executor (fresh runs and
// approval resumes), and the Routed() clients of the specialist agents — is
// covered without being changed. Per call, the wrapper reads the org from the
// context (WithOrg, else the executor's tools.WithOrg, else the Langfuse trace
// label) and, when that org has an active chain, sends the call down the chain
// instead of to the env client. Calls with no org, or for an org without
// active configs (or with routing disabled), go to the env client unchanged.
//
// The model is swappable by configuration only: a chain hop's model (or its
// config's default_model) replaces whatever model the agent asked for, since
// an agent's model name rarely exists on the org's provider.
package orgroute

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/llm"
	"github.com/jobshout/server/internal/llmtrace"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/tools"
)

// Store is what the resolver reads: the org's active configs (secrets opened)
// and its optional routing row.
type Store interface {
	ListActiveWithSecrets(ctx context.Context, orgID uuid.UUID) ([]model.LLMProviderConfig, error)
	Get(ctx context.Context, orgID uuid.UUID) (*model.OrgLLMRouting, error)
}

// Step is one resolved hop of an org's chain.
type Step struct {
	ConfigID     uuid.UUID  `json:"provider_config_id"`
	Name         string     `json:"name"`
	ProviderType string     `json:"provider_type"`
	Model        string     `json:"model"`
	Client       llm.Client `json:"-"`
}

// Options tune a Resolver. Zero values are sensible.
type Options struct {
	// TTL bounds how long an org's resolved chain is cached. Default 30s.
	// Writes through the API invalidate immediately (Invalidate).
	TTL time.Duration
	// Decorate wraps every client the resolver builds (Langfuse tracing, the
	// LLM benchmark), so org-routed calls are recorded like env ones.
	Decorate func(llm.Client) llm.Client
	// Build constructs a client for a config. Default: BuildClient.
	Build  func(model.LLMProviderConfig) (llm.Client, error)
	Logger *zap.Logger
}

// Resolver resolves and caches org chains.
type Resolver struct {
	store Store
	opts  Options

	mu      sync.Mutex
	chains  map[uuid.UUID]cachedChain
	clients map[uuid.UUID]cachedClient
	now     func() time.Time
}

type cachedChain struct {
	steps []Step
	// agentFirst is routing mode "fallback": the caller's own provider and
	// model answer first, and the chain only takes over when that fails.
	agentFirst bool
	expires    time.Time
}

type cachedClient struct {
	updatedAt time.Time
	client    llm.Client
}

// NewResolver builds a Resolver over store.
func NewResolver(store Store, opts Options) *Resolver {
	if opts.TTL <= 0 {
		opts.TTL = 30 * time.Second
	}
	if opts.Build == nil {
		opts.Build = BuildClient
	}
	if opts.Logger == nil {
		opts.Logger = zap.NewNop()
	}
	return &Resolver{
		store:   store,
		opts:    opts,
		chains:  map[uuid.UUID]cachedChain{},
		clients: map[uuid.UUID]cachedClient{},
		now:     time.Now,
	}
}

// Invalidate drops the org's cached chain; the next call re-reads it.
func (r *Resolver) Invalidate(orgID uuid.UUID) {
	r.mu.Lock()
	delete(r.chains, orgID)
	r.mu.Unlock()
}

// Chain returns the org's effective chain, or nil when the org's calls should
// use the env providers (no active configs, or routing disabled).
func (r *Resolver) Chain(ctx context.Context, orgID uuid.UUID) ([]Step, error) {
	c, err := r.chain(ctx, orgID)
	return c.steps, err
}

func (r *Resolver) chain(ctx context.Context, orgID uuid.UUID) (cachedChain, error) {
	r.mu.Lock()
	if c, ok := r.chains[orgID]; ok && r.now().Before(c.expires) {
		r.mu.Unlock()
		return c, nil
	}
	r.mu.Unlock()

	routing, err := r.store.Get(ctx, orgID)
	if err != nil {
		return cachedChain{}, err
	}
	steps, err := r.resolve(ctx, orgID, routing)
	if err != nil {
		return cachedChain{}, err
	}
	c := cachedChain{
		steps:      steps,
		agentFirst: routing != nil && routing.Mode == model.RoutingModeFallback,
		expires:    r.now().Add(r.opts.TTL),
	}
	r.mu.Lock()
	r.chains[orgID] = c
	r.mu.Unlock()
	return c, nil
}

func (r *Resolver) resolve(ctx context.Context, orgID uuid.UUID, routing *model.OrgLLMRouting) ([]Step, error) {
	if routing != nil && !routing.Enabled {
		return nil, nil
	}
	configs, err := r.store.ListActiveWithSecrets(ctx, orgID)
	if err != nil {
		return nil, err
	}
	if len(configs) == 0 {
		return nil, nil
	}
	byID := make(map[uuid.UUID]model.LLMProviderConfig, len(configs))
	for _, c := range configs {
		byID[c.ID] = c
	}

	// Explicit chain when the org set one; else every active config, default
	// first (the store's order).
	var plan []model.LLMRouteStep
	if routing != nil && len(routing.Chain) > 0 {
		plan = routing.Chain
	} else {
		for _, c := range configs {
			plan = append(plan, model.LLMRouteStep{ProviderConfigID: c.ID})
		}
	}

	steps := make([]Step, 0, len(plan))
	for _, p := range plan {
		cfg, ok := byID[p.ProviderConfigID]
		if !ok {
			// Deleted or deactivated since the chain was saved: skip the hop.
			r.opts.Logger.Warn("orgroute: chain hop references a missing or inactive provider config",
				zap.String("org_id", orgID.String()), zap.String("provider_config_id", p.ProviderConfigID.String()))
			continue
		}
		client, err := r.client(cfg)
		if err != nil {
			r.opts.Logger.Warn("orgroute: cannot build provider client; skipping hop",
				zap.String("org_id", orgID.String()), zap.String("provider_config_id", cfg.ID.String()), zap.Error(err))
			continue
		}
		m := strings.TrimSpace(p.Model)
		if m == "" {
			m = cfg.DefaultModel
		}
		steps = append(steps, Step{ConfigID: cfg.ID, Name: cfg.Name, ProviderType: cfg.ProviderType, Model: m, Client: client})
	}
	if len(steps) == 0 {
		return nil, nil
	}
	return steps, nil
}

// client returns a (cached) decorated client for cfg, rebuilt when the config
// changed.
func (r *Resolver) client(cfg model.LLMProviderConfig) (llm.Client, error) {
	r.mu.Lock()
	c, ok := r.clients[cfg.ID]
	r.mu.Unlock()
	if ok && c.updatedAt.Equal(cfg.UpdatedAt) {
		return c.client, nil
	}
	built, err := r.opts.Build(cfg)
	if err != nil {
		return nil, err
	}
	if r.opts.Decorate != nil {
		built = r.opts.Decorate(built)
	}
	r.mu.Lock()
	r.clients[cfg.ID] = cachedClient{updatedAt: cfg.UpdatedAt, client: built}
	r.mu.Unlock()
	return built, nil
}

// Generate runs req down the chain: the first hop that succeeds answers; any
// failure moves to the next hop, except the caller giving up (context done).
// When every hop fails the last error is returned wrapped together with
// llm.ErrOrgChainExhausted (errors.As still finds its *llm.ProviderError; the
// env Ollama fallback skips it).
func (r *Resolver) Generate(ctx context.Context, orgID uuid.UUID, steps []Step, req llm.GenerateRequest) (*llm.GenerateResponse, error) {
	var lastErr error
	for i, s := range steps {
		hop := req
		hop.Model = s.Model
		resp, err := s.Client.Generate(ctx, hop)
		if err == nil {
			return resp, nil
		}
		lastErr = err
		if ctx.Err() != nil {
			return nil, err
		}
		if i < len(steps)-1 {
			r.opts.Logger.Warn("orgroute: provider failed, trying next in org chain",
				zap.String("org_id", orgID.String()),
				zap.String("provider", s.Name),
				zap.String("next", steps[i+1].Name),
				zap.Error(err))
		}
	}
	return nil, fmt.Errorf("%w (%d tried): %w", llm.ErrOrgChainExhausted, len(steps), lastErr)
}

// ─── Org on the context ─────────────────────────────────────────────────────

type orgCtxKey struct{}

// WithOrg pins the org whose chain routes LLM calls made under ctx.
func WithOrg(ctx context.Context, orgID uuid.UUID) context.Context {
	return context.WithValue(ctx, orgCtxKey{}, orgID)
}

// OrgFromContext finds the org of a call: WithOrg, else the executor's
// tools.WithOrg, else the Langfuse trace label (set on approval resumes and by
// most specialist runners).
func OrgFromContext(ctx context.Context) (uuid.UUID, bool) {
	if id, ok := ctx.Value(orgCtxKey{}).(uuid.UUID); ok && id != uuid.Nil {
		return id, true
	}
	if id, ok := tools.OrgFromContext(ctx); ok && id != uuid.Nil {
		return id, true
	}
	if info, ok := llmtrace.FromContext(ctx); ok && info.OrgID != "" {
		if id, err := uuid.Parse(info.OrgID); err == nil && id != uuid.Nil {
			return id, true
		}
	}
	return uuid.Nil, false
}

// ─── Router wrapper ─────────────────────────────────────────────────────────

// Wrap is an llm.Router.WrapClients decorator: calls under an org with an
// active chain go down the chain, every other call to inner. Install it LAST
// so it is the outermost wrapper (env tracing stays on the env path; org hops
// are decorated by Options.Decorate).
func (r *Resolver) Wrap(inner llm.Client) llm.Client {
	oc := &orgClient{inner: inner, res: r}
	if _, ok := inner.(llm.ModelLister); ok {
		return &orgListerClient{oc}
	}
	return oc
}

type orgClient struct {
	inner llm.Client
	res   *Resolver
}

func (c *orgClient) Generate(ctx context.Context, req llm.GenerateRequest) (*llm.GenerateResponse, error) {
	orgID, ok := OrgFromContext(ctx)
	if !ok {
		return c.inner.Generate(ctx, req)
	}
	chain, err := c.res.chain(ctx, orgID)
	if err != nil {
		// A lookup failure must not break the run; the org then behaves as
		// if it had no configs (env routing), which is today's behaviour.
		c.res.opts.Logger.Warn("orgroute: chain lookup failed; using env provider",
			zap.String("org_id", orgID.String()), zap.Error(err))
		return c.inner.Generate(ctx, req)
	}
	if len(chain.steps) == 0 {
		return c.inner.Generate(ctx, req)
	}
	if chain.agentFirst {
		// Fallback mode: the agent's own provider and model answer when they
		// can. The chain is what keeps the run going when they cannot — a
		// self-hosted model that is busy or unreachable, a key over quota.
		resp, err := c.inner.Generate(llm.WithFallbackAvailable(ctx), req)
		if err == nil || ctx.Err() != nil {
			return resp, err
		}
		c.res.opts.Logger.Warn("orgroute: agent's provider failed, falling back to the org chain",
			zap.String("org_id", orgID.String()),
			zap.String("provider", c.inner.ProviderName()),
			zap.String("model", req.Model),
			zap.String("next", chain.steps[0].Name),
			zap.Error(err))
	}
	return c.res.Generate(ctx, orgID, chain.steps, req)
}

func (c *orgClient) ProviderName() string { return c.inner.ProviderName() }

// SupportsTools forwards the env client's capability: it is asked before any
// call (without a context), so the org cannot be known. Org hops that cannot
// do native tools ignore ToolDefs, per the llm.Client contract.
func (c *orgClient) SupportsTools() bool {
	tc, ok := c.inner.(llm.ToolCapableClient)
	return ok && tc.SupportsTools()
}

type orgListerClient struct{ *orgClient }

func (c *orgListerClient) ListModels(ctx context.Context) ([]llm.ModelInfo, error) {
	return c.inner.(llm.ModelLister).ListModels(ctx)
}

// ─── Client construction ────────────────────────────────────────────────────

// defaultBaseURL is where an OpenAI-compatible provider lives when its config
// names no base_url.
var defaultBaseURL = map[string]string{
	"mistral": "https://api.mistral.ai",
	"grok":    "https://api.x.ai",
}

// ErrUnsupportedProvider is returned for a provider_type with no client.
var ErrUnsupportedProvider = errors.New("orgroute: unsupported provider_type")

// BuildClient makes an llm.Client for an org provider config. provider_type
// "openai" is any OpenAI-compatible endpoint (OpenAI, OpenRouter, vLLM, Groq,
// LM Studio, ...): base_url may be given with or without the trailing /v1.
func BuildClient(cfg model.LLMProviderConfig) (llm.Client, error) {
	base := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if base != "" && !strings.HasPrefix(base, "http://") && !strings.HasPrefix(base, "https://") {
		return nil, fmt.Errorf("orgroute: base_url must be http(s): %q", cfg.BaseURL)
	}
	switch strings.ToLower(strings.TrimSpace(cfg.ProviderType)) {
	case "openai":
		if base == "" {
			base = "https://api.openai.com"
		}
		base = strings.TrimSuffix(base, "/v1")
		if cfg.APIKey == "" {
			return nil, fmt.Errorf("orgroute: openai config %s has no api_key", cfg.ID)
		}
		return llm.NewOpenAIClient(base, cfg.APIKey, cfg.DefaultModel), nil
	case "mistral", "grok":
		// Both serve the OpenAI chat-completions API. The client is named for
		// the provider so usage and benchmarks say which one answered.
		pt := strings.ToLower(strings.TrimSpace(cfg.ProviderType))
		if base == "" {
			base = defaultBaseURL[pt]
		}
		if cfg.APIKey == "" {
			return nil, fmt.Errorf("orgroute: %s config %s has no api_key", pt, cfg.ID)
		}
		return llm.NewOpenAICompatibleClient(pt, strings.TrimSuffix(base, "/v1"), cfg.APIKey, cfg.DefaultModel), nil
	case "claude":
		if cfg.APIKey == "" {
			return nil, fmt.Errorf("orgroute: claude config %s has no api_key", cfg.ID)
		}
		return llm.NewClaudeClient(strings.TrimSuffix(base, "/v1"), cfg.APIKey, cfg.DefaultModel), nil
	case "gemini":
		if cfg.APIKey == "" {
			return nil, fmt.Errorf("orgroute: gemini config %s has no api_key", cfg.ID)
		}
		return llm.NewGeminiClient(base, cfg.APIKey, cfg.DefaultModel, 0), nil
	case "ollama":
		if base == "" {
			return nil, fmt.Errorf("orgroute: ollama config %s has no base_url", cfg.ID)
		}
		return llm.NewOllamaClient(base, cfg.DefaultModel), nil
	default:
		return nil, fmt.Errorf("%w %q", ErrUnsupportedProvider, cfg.ProviderType)
	}
}

// TestResult is the outcome of a provider test call.
type TestResult struct {
	OK        bool   `json:"ok"`
	Provider  string `json:"provider"`
	Model     string `json:"model"`
	LatencyMs int64  `json:"latency_ms"`
	Reply     string `json:"reply,omitempty"`
	Error     string `json:"error,omitempty"`
}

// Test sends one tiny prompt to a config (cheap: ~10 tokens each way) and
// reports whether it answered.
func Test(ctx context.Context, cfg model.LLMProviderConfig, modelOverride string) TestResult {
	m := strings.TrimSpace(modelOverride)
	if m == "" {
		m = cfg.DefaultModel
	}
	out := TestResult{Provider: cfg.ProviderType, Model: m}
	client, err := BuildClient(cfg)
	if err != nil {
		out.Error = err.Error()
		return out
	}
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	start := time.Now()
	resp, err := client.Generate(ctx, llm.GenerateRequest{
		Messages:  []llm.Message{{Role: llm.RoleUser, Content: "Reply with the single word OK."}},
		Model:     m,
		MaxTokens: 16,
	})
	out.LatencyMs = time.Since(start).Milliseconds()
	if err != nil {
		out.Error = err.Error()
		return out
	}
	out.OK = true
	out.Reply = strings.TrimSpace(resp.Content)
	if len(out.Reply) > 200 {
		out.Reply = out.Reply[:200]
	}
	if resp.Model != "" {
		out.Model = resp.Model
	}
	return out
}

// Stores adapts the two repositories to Store.
type Stores struct {
	Providers interface {
		ListActiveWithSecrets(ctx context.Context, orgID uuid.UUID) ([]model.LLMProviderConfig, error)
	}
	Routing interface {
		Get(ctx context.Context, orgID uuid.UUID) (*model.OrgLLMRouting, error)
	}
}

func (s Stores) ListActiveWithSecrets(ctx context.Context, orgID uuid.UUID) ([]model.LLMProviderConfig, error) {
	return s.Providers.ListActiveWithSecrets(ctx, orgID)
}

func (s Stores) Get(ctx context.Context, orgID uuid.UUID) (*model.OrgLLMRouting, error) {
	return s.Routing.Get(ctx, orgID)
}
