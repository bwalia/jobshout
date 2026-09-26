package config

import "testing"

func TestChatModelConstants(t *testing.T) {
	if DefaultChatModel != "qwen3-coder:30b" {
		t.Fatalf("DefaultChatModel = %q", DefaultChatModel)
	}
	if DefaultChatModelFallback != "llama3.1:8b" {
		t.Fatalf("fallback = %q", DefaultChatModelFallback)
	}
	if DefaultChatNumCtx != 16384 {
		t.Fatalf("num_ctx = %d", DefaultChatNumCtx)
	}
	if DefaultChatModel == "llama3:latest" || DefaultChatModelFallback == "llama3:latest" {
		t.Fatal("llama3:latest is forbidden for chat")
	}
}
