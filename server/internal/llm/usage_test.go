package llm

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// These tests pin what each provider reports back for usage tracking: the
// model its reply names, its request ID, and token counts exactly as sent —
// nil when absent, never an invented 0.

func eqPtr(p *int, want int) bool { return p != nil && *p == want }

func TestGeminiReportsModelVersionRequestIDAndTotal(t *testing.T) {
	c, _ := newTestGemini(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"candidates":[{"content":{"parts":[{"text":"hi"}]},"finishReason":"STOP"}],
		  "usageMetadata":{"promptTokenCount":12,"candidatesTokenCount":5,"thoughtsTokenCount":7,"totalTokenCount":24},
		  "modelVersion":"gemini-2.5-flash-preview-09-2025","responseId":"abc123"}`)
	})
	resp, err := c.Generate(context.Background(), GenerateRequest{Messages: []Message{{Role: RoleUser, Content: "q"}}})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Model != "gemini-flash-latest" || resp.ProviderModel != "gemini-2.5-flash-preview-09-2025" {
		t.Errorf("model sent=%q reported=%q", resp.Model, resp.ProviderModel)
	}
	if resp.RequestID != "abc123" {
		t.Errorf("request id = %q", resp.RequestID)
	}
	u := resp.Usage
	if !eqPtr(u.InputTokens, 12) || !eqPtr(u.OutputTokens, 12) || !eqPtr(u.TotalTokens, 24) || !eqPtr(u.ReasoningTokens, 7) {
		t.Errorf("usage = in %v out %v total %v reasoning %v", u.InputTokens, u.OutputTokens, u.TotalTokens, u.ReasoningTokens)
	}
	if resp.InputTokens != 12 || resp.OutputTokens != 12 {
		t.Errorf("legacy ints = %d/%d", resp.InputTokens, resp.OutputTokens)
	}
}

func TestGeminiMissingUsageStaysNil(t *testing.T) {
	c, _ := newTestGemini(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"candidates":[{"content":{"parts":[{"text":"hi"}]},"finishReason":"STOP"}]}`)
	})
	resp, err := c.Generate(context.Background(), GenerateRequest{Messages: []Message{{Role: RoleUser, Content: "q"}}})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Usage.Reported() || resp.ProviderModel != "" || resp.RequestID != "" {
		t.Errorf("usage=%+v model=%q id=%q, want nothing reported", resp.Usage, resp.ProviderModel, resp.RequestID)
	}
}

func TestGeminiAPIDurationExcludesBackoff(t *testing.T) {
	var hits atomic.Int32
	c, _ := newTestGemini(t, func(w http.ResponseWriter, _ *http.Request) {
		if hits.Add(1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = io.WriteString(w, `{"error":{"code":503,"message":"overloaded"}}`)
			return
		}
		_, _ = io.WriteString(w, geminiOK)
	})
	// A 200ms backoff the API time must not include.
	c.retry = retryPolicy{attempts: 2, base: 200 * time.Millisecond, max: time.Second}
	ctx, stats := WithCallStats(context.Background())
	start := time.Now()
	if _, err := c.Generate(ctx, GenerateRequest{Messages: []Message{{Role: RoleUser, Content: "q"}}}); err != nil {
		t.Fatal(err)
	}
	wall := time.Since(start)
	api, ok := stats.APIDuration()
	if !ok {
		t.Fatal("API duration not measured")
	}
	if wall < 200*time.Millisecond || api >= 200*time.Millisecond {
		t.Errorf("wall=%v api=%v: API time should exclude the backoff", wall, api)
	}
}

func TestOpenAIReportsModelIDTotalAndHeaderRequestID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("x-request-id", "req_hdr_1")
		_, _ = io.WriteString(w, `{"id":"chatcmpl-9","model":"gpt-4o-mini-2024-07-18",
		  "choices":[{"message":{"role":"assistant","content":"hi"},"finish_reason":"stop"}],
		  "usage":{"prompt_tokens":10,"completion_tokens":3,"total_tokens":13,
		           "completion_tokens_details":{"reasoning_tokens":0}}}`)
	}))
	defer srv.Close()
	ctx, stats := WithCallStats(context.Background())
	resp, err := NewOpenAIClient(srv.URL, "k", "gpt-4o-mini").Generate(ctx, GenerateRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if resp.ProviderModel != "gpt-4o-mini-2024-07-18" || resp.RequestID != "chatcmpl-9" {
		t.Errorf("model=%q id=%q", resp.ProviderModel, resp.RequestID)
	}
	if stats.RequestID() != "req_hdr_1" {
		t.Errorf("header request id = %q", stats.RequestID())
	}
	u := resp.Usage
	// A reported 0 is kept as 0.
	if !eqPtr(u.InputTokens, 10) || !eqPtr(u.OutputTokens, 3) || !eqPtr(u.TotalTokens, 13) || !eqPtr(u.ReasoningTokens, 0) {
		t.Errorf("usage = %+v", u)
	}
	if _, ok := stats.APIDuration(); !ok {
		t.Error("API duration not measured")
	}
}

func TestOpenAIMissingUsageStaysNil(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"hi"},"finish_reason":"stop"}]}`)
	}))
	defer srv.Close()
	resp, err := NewOpenAIClient(srv.URL, "k", "m").Generate(context.Background(), GenerateRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Usage.Reported() || resp.ProviderModel != "" || resp.RequestID != "" {
		t.Errorf("usage=%+v model=%q id=%q", resp.Usage, resp.ProviderModel, resp.RequestID)
	}
}

func TestClaudeReportsModelIDAndNoTotal(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("request-id", "req_011")
		_, _ = io.WriteString(w, `{"id":"msg_01","type":"message","role":"assistant","model":"claude-sonnet-5-5-20260101",
		  "content":[{"type":"text","text":"hi"}],"stop_reason":"end_turn",
		  "usage":{"input_tokens":20,"output_tokens":4}}`)
	}))
	defer srv.Close()
	ctx, stats := WithCallStats(context.Background())
	resp, err := NewClaudeClient(srv.URL, "k", "claude-sonnet-5-5").Generate(ctx, GenerateRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if resp.ProviderModel != "claude-sonnet-5-5-20260101" || resp.RequestID != "msg_01" || stats.RequestID() != "req_011" {
		t.Errorf("model=%q id=%q header=%q", resp.ProviderModel, resp.RequestID, stats.RequestID())
	}
	u := resp.Usage
	if !eqPtr(u.InputTokens, 20) || !eqPtr(u.OutputTokens, 4) || u.TotalTokens != nil || u.ReasoningTokens != nil {
		t.Errorf("usage = %+v (Anthropic reports no total)", u)
	}
}

func TestOllamaReportsServedModelAndNilWhenPromptCountMissing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"model":"qwen3:8b","message":{"role":"assistant","content":"he"},"done":false}
{"model":"qwen3:8b","message":{"role":"assistant","content":"llo"},"done":false}
{"model":"qwen3:8b","message":{"role":"assistant","content":""},"done":true,"done_reason":"stop","eval_count":9}
`)
	}))
	defer srv.Close()
	ctx, stats := WithCallStats(context.Background())
	resp, err := NewOllamaClient(srv.URL, "qwen3").Generate(ctx, GenerateRequest{Messages: []Message{{Role: RoleUser, Content: "q"}}})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Model != "qwen3" || resp.ProviderModel != "qwen3:8b" || resp.RequestID != "" {
		t.Errorf("sent=%q reported=%q id=%q", resp.Model, resp.ProviderModel, resp.RequestID)
	}
	u := resp.Usage
	// prompt_eval_count was left out (a cached prompt): unknown, not 0.
	if u.InputTokens != nil || !eqPtr(u.OutputTokens, 9) || u.TotalTokens != nil {
		t.Errorf("usage = in %v out %v total %v", u.InputTokens, u.OutputTokens, u.TotalTokens)
	}
	if _, ok := stats.APIDuration(); !ok {
		t.Error("API duration not measured")
	}
}

func TestFailedHostedCallKeepsHeaderRequestID(t *testing.T) {
	c, _ := newTestGemini(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("x-request-id", "failed-req")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"error":{"code":400,"message":"bad"}}`)
	})
	ctx, stats := WithCallStats(context.Background())
	if _, err := c.Generate(ctx, GenerateRequest{Messages: []Message{{Role: RoleUser, Content: "q"}}}); err == nil {
		t.Fatal("want error")
	}
	if stats.RequestID() != "failed-req" {
		t.Errorf("request id = %q", stats.RequestID())
	}
}

func TestGeminiRetryKeepsEachHTTPAttempt(t *testing.T) {
	var hits atomic.Int32
	c, _ := newTestGemini(t, func(w http.ResponseWriter, _ *http.Request) {
		if hits.Add(1) == 1 {
			w.Header().Set("x-request-id", "first")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = io.WriteString(w, `{"error":{"code":503,"message":"overloaded"}}`)
			return
		}
		_, _ = io.WriteString(w, geminiOK)
	})
	ctx, stats := WithCallStats(context.Background())
	if _, err := c.Generate(ctx, GenerateRequest{Messages: []Message{{Role: RoleUser, Content: "q"}}}); err != nil {
		t.Fatal(err)
	}
	at := stats.Attempts()
	if len(at) != 2 || stats.Retries() != 1 {
		t.Fatalf("attempts = %+v retries = %d, want 2 attempts and 1 retry", at, stats.Retries())
	}
	if at[0].HTTPStatus != 503 || at[0].RequestID != "first" || at[0].Error == "" {
		t.Errorf("first attempt = %+v", at[0])
	}
	if at[1].HTTPStatus != 200 || at[1].Error != "" || !at[1].StartedAt.After(at[0].StartedAt) {
		t.Errorf("second attempt = %+v", at[1])
	}
}

func TestGeminiRejectedReplyStillReportsUsage(t *testing.T) {
	c, _ := newTestGemini(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"candidates":[{"content":{"parts":[{"text":"hmm","thought":true}]},"finishReason":"MAX_TOKENS"}],
		  "usageMetadata":{"promptTokenCount":40,"thoughtsTokenCount":100,"totalTokenCount":140},
		  "modelVersion":"gemini-2.5-flash","responseId":"r-think"}`)
	})
	ctx, stats := WithCallStats(context.Background())
	_, err := c.Generate(ctx, GenerateRequest{Messages: []Message{{Role: RoleUser, Content: "q"}}})
	if err == nil {
		t.Fatal("want ErrOnlyThinking")
	}
	r := stats.Reply()
	if r == nil || r.Model != "gemini-2.5-flash" || r.RequestID != "r-think" ||
		!eqPtr(r.Usage.InputTokens, 40) || !eqPtr(r.Usage.OutputTokens, 100) || !eqPtr(r.Usage.TotalTokens, 140) {
		t.Errorf("reply = %+v", r)
	}
}

func TestOllamaServerErrorIsAFailedAttempt(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, `{"error":"model 'qwen3' not found"}`)
	}))
	defer srv.Close()
	ctx, stats := WithCallStats(context.Background())
	if _, err := NewOllamaClient(srv.URL, "qwen3").Generate(ctx, GenerateRequest{}); err == nil {
		t.Fatal("want error")
	}
	at := stats.Attempts()
	if len(at) != 1 || at[0].HTTPStatus != 500 || at[0].Error == "" {
		t.Errorf("attempts = %+v", at)
	}
	if stats.Reply() != nil {
		t.Errorf("reply = %+v, want none (no reply was decoded)", stats.Reply())
	}
}

func TestConnectionFailureIsAnAttemptWithNoStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL
	srv.Close() // nothing listens any more
	ctx, stats := WithCallStats(context.Background())
	if _, err := NewOpenAIClient(url, "k", "m").Generate(ctx, GenerateRequest{}); err == nil {
		t.Fatal("want error")
	}
	at := stats.Attempts()
	if len(at) != 1 || at[0].HTTPStatus != 0 || at[0].Error == "" {
		t.Errorf("attempts = %+v", at)
	}
}
