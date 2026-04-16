// Package costengine calculates USD cost for LLM API calls.
// It maintains a catalog of per-model pricing and supports per-token pricing
// (OpenAI, Anthropic) and per-compute-second pricing (Ollama/self-hosted).
package costengine

import "sync"

// PricingEntry holds the unit costs for a single model.
type PricingEntry struct {
	// InputPricePerMToken is USD per 1 million input tokens (token-based models).
	InputPricePerMToken float64
	// OutputPricePerMToken is USD per 1 million output tokens (token-based models).
	OutputPricePerMToken float64
	// ComputePricePerSec is USD per second of compute time (self-hosted models).
	ComputePricePerSec float64
}

// Engine calculates cost given a provider, model, tokens, and latency.
type Engine struct {
	mu      sync.RWMutex
	catalog map[string]PricingEntry // key: "provider:model"
}

// New creates an Engine pre-populated with known model pricing.
func New() *Engine {
	e := &Engine{catalog: make(map[string]PricingEntry)}
	e.loadDefaults()
	return e
}

// Calculate returns the estimated cost in USD for a single LLM call.
// For token-based models the formula is:
//
//	cost = (inputTokens / 1_000_000 * inputRate) + (outputTokens / 1_000_000 * outputRate)
//
// For compute-based models (e.g. Ollama) it uses latencyMs:
//
//	cost = (latencyMs / 1000) * computeRate
//
// Returns 0 for unknown models (graceful degradation).
func (e *Engine) Calculate(provider, model string, inputTokens, outputTokens, latencyMs int) float64 {
	e.mu.RLock()
	defer e.mu.RUnlock()

	// Try exact match first, then provider-level fallback.
	entry, ok := e.catalog[catalogKey(provider, model)]
	if !ok {
		entry, ok = e.catalog[catalogKey(provider, "*")]
		if !ok {
			return 0
		}
	}

	var cost float64

	// Token-based pricing.
	if entry.InputPricePerMToken > 0 || entry.OutputPricePerMToken > 0 {
		cost += float64(inputTokens) / 1_000_000 * entry.InputPricePerMToken
		cost += float64(outputTokens) / 1_000_000 * entry.OutputPricePerMToken
	}

	// Compute-based pricing (additive — allows hybrid pricing).
	if entry.ComputePricePerSec > 0 && latencyMs > 0 {
		cost += float64(latencyMs) / 1000 * entry.ComputePricePerSec
	}

	return cost
}

// Register adds or replaces a pricing entry. Thread-safe.
func (e *Engine) Register(provider, model string, entry PricingEntry) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.catalog[catalogKey(provider, model)] = entry
}

// CalculateWithOverride checks for a tenant-specific PricingEntry first.
// If override is non-nil, it takes precedence over the in-memory catalog.
func (e *Engine) CalculateWithOverride(override *PricingEntry, provider, model string, inputTokens, outputTokens, latencyMs int) float64 {
	if override != nil {
		return computeCost(*override, inputTokens, outputTokens, latencyMs)
	}
	return e.Calculate(provider, model, inputTokens, outputTokens, latencyMs)
}

// LoadFromConfigs bulk-loads pricing entries (e.g. from a database).
func (e *Engine) LoadFromConfigs(entries []PricingEntry, keys []string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for i, key := range keys {
		if i < len(entries) {
			e.catalog[key] = entries[i]
		}
	}
}

// KnownModels returns all catalog keys.
func (e *Engine) KnownModels() []string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	keys := make([]string, 0, len(e.catalog))
	for k := range e.catalog {
		keys = append(keys, k)
	}
	return keys
}

func catalogKey(provider, model string) string {
	return provider + ":" + model
}

func computeCost(entry PricingEntry, inputTokens, outputTokens, latencyMs int) float64 {
	var cost float64
	if entry.InputPricePerMToken > 0 || entry.OutputPricePerMToken > 0 {
		cost += float64(inputTokens) / 1_000_000 * entry.InputPricePerMToken
		cost += float64(outputTokens) / 1_000_000 * entry.OutputPricePerMToken
	}
	if entry.ComputePricePerSec > 0 && latencyMs > 0 {
		cost += float64(latencyMs) / 1000 * entry.ComputePricePerSec
	}
	return cost
}

// loadDefaults populates the catalog with well-known model pricing.
// Prices as of early 2025 — update periodically.
func (e *Engine) loadDefaults() {
	// ── OpenAI ──────────────────────────────────────────────────────────
	e.catalog["openai:gpt-4o"] = PricingEntry{
		InputPricePerMToken:  2.50,
		OutputPricePerMToken: 10.00,
	}
	e.catalog["openai:gpt-4o-mini"] = PricingEntry{
		InputPricePerMToken:  0.15,
		OutputPricePerMToken: 0.60,
	}
	e.catalog["openai:gpt-4-turbo"] = PricingEntry{
		InputPricePerMToken:  10.00,
		OutputPricePerMToken: 30.00,
	}
	e.catalog["openai:gpt-3.5-turbo"] = PricingEntry{
		InputPricePerMToken:  0.50,
		OutputPricePerMToken: 1.50,
	}

	// ── Anthropic / Claude ──────────────────────────────────────────────
	e.catalog["claude:claude-sonnet-4-20250514"] = PricingEntry{
		InputPricePerMToken:  3.00,
		OutputPricePerMToken: 15.00,
	}
	e.catalog["claude:claude-3-5-sonnet-20241022"] = PricingEntry{
		InputPricePerMToken:  3.00,
		OutputPricePerMToken: 15.00,
	}
	e.catalog["claude:claude-3-haiku-20240307"] = PricingEntry{
		InputPricePerMToken:  0.25,
		OutputPricePerMToken: 1.25,
	}
	e.catalog["claude:claude-3-5-haiku-20241022"] = PricingEntry{
		InputPricePerMToken:  1.00,
		OutputPricePerMToken: 5.00,
	}
	e.catalog["claude:claude-opus-4-20250514"] = PricingEntry{
		InputPricePerMToken:  15.00,
		OutputPricePerMToken: 75.00,
	}

	// ── Ollama (local, compute-based) ───────────────────────────────────
	// Default: $0 for local usage. Users can override via Register().
	e.catalog["ollama:*"] = PricingEntry{
		ComputePricePerSec: 0,
	}
}
