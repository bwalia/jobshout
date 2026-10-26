package config

import (
	"slices"
	"strings"
	"testing"
)

func TestLoad_HostedModelSettings(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/test")
	t.Setenv("JWT_SECRET", strings.Repeat("j", 32))
	t.Setenv("CLAUDE_API_KEY", "")
	t.Setenv("ANTHROPIC_API_KEY", "anthropic-name")
	t.Setenv("CLAUDE_MODELS", " claude-opus-5-5, ,claude-haiku-5-5 ")
	t.Setenv("OPENAI_MODELS", "")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ClaudeAPIKey != "anthropic-name" {
		t.Errorf("ANTHROPIC_API_KEY not honoured: %q", cfg.ClaudeAPIKey)
	}
	if !slices.Equal(cfg.ClaudeModels, []string{"claude-opus-5-5", "claude-haiku-5-5"}) {
		t.Errorf("CLAUDE_MODELS = %q", cfg.ClaudeModels)
	}
	if cfg.OpenAIModels != nil {
		t.Errorf("unset OPENAI_MODELS = %q, want nil (built-in list)", cfg.OpenAIModels)
	}
}
