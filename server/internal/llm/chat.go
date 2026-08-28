package llm

import (
	"context"
	"strings"

	"go.uber.org/zap"

	"github.com/jobshout/server/internal/config"
)

// ChatFallbackClient pins GenerateRequest.Model to the chat primary and
// retries once on the Ollama fallback when the primary fails or returns empty.
type ChatFallbackClient struct {
	inner    Client
	primary  string
	fallback string
	logger   *zap.Logger
}

// NewChatInner builds the unwrapped chat generate client. For Ollama this is a
// dedicated client whose DefaultModel is CHAT_MODEL (not the worker default).
// Hosted providers reuse the router default; ChatFallbackClient still sets Model.
func NewChatInner(cfg *config.Config, router *Router) Client {
	primary := SanitizeChatModel(cfg.ChatModel)
	numCtx := cfg.ChatNumCtx
	if numCtx <= 0 {
		numCtx = config.DefaultChatNumCtx
	}
	switch strings.ToLower(strings.TrimSpace(cfg.LLMProvider)) {
	case "openai", "claude":
		if router != nil {
			return router.Default()
		}
	}
	return NewOllamaClientWithAuth(cfg.OllamaBaseURL, primary, cfg.OllamaJWTSecret, cfg.OllamaTimeout).
		WithNumCtx(numCtx)
}

// NewChatClient wraps inner with primary/fallback model routing.
func NewChatClient(inner Client, primary, fallback string, logger *zap.Logger) *ChatFallbackClient {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &ChatFallbackClient{
		inner:    inner,
		primary:  SanitizeChatModel(primary),
		fallback: SanitizeChatFallback(fallback),
		logger:   logger,
	}
}

func (c *ChatFallbackClient) ProviderName() string {
	if c.inner == nil {
		return "chat"
	}
	return c.inner.ProviderName()
}

func (c *ChatFallbackClient) SupportsTools() bool {
	tc, ok := c.inner.(ToolCapableClient)
	return ok && tc.SupportsTools()
}

func (c *ChatFallbackClient) Generate(ctx context.Context, req GenerateRequest) (*GenerateResponse, error) {
	req.Model = c.primary
	resp, err := c.inner.Generate(ctx, req)
	if !shouldFallback(resp, err) {
		return resp, err
	}
	if c.inner == nil || c.inner.ProviderName() != "ollama" {
		return resp, err
	}
	if c.fallback == "" || c.fallback == c.primary || forbiddenChatModel(c.fallback) {
		return resp, err
	}
	c.logger.Info("chatagent: falling back to " + c.fallback)
	req.Model = c.fallback
	return c.inner.Generate(ctx, req)
}

func shouldFallback(resp *GenerateResponse, err error) bool {
	if err != nil {
		return true
	}
	if resp == nil {
		return true
	}
	if len(resp.ToolCalls) > 0 {
		return false
	}
	return strings.TrimSpace(resp.Content) == ""
}

// SanitizeChatModel replaces empty or forbidden chat model names with the
// tool-capable default. llama3:latest is never a chat primary.
func SanitizeChatModel(name string) string {
	name = strings.TrimSpace(name)
	if name == "" || forbiddenChatModel(name) {
		return config.DefaultChatModel
	}
	return name
}

// SanitizeChatFallback replaces empty or forbidden fallback names.
func SanitizeChatFallback(name string) string {
	name = strings.TrimSpace(name)
	if name == "" || forbiddenChatModel(name) {
		return config.DefaultChatModelFallback
	}
	return name
}

func forbiddenChatModel(name string) bool {
	n := strings.ToLower(strings.TrimSpace(name))
	base, _, _ := strings.Cut(n, ":")
	switch base {
	case "llama3", "minicpm-v", "muse-glimmer":
		return true
	}
	return false
}
