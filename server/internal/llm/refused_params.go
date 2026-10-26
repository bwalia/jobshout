package llm

import (
	"bytes"
	"net/http"
	"sync"
)

// refusedParams remembers, per model, the request parameters a hosted provider
// refused with a 400.
//
// Hosted models drop parameters over time: current Claude models and OpenAI's
// reasoning models refuse a custom temperature, and OpenAI's newer models
// refuse max_tokens, asking for max_completion_tokens instead. A client that
// always sends the same fields works only for the models it was written
// against. These clients instead retry a 400 that names such a parameter once
// without it, and remember the refusal — so any model works by name alone, set
// in the environment, at the cost of one refused request per model per process.
var refusedParams sync.Map

// refused reports whether model has already refused the parameter named key.
func refused(provider, model, key string) bool {
	_, ok := refusedParams.Load(provider + "|" + model + "|" + key)
	return ok
}

// refuses reports whether a failed request's 400 body names key, and if so
// remembers that model refuses it.
func refuses(provider, model string, status int, body []byte, key string) bool {
	if status != http.StatusBadRequest || !bytes.Contains(body, []byte(key)) {
		return false
	}
	refusedParams.Store(provider+"|"+model+"|"+key, true)
	return true
}

// minThinkingTokens is the smallest output ceiling sent to a model that may
// think first. Thinking is spent from the same budget as the answer, so a
// caller's small ceiling (16 tokens for a one-word label) can leave nothing for
// the answer itself. It is a ceiling, not a target: a short answer still stops
// early and costs only what it used.
const minThinkingTokens = 4096
