package costengine

import (
	"math"
	"testing"
)

func TestCalculate_OpenAI(t *testing.T) {
	e := New()

	tests := []struct {
		name         string
		provider     string
		model        string
		inputTokens  int
		outputTokens int
		latencyMs    int
		wantUSD      float64
	}{
		{
			name:         "gpt-4o 1000 input 500 output",
			provider:     "openai",
			model:        "gpt-4o",
			inputTokens:  1000,
			outputTokens: 500,
			wantUSD:      (1000.0/1_000_000)*2.50 + (500.0/1_000_000)*10.00,
		},
		{
			name:         "gpt-4o-mini 10k input 2k output",
			provider:     "openai",
			model:        "gpt-4o-mini",
			inputTokens:  10_000,
			outputTokens: 2_000,
			wantUSD:      (10_000.0/1_000_000)*0.15 + (2_000.0/1_000_000)*0.60,
		},
		{
			name:         "gpt-3.5-turbo",
			provider:     "openai",
			model:        "gpt-3.5-turbo",
			inputTokens:  5000,
			outputTokens: 1000,
			wantUSD:      (5000.0/1_000_000)*0.50 + (1000.0/1_000_000)*1.50,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := e.Calculate(tt.provider, tt.model, tt.inputTokens, tt.outputTokens, tt.latencyMs)
			if !almostEqual(got, tt.wantUSD) {
				t.Errorf("Calculate() = %f, want %f", got, tt.wantUSD)
			}
		})
	}
}

func TestCalculate_Claude(t *testing.T) {
	e := New()

	got := e.Calculate("claude", "claude-3-haiku-20240307", 100_000, 10_000, 0)
	want := (100_000.0/1_000_000)*0.25 + (10_000.0/1_000_000)*1.25
	if !almostEqual(got, want) {
		t.Errorf("Claude haiku: got %f, want %f", got, want)
	}
}

func TestCalculate_OllamaDefaultFree(t *testing.T) {
	e := New()

	// Ollama defaults to $0 (local compute).
	got := e.Calculate("ollama", "llama3", 5000, 2000, 3000)
	if got != 0 {
		t.Errorf("Ollama default should be free, got %f", got)
	}
}

func TestCalculate_OllamaWithCustomPricing(t *testing.T) {
	e := New()

	// Register custom compute pricing for Ollama.
	e.Register("ollama", "*", PricingEntry{ComputePricePerSec: 0.001})

	got := e.Calculate("ollama", "llama3", 5000, 2000, 3000) // 3 seconds
	want := 3.0 * 0.001
	if !almostEqual(got, want) {
		t.Errorf("Ollama custom pricing: got %f, want %f", got, want)
	}
}

func TestCalculate_UnknownModelReturnsZero(t *testing.T) {
	e := New()

	got := e.Calculate("unknown_provider", "unknown_model", 1000, 500, 1000)
	if got != 0 {
		t.Errorf("Unknown model should return 0, got %f", got)
	}
}

func TestRegister_OverridesExisting(t *testing.T) {
	e := New()

	// Override gpt-4o pricing.
	e.Register("openai", "gpt-4o", PricingEntry{
		InputPricePerMToken:  5.00,
		OutputPricePerMToken: 20.00,
	})

	got := e.Calculate("openai", "gpt-4o", 1_000_000, 1_000_000, 0)
	want := 5.00 + 20.00
	if !almostEqual(got, want) {
		t.Errorf("After override: got %f, want %f", got, want)
	}
}

func TestKnownModels(t *testing.T) {
	e := New()

	models := e.KnownModels()
	if len(models) == 0 {
		t.Error("KnownModels() returned empty list")
	}

	// Verify at least a few expected entries.
	found := map[string]bool{}
	for _, m := range models {
		found[m] = true
	}
	for _, expected := range []string{"openai:gpt-4o", "claude:claude-3-haiku-20240307", "ollama:*"} {
		if !found[expected] {
			t.Errorf("KnownModels() missing expected entry %q", expected)
		}
	}
}

func TestCalculate_ZeroTokens(t *testing.T) {
	e := New()

	got := e.Calculate("openai", "gpt-4o", 0, 0, 0)
	if got != 0 {
		t.Errorf("Zero tokens should cost $0, got %f", got)
	}
}

func almostEqual(a, b float64) bool {
	return math.Abs(a-b) < 1e-10
}
