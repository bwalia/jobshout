package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOpenAIEmbedderEmbed(t *testing.T) {
	var gotPath string
	var gotBody openAIEmbeddingRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		// Return embeddings out of order to verify index reordering.
		resp := openAIEmbeddingResponse{}
		resp.Data = []struct {
			Index     int       `json:"index"`
			Embedding []float32 `json:"embedding"`
		}{
			{Index: 1, Embedding: []float32{0.3, 0.4}},
			{Index: 0, Embedding: []float32{0.1, 0.2}},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	e := NewOpenAIEmbedder(srv.URL, "test-key", "text-embedding-3-small", 2)
	if e.Dimensions() != 2 {
		t.Fatalf("Dimensions() = %d; want 2", e.Dimensions())
	}
	if e.EmbedderName() != "openai" {
		t.Fatalf("EmbedderName() = %q; want openai", e.EmbedderName())
	}

	vecs, err := e.Embed(context.Background(), []string{"hello", "world"})
	if err != nil {
		t.Fatalf("Embed error: %v", err)
	}
	if gotPath != "/v1/embeddings" {
		t.Errorf("path = %q; want /v1/embeddings", gotPath)
	}
	if gotBody.Model != "text-embedding-3-small" || len(gotBody.Input) != 2 {
		t.Errorf("request body = %+v; unexpected", gotBody)
	}
	if len(vecs) != 2 {
		t.Fatalf("got %d vectors; want 2", len(vecs))
	}
	if vecs[0][0] != 0.1 || vecs[1][0] != 0.3 {
		t.Errorf("vectors not reordered by index: %v", vecs)
	}
}

func TestOpenAIEmbedderAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"bad key","type":"auth"}}`))
	}))
	defer srv.Close()

	e := NewOpenAIEmbedder(srv.URL, "", "text-embedding-3-small", 2)
	if _, err := e.Embed(context.Background(), []string{"x"}); err == nil {
		t.Fatal("expected error on non-200 status")
	}
}

func TestOpenAIEmbedderEmptyInput(t *testing.T) {
	e := NewOpenAIEmbedder("http://unused", "", "", 1536)
	vecs, err := e.Embed(context.Background(), nil)
	if err != nil || vecs != nil {
		t.Fatalf("empty input should be a no-op; got %v, %v", vecs, err)
	}
}

func TestOllamaEmbedderEmbed(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/api/embeddings" {
			t.Errorf("path = %q; want /api/embeddings", r.URL.Path)
		}
		var req ollamaEmbeddingRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ollamaEmbeddingResponse{Embedding: []float32{1, 2, 3}})
	}))
	defer srv.Close()

	e := NewOllamaEmbedder(srv.URL, "nomic-embed-text", 3)
	vecs, err := e.Embed(context.Background(), []string{"a", "b"})
	if err != nil {
		t.Fatalf("Embed error: %v", err)
	}
	if calls != 2 {
		t.Errorf("expected 2 HTTP calls (one per text); got %d", calls)
	}
	if len(vecs) != 2 || len(vecs[0]) != 3 {
		t.Fatalf("unexpected vectors: %v", vecs)
	}
	if e.EmbedderName() != "ollama" || e.Dimensions() != 3 {
		t.Errorf("name/dims mismatch: %q %d", e.EmbedderName(), e.Dimensions())
	}
}

func TestRouterEmbedderRegistration(t *testing.T) {
	// Default embedding provider openai but no API key -> only ollama registered.
	r := &Router{
		embedders:                map[string]Embedder{"ollama": NewOllamaEmbedder("http://x", "m", 1536)},
		defaultEmbeddingProvider: "openai",
	}
	if _, err := r.Embedder(); err == nil {
		t.Error("expected error when default embedding provider not registered")
	}
	if _, err := r.EmbedderFor("ollama"); err != nil {
		t.Errorf("ollama embedder should resolve: %v", err)
	}
}
