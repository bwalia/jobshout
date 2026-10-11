package orgroute

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/llm"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/tools"
)

type fakeStore struct {
	configs map[uuid.UUID][]model.LLMProviderConfig
	routing map[uuid.UUID]*model.OrgLLMRouting
	reads   int
}

func (f *fakeStore) ListActiveWithSecrets(_ context.Context, orgID uuid.UUID) ([]model.LLMProviderConfig, error) {
	f.reads++
	return f.configs[orgID], nil
}
func (f *fakeStore) Get(_ context.Context, orgID uuid.UUID) (*model.OrgLLMRouting, error) {
	return f.routing[orgID], nil
}

// stubClient answers with its own name, or fails with err.
type stubClient struct {
	name   string
	err    error
	models []string
}

func (s *stubClient) Generate(_ context.Context, req llm.GenerateRequest) (*llm.GenerateResponse, error) {
	s.models = append(s.models, req.Model)
	if s.err != nil {
		return nil, s.err
	}
	return &llm.GenerateResponse{Content: s.name, Model: req.Model}, nil
}
func (s *stubClient) ProviderName() string { return s.name }

func cfg(name string, isDefault bool) model.LLMProviderConfig {
	return model.LLMProviderConfig{ID: uuid.New(), Name: name, ProviderType: "openai",
		DefaultModel: name + "-model", IsDefault: isDefault, IsActive: true, UpdatedAt: time.Unix(1, 0)}
}

// harness wires a resolver whose Build returns a stub per config name.
func harness(store *fakeStore, stubs map[string]*stubClient) (*Resolver, *stubClient) {
	res := NewResolver(store, Options{Build: func(c model.LLMProviderConfig) (llm.Client, error) {
		return stubs[c.Name], nil
	}})
	env := &stubClient{name: "env"}
	return res, env
}

func gen(t *testing.T, c llm.Client, ctx context.Context) string {
	t.Helper()
	resp, err := c.Generate(ctx, llm.GenerateRequest{Model: "agent-model"})
	if err != nil {
		t.Fatal(err)
	}
	return resp.Content
}

func TestWrap_EnvWhenNoOrgOrNoConfigs(t *testing.T) {
	org := uuid.New()
	store := &fakeStore{configs: map[uuid.UUID][]model.LLMProviderConfig{}}
	res, env := harness(store, nil)
	c := res.Wrap(env)
	if got := gen(t, c, context.Background()); got != "env" {
		t.Fatalf("no org: %s", got)
	}
	if got := gen(t, c, tools.WithOrg(context.Background(), org)); got != "env" {
		t.Fatalf("org without configs: %s", got)
	}
	if env.models[1] != "agent-model" {
		t.Fatalf("env path must keep the agent's model, got %q", env.models[1])
	}
}

func TestWrap_ImplicitChainDefaultFirstAndModelFromConfig(t *testing.T) {
	org := uuid.New()
	a, b := cfg("a", true), cfg("b", false)
	stubs := map[string]*stubClient{"a": {name: "a"}, "b": {name: "b"}}
	store := &fakeStore{configs: map[uuid.UUID][]model.LLMProviderConfig{org: {a, b}}}
	res, env := harness(store, stubs)
	got := gen(t, res.Wrap(env), WithOrg(context.Background(), org))
	if got != "a" {
		t.Fatalf("answered by %s, want default config a", got)
	}
	if stubs["a"].models[0] != "a-model" {
		t.Fatalf("model = %q, want the config's default model", stubs["a"].models[0])
	}
	if len(env.models) != 0 {
		t.Fatal("env client called for an org with a chain")
	}
}

func TestWrap_ExplicitChainOrderFallbackAndModelOverride(t *testing.T) {
	org := uuid.New()
	a, b, c := cfg("a", true), cfg("b", false), cfg("c", false)
	rate := &llm.ProviderError{Provider: "openai", Kind: llm.ErrProviderRateLimited, Status: 429}
	stubs := map[string]*stubClient{"a": {name: "a"}, "b": {name: "b", err: rate}, "c": {name: "c"}}
	store := &fakeStore{
		configs: map[uuid.UUID][]model.LLMProviderConfig{org: {a, b, c}},
		routing: map[uuid.UUID]*model.OrgLLMRouting{org: {OrgID: org, Enabled: true, Chain: []model.LLMRouteStep{
			{ProviderConfigID: b.ID, Model: "b-big"},
			{ProviderConfigID: uuid.New()}, // deleted config: skipped
			{ProviderConfigID: c.ID, Model: "c-small"},
		}}},
	}
	res, env := harness(store, stubs)
	got := gen(t, res.Wrap(env), WithOrg(context.Background(), org))
	if got != "c" {
		t.Fatalf("answered by %s, want c (b rate-limited, a not in chain)", got)
	}
	if stubs["b"].models[0] != "b-big" || stubs["c"].models[0] != "c-small" {
		t.Fatalf("models = %v / %v", stubs["b"].models, stubs["c"].models)
	}
	if len(stubs["a"].models) != 0 {
		t.Fatal("config a is not in the explicit chain but was called")
	}
}

func TestWrap_AllFailWrapsLastError(t *testing.T) {
	org := uuid.New()
	a := cfg("a", true)
	pe := &llm.ProviderError{Provider: "openai", Kind: llm.ErrProviderUnavailable, Status: 503}
	store := &fakeStore{configs: map[uuid.UUID][]model.LLMProviderConfig{org: {a}}}
	res, env := harness(store, map[string]*stubClient{"a": {name: "a", err: pe}})
	_, err := res.Wrap(env).Generate(WithOrg(context.Background(), org), llm.GenerateRequest{})
	var got *llm.ProviderError
	if !errors.As(err, &got) || got.Kind != llm.ErrProviderUnavailable {
		t.Fatalf("err = %v, want wrapped ProviderError", err)
	}
	if len(env.models) != 0 {
		t.Fatal("org chain failure must not silently use the env client")
	}
}

func TestWrap_DisabledRoutingUsesEnv(t *testing.T) {
	org := uuid.New()
	store := &fakeStore{
		configs: map[uuid.UUID][]model.LLMProviderConfig{org: {cfg("a", true)}},
		routing: map[uuid.UUID]*model.OrgLLMRouting{org: {OrgID: org, Enabled: false}},
	}
	res, env := harness(store, map[string]*stubClient{"a": {name: "a"}})
	if got := gen(t, res.Wrap(env), WithOrg(context.Background(), org)); got != "env" {
		t.Fatalf("disabled routing answered by %s", got)
	}
}

func TestChain_CacheAndInvalidate(t *testing.T) {
	org := uuid.New()
	store := &fakeStore{configs: map[uuid.UUID][]model.LLMProviderConfig{org: {cfg("a", true)}}}
	res, _ := harness(store, map[string]*stubClient{"a": {name: "a"}})
	ctx := context.Background()
	_, _ = res.Chain(ctx, org)
	_, _ = res.Chain(ctx, org)
	if store.reads != 1 {
		t.Fatalf("reads = %d, want 1 (cached)", store.reads)
	}
	res.Invalidate(org)
	_, _ = res.Chain(ctx, org)
	if store.reads != 2 {
		t.Fatalf("reads = %d, want 2 after invalidate", store.reads)
	}
}

// An org's calls must never use another org's chain.
func TestWrap_OrgsAreIsolated(t *testing.T) {
	orgA, orgB := uuid.New(), uuid.New()
	store := &fakeStore{configs: map[uuid.UUID][]model.LLMProviderConfig{orgA: {cfg("a", true)}}}
	res, env := harness(store, map[string]*stubClient{"a": {name: "a"}})
	c := res.Wrap(env)
	if got := gen(t, c, WithOrg(context.Background(), orgB)); got != "env" {
		t.Fatalf("org B used %s", got)
	}
}

// BuildClient + Test against a fake OpenAI-compatible endpoint: base_url with
// a trailing /v1 works, the org's key is sent, and the configured model is used.
func TestBuildClientAndTest_OpenAICompatible(t *testing.T) {
	key := strings.Join([]string{"org", "key"}, "-")
	var gotAuth, gotModel, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth, gotPath = r.Header.Get("Authorization"), r.URL.Path
		var body struct {
			Model string `json:"model"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		gotModel = body.Model
		_, _ = w.Write([]byte(`{"model":"` + body.Model + `","choices":[{"message":{"role":"assistant","content":"OK"},"finish_reason":"stop"}]}`))
	}))
	defer srv.Close()

	c := model.LLMProviderConfig{ID: uuid.New(), ProviderType: "openai", BaseURL: srv.URL + "/v1/", APIKey: key, DefaultModel: "org-model"}
	res := Test(context.Background(), c, "")
	if !res.OK || res.Reply != "OK" {
		t.Fatalf("test = %+v", res)
	}
	if gotAuth != "Bearer "+key || gotModel != "org-model" || gotPath != "/v1/chat/completions" {
		t.Fatalf("auth=%q model=%q path=%q", gotAuth, gotModel, gotPath)
	}
	if _, err := BuildClient(model.LLMProviderConfig{ProviderType: "openai", BaseURL: "file:///etc", APIKey: "k"}); err == nil {
		t.Fatal("non-http base_url accepted")
	}
	if _, err := BuildClient(model.LLMProviderConfig{ProviderType: "nope"}); !errors.Is(err, ErrUnsupportedProvider) {
		t.Fatalf("unknown type err = %v", err)
	}
}

// Installed on the real router, the wrapper covers both execution entry
// points: ForWithFallback (executor) and Routed (specialist agents).
func TestRouterIntegration(t *testing.T) {
	org := uuid.New()
	store := &fakeStore{configs: map[uuid.UUID][]model.LLMProviderConfig{org: {cfg("a", true)}}}
	res, env := harness(store, map[string]*stubClient{"a": {name: "a"}})
	router := llm.NewTestRouter("ollama", map[string]llm.Client{"ollama": env})
	router.WrapClients(res.Wrap)

	exec, err := router.ForWithFallback("")
	if err != nil {
		t.Fatal(err)
	}
	if got := gen(t, exec, tools.WithOrg(context.Background(), org)); got != "a" {
		t.Fatalf("executor path answered by %s", got)
	}
	if got := gen(t, router.Routed(""), WithOrg(context.Background(), org)); got != "a" {
		t.Fatalf("routed path answered by %s", got)
	}
	if got := gen(t, exec, context.Background()); got != "env" {
		t.Fatalf("org-less call answered by %s", got)
	}
}

// The env Ollama fallback must not re-run (or bypass) an org's exhausted chain.
func TestRouterIntegration_NoEnvOllamaFallbackForOrgChain(t *testing.T) {
	org := uuid.New()
	rate := &llm.ProviderError{Provider: "openai", Kind: llm.ErrProviderRateLimited, Status: 429}
	a := &stubClient{name: "a", err: rate}
	store := &fakeStore{configs: map[uuid.UUID][]model.LLMProviderConfig{org: {cfg("a", true)}}}
	res := NewResolver(store, Options{Build: func(model.LLMProviderConfig) (llm.Client, error) { return a, nil }})
	ollama := &stubClient{name: "ollama"}
	cloud := &stubClient{name: "openai", err: rate}
	router := llm.NewTestRouter("openai", map[string]llm.Client{"ollama": ollama, "openai": cloud})
	router.WrapClients(res.Wrap)
	router.EnableOllamaFallback("", nil)

	c, err := router.ForWithFallback("openai")
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.Generate(WithOrg(context.Background(), org), llm.GenerateRequest{})
	if !errors.Is(err, llm.ErrOrgChainExhausted) {
		t.Fatalf("err = %v, want ErrOrgChainExhausted", err)
	}
	if len(a.models) != 1 || len(ollama.models) != 0 {
		t.Fatalf("org hop calls = %d (want 1), ollama calls = %d (want 0)", len(a.models), len(ollama.models))
	}
	// Without an org the env fallback still works as before.
	resp, err := c.Generate(context.Background(), llm.GenerateRequest{})
	if err != nil || resp.Content != "ollama" {
		t.Fatalf("env fallback = %v, %v", resp, err)
	}
}
