package llm

import (
	"os"
	"strconv"
	"sync"
)

// Sampling defaults make model output consistent without each agent setting a
// temperature by hand. They answer one production problem: a structured call
// (JSON extraction, scoring, a yes/no judgement) left at a provider's default
// temperature — 0.8 for Ollama, 1.0 for Gemini — gives a different answer each
// run and sometimes unparseable JSON. A low temperature on those calls makes
// them repeatable and more likely to parse.
//
// The rule, applied to every provider in one place:
//   - A call that already names a temperature keeps it. The agent wins.
//   - A JSON call with none gets a low default (deterministic-leaning).
//   - A prose call with none is left untouched (temperature 0 → the client
//     omits it → the provider's own default), unless an operator sets one.
//
// Both defaults are environment-tunable, so a ring can dial them without a code
// change: LLM_TEMPERATURE_JSON (default 0.2) and LLM_TEMPERATURE (default 0,
// meaning "leave prose at the provider default").
const (
	defaultJSONTemperature  = 0.2
	defaultProseTemperature = 0.0 // 0 = omit, keep the provider's own default
)

var (
	tempOnce  sync.Once
	tempJSON  float64
	tempProse float64
)

func loadSamplingDefaults() {
	tempJSON = envFloat("LLM_TEMPERATURE_JSON", defaultJSONTemperature)
	tempProse = envFloat("LLM_TEMPERATURE", defaultProseTemperature)
}

// resolvedTemperature returns the temperature a provider client should use for
// req: the caller's own value if it set one (non-zero), otherwise the JSON or
// prose default. A returned 0 means "send no temperature", so the provider
// applies its own default.
func resolvedTemperature(req GenerateRequest) float64 {
	if req.Temperature > 0 {
		return req.Temperature
	}
	tempOnce.Do(loadSamplingDefaults)
	if req.JSON {
		return tempJSON
	}
	return tempProse
}

// envFloat reads a float from the environment, falling back to def when unset
// or unparseable. A negative value is treated as unset so an operator cannot
// accidentally send a nonsensical temperature.
func envFloat(name string, def float64) float64 {
	v := os.Getenv(name)
	if v == "" {
		return def
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil || f < 0 {
		return def
	}
	return f
}
