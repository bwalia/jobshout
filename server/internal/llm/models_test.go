package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// tagsFixture is a trimmed capture of a real Ollama 0.32.9 /api/tags response.
// It deliberately keeps the real nesting: capabilities is a SIBLING of details,
// while context_length and parameter_size live inside it.
const tagsFixture = `{"models":[
 {"name":"qwen3-coder:30b","model":"qwen3-coder:30b",
  "details":{"family":"qwen3moe","parameter_size":"30.5B","quantization_level":"Q4_K_M","context_length":262144},
  "capabilities":["completion","tools"]},
 {"name":"llama3:latest","model":"llama3:latest",
  "details":{"family":"llama","parameter_size":"8.0B","quantization_level":"Q4_0","context_length":8192},
  "capabilities":["completion"]},
 {"name":"all-minilm:latest","model":"all-minilm:latest",
  "details":{"family":"bert","parameter_size":"23M","quantization_level":"F16","context_length":512},
  "capabilities":["embedding"]},
 {"name":"ancient:7b","model":"ancient:7b",
  "details":{"family":"llama"}}
]}`

func tagsServer(t *testing.T, body string, status int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tags" {
			t.Errorf("unexpected path %q, want /api/tags", r.URL.Path)
		}
		if r.Method != http.MethodGet {
			t.Errorf("unexpected method %q, want GET", r.Method)
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
}

func TestOllamaListModelsParsesTags(t *testing.T) {
	srv := tagsServer(t, tagsFixture, http.StatusOK)
	defer srv.Close()

	models, err := NewOllamaClient(srv.URL, "llama3").ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if len(models) != 4 {
		t.Fatalf("got %d models, want 4", len(models))
	}

	// Sorted by name, so all-minilm is first.
	byName := map[string]ModelInfo{}
	for _, m := range models {
		byName[m.Name] = m
		if m.Provider != "ollama" {
			t.Errorf("%s: provider = %q, want ollama", m.Name, m.Provider)
		}
	}

	coder := byName["qwen3-coder:30b"]
	if coder.ContextTokens != 262144 {
		t.Errorf("qwen3-coder context = %d, want 262144", coder.ContextTokens)
	}
	if coder.ParameterSize != "30.5B" {
		t.Errorf("qwen3-coder params = %q, want 30.5B", coder.ParameterSize)
	}
	if coder.Family != "qwen3moe" {
		t.Errorf("qwen3-coder family = %q, want qwen3moe", coder.Family)
	}
	if !coder.SupportsTools() {
		t.Error("qwen3-coder should support tools")
	}
	if coder.IsEmbeddingOnly() {
		t.Error("qwen3-coder must not be embedding-only")
	}

	if byName["llama3:latest"].SupportsTools() {
		t.Error("llama3:latest must not report tool support")
	}

	if !byName["all-minilm:latest"].IsEmbeddingOnly() {
		t.Error("all-minilm should be embedding-only")
	}

	// An entry from a pre-0.6 server reports no capabilities at all. It must
	// degrade to "can chat, cannot use tools" rather than to "can do nothing".
	old := byName["ancient:7b"]
	if !old.Has(CapCompletion) {
		t.Error("model without capabilities should be assumed completion-capable")
	}
	if old.SupportsTools() {
		t.Error("model without capabilities must not be assumed tool-capable")
	}
	if old.ContextTokens != 0 {
		t.Errorf("unknown context should stay 0, got %d", old.ContextTokens)
	}
}

func TestOllamaListModelsErrorsOnBadStatus(t *testing.T) {
	srv := tagsServer(t, `nope`, http.StatusInternalServerError)
	defer srv.Close()

	if _, err := NewOllamaClient(srv.URL, "llama3").ListModels(context.Background()); err == nil {
		t.Fatal("expected an error for a 500 response")
	}
}

// TestOllamaNumCtxIsAlwaysSent guards the silent-truncation trap: without
// num_ctx, Ollama applies its own server default and quietly truncates a long
// prompt instead of refusing it.
func TestOllamaNumCtxIsAlwaysSent(t *testing.T) {
	var captured map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&captured)
		_, _ = w.Write([]byte(`{"model":"m","message":{"role":"assistant","content":"hi"},"done":true}`))
	}))
	defer srv.Close()

	c := NewOllamaClient(srv.URL, "llama3").WithNumCtx(32768)
	if _, err := c.Generate(context.Background(), GenerateRequest{
		Messages: []Message{{Role: RoleUser, Content: "hello"}},
	}); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	opts, ok := captured["options"].(map[string]any)
	if !ok {
		t.Fatalf("no options in request: %#v", captured)
	}
	if got := opts["num_ctx"]; got != float64(32768) {
		t.Fatalf("num_ctx = %v, want 32768", got)
	}
}

// TestOllamaNumCtxCappedByDiscoveredLimit checks the ceiling that keeps the
// client and the selector believing the same thing about a context window.
func TestOllamaNumCtxCappedByDiscoveredLimit(t *testing.T) {
	c := NewOllamaClient("http://x", "llama3").WithNumCtx(131072)
	c.rememberModels([]ModelInfo{
		{Provider: "ollama", Name: "llama3:latest", ContextTokens: 8192},
	})

	if got := c.effectiveNumCtx("llama3:latest"); got != 8192 {
		t.Errorf("effectiveNumCtx = %d, want it capped to the model limit 8192", got)
	}
	// ":latest" is implicit in Ollama, so the bare name must resolve too.
	if got := c.effectiveNumCtx("llama3"); got != 8192 {
		t.Errorf("effectiveNumCtx(bare name) = %d, want 8192", got)
	}
	// An unknown model keeps the configured value.
	if got := c.effectiveNumCtx("who:knows"); got != 131072 {
		t.Errorf("effectiveNumCtx(unknown) = %d, want the configured 131072", got)
	}
}

func TestRouterAvailableModelsDegradesPerProvider(t *testing.T) {
	// A router built via the test helper (composite literal) must not panic:
	// the model cache is a value field with a lazily-created map precisely so
	// this works.
	r := NewTestRouter("ollama", map[string]Client{
		"ollama": NewOllamaClient("http://127.0.0.1:1", "llama3"), // refused
	})

	got := r.AvailableModels(context.Background())
	if len(got) != 1 {
		t.Fatalf("got %d providers, want 1", len(got))
	}
	if got[0].Error == "" {
		t.Error("a failed probe should report its error")
	}
	if got[0].Source != SourceStatic {
		t.Errorf("source = %q, want %q after a failed probe with no prior result", got[0].Source, SourceStatic)
	}

	// CachedModels must never do I/O, and must never panic on a cold cache.
	if len(r.CachedModels()) != 1 {
		t.Error("CachedModels should still report the provider")
	}
}

func TestStaticModelsCoverCloudProviders(t *testing.T) {
	for _, p := range []string{"openai", "claude"} {
		models := StaticModels(p)
		if len(models) == 0 {
			t.Errorf("%s: expected a static fallback list", p)
		}
		for _, m := range models {
			if m.Provider != p {
				t.Errorf("%s: model %q has provider %q", p, m.Name, m.Provider)
			}
			if m.ContextTokens <= 0 {
				t.Errorf("%s/%s: context window must be known", p, m.Name)
			}
		}
	}
	if StaticModels("ollama") != nil {
		t.Error("ollama has real discovery; it should have no static list")
	}
}
