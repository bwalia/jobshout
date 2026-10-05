package llm

import (
	"sync"
	"testing"
)

// reset lets a test re-read the environment, since the real resolver caches once.
func resetSampling() { tempOnce = sync.Once{} }

func TestResolvedTemperature(t *testing.T) {
	resetSampling()
	t.Setenv("LLM_TEMPERATURE_JSON", "")
	t.Setenv("LLM_TEMPERATURE", "")

	// Caller's explicit value always wins, JSON or not.
	if got := resolvedTemperature(GenerateRequest{Temperature: 0.7, JSON: true}); got != 0.7 {
		t.Fatalf("explicit should win, got %v", got)
	}
	// JSON call with no temperature gets the deterministic-leaning default.
	if got := resolvedTemperature(GenerateRequest{JSON: true}); got != defaultJSONTemperature {
		t.Fatalf("json default = %v, want %v", got, defaultJSONTemperature)
	}
	// Prose call with none is left for the provider default (0 = omit).
	if got := resolvedTemperature(GenerateRequest{JSON: false}); got != 0 {
		t.Fatalf("prose default = %v, want 0", got)
	}
}

func TestResolvedTemperature_EnvOverride(t *testing.T) {
	resetSampling()
	t.Setenv("LLM_TEMPERATURE_JSON", "0.05")
	t.Setenv("LLM_TEMPERATURE", "0.6")
	if got := resolvedTemperature(GenerateRequest{JSON: true}); got != 0.05 {
		t.Fatalf("json env override = %v", got)
	}
	if got := resolvedTemperature(GenerateRequest{JSON: false}); got != 0.6 {
		t.Fatalf("prose env override = %v", got)
	}
	resetSampling()
}

func TestEnvFloat(t *testing.T) {
	t.Setenv("X_TEMP", "")
	if got := envFloat("X_TEMP", 0.2); got != 0.2 {
		t.Fatalf("unset = %v", got)
	}
	t.Setenv("X_TEMP", "nonsense")
	if got := envFloat("X_TEMP", 0.2); got != 0.2 {
		t.Fatalf("unparseable = %v", got)
	}
	t.Setenv("X_TEMP", "-1")
	if got := envFloat("X_TEMP", 0.2); got != 0.2 {
		t.Fatalf("negative = %v", got)
	}
	t.Setenv("X_TEMP", "0.9")
	if got := envFloat("X_TEMP", 0.2); got != 0.9 {
		t.Fatalf("set = %v", got)
	}
}
