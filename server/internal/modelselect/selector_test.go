package modelselect

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jobshout/server/internal/costengine"
	"github.com/jobshout/server/internal/llm"
)

// toolClient implements llm.Client and llm.ToolCapableClient.
type toolClient struct {
	name  string
	tools bool
}

func (c toolClient) Generate(context.Context, llm.GenerateRequest) (*llm.GenerateResponse, error) {
	return &llm.GenerateResponse{}, nil
}
func (c toolClient) ProviderName() string { return c.name }
func (c toolClient) SupportsTools() bool  { return c.tools }

// plainClient implements llm.Client only — no tool-calling capability at all.
type plainClient struct{ name string }

func (c plainClient) Generate(context.Context, llm.GenerateRequest) (*llm.GenerateResponse, error) {
	return &llm.GenerateResponse{}, nil
}
func (c plainClient) ProviderName() string { return c.name }

// fakeRegistry lets each test declare exactly which providers exist.
type fakeRegistry struct{ clients map[string]llm.Client }

func (r fakeRegistry) RegisteredProviders() []llm.ProviderInfo {
	infos := make([]llm.ProviderInfo, 0, len(r.clients))
	for name := range r.clients {
		infos = append(infos, llm.ProviderInfo{Name: name})
	}
	return infos
}

func (r fakeRegistry) For(name string) (llm.Client, error) {
	c, ok := r.clients[name]
	if !ok {
		return nil, errors.New("not registered")
	}
	return c, nil
}

// allProviders registers every provider in the default catalog, with tool
// support matching the real clients (ollama cannot, the hosted ones can).
func allProviders() fakeRegistry {
	return fakeRegistry{clients: map[string]llm.Client{
		"ollama": toolClient{name: "ollama", tools: false},
		"openai": toolClient{name: "openai", tools: true},
		"claude": toolClient{name: "claude", tools: true},
	}}
}

func newSelector(reg Registry) *Selector {
	return New(reg, costengine.New(), nil)
}

func TestSelect_PrefersCheapestThatClearsTheBar(t *testing.T) {
	tests := []struct {
		name         string
		kind         string
		wantProvider string
		wantModel    string
	}{
		{
			name:         "classify takes the free local model",
			kind:         KindClassify,
			wantProvider: "ollama",
			wantModel:    "llama3:latest",
		},
		{
			name:         "step still clears with the local model",
			kind:         KindStep,
			wantProvider: "ollama",
			wantModel:    "llama3:latest",
		},
		{
			name:         "code needs quality 7, so the local model is out",
			kind:         KindCode,
			wantProvider: "openai",
			wantModel:    "gpt-4o",
		},
		{
			name:         "planning needs quality 8",
			kind:         KindPlan,
			wantProvider: "openai",
			wantModel:    "gpt-4o",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newSelector(allProviders())

			got, err := s.Select(TaskSignals{Kind: tt.kind, PromptTokens: 1000}, Constraints{})
			if err != nil {
				t.Fatalf("Select() error = %v", err)
			}
			if got.Provider != tt.wantProvider || got.Model != tt.wantModel {
				t.Errorf("Select() = %s/%s, want %s/%s", got.Provider, got.Model, tt.wantProvider, tt.wantModel)
			}
			if got.Reason == "" {
				t.Error("Reason is empty; the decision must be explainable")
			}
		})
	}
}

// Native tool-calling ranks a candidate; it does not disqualify one. A model
// without it still runs tool tasks through the executor's ReAct JSON-in-prompt
// loop, which is how every Ollama agent works today — so filtering on it would
// exclude every local model from tool work for no reason.
func TestSelect_ToolTasksPreferNativeToolCallingButStillAllowReAct(t *testing.T) {
	s := newSelector(allProviders())

	// ollama is free, so it wins on cost. It cannot do native tool-calling, but
	// it remains eligible.
	got, err := s.Select(TaskSignals{Kind: KindStep, PromptTokens: 500, NeedsTools: true}, Constraints{})
	if err != nil {
		t.Fatalf("Select() error = %v", err)
	}
	if got.Provider != "ollama" {
		t.Fatalf("Select() = %s/%s; the free local model should still win a tool task on cost",
			got.Provider, got.Model)
	}
}

// At equal cost, the tie-break must favour the model that can do it natively.
func TestSelect_ToolTasksBreakTiesTowardNativeToolCalling(t *testing.T) {
	// Two candidates, same provider and therefore the same (zero) price, but
	// only one advertises tools.
	catalog := []Candidate{
		{Provider: "ollama", Model: "no-tools", ContextTokens: 32_000, SupportsTools: false, Quality: 5, Speed: 7},
		{Provider: "ollama", Model: "has-tools", ContextTokens: 32_000, SupportsTools: true, Quality: 5, Speed: 7},
	}
	// The ollama client reports no native tool support, so neither candidate is
	// natively capable and the tie falls through to the later keys.
	s := New(allProviders(), costengine.New(), catalog)
	got, err := s.Select(TaskSignals{Kind: KindStep, NeedsTools: true}, Constraints{})
	if err != nil {
		t.Fatalf("Select() error = %v", err)
	}
	if got.Model != "has-tools" && got.Model != "no-tools" {
		t.Fatalf("unexpected model %q", got.Model)
	}

	// Now with a client that DOES implement the capability, the tools-capable
	// candidate must come first.
	reg := fakeRegistry{clients: map[string]llm.Client{
		"ollama": toolClient{name: "ollama"},
	}}
	got, err = New(reg, costengine.New(), catalog).
		Select(TaskSignals{Kind: KindStep, NeedsTools: true}, Constraints{})
	if err != nil {
		t.Fatalf("Select() error = %v", err)
	}
	if got.Model != "has-tools" {
		t.Errorf("Select() = %q, want has-tools to win the tie-break", got.Model)
	}
}

func TestSelect_UnregisteredProvidersAreNeverChosen(t *testing.T) {
	// Only ollama is configured — the API-key providers are absent, exactly as
	// llm.NewRouter leaves them when no key is set.
	reg := fakeRegistry{clients: map[string]llm.Client{
		"ollama": toolClient{name: "ollama", tools: false},
	}}
	s := newSelector(reg)

	got, err := s.Select(TaskSignals{Kind: KindClassify}, Constraints{})
	if err != nil {
		t.Fatalf("Select() error = %v", err)
	}
	if got.Provider != "ollama" {
		t.Errorf("Provider = %q, want ollama", got.Provider)
	}

	// And a task the local model is too weak for must fail rather than reach
	// for an unconfigured provider.
	if _, err := s.Select(TaskSignals{Kind: KindPlan}, Constraints{}); !errors.Is(err, ErrNoCandidate) {
		t.Errorf("planning err = %v, want ErrNoCandidate", err)
	}
}

func TestSelect_RespectsGovernanceAllowlists(t *testing.T) {
	tests := []struct {
		name    string
		cons    Constraints
		wantErr bool
		want    string // provider
	}{
		{
			name: "provider allowlist steers away from the cheapest",
			cons: Constraints{AllowedProviders: []string{"claude"}},
			want: "claude",
		},
		{
			name: "model allowlist pins one model",
			cons: Constraints{AllowedModels: []string{"claude-3-haiku-20240307"}},
			want: "claude",
		},
		{
			name:    "allowlist with nothing viable fails closed",
			cons:    Constraints{AllowedProviders: []string{"nonexistent"}},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newSelector(allProviders())

			got, err := s.Select(TaskSignals{Kind: KindClassify, PromptTokens: 100}, tt.cons)
			if tt.wantErr {
				if !errors.Is(err, ErrNoCandidate) {
					t.Fatalf("err = %v, want ErrNoCandidate", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Select() error = %v", err)
			}
			if got.Provider != tt.want {
				t.Errorf("Provider = %q, want %q", got.Provider, tt.want)
			}
		})
	}
}

func TestSelect_BudgetCapRejectsExpensiveModels(t *testing.T) {
	s := newSelector(allProviders())

	// Planning needs quality 8, which rules out the free local model, so every
	// survivor is metered. A cap below the cheapest of them leaves nothing.
	sig := TaskSignals{Kind: KindPlan, PromptTokens: 1000, MaxOutputTokens: 4096}

	if _, err := s.Select(sig, Constraints{MaxUSDPerCall: 0.0001}); !errors.Is(err, ErrNoCandidate) {
		t.Fatalf("err = %v, want ErrNoCandidate under a tiny cap", err)
	}

	// A generous cap admits the cheapest qualifying model again.
	got, err := s.Select(sig, Constraints{MaxUSDPerCall: 1.00})
	if err != nil {
		t.Fatalf("Select() error = %v", err)
	}
	if got.Model != "gpt-4o" {
		t.Errorf("Model = %q, want gpt-4o as the cheapest quality-8 option", got.Model)
	}
}

func TestSelect_ContextWindowMustHoldPromptPlusHeadroom(t *testing.T) {
	s := newSelector(allProviders())

	// 20k tokens overflows llama3:latest (8k) and gpt-3.5-turbo (16k).
	// Wide-context local models (llama3.1:8b, qwen3-coder:30b) are eligible.
	got, err := s.Select(TaskSignals{Kind: KindSummarize, PromptTokens: 20_000}, Constraints{})
	if err != nil {
		t.Fatalf("Select() error = %v", err)
	}
	if got.Model == "llama3:latest" || got.Model == "gpt-3.5-turbo" {
		t.Errorf("selected %s/%s, which cannot hold 20k tokens plus headroom", got.Provider, got.Model)
	}
	if got.Chosen.ContextTokens < 20_000+responseHeadroomTokens {
		t.Errorf("selected %s/%s context %d cannot hold 20k tokens plus headroom",
			got.Provider, got.Model, got.Chosen.ContextTokens)
	}

	// Nothing in the catalog holds a million tokens.
	if _, err := s.Select(TaskSignals{Kind: KindSummarize, PromptTokens: 1_000_000}, Constraints{}); !errors.Is(err, ErrNoCandidate) {
		t.Errorf("err = %v, want ErrNoCandidate for an impossible context", err)
	}
}

func TestSelect_PriorFailuresEscalateQuality(t *testing.T) {
	s := newSelector(allProviders())

	first, err := s.Select(TaskSignals{Kind: KindStep, PromptTokens: 100}, Constraints{})
	if err != nil {
		t.Fatalf("Select() error = %v", err)
	}
	if first.Provider != "ollama" {
		t.Fatalf("first attempt = %s, want the cheap local model", first.Provider)
	}

	// Four failures push the step bar from 4 to 8, well past the local model.
	escalated, err := s.Select(TaskSignals{Kind: KindStep, PromptTokens: 100, PriorFailures: 4}, Constraints{})
	if err != nil {
		t.Fatalf("Select() after failures error = %v", err)
	}
	if escalated.Provider == "ollama" {
		t.Error("escalation re-picked the model that just failed")
	}
	if escalated.Chosen.Quality < 8 {
		t.Errorf("escalated pick %s/%s has quality %d, want >= 8",
			escalated.Provider, escalated.Model, escalated.Chosen.Quality)
	}
	if !strings.Contains(escalated.Reason, "escalated") {
		t.Errorf("Reason should record the escalation, got: %q", escalated.Reason)
	}
}

func TestSelectLargerContext_EscalatesWindowNotJustPrice(t *testing.T) {
	s := newSelector(allProviders())

	got, err := s.SelectLargerContext(TaskSignals{Kind: KindSummarize}, Constraints{}, 100_000)
	if err != nil {
		t.Fatalf("SelectLargerContext() error = %v", err)
	}
	cand, ok := findCandidate(got.Provider, got.Model)
	if !ok {
		t.Fatalf("selected %s/%s is not in the catalog", got.Provider, got.Model)
	}
	if cand.ContextTokens < 100_000 {
		t.Errorf("context = %d, want >= 100000", cand.ContextTokens)
	}
}

func TestSelect_IsDeterministicAndOrdersFallbacks(t *testing.T) {
	s := newSelector(allProviders())
	sig := TaskSignals{Kind: KindPlan, PromptTokens: 1000}

	first, err := s.Select(sig, Constraints{})
	if err != nil {
		t.Fatalf("Select() error = %v", err)
	}
	for i := 0; i < 20; i++ {
		again, err := s.Select(sig, Constraints{})
		if err != nil {
			t.Fatalf("Select() error = %v", err)
		}
		if again.Provider != first.Provider || again.Model != first.Model {
			t.Fatalf("selection is not deterministic: %s/%s then %s/%s",
				first.Provider, first.Model, again.Provider, again.Model)
		}
	}

	if len(first.Fallbacks) == 0 {
		t.Fatal("expected fallbacks for a task with several viable models")
	}
	for _, f := range first.Fallbacks {
		if f.Provider == first.Provider && f.Model == first.Model {
			t.Errorf("the chosen model %s/%s also appears in its own fallback chain", f.Provider, f.Model)
		}
	}
}

func TestSelect_EmptyKindFallsBackToStepRequirements(t *testing.T) {
	s := newSelector(allProviders())

	got, err := s.Select(TaskSignals{PromptTokens: 100}, Constraints{})
	if err != nil {
		t.Fatalf("Select() error = %v", err)
	}
	if got.Provider != "ollama" {
		t.Errorf("Provider = %q, want ollama (unknown kind should behave like a step)", got.Provider)
	}
}

func TestIsAuto(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"auto", true},
		{"AUTO", true},
		{"  auto  ", true},
		{"", false},
		{"openai", false},
		{"automatic", false},
	}
	for _, tt := range tests {
		if got := IsAuto(tt.in); got != tt.want {
			t.Errorf("IsAuto(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestSelect_WorksWithTheRealRouter(t *testing.T) {
	// Guards the Registry interface against drift in llm.Router's signatures.
	var reg Registry = llm.NewTestRouter("ollama", map[string]llm.Client{
		"ollama": toolClient{name: "ollama", tools: false},
	})
	s := newSelector(reg)

	got, err := s.Select(TaskSignals{Kind: KindClassify}, Constraints{})
	if err != nil {
		t.Fatalf("Select() error = %v", err)
	}
	if got.Provider != "ollama" {
		t.Errorf("Provider = %q, want ollama", got.Provider)
	}
}

func findCandidate(provider, model string) (Candidate, bool) {
	for _, c := range DefaultCatalog() {
		if c.Provider == provider && c.Model == model {
			return c, true
		}
	}
	return Candidate{}, false
}
