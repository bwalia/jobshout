package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Generate must request a streamed chat reply. With stream:false Ollama holds
// headers until the whole answer is ready, which is what produced
// "Client.Timeout exceeded while awaiting headers" on long article drafts.
func TestOllamaGenerate_RequestsStream(t *testing.T) {
	var captured map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&captured)
		w.Header().Set("Content-Type", "application/x-ndjson")
		_, _ = w.Write([]byte(`{"message":{"role":"assistant","content":"Hel"},"done":false}` + "\n"))
		_, _ = w.Write([]byte(`{"message":{"role":"assistant","content":"lo"},"done":false}` + "\n"))
		_, _ = w.Write([]byte(`{"message":{"role":"assistant","content":""},"done":true,"prompt_eval_count":3,"eval_count":2}` + "\n"))
	}))
	defer srv.Close()

	resp, err := NewOllamaClient(srv.URL, "llama3").Generate(context.Background(), GenerateRequest{
		Messages: []Message{{Role: RoleUser, Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if stream, _ := captured["stream"].(bool); !stream {
		t.Fatalf("stream = %v, want true; request was %#v", captured["stream"], captured)
	}
	if resp.Content != "Hello" {
		t.Errorf("content = %q, want Hello from concatenated chunks", resp.Content)
	}
	if resp.InputTokens != 3 || resp.OutputTokens != 2 {
		t.Errorf("tokens = %d/%d, want 3/2 from the done chunk", resp.InputTokens, resp.OutputTokens)
	}
}

func TestOllamaGenerate_StreamThinkingOnlyIsNamed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"message":{"role":"assistant","content":"","thinking":"hmm"},"done":true}` + "\n"))
	}))
	defer srv.Close()

	_, err := NewOllamaClient(srv.URL, "reasoner").Generate(context.Background(), GenerateRequest{
		MaxTokens: 10,
		Messages:  []Message{{Role: RoleUser, Content: "hi"}},
	})
	if err == nil {
		t.Fatal("expected an error for a thinking-only reply")
	}
	if !strings.Contains(err.Error(), "only reasoning") {
		t.Errorf("error %q should name the thinking-only failure", err)
	}
}

func TestOllamaGenerate_OnTokenReceivesEachChunk(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"message":{"role":"assistant","content":"Hel"},"done":false}` + "\n"))
		_, _ = w.Write([]byte(`{"message":{"role":"assistant","content":"lo"},"done":false}` + "\n"))
		_, _ = w.Write([]byte(`{"message":{"role":"assistant","content":""},"done":true}` + "\n"))
	}))
	defer srv.Close()

	var chunks []string
	resp, err := NewOllamaClient(srv.URL, "llama3").Generate(context.Background(), GenerateRequest{
		Messages: []Message{{Role: RoleUser, Content: "hi"}},
		OnToken:  func(s string) { chunks = append(chunks, s) },
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if len(chunks) != 2 || chunks[0] != "Hel" || chunks[1] != "lo" {
		t.Fatalf("chunks = %#v, want [Hel lo]", chunks)
	}
	if resp.Content != "Hello" {
		t.Errorf("content = %q, want Hello", resp.Content)
	}
}
