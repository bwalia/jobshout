package strix

import (
	"os"

	"go.uber.org/zap"
)

type Config struct {
	Enabled    bool
	StrixPath  string
	RunsDir    string
	LLMModel   string
	LLMKey     string
}

func LoadConfig(logger *zap.Logger) Config {
	return Config{
		Enabled:   os.Getenv("STRIX_ENABLED") != "false",
		StrixPath: getEnvOrDefault("STRIX_PATH", "strix"),
		RunsDir:   getEnvOrDefault("STRIX_RUNS_DIR", "./strix_runs"),
		LLMModel:  os.Getenv("STRIX_LLM"),
		LLMKey:    os.Getenv("STRIX_LLM_API_KEY"),
	}
}

func getEnvOrDefault(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}
