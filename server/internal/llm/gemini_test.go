package llm

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// testKey is assembled at runtime so no key-shaped literal sits in the repo.
var testKey = strings.Join([]string{"test", "gemini", "key"}, "-")

// fastRetry keeps retry tests quick while exercising the real policy code.
var fastRetry = retryPolicy{attempts: 3, base: time.Millisecond, max: 50 * time.Millisecond}

func newTestGemini(t *testing.T, h http.HandlerFunc) (*GeminiClient, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c := NewGeminiClient(srv.URL, testKey, "", 5*time.Second)
	c.retry = fastRetry
	return c, srv
}

const geminiOK = `{"candidates":[{"content":{"role":"model","parts":[
  {"text":"thinking out loud","thought":true},{"text":"Hello "},{"text":"world"}]},
  "finishReason":"STOP"}],
  "usageMetadata":{"promptTokenCount":12,"candidatesTokenCount":5,"thoughtsTokenCount":7}}`

func TestGeminiRequestShape(t *testing.T) {
	var gotPath, gotKey, gotQuery string
	var body geminiRequest
	c, _ := newTestGemini(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotKey, gotQuery = r.URL.Path, r.Header.Get("x-goog-api-key"), r.URL.RawQuery
		raw, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Errorf("request is not JSON: %v", err)
		}
		_, _ = io.WriteString(w, geminiOK)
	})

	_, err := c.Generate(context.Background(), GenerateRequest{
		MaxTokens: 256, Temperature: 0.3, JSON: true,
		Messages: []Message{
			{Role: RoleSystem, Content: "be brief"},
			{Role: RoleUser, Content: "one"},
			{Role: RoleUser, Content: "two"},
			{Role: RoleAssistant, Content: "ok"},
			{Role: RoleUser, Content: "three"},
		},
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	if gotPath != "/v1beta/models/gemini-flash-latest:generateContent" {
		t.Errorf("path = %q, want the default model's generateContent", gotPath)
	}
	if gotKey != testKey {
		t.Errorf("x-goog-api-key = %q", gotKey)
	}
	if strings.Contains(gotQuery, testKey) {
		t.Error("the API key must not appear in the URL")
	}
	if body.SystemInstruction == nil || body.SystemInstruction.Parts[0].Text != "be brief" {
		t.Errorf("system prompt must become systemInstruction, got %+v", body.SystemInstruction)
	}
	if len(body.Contents) != 3 {
		t.Fatalf("adjacent user turns must merge: got %d contents", len(body.Contents))
	}
	if body.Contents[0].Role != "user" || len(body.Contents[0].Parts) != 2 ||
		body.Contents[1].Role != "model" || body.Contents[2].Role != "user" {
		t.Errorf("roles = %+v", body.Contents)
	}
	gc := body.GenerationConfig
	if gc.MaxOutputTokens != 256 || gc.Temperature == nil || *gc.Temperature != 0.3 || gc.ResponseMIMEType != "application/json" {
		t.Errorf("generationConfig = %+v", gc)
	}
}

func TestGeminiExplicitModelOverridesDefault(t *testing.T) {
	var gotPath string
	c, _ := newTestGemini(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = io.WriteString(w, geminiOK)
	})
	resp, err := c.Generate(context.Background(), GenerateRequest{
		Model: "models/gemini-2.5-pro", Messages: []Message{{Role: RoleUser, Content: "hi"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/v1beta/models/gemini-2.5-pro:generateContent" || resp.Model != "gemini-2.5-pro" {
		t.Errorf("path %q model %q", gotPath, resp.Model)
	}
}

func TestGeminiResponseParsing(t *testing.T) {
	c, _ := newTestGemini(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, geminiOK)
	})
	resp, err := c.Generate(context.Background(), GenerateRequest{Messages: []Message{{Role: RoleUser, Content: "hi"}}})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Content != "Hello world" {
		t.Errorf("thought parts must be dropped: content = %q", resp.Content)
	}
	if resp.FinishReason != "stop" || resp.InputTokens != 12 || resp.OutputTokens != 12 {
		t.Errorf("finish %q in %d out %d", resp.FinishReason, resp.InputTokens, resp.OutputTokens)
	}
}

func TestGeminiMissingKey(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hits.Add(1) }))
	defer srv.Close()

	c := NewGeminiClient(srv.URL, "", "", time.Second)
	_, err := c.Generate(context.Background(), GenerateRequest{Messages: []Message{{Role: RoleUser, Content: "hi"}}})
	if !errors.Is(err, ErrProviderNotConfigured) || !strings.Contains(err.Error(), "GEMINI_API_KEY") {
		t.Fatalf("err = %v, want not-configured naming GEMINI_API_KEY", err)
	}
	if hits.Load() != 0 {
		t.Error("no request may be sent without a key")
	}
}

func TestGeminiInvalidKeyIsAuthAndNotRetried(t *testing.T) {
	var hits atomic.Int32
	c, _ := newTestGemini(t, func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"error":{"code":400,"message":"API key not valid. Please pass a valid API key.",
		  "status":"INVALID_ARGUMENT","details":[{"@type":"type.googleapis.com/google.rpc.ErrorInfo","reason":"API_KEY_INVALID"}]}}`)
	})
	_, err := c.Generate(context.Background(), GenerateRequest{Messages: []Message{{Role: RoleUser, Content: "hi"}}})
	if !errors.Is(err, ErrProviderAuth) {
		t.Fatalf("err = %v, want ErrProviderAuth", err)
	}
	if hits.Load() != 1 {
		t.Errorf("auth failures must not be retried: %d attempts", hits.Load())
	}
	if strings.Contains(err.Error(), testKey) {
		t.Error("error must not contain the API key")
	}
}

func TestGeminiRateLimitRetriesThenSucceeds(t *testing.T) {
	var hits atomic.Int32
	c, _ := newTestGemini(t, func(w http.ResponseWriter, _ *http.Request) {
		if hits.Add(1) == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = io.WriteString(w, `{"error":{"code":429,"message":"slow down","status":"RESOURCE_EXHAUSTED",
			  "details":[{"@type":"type.googleapis.com/google.rpc.RetryInfo","retryDelay":"0.002s"}]}}`)
			return
		}
		_, _ = io.WriteString(w, geminiOK)
	})
	resp, err := c.Generate(context.Background(), GenerateRequest{Messages: []Message{{Role: RoleUser, Content: "hi"}}})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if hits.Load() != 2 || resp.Content != "Hello world" {
		t.Errorf("attempts %d content %q", hits.Load(), resp.Content)
	}
}

func TestGeminiServerErrorRetriesAreBounded(t *testing.T) {
	var hits atomic.Int32
	c, _ := newTestGemini(t, func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = io.WriteString(w, `{"error":{"code":503,"message":"overloaded","status":"UNAVAILABLE"}}`)
	})
	_, err := c.Generate(context.Background(), GenerateRequest{Messages: []Message{{Role: RoleUser, Content: "hi"}}})
	if !errors.Is(err, ErrProviderUnavailable) {
		t.Fatalf("err = %v, want ErrProviderUnavailable", err)
	}
	var pe *ProviderError
	if !errors.As(err, &pe) || pe.Status != http.StatusServiceUnavailable || pe.Message != "overloaded" {
		t.Errorf("ProviderError = %+v", pe)
	}
	if hits.Load() != int32(fastRetry.attempts) {
		t.Errorf("attempts = %d, want %d", hits.Load(), fastRetry.attempts)
	}
}

// A quota that resets tomorrow is not worth blocking a run on.
func TestGeminiLongRetryDelayIsNotWaitedOn(t *testing.T) {
	var hits atomic.Int32
	c, _ := newTestGemini(t, func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(w, `{"error":{"code":429,"message":"quota","details":[{"retryDelay":"3600s"}]}}`)
	})
	_, err := c.Generate(context.Background(), GenerateRequest{Messages: []Message{{Role: RoleUser, Content: "hi"}}})
	if !errors.Is(err, ErrProviderRateLimited) || hits.Load() != 1 {
		t.Fatalf("err = %v after %d attempts, want one rate-limited attempt", err, hits.Load())
	}
}

func TestGeminiMalformedResponses(t *testing.T) {
	cases := map[string]string{
		"not json":      `<html>gateway</html>`,
		"no candidates": `{"candidates":[]}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			c, _ := newTestGemini(t, func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, body) })
			_, err := c.Generate(context.Background(), GenerateRequest{Messages: []Message{{Role: RoleUser, Content: "hi"}}})
			if !errors.Is(err, ErrMalformedResponse) {
				t.Fatalf("err = %v, want ErrMalformedResponse", err)
			}
		})
	}
}

func TestGeminiBlockedPromptIsBadRequest(t *testing.T) {
	c, _ := newTestGemini(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"promptFeedback":{"blockReason":"SAFETY"}}`)
	})
	_, err := c.Generate(context.Background(), GenerateRequest{Messages: []Message{{Role: RoleUser, Content: "hi"}}})
	if !errors.Is(err, ErrProviderBadRequest) || !strings.Contains(err.Error(), "SAFETY") {
		t.Fatalf("err = %v", err)
	}
}

// Callers already handle ErrOnlyThinking; a budget spent entirely on thinking
// must surface as it rather than as an empty success.
func TestGeminiThinkingOnlyReplyIsErrOnlyThinking(t *testing.T) {
	c, _ := newTestGemini(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"candidates":[{"content":{"parts":[{"text":"hmm","thought":true}]},"finishReason":"MAX_TOKENS"}],
		  "usageMetadata":{"promptTokenCount":3,"thoughtsTokenCount":64}}`)
	})
	_, err := c.Generate(context.Background(), GenerateRequest{MaxTokens: 64, Messages: []Message{{Role: RoleUser, Content: "hi"}}})
	if !errors.Is(err, ErrOnlyThinking) {
		t.Fatalf("err = %v, want ErrOnlyThinking", err)
	}
}

func TestGeminiTimeoutIsClassifiedAndNotRetried(t *testing.T) {
	var hits atomic.Int32
	release := make(chan struct{})
	c, _ := newTestGemini(t, func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		select {
		case <-release:
		case <-r.Context().Done():
		}
	})
	defer close(release)
	c.httpClient.Timeout = 30 * time.Millisecond

	_, err := c.Generate(context.Background(), GenerateRequest{Messages: []Message{{Role: RoleUser, Content: "hi"}}})
	if !errors.Is(err, ErrProviderTimeout) {
		t.Fatalf("err = %v, want ErrProviderTimeout", err)
	}
	if hits.Load() != 1 {
		t.Errorf("timeouts must not be retried: %d attempts", hits.Load())
	}
}

// Cancelling the task must come back as the context's own error, so the task
// lifecycle records a cancel rather than a provider failure.
func TestGeminiCancellationIsReturnedUnchanged(t *testing.T) {
	c, _ := newTestGemini(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	})
	c.retry = retryPolicy{attempts: 3, base: time.Hour, max: time.Hour}

	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(20*time.Millisecond, cancel)
	start := time.Now()
	_, err := c.Generate(ctx, GenerateRequest{Messages: []Message{{Role: RoleUser, Content: "hi"}}})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if time.Since(start) > 5*time.Second {
		t.Error("cancellation must interrupt the backoff wait")
	}
}
