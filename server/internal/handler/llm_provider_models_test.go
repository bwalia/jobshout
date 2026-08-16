package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jobshout/server/internal/llm"
)

// ollamaTagsStub serves a realistic /api/tags payload, including an
// embedding-only model that must never reach the picker.
func ollamaTagsStub(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"models":[
		 {"name":"qwen3-coder:30b",
		  "details":{"family":"qwen3moe","parameter_size":"30.5B","context_length":262144},
		  "capabilities":["completion","tools"]},
		 {"name":"llama3:latest",
		  "details":{"family":"llama","parameter_size":"8.0B","context_length":8192},
		  "capabilities":["completion"]},
		 {"name":"all-minilm:latest",
		  "details":{"family":"bert","parameter_size":"23M","context_length":512},
		  "capabilities":["embedding"]}
		]}`))
	}))
}

func TestListModelsRendersPickerPayload(t *testing.T) {
	stub := ollamaTagsStub(t)
	defer stub.Close()

	router := llm.NewTestRouter("ollama", map[string]llm.Client{
		"ollama": llm.NewOllamaClient(stub.URL, "llama3"),
	})
	h := NewLLMProviderHandler(nil, router, true)

	rec := httptest.NewRecorder()
	h.ListModels(rec, httptest.NewRequest(http.MethodGet, "/llm-providers/models", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var got AvailableModelsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v (body: %s)", err, rec.Body.String())
	}

	if !got.Auto.Available {
		t.Error("Auto should be offered when auto-selection is enabled")
	}
	if len(got.Providers) != 1 {
		t.Fatalf("got %d provider groups, want 1", len(got.Providers))
	}

	g := got.Providers[0]
	if g.Provider != "ollama" || !g.IsDefault {
		t.Errorf("group = %q (default=%v), want ollama default", g.Provider, g.IsDefault)
	}
	if g.Source != llm.SourceDiscovered {
		t.Errorf("source = %q, want %q", g.Source, llm.SourceDiscovered)
	}
	if g.Error != "" {
		t.Errorf("unexpected error on a healthy probe: %q", g.Error)
	}

	if len(g.Models) != 2 {
		t.Fatalf("got %d models, want 2 (the embedding model must be filtered out)", len(g.Models))
	}
	for _, m := range g.Models {
		if m.Name == "all-minilm:latest" {
			t.Fatal("embedding-only model leaked into the picker")
		}
	}

	byName := map[string]AvailableModel{}
	for _, m := range g.Models {
		byName[m.Name] = m
	}
	coder := byName["qwen3-coder:30b"]
	if coder.ContextTokens != 262144 || !coder.SupportsTools || coder.ParameterSize != "30.5B" {
		t.Errorf("qwen3-coder rendered as %+v", coder)
	}
	if byName["llama3:latest"].SupportsTools {
		t.Error("llama3:latest must not claim tool support")
	}
}

// A provider that cannot be reached must still appear, carrying its error and a
// static fallback, rather than vanishing from the picker.
func TestListModelsDegradesWhenProviderUnreachable(t *testing.T) {
	router := llm.NewTestRouter("ollama", map[string]llm.Client{
		"ollama": llm.NewOllamaClient("http://127.0.0.1:1", "llama3"),
	})
	h := NewLLMProviderHandler(nil, router, false)

	rec := httptest.NewRecorder()
	h.ListModels(rec, httptest.NewRequest(http.MethodGet, "/llm-providers/models", nil))

	var got AvailableModelsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Auto.Available {
		t.Error("Auto must not be offered when auto-selection is disabled")
	}
	if len(got.Providers) != 1 {
		t.Fatalf("got %d providers, want the unreachable one still listed", len(got.Providers))
	}
	if got.Providers[0].Error == "" {
		t.Error("an unreachable provider should report why")
	}
}
