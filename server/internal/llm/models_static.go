package llm

import "slices"

// StaticModels returns the hand-maintained model list for a provider that has no
// usable discovery here. It is only the fallback: a ring offers whatever its
// <PROVIDER>_MODELS setting names (see ConfiguredModels).
//
// OpenAI's /v1/models reports neither a context window nor tool capability —
// the only two facts this list exists to supply — and lists audio, image and
// embedding models alongside chat ones, so it is deliberately not called.
//
// This lives in llm, not modelselect, because the dependency runs
// modelselect -> llm: the lower package owns the facts (id, context window,
// capabilities) and the upper one adds the judgements (quality, speed). Keeping
// them apart is what stops the two catalogs drifting, as they had.
func StaticModels(provider string) []ModelInfo {
	switch provider {
	case "openai":
		// The first entry's facts are also lent to any model this list does not
		// name (ConfiguredModels), so each list leads with a typical model.
		return []ModelInfo{
			{Provider: "openai", Name: "gpt-4o", ContextTokens: 128_000,
				Capabilities: []string{CapCompletion, CapTools, CapVision}},
			{Provider: "openai", Name: "gpt-5", ContextTokens: 400_000,
				Capabilities: []string{CapCompletion, CapTools, CapVision, CapThinking}},
			{Provider: "openai", Name: "gpt-5-mini", ContextTokens: 400_000,
				Capabilities: []string{CapCompletion, CapTools, CapVision, CapThinking}},
			{Provider: "openai", Name: "gpt-4.1", ContextTokens: 1_047_576,
				Capabilities: []string{CapCompletion, CapTools, CapVision}},
			{Provider: "openai", Name: "gpt-4o-mini", ContextTokens: 128_000,
				Capabilities: []string{CapCompletion, CapTools, CapVision}},
		}
	case "claude":
		return []ModelInfo{
			{Provider: "claude", Name: "claude-sonnet-5-5", ContextTokens: 1_000_000,
				Capabilities: []string{CapCompletion, CapTools, CapVision, CapThinking}},
			{Provider: "claude", Name: "claude-opus-5-5", ContextTokens: 1_000_000,
				Capabilities: []string{CapCompletion, CapTools, CapVision, CapThinking}},
			{Provider: "claude", Name: "claude-haiku-5-5", ContextTokens: 1_000_000,
				Capabilities: []string{CapCompletion, CapTools, CapVision, CapThinking}},
		}
	case "gemini":
		// Gemini's /models lists every model the key can reach, image and
		// embedding ones included, without a tool flag — so, like OpenAI and
		// Claude, it is listed by hand.
		return []ModelInfo{
			{Provider: "gemini", Name: GeminiDefaultModel, ContextTokens: 1_048_576,
				Capabilities: []string{CapCompletion, CapVision, CapThinking}},
			{Provider: "gemini", Name: "gemini-2.5-flash", ContextTokens: 1_048_576,
				Capabilities: []string{CapCompletion, CapVision, CapThinking}},
			{Provider: "gemini", Name: "gemini-2.5-pro", ContextTokens: 1_048_576,
				Capabilities: []string{CapCompletion, CapVision, CapThinking}},
		}
	default:
		return nil
	}
}

// ConfiguredModels is what a hosted provider offers in the model picker: names
// (its <PROVIDER>_MODELS setting), or the built-in list when names is empty,
// led by defaultModel whether or not it was listed.
//
// A model the built-in list knows keeps its facts; any other borrows the facts
// of the provider's first built-in model. That is what lets a new model be
// offered by setting an environment variable alone.
func ConfiguredModels(provider, defaultModel string, names []string) []ModelInfo {
	builtin := StaticModels(provider)
	if len(builtin) == 0 {
		return nil
	}
	if len(names) == 0 {
		for _, m := range builtin {
			names = append(names, m.Name)
		}
	}
	if defaultModel != "" {
		rest := slices.DeleteFunc(slices.Clone(names), func(n string) bool { return n == defaultModel })
		names = append([]string{defaultModel}, rest...)
	}
	out := make([]ModelInfo, 0, len(names))
	for _, n := range names {
		m := builtin[0]
		if i := slices.IndexFunc(builtin, func(b ModelInfo) bool { return b.Name == n }); i >= 0 {
			m = builtin[i]
		}
		m.Name = n
		out = append(out, m)
	}
	return out
}
