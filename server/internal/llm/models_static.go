package llm

// StaticModels returns the hand-maintained model list for a provider that has no
// usable discovery here.
//
// OpenAI and Claude both expose /v1/models, but neither reports a context window
// or tool capability — the only two facts this list exists to supply. A live call
// would replace good data with worse, so it is deliberately not made.
//
// This lives in llm, not modelselect, because the dependency runs
// modelselect -> llm: the lower package owns the facts (id, context window,
// capabilities) and the upper one adds the judgements (quality, speed). Keeping
// them apart is what stops the two catalogs drifting, as they had.
func StaticModels(provider string) []ModelInfo {
	switch provider {
	case "openai":
		return []ModelInfo{
			{Provider: "openai", Name: "gpt-4o", ContextTokens: 128_000,
				Capabilities: []string{CapCompletion, CapTools, CapVision}},
			{Provider: "openai", Name: "gpt-4-turbo", ContextTokens: 128_000,
				Capabilities: []string{CapCompletion, CapTools, CapVision}},
			{Provider: "openai", Name: "gpt-4o-mini", ContextTokens: 128_000,
				Capabilities: []string{CapCompletion, CapTools, CapVision}},
			{Provider: "openai", Name: "gpt-3.5-turbo", ContextTokens: 16_385,
				Capabilities: []string{CapCompletion, CapTools}},
		}
	case "claude":
		return []ModelInfo{
			{Provider: "claude", Name: "claude-opus-4-20250514", ContextTokens: 200_000,
				Capabilities: []string{CapCompletion, CapTools, CapVision}},
			{Provider: "claude", Name: "claude-sonnet-4-20250514", ContextTokens: 200_000,
				Capabilities: []string{CapCompletion, CapTools, CapVision}},
			{Provider: "claude", Name: "claude-3-5-sonnet-20241022", ContextTokens: 200_000,
				Capabilities: []string{CapCompletion, CapTools, CapVision}},
			{Provider: "claude", Name: "claude-3-5-haiku-20241022", ContextTokens: 200_000,
				Capabilities: []string{CapCompletion, CapTools}},
			{Provider: "claude", Name: "claude-3-haiku-20240307", ContextTokens: 200_000,
				Capabilities: []string{CapCompletion, CapTools, CapVision}},
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
