// Package llm provides a provider-agnostic interface for calling large language
// models. Concrete implementations live alongside this file (ollama.go,
// openai.go). The Router type selects the right client at runtime based on
// configuration or per-agent overrides.
package llm

import (
	"context"
	"errors"
)

// Role constants for chat messages.
const (
	RoleSystem    = "system"
	RoleUser      = "user"
	RoleAssistant = "assistant"
	// RoleTool marks a message carrying the result of a native tool call. Each
	// provider translates it into its own wire shape (OpenAI: a role:"tool"
	// message; Claude: a user message with a tool_result block).
	RoleTool = "tool"
)

// Message is a single turn in a chat conversation.
//
// The trailing fields support the native tool-calling path and are ignored on
// the ReAct path (they marshal only within each provider's own request shape,
// never via these json tags):
//   - ToolCalls is set on an assistant message that requested tool calls, so a
//     follow-up request echoes the assistant turn in the provider's format.
//   - ToolCallID + Content together form a tool-result message (RoleTool)
//     replying to the ToolCall with that ID.
type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"-"`
	ToolCallID string     `json:"-"`
}

// ToolDef is a native function/tool definition passed to providers that support
// tool-calling. Parameters is a JSON-Schema object.
type ToolDef struct {
	Name        string
	Description string
	Parameters  map[string]any
}

// ToolCall is a tool invocation the model requested via native tool-calling.
type ToolCall struct {
	ID        string
	Name      string
	Arguments map[string]any
}

// GenerateRequest is the input to a Generate call.
type GenerateRequest struct {
	// Messages is the ordered conversation history including system prompt.
	Messages []Message
	// Model overrides the client's configured default model.
	Model string
	// MaxTokens caps the response length (0 means use the client default).
	MaxTokens int
	// Temperature controls randomness (0.0–1.0; 0 means use client default).
	Temperature float64
	// ToolDefs, when non-empty and supported by the client, are sent as native
	// function definitions so the model can request tool calls directly. Empty
	// (the default) preserves the previous behavior; clients that don't support
	// tool-calling ignore this field.
	ToolDefs []ToolDef
	// Think asks a reasoning model to run its thinking phase before answering.
	// It is honoured only when the resolved model advertises the thinking
	// capability — on every other model (and every non-Ollama provider today)
	// it is a no-op. Callers that set it should budget MaxTokens generously:
	// thinking tokens count against the same limit as the answer.
	Think bool
	// OnToken, when set, receives each content chunk as it arrives from a
	// streaming provider, before the full Content is returned on the response.
	// It is a process-local callback, never serialised; clients that do not
	// stream simply ignore it.
	OnToken func(string) `json:"-"`
}

// GenerateResponse holds the model's reply and usage metadata.
type GenerateResponse struct {
	// Content is the raw text returned by the model.
	Content string
	// FinishReason indicates why generation stopped ("stop", "length", etc.).
	FinishReason string
	// Model is the effective model that served the call — the client's default
	// when GenerateRequest.Model was empty. Telemetry reads it so the model a
	// call is attributed to is the one that actually ran.
	Model string
	// InputTokens is the number of tokens in the prompt (if reported).
	InputTokens int
	// OutputTokens is the number of tokens in the completion (if reported).
	OutputTokens int
	// ToolCalls holds any native tool invocations the model requested. Empty
	// unless ToolDefs were sent and the provider returned tool calls.
	ToolCalls []ToolCall
}

// ErrOnlyThinking reports a reply whose entire token budget went to a reasoning
// model's thinking phase, leaving no answer. Callers that requested thinking
// can errors.Is on it and retry without.
var ErrOnlyThinking = errors.New("returned only reasoning and no content")

// Client is the interface every LLM provider must satisfy.
type Client interface {
	// Generate sends a chat request and returns the model's reply.
	Generate(ctx context.Context, req GenerateRequest) (*GenerateResponse, error)
	// ProviderName returns a human-readable name for logging/metrics.
	ProviderName() string
}

// ToolCapableClient is the OPTIONAL capability interface a Client may implement
// to signal support for native tool-calling. It is kept separate from Client so
// existing implementations (and test stubs) satisfy Client unchanged; callers
// type-assert to discover the capability. A client that reports true accepts
// GenerateRequest.ToolDefs and populates GenerateResponse.ToolCalls.
type ToolCapableClient interface {
	SupportsTools() bool
}
