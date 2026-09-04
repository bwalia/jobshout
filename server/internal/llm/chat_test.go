package llm

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jobshout/server/internal/config"
)

type recordingClient struct {
	defaultModel string
	tools        bool
	reqs         []GenerateRequest
	failFirst    int
	emptyFirst   int
	provider     string
}

func (c *recordingClient) Generate(_ context.Context, req GenerateRequest) (*GenerateResponse, error) {
	c.reqs = append(c.reqs, req)
	n := len(c.reqs)
	if c.failFirst > 0 && n <= c.failFirst {
		return nil, errors.New("primary down")
	}
	if c.emptyFirst > 0 && n <= c.emptyFirst {
		return &GenerateResponse{Content: "", Model: req.Model}, nil
	}
	return &GenerateResponse{Content: "ok", Model: req.Model, FinishReason: "stop"}, nil
}
func (c *recordingClient) ProviderName() string { return c.provider }
func (c *recordingClient) SupportsTools() bool  { return c.tools }

func TestSanitizeChatModel_Forbidden(t *testing.T) {
	for _, name := range []string{"llama3", "llama3:latest", "minicpm-v", "muse-glimmer", "muse-glimmer:latest", ""} {
		if got := SanitizeChatModel(name); got != config.DefaultChatModel {
			t.Errorf("SanitizeChatModel(%q) = %q; want %s", name, got, config.DefaultChatModel)
		}
	}
	if got := SanitizeChatModel("qwen3-coder:30b"); got != "qwen3-coder:30b" {
		t.Errorf("coder should pass through, got %q", got)
	}
	if got := SanitizeChatFallback("llama3:latest"); got != config.DefaultChatModelFallback {
		t.Errorf("fallback forbidden = %q", got)
	}
	if SanitizeChatModel("llama3.1:8b") != "llama3.1:8b" {
		t.Fatal("llama3.1:8b must not be treated as llama3")
	}
}

func TestChatFallbackClient_SetsModelAndRetries(t *testing.T) {
	inner := &recordingClient{provider: "ollama", tools: true, failFirst: 1}
	c := NewChatClient(inner, "qwen3-coder:30b", "llama3.1:8b", nil)
	if !c.SupportsTools() {
		t.Fatal("SupportsTools should follow the chat inner client")
	}
	resp, err := c.Generate(context.Background(), GenerateRequest{Messages: []Message{{Role: RoleUser, Content: "hi"}}})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Model != "llama3.1:8b" {
		t.Fatalf("model = %q; want fallback", resp.Model)
	}
	if len(inner.reqs) != 2 {
		t.Fatalf("reqs = %d; want 2", len(inner.reqs))
	}
	if inner.reqs[0].Model != "qwen3-coder:30b" {
		t.Fatalf("primary model = %q", inner.reqs[0].Model)
	}
	if inner.reqs[1].Model != "llama3.1:8b" {
		t.Fatalf("fallback model = %q", inner.reqs[1].Model)
	}
}

func TestChatFallbackClient_EmptyRetries(t *testing.T) {
	inner := &recordingClient{provider: "ollama", tools: true, emptyFirst: 1}
	c := NewChatClient(inner, "qwen3-coder:30b", "llama3.1:8b", nil)
	resp, err := c.Generate(context.Background(), GenerateRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Model != "llama3.1:8b" {
		t.Fatalf("model = %q", resp.Model)
	}
}

func TestChatFallbackClient_NeverSendsForbidden(t *testing.T) {
	inner := &recordingClient{provider: "ollama", tools: true}
	c := NewChatClient(inner, "llama3:latest", "llama3", nil)
	if c.primary != config.DefaultChatModel {
		t.Fatalf("primary = %q", c.primary)
	}
	if c.fallback != config.DefaultChatModelFallback {
		t.Fatalf("fallback = %q", c.fallback)
	}
	_, _ = c.Generate(context.Background(), GenerateRequest{})
	if strings.Contains(inner.reqs[0].Model, "llama3:latest") || inner.reqs[0].Model == "llama3" {
		t.Fatalf("forbidden model sent: %q", inner.reqs[0].Model)
	}
}

func TestChatFallbackClient_NoCloudFallback(t *testing.T) {
	inner := &recordingClient{provider: "openai", tools: true, failFirst: 1}
	c := NewChatClient(inner, "gpt-4o-mini", "llama3.1:8b", nil)
	_, err := c.Generate(context.Background(), GenerateRequest{})
	if err == nil {
		t.Fatal("openai should not retry onto an ollama fallback")
	}
	if len(inner.reqs) != 1 {
		t.Fatalf("reqs = %d; want 1", len(inner.reqs))
	}
}

func TestNewChatInner_OllamaUsesChatNumCtx(t *testing.T) {
	cfg := &config.Config{
		LLMProvider:        "ollama",
		OllamaBaseURL:      "http://127.0.0.1:1",
		OllamaDefaultModel: "llama3:latest",
		ChatModel:          "qwen3-coder:30b",
		ChatNumCtx:         16384,
	}
	inner := NewChatInner(cfg, nil)
	oc, ok := inner.(*OllamaClient)
	if !ok {
		t.Fatalf("type %T", inner)
	}
	if oc.DefaultModel != "qwen3-coder:30b" {
		t.Fatalf("DefaultModel = %q", oc.DefaultModel)
	}
	if oc.NumCtx != 16384 {
		t.Fatalf("NumCtx = %d", oc.NumCtx)
	}
	if oc.effectiveNumCtx("qwen3-coder:30b") != 16384 && oc.NumCtx != 16384 {
		t.Fatal("chat num_ctx should be 16384 before model clamp")
	}
}

// budgetClient records how much context budget each call was given, so the
// primary/fallback split can be asserted without actually waiting.
type budgetClient struct {
	provider  string
	budgets   []time.Duration
	hasDeadln []bool
	failFirst int
}

func (c *budgetClient) Generate(ctx context.Context, req GenerateRequest) (*GenerateResponse, error) {
	if d, ok := ctx.Deadline(); ok {
		c.budgets = append(c.budgets, time.Until(d))
		c.hasDeadln = append(c.hasDeadln, true)
	} else {
		c.budgets = append(c.budgets, 0)
		c.hasDeadln = append(c.hasDeadln, false)
	}
	if c.failFirst > 0 && len(c.budgets) <= c.failFirst {
		return nil, errors.New("primary hung")
	}
	return &GenerateResponse{Content: "ok", Model: req.Model, FinishReason: "stop"}, nil
}
func (c *budgetClient) ProviderName() string { return c.provider }

func TestChatFallbackClient_ReservesBudgetForFallback(t *testing.T) {
	inner := &budgetClient{provider: "ollama", failFirst: 1}
	c := NewChatClient(inner, "qwen3-coder:30b", "llama3.1:8b", nil)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	if _, err := c.Generate(ctx, GenerateRequest{}); err != nil {
		t.Fatal(err)
	}
	if len(inner.budgets) != 2 {
		t.Fatalf("calls = %d; want 2", len(inner.budgets))
	}
	// The primary must stop short so the fallback is not handed a dead context.
	want := 10*time.Minute - chatFallbackReserve
	if inner.budgets[0] > want || inner.budgets[0] < want-time.Second {
		t.Fatalf("primary budget = %s; want ~%s", inner.budgets[0], want)
	}
	if inner.budgets[1] < chatFallbackReserve {
		t.Fatalf("fallback budget = %s; want >= %s", inner.budgets[1], chatFallbackReserve)
	}
}

func TestChatFallbackClient_KeepsShortBudgetWhole(t *testing.T) {
	inner := &budgetClient{provider: "ollama"}
	c := NewChatClient(inner, "qwen3-coder:30b", "llama3.1:8b", nil)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err := c.Generate(ctx, GenerateRequest{}); err != nil {
		t.Fatal(err)
	}
	if inner.budgets[0] < 29*time.Second {
		t.Fatalf("short budget was trimmed: %s", inner.budgets[0])
	}
}

func TestChatFallbackClient_NoFallbackKeepsFullBudget(t *testing.T) {
	// Hosted providers never fall back, so nothing should be held in reserve.
	inner := &budgetClient{provider: "openai"}
	c := NewChatClient(inner, "gpt-4o", "llama3.1:8b", nil)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	if _, err := c.Generate(ctx, GenerateRequest{}); err != nil {
		t.Fatal(err)
	}
	if inner.budgets[0] < 10*time.Minute-time.Second {
		t.Fatalf("budget = %s; want the full 10m", inner.budgets[0])
	}
}

func TestChatFallbackClient_NoDeadlinePassesThrough(t *testing.T) {
	inner := &budgetClient{provider: "ollama", failFirst: 1}
	c := NewChatClient(inner, "qwen3-coder:30b", "llama3.1:8b", nil)
	if _, err := c.Generate(context.Background(), GenerateRequest{}); err != nil {
		t.Fatal(err)
	}
	for i, has := range inner.hasDeadln {
		if has {
			t.Fatalf("call %d gained a deadline the caller never set", i)
		}
	}
}
