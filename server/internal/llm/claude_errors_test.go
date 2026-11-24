package llm

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// A throttled Claude (Bedrock throttles often) is a classified rate limit, so
// the Ollama fallback and org chains treat it like any other provider's.
func TestClaudeClient_ThrottleIsARateLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"message":"Too many requests, please wait before trying again."}`))
	}))
	defer srv.Close()
	_, err := NewClaudeClient(srv.URL, "k", "eu.anthropic.claude-sonnet-5-5").Generate(context.Background(), GenerateRequest{
		Messages: []Message{{Role: RoleUser, Content: "hi"}},
	})
	var pe *ProviderError
	if !errors.As(err, &pe) || !errors.Is(err, ErrProviderRateLimited) || !pe.Retryable() {
		t.Fatalf("err = %v, want a retryable rate limit", err)
	}
}
