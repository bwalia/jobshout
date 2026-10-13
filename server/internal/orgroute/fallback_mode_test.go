package orgroute

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/llm"
	"github.com/jobshout/server/internal/model"
)

func fallbackStore(org uuid.UUID, configs ...model.LLMProviderConfig) *fakeStore {
	chain := make([]model.LLMRouteStep, 0, len(configs))
	for _, c := range configs {
		chain = append(chain, model.LLMRouteStep{ProviderConfigID: c.ID})
	}
	return &fakeStore{
		configs: map[uuid.UUID][]model.LLMProviderConfig{org: configs},
		routing: map[uuid.UUID]*model.OrgLLMRouting{org: {
			OrgID: org, Enabled: true, Mode: model.RoutingModeFallback, Chain: chain,
		}},
	}
}

// Fallback mode: the agent's own provider and model answer when they can.
func TestFallbackMode_AgentModelFirst(t *testing.T) {
	org := uuid.New()
	a := cfg("a", true)
	stubs := map[string]*stubClient{"a": {name: "a"}}
	res, env := harness(fallbackStore(org, a), stubs)
	if got := gen(t, res.Wrap(env), WithOrg(context.Background(), org)); got != "env" {
		t.Fatalf("answered by %s, want the agent's own provider", got)
	}
	if env.models[0] != "agent-model" {
		t.Fatalf("agent's model = %v, want agent-model unchanged", env.models)
	}
	if len(stubs["a"].models) != 0 {
		t.Fatal("chain was called although the agent's provider answered")
	}
}

// …and when they cannot — Ollama busy or unreachable — the chain takes over
// in order, with each hop's own model.
func TestFallbackMode_ChainWhenAgentProviderFails(t *testing.T) {
	org := uuid.New()
	a, b := cfg("a", true), cfg("b", false)
	stubs := map[string]*stubClient{
		"a": {name: "a", err: errors.New("401 bad key")},
		"b": {name: "b"},
	}
	res, _ := harness(fallbackStore(org, a, b), stubs)
	busy := &stubClient{name: "ollama", err: errors.New(`ollama: unexpected status 503: {"error":"server busy"}`)}
	if got := gen(t, res.Wrap(busy), WithOrg(context.Background(), org)); got != "b" {
		t.Fatalf("answered by %s, want b after ollama and a failed", got)
	}
	if stubs["b"].models[0] != "b-model" {
		t.Fatalf("hop model = %v, want the config's default", stubs["b"].models)
	}
}

// A cancelled run stops; it does not walk the chain.
func TestFallbackMode_CancelledDoesNotFallBack(t *testing.T) {
	org := uuid.New()
	a := cfg("a", true)
	stubs := map[string]*stubClient{"a": {name: "a"}}
	res, _ := harness(fallbackStore(org, a), stubs)
	ctx, cancel := context.WithCancel(WithOrg(context.Background(), org))
	cancel()
	failing := &stubClient{name: "ollama", err: context.Canceled}
	if _, err := res.Wrap(failing).Generate(ctx, llm.GenerateRequest{}); err == nil {
		t.Fatal("want the cancellation error")
	}
	if len(stubs["a"].models) != 0 {
		t.Fatal("chain was called for a cancelled run")
	}
}

// Mistral and Grok are OpenAI-compatible, default to their own API, and say
// which provider they are.
func TestBuildClient_MistralAndGrok(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = w.Write([]byte(`{"model":"m","choices":[{"message":{"role":"assistant","content":"OK"},"finish_reason":"stop"}]}`))
	}))
	defer srv.Close()
	key := strings.Join([]string{"test", "key"}, "-")

	for _, pt := range []string{"mistral", "grok"} {
		c, err := BuildClient(model.LLMProviderConfig{ID: uuid.New(), ProviderType: pt, BaseURL: srv.URL + "/v1", APIKey: key, DefaultModel: "m"})
		if err != nil {
			t.Fatalf("%s: %v", pt, err)
		}
		if c.ProviderName() != pt {
			t.Errorf("%s: provider name = %q", pt, c.ProviderName())
		}
		if _, err := c.Generate(context.Background(), llm.GenerateRequest{Messages: []llm.Message{{Role: llm.RoleUser, Content: "hi"}}}); err != nil {
			t.Fatalf("%s generate: %v", pt, err)
		}
		if gotPath != "/v1/chat/completions" {
			t.Errorf("%s path = %q", pt, gotPath)
		}
		noBase, err := BuildClient(model.LLMProviderConfig{ID: uuid.New(), ProviderType: pt, APIKey: key, DefaultModel: "m"})
		if err != nil {
			t.Fatalf("%s default base: %v", pt, err)
		}
		if got := noBase.(*llm.OpenAIClient).BaseURL; got != defaultBaseURL[pt] {
			t.Errorf("%s default base = %q", pt, got)
		}
	}
	if _, err := BuildClient(model.LLMProviderConfig{ID: uuid.New(), ProviderType: "grok", DefaultModel: "m"}); err == nil {
		t.Error("grok without an api_key must be refused")
	}
}
