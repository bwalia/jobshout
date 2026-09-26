package executor

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/llm"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/tools"
)

// ─── max_tokens_per_exec ────────────────────────────────────────────────────

// capTokens is the whole of max_tokens_per_exec enforcement: if it returns the
// call site default when a policy is set, the cap is silently a no-op again.
func TestCapTokens(t *testing.T) {
	tests := []struct {
		name   string
		policy int
		def    int
		want   int
	}{
		{"no options on context uses the default", 0, 4096, 4096},
		{"policy tighter than default wins", 1000, 4096, 1000},
		{"policy looser than default is ignored", 8000, 4096, 4096},
		{"policy equal to default changes nothing", 4096, 4096, 4096},
		{"negative policy is treated as unset", -1, 4096, 4096},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			if tt.policy != 0 {
				ctx = WithRunOptions(ctx, RunOptions{MaxTokens: tt.policy})
			}
			if got := capTokens(ctx, tt.def); got != tt.want {
				t.Errorf("capTokens(%d) = %d, want %d", tt.def, got, tt.want)
			}
		})
	}
}

// The cap has to survive alongside the option RunOptions already carried, or
// wiring one in would quietly drop the other.
func TestCapTokens_CoexistsWithSkillSlugs(t *testing.T) {
	ctx := WithRunOptions(context.Background(), RunOptions{
		SkillSlugs: []string{"research"},
		MaxTokens:  512,
	})

	if got := capTokens(ctx, 4096); got != 512 {
		t.Errorf("capTokens = %d, want 512", got)
	}
	if slugs := runOptionsFrom(ctx).SkillSlugs; len(slugs) != 1 || slugs[0] != "research" {
		t.Errorf("SkillSlugs = %v, want [research]", slugs)
	}
}

// ─── max_cost_per_exec ──────────────────────────────────────────────────────

// pricePerOutputToken is a stand-in for the real cost engine: cost rises only
// with output tokens, which keeps the arithmetic in these tests obvious.
type pricePerOutputToken float64

func (p pricePerOutputToken) Calculate(_, _ string, _, outputTokens, _ int) float64 {
	return float64(outputTokens) * float64(p)
}

func TestCostCapExceeded(t *testing.T) {
	tests := []struct {
		name      string
		limit     float64
		estimator CostEstimator
		outTokens int
		wantStop  bool
	}{
		{"no cap set", 0, pricePerOutputToken(0.01), 10_000, false},
		{"under the cap", 1.00, pricePerOutputToken(0.01), 50, false},
		{"exactly at the cap stops", 0.50, pricePerOutputToken(0.01), 50, true},
		{"over the cap stops", 0.50, pricePerOutputToken(0.01), 500, true},
		{"no estimator wired means no enforcement", 0.01, nil, 10_000, false},
		// Self-hosted models price at zero in the cost engine, so a cap on them
		// must never fire — otherwise setting one would break free inference.
		{"zero-priced model never trips", 0.01, pricePerOutputToken(0), 10_000, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := &Executor{cost: tt.estimator}
			ctx := WithRunOptions(context.Background(), RunOptions{MaxCostUSD: tt.limit})

			err := e.costCapExceeded(ctx, "openai", "gpt-4o", 0, tt.outTokens, time.Now())

			if tt.wantStop && err == nil {
				t.Error("expected the run to be stopped, got nil")
			}
			if !tt.wantStop && err != nil {
				t.Errorf("expected the run to continue, got %v", err)
			}
		})
	}
}

// ─── the cap actually ends a run ────────────────────────────────────────────

// loopingClient never produces a final answer, so without a cap the run only
// ends when it exhausts MaxIterations. That makes an early stop unambiguous.
type loopingClient struct{ calls int }

func (c *loopingClient) ProviderName() string { return "fake" }
func (c *loopingClient) SupportsTools() bool  { return true }

func (c *loopingClient) Generate(_ context.Context, _ llm.GenerateRequest) (*llm.GenerateResponse, error) {
	c.calls++
	return &llm.GenerateResponse{
		ToolCalls: []llm.ToolCall{{
			ID:        "call",
			Name:      "echo",
			Arguments: map[string]any{"text": "again"},
		}},
		InputTokens:  0,
		OutputTokens: 10,
	}, nil
}

func costCapAgent() *model.Agent {
	return &model.Agent{
		ID:            uuid.New(),
		Name:          "Spender",
		Role:          "assistant",
		ModelProvider: strPtr("fake"),
		ModelName:     strPtr("fake-model"),
	}
}

func newCostCapExecutor(client llm.Client, estimator CostEstimator) *Executor {
	router := llm.NewTestRouter("fake", map[string]llm.Client{"fake": client})
	registry := tools.NewRegistry()
	registry.Register(&echoTool{})
	return New(router, registry, zap.NewNop()).WithCostEstimator(estimator)
}

// Each turn costs $0.10 (10 output tokens at $0.01). With a $0.25 cap the run
// must stop before the turn that would take it past the line: turns 1-3 bring
// spend to $0.30, and the check before turn 4 refuses to spend more.
func TestExecutor_StopsRunOnCostCap(t *testing.T) {
	client := &loopingClient{}
	exec := newCostCapExecutor(client, pricePerOutputToken(0.01))

	ctx := WithRunOptions(context.Background(), RunOptions{MaxCostUSD: 0.25})
	res := exec.Run(ctx, uuid.New(), costCapAgent(), "spend it all", []string{"echo"})

	if res.Err == nil {
		t.Fatal("expected the run to fail on the cost cap")
	}
	if !strings.Contains(res.Err.Error(), "cost cap") {
		t.Errorf("error = %v, want it to name the cost cap", res.Err)
	}
	if client.calls != 3 {
		t.Errorf("model turns = %d, want 3 (stopped before the 4th)", client.calls)
	}
	if client.calls >= MaxIterations {
		t.Errorf("run reached MaxIterations (%d) — the cap did not stop it", MaxIterations)
	}
}

// The control: the same client with no cap runs until iterations are exhausted.
// Without this, the test above could pass for the wrong reason.
func TestExecutor_NoCostCapRunsToIterationLimit(t *testing.T) {
	client := &loopingClient{}
	exec := newCostCapExecutor(client, pricePerOutputToken(0.01))

	res := exec.Run(context.Background(), uuid.New(), costCapAgent(), "spend it all", []string{"echo"})

	if res.Err == nil {
		t.Fatal("expected the run to end on the iteration limit")
	}
	if strings.Contains(res.Err.Error(), "cost cap") {
		t.Errorf("run stopped on a cost cap that was never set: %v", res.Err)
	}
	if client.calls != MaxIterations {
		t.Errorf("model turns = %d, want %d", client.calls, MaxIterations)
	}
}
