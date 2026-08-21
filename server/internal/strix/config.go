package strix

import (
	"os"

	"go.uber.org/zap"
)

type Config struct {
	Enabled   bool
	StrixPath string
	RunsDir   string
	LLMModel  string
	LLMKey    string
	// LLMAPIBase overrides the LLM endpoint (LiteLLM's LLM_API_BASE). It is how a
	// local model is used instead of a hosted provider: point it at an Ollama /
	// LMStudio server and set LLMModel to e.g. "ollama_chat/qwen3-coder:30b".
	// Empty means the provider's default endpoint (the hosted APIs).
	LLMAPIBase string
}

func LoadConfig(logger *zap.Logger) Config {
	return Config{
		Enabled:    os.Getenv("STRIX_ENABLED") != "false",
		StrixPath:  getEnvOrDefault("STRIX_PATH", "strix"),
		RunsDir:    getEnvOrDefault("STRIX_RUNS_DIR", "./strix_runs"),
		LLMModel:   os.Getenv("STRIX_LLM"),
		LLMKey:     os.Getenv("STRIX_LLM_API_KEY"),
		LLMAPIBase: os.Getenv("STRIX_LLM_API_BASE"),
	}
}

func getEnvOrDefault(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}
