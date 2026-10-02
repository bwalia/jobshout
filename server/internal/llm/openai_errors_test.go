package llm

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
)

func newTestOpenAI(t *testing.T, h http.HandlerFunc) *OpenAIClient {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c := NewOpenAIClient(srv.URL, "k", "gpt-test")
	c.retry = fastRetry
	return c
}

const openAIOK = `{"choices":[{"message":{"role":"assistant","content":"hi"},"finish_reason":"stop"}],
  "usage":{"prompt_tokens":3,"completion_tokens":1}}`

func TestOpenAIErrorClassification(t *testing.T) {
	cases := []struct {
		name     string
		status   int
		body     string
		want     error
		attempts int32
	}{
		{"bad key", 401, `{"error":{"message":"Incorrect API key provided","type":"invalid_request_error","code":"invalid_api_key"}}`, ErrProviderAuth, 1},
		{"quota is not retried", 429, `{"error":{"message":"You exceeded your current quota","type":"insufficient_quota","code":"insufficient_quota"}}`, ErrProviderRateLimited, 1},
		{"rate limit is retried", 429, `{"error":{"message":"Rate limit reached","type":"requests","code":"rate_limit_exceeded"}}`, ErrProviderRateLimited, 3},
		{"server error is retried", 502, `bad gateway`, ErrProviderUnavailable, 3},
		{"unknown model", 404, `{"error":{"message":"The model does not exist","code":"model_not_found"}}`, ErrProviderBadRequest, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var hits atomic.Int32
			c := newTestOpenAI(t, func(w http.ResponseWriter, _ *http.Request) {
				hits.Add(1)
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			})
			_, err := c.Generate(context.Background(), GenerateRequest{Messages: []Message{{Role: RoleUser, Content: "x"}}})
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
			if hits.Load() != tc.attempts {
				t.Errorf("attempts = %d, want %d", hits.Load(), tc.attempts)
			}
		})
	}
}

func TestOpenAIRetryAfterThenSuccess(t *testing.T) {
	var hits atomic.Int32
	c := newTestOpenAI(t, func(w http.ResponseWriter, _ *http.Request) {
		if hits.Add(1) == 1 {
			w.Header().Set("retry-after-ms", "2")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = io.WriteString(w, openAIOK)
	})
	resp, err := c.Generate(context.Background(), GenerateRequest{Messages: []Message{{Role: RoleUser, Content: "x"}}})
	if err != nil || resp.Content != "hi" || hits.Load() != 2 {
		t.Fatalf("resp %+v err %v attempts %d", resp, err, hits.Load())
	}
}

func TestOpenAIMalformedResponse(t *testing.T) {
	for name, body := range map[string]string{"not json": "<html>", "no choices": `{"choices":[]}`} {
		t.Run(name, func(t *testing.T) {
			c := newTestOpenAI(t, func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, body) })
			_, err := c.Generate(context.Background(), GenerateRequest{Messages: []Message{{Role: RoleUser, Content: "x"}}})
			if !errors.Is(err, ErrMalformedResponse) {
				t.Fatalf("err = %v, want ErrMalformedResponse", err)
			}
		})
	}
}

// rewriteTransport sends every request to target, so a client whose BaseURL
// is api.openai.com can be exercised against a local server.
type rewriteTransport struct{ target *url.URL }

func (rt rewriteTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.URL.Scheme, r.URL.Host = rt.target.Scheme, rt.target.Host
	return http.DefaultTransport.RoundTrip(r)
}

func captureOpenAIBody(t *testing.T, baseURL string, req GenerateRequest) map[string]any {
	t.Helper()
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		_, _ = io.WriteString(w, openAIOK)
	}))
	t.Cleanup(srv.Close)
	target, _ := url.Parse(srv.URL)
	if baseURL == "" {
		baseURL = srv.URL
	}
	c := NewOpenAIClient(baseURL, "k", "gpt-test")
	c.httpClient.Transport = rewriteTransport{target}
	if _, err := c.Generate(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	return body
}

func TestOpenAIOfficialAPIUsesCurrentParameters(t *testing.T) {
	body := captureOpenAIBody(t, "https://api.openai.com", GenerateRequest{
		MaxTokens: 100, JSON: true,
		Messages: []Message{{Role: RoleUser, Content: "Reply as JSON."}},
	})
	if body["max_completion_tokens"] != float64(100) || body["max_tokens"] != nil {
		t.Errorf("OpenAI itself must get max_completion_tokens: %v", body)
	}
	if rf, _ := body["response_format"].(map[string]any); rf["type"] != "json_object" {
		t.Errorf("JSON calls must ask for json_object: %v", body["response_format"])
	}
}

// LM Studio, vLLM and friends keep the request they have always received.
func TestOpenAICompatibleEndpointKeepsLegacyShape(t *testing.T) {
	body := captureOpenAIBody(t, "", GenerateRequest{
		MaxTokens: 100, JSON: true,
		Messages: []Message{{Role: RoleUser, Content: "Reply as JSON."}},
	})
	if body["max_tokens"] != float64(100) || body["max_completion_tokens"] != nil || body["response_format"] != nil {
		t.Errorf("compatible endpoint body changed: %v", body)
	}
}
