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
	// NumCtx asks for a context window for this one call, in tokens. Zero
	// keeps the client's own. Only Ollama reads it; the window there is a
	// per-request setting, and a call that needs a long prompt and a long
	// reply together should not have to raise it for every other caller.
	NumCtx int
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
	// JSON asks for a reply constrained to a JSON value. On Ollama it sends
	// format:"json" and forces thinking off, so a reasoning model cannot write
	// its monologue into the content ahead of the JSON — the failure that made
	// every structured stage of a qwen3 run parse-fail and retry. Other
	// providers ignore it today; callers must still decode tolerantly.
	JSON bool
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

	// ProviderModel is the model the provider's reply says served the call
	// (Gemini's modelVersion, the "model" field elsewhere). It can differ from
	// Model when Model is an alias such as gemini-flash-latest. Empty when the
	// reply named none.
	ProviderModel string
	// RequestID is the provider's own ID for this call, from the reply body
	// (Gemini responseId, OpenAI/Claude id). Empty when it gave none.
	RequestID string
	// Usage is the token usage exactly as the provider reported it. Unlike
	// InputTokens/OutputTokens, a count the provider did not send stays nil
	// rather than reading as 0.
	Usage Usage
}

// Usage is a provider's reported token usage. Each field is nil when the
// provider did not report it; nothing here is estimated or derived.
type Usage struct {
	InputTokens  *int
	OutputTokens *int
	// TotalTokens is set only when the provider reports a total itself.
	TotalTokens *int
	// ReasoningTokens are thinking tokens, where the provider counts them
	// separately. They are already included in OutputTokens.
	ReasoningTokens *int
}

// Reported reports whether the provider sent any usage at all.
func (u Usage) Reported() bool {
	return u.InputTokens != nil || u.OutputTokens != nil || u.TotalTokens != nil || u.ReasoningTokens != nil
}

// intOr returns *p, or 0 when the provider did not report it. It is for the
// legacy int fields that existing cost and token totals read.
func intOr(p *int) int {
	if p == nil {
		return 0
	}
	return *p
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

// ModelNamed is an OPTIONAL capability interface: the client can say which
// model it is about to use. Chat announces it before the first call so the
// user knows who is answering while the model is still thinking.
type ModelNamed interface {
	ModelName() string
}
