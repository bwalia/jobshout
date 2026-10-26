package llm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/jobshout/server/internal/config"
)

// fakeAPI records each request body and answers with the first matching rule,
// or reply when none matches.
type fakeAPI struct {
	mu     sync.Mutex
	bodies []map[string]any
	refuse func(body map[string]any) string // non-empty: a 400 with this body
	reply  string
}

func (f *fakeAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	raw, _ := io.ReadAll(r.Body)
	var body map[string]any
	_ = json.Unmarshal(raw, &body)
	f.mu.Lock()
	f.bodies = append(f.bodies, body)
	f.mu.Unlock()
	if msg := f.refuse(body); msg != "" {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, msg)
		return
	}
	_, _ = io.WriteString(w, f.reply)
}

// A Claude model that refuses temperature still answers, and the next call to
// it does not send one — the state of every current Claude model.
func TestClaude_ModelThatRefusesTemperature(t *testing.T) {
	api := &fakeAPI{
		refuse: func(b map[string]any) string {
			if _, ok := b["temperature"]; ok {
				return `{"type":"error","error":{"type":"invalid_request_error","message":"temperature is deprecated for this model."}}`
			}
			return ""
		},
		reply: `{"id":"msg_1","model":"claude-new","content":[{"type":"thinking","thinking":""},{"type":"text","text":"ok"}],"stop_reason":"end_turn"}`,
	}
	srv := httptest.NewServer(api)
	defer srv.Close()
	c := NewClaudeClient(srv.URL, "k", "claude-refuses-temperature")

	for i := 0; i < 2; i++ {
		resp, err := c.Generate(context.Background(), GenerateRequest{
			Messages: []Message{{Role: RoleUser, Content: "label this"}}, JSON: true, MaxTokens: 16,
		})
		if err != nil || resp.Content != "ok" {
			t.Fatalf("call %d: %v, %+v", i+1, err, resp)
		}
	}
	if len(api.bodies) != 3 {
		t.Fatalf("want 1 refused + 2 accepted requests, got %d", len(api.bodies))
	}
	if got := api.bodies[0]["max_tokens"]; got != float64(minThinkingTokens) {
		t.Errorf("max_tokens = %v: a 16-token ceiling leaves a thinking model no room to answer", got)
	}
}

// A Claude model that accepts temperature keeps getting it.
func TestClaude_KeepsTemperatureWhenAccepted(t *testing.T) {
	api := &fakeAPI{
		refuse: func(map[string]any) string { return "" },
		reply:  `{"id":"msg_1","content":[{"type":"text","text":"ok"}]}`,
	}
	srv := httptest.NewServer(api)
	defer srv.Close()
	c := NewClaudeClient(srv.URL, "k", "claude-takes-temperature")

	if _, err := c.Generate(context.Background(), GenerateRequest{Messages: []Message{{Role: RoleUser, Content: "x"}}, Temperature: 0.3}); err != nil {
		t.Fatal(err)
	}
	if api.bodies[0]["temperature"] != 0.3 {
		t.Fatalf("temperature dropped: %v", api.bodies[0])
	}
}

// OpenAI's reasoning models refuse max_tokens and then temperature, one 400 at
// a time; the call still succeeds, and later calls send the accepted shape.
func TestOpenAI_ReasoningModelRefusals(t *testing.T) {
	api := &fakeAPI{
		refuse: func(b map[string]any) string {
			if _, ok := b["max_tokens"]; ok {
				return `{"error":{"message":"Unsupported parameter: 'max_tokens' is not supported with this model. Use 'max_completion_tokens' instead.","type":"invalid_request_error","param":"max_tokens","code":"unsupported_parameter"}}`
			}
			if _, ok := b["temperature"]; ok {
				return `{"error":{"message":"Unsupported value: 'temperature' does not support 0.2 with this model. Only the default (1) value is supported.","param":"temperature","code":"unsupported_value"}}`
			}
			return ""
		},
		reply: `{"id":"c1","model":"gpt-next","choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`,
	}
	srv := httptest.NewServer(api)
	defer srv.Close()
	c := NewOpenAIClient(srv.URL, "k", "gpt-refuses-both")

	req := GenerateRequest{Messages: []Message{{Role: RoleUser, Content: "x"}}, JSON: true, MaxTokens: 800}
	for i := 0; i < 2; i++ {
		if resp, err := c.Generate(context.Background(), req); err != nil || resp.Content != "ok" {
			t.Fatalf("call %d: %v", i+1, err)
		}
	}
	if len(api.bodies) != 4 {
		t.Fatalf("want 2 refused + 2 accepted requests, got %d", len(api.bodies))
	}
	last := api.bodies[3]
	if last["max_completion_tokens"] != float64(minThinkingTokens) || last["max_tokens"] != nil || last["temperature"] != nil {
		t.Fatalf("second call did not reuse the accepted shape: %v", last)
	}
}

// An unrelated 400 is returned as an error, not retried.
func TestOpenAI_OtherBadRequestIsNotRetried(t *testing.T) {
	api := &fakeAPI{
		refuse: func(map[string]any) string { return `{"error":{"message":"The model 'nope' does not exist"}}` },
	}
	srv := httptest.NewServer(api)
	defer srv.Close()
	c := NewOpenAIClient(srv.URL, "k", "nope")

	_, err := c.Generate(context.Background(), GenerateRequest{Messages: []Message{{Role: RoleUser, Content: "x"}}, Temperature: 0.5, MaxTokens: 10})
	if err == nil || !strings.Contains(err.Error(), "does not exist") || len(api.bodies) != 1 {
		t.Fatalf("err=%v requests=%d", err, len(api.bodies))
	}
}

// The picker lists exactly what the environment names, with the default model
// added, and a model the code has never heard of is offered with sane facts.
func TestConfiguredModels(t *testing.T) {
	got := ConfiguredModels("claude", "claude-sonnet-5-5", []string{"claude-opus-5-5", "claude-next-6"})
	var names []string
	for _, m := range got {
		names = append(names, m.Name)
	}
	if strings.Join(names, ",") != "claude-sonnet-5-5,claude-opus-5-5,claude-next-6" {
		t.Fatalf("names = %v", names)
	}
	if next := got[2]; next.Provider != "claude" || next.ContextTokens == 0 || !next.SupportsTools() {
		t.Fatalf("an unlisted model needs the provider's facts: %+v", next)
	}

	// Unset list: the built-in one, plus a default it does not contain.
	builtin := ConfiguredModels("openai", "gpt-brand-new", nil)
	if builtin[0].Name != "gpt-brand-new" || len(builtin) != len(StaticModels("openai"))+1 {
		t.Fatalf("built-in fallback = %+v", builtin)
	}
	if ConfiguredModels("ollama", "llama3", []string{"x"}) != nil {
		t.Fatal("ollama is discovered live, not configured")
	}
}

// End to end through the router: the models endpoint's source.
func TestRouter_OffersEnvConfiguredModels(t *testing.T) {
	r := NewRouter(&config.Config{
		LLMProvider:  "ollama",
		GeminiAPIKey: "g", GeminiDefaultModel: "gemini-flash-latest", GeminiModels: []string{"gemini-3-pro"},
		ClaudeAPIKey: "c", ClaudeDefaultModel: "claude-opus-5-5",
	})
	byProvider := map[string][]ModelInfo{}
	for _, pm := range r.CachedModels() {
		byProvider[pm.Provider] = pm.Models
	}
	if g := byProvider["gemini"]; len(g) != 2 || g[0].Name != "gemini-flash-latest" || g[1].Name != "gemini-3-pro" {
		t.Fatalf("gemini = %+v", g)
	}
	if c := byProvider["claude"]; len(c) == 0 || c[0].Name != "claude-opus-5-5" {
		t.Fatalf("claude = %+v", c)
	}
	if _, ok := byProvider["openai"]; ok {
		t.Fatal("openai has no key and must not be offered")
	}
}
