package llm

import (
	"context"
	"errors"
	"testing"
)

func TestOllamaGenerate_SendsThinkWhenModelSupportsIt(t *testing.T) {
	srv, captured := ollamaToolServer(t, []string{"completion", "thinking"}, []string{
		`{"message":{"role":"assistant","content":"ok"},"done":true}`,
	})
	defer srv.Close()

	_, err := NewOllamaClient(srv.URL, "qwen3-coder:30b").Generate(context.Background(), GenerateRequest{
		Messages: []Message{{Role: RoleUser, Content: "hi"}},
		Think:    true,
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if think, _ := (*captured)["think"].(bool); !think {
		t.Errorf("think = %#v, want true for a thinking-capable model", (*captured)["think"])
	}
}

func TestOllamaGenerate_OmitsThinkForNonThinkingModel(t *testing.T) {
	srv, captured := ollamaToolServer(t, []string{"completion"}, []string{
		`{"message":{"role":"assistant","content":"ok"},"done":true}`,
	})
	defer srv.Close()

	_, err := NewOllamaClient(srv.URL, "qwen3-coder:30b").Generate(context.Background(), GenerateRequest{
		Messages: []Message{{Role: RoleUser, Content: "hi"}},
		Think:    true,
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if think, _ := (*captured)["think"].(bool); think {
		t.Error("think sent to a model without the thinking capability")
	}
}

func TestOllamaGenerate_NeverThinksUnrequested(t *testing.T) {
	srv, captured := ollamaToolServer(t, []string{"completion", "thinking"}, []string{
		`{"message":{"role":"assistant","content":"ok"},"done":true}`,
	})
	defer srv.Close()

	_, err := NewOllamaClient(srv.URL, "qwen3-coder:30b").Generate(context.Background(), GenerateRequest{
		Messages: []Message{{Role: RoleUser, Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if think, _ := (*captured)["think"].(bool); think {
		t.Error("think sent although the request did not ask for it")
	}
}

func TestOllamaGenerate_OnlyThinkingIsNamedError(t *testing.T) {
	srv, _ := ollamaToolServer(t, []string{"completion", "thinking"}, []string{
		`{"message":{"role":"assistant","content":"","thinking":"hmm, let me consider"},"done":true}`,
	})
	defer srv.Close()

	_, err := NewOllamaClient(srv.URL, "qwen3-coder:30b").Generate(context.Background(), GenerateRequest{
		Messages: []Message{{Role: RoleUser, Content: "hi"}},
		Think:    true,
	})
	if !errors.Is(err, ErrOnlyThinking) {
		t.Fatalf("want ErrOnlyThinking, got %v", err)
	}
}

func TestOllamaGenerate_JSONSendsFormatAndDisablesThinking(t *testing.T) {
	srv, captured := ollamaToolServer(t, []string{"completion", "thinking"}, []string{
		`{"message":{"role":"assistant","content":"{}"},"done":true}`,
	})
	defer srv.Close()

	_, err := NewOllamaClient(srv.URL, "qwen3:30b-a3b").Generate(context.Background(), GenerateRequest{
		Messages: []Message{{Role: RoleUser, Content: "hi"}},
		Think:    true,
		JSON:     true,
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if got := (*captured)["format"]; got != "json" {
		t.Errorf("format = %#v, want \"json\"", got)
	}
	if think, _ := (*captured)["think"].(bool); think {
		t.Error("think sent on a JSON request; reasoning would precede the JSON")
	}
}

func TestOllamaGenerate_OmitsFormatForFreeText(t *testing.T) {
	srv, captured := ollamaToolServer(t, []string{"completion"}, []string{
		`{"message":{"role":"assistant","content":"ok"},"done":true}`,
	})
	defer srv.Close()

	_, err := NewOllamaClient(srv.URL, "qwen3:30b-a3b").Generate(context.Background(), GenerateRequest{
		Messages: []Message{{Role: RoleUser, Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if _, ok := (*captured)["format"]; ok {
		t.Errorf("format sent on a free-text request: %#v", (*captured)["format"])
	}
}
