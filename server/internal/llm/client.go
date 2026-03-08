// Package llm provides a provider-agnostic interface for calling large language
// models. Concrete implementations live alongside this file (ollama.go,
// openai.go). The Router type selects the right client at runtime based on
// configuration or per-agent overrides.
package llm

import "context"

// Role constants for chat messages.
const (
	RoleSystem    = "system"
	RoleUser      = "user"
	RoleAssistant = "assistant"
)

// Message is a single turn in a chat conversation.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
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
}

// GenerateResponse holds the model's reply and usage metadata.
type GenerateResponse struct {
	// Content is the raw text returned by the model.
	Content string
	// FinishReason indicates why generation stopped ("stop", "length", etc.).
	FinishReason string
	// InputTokens is the number of tokens in the prompt (if reported).
	InputTokens int
	// OutputTokens is the number of tokens in the completion (if reported).
	OutputTokens int
}

// Client is the interface every LLM provider must satisfy.
type Client interface {
	// Generate sends a chat request and returns the model's reply.
	Generate(ctx context.Context, req GenerateRequest) (*GenerateResponse, error)
	// ProviderName returns a human-readable name for logging/metrics.
	ProviderName() string
}
