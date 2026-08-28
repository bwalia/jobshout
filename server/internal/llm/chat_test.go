package llm

import (
	"context"
	"errors"
	"strings"
	"testing"

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
