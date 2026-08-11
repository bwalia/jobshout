// Package modelselect chooses which LLM provider and model should run a given
// task, so agents can be configured for "auto" instead of being pinned to one
// model for every kind of work.
//
// The selection is deliberately data-driven: everything the selector reasons
// about lives in the Candidate catalog below, and the scoring rules read those
// fields. Adding a model should never require touching the selection logic, and
// the logic should never branch on a provider name.
//
// Pricing is NOT duplicated here. Cost comes from internal/costengine, which is
// already the single source of truth for what a call is worth.
package modelselect

// Candidate is one model the selector may choose, along with everything it
// needs to reason about the model without knowing which provider it belongs to.
type Candidate struct {
	// Provider matches the names registered in llm.Router ("ollama", "openai",
	// "claude"). A candidate whose provider is not registered is never chosen.
	Provider string
	// Model is the provider-specific model id, passed through as
	// llm.GenerateRequest.Model.
	Model string
	// ContextTokens is the model's total context window. The selector reserves
	// headroom for the response on top of the prompt estimate.
	ContextTokens int
	// SupportsTools reports whether the model can do native tool-calling. This
	// is the model's own capability; the client must also advertise it (see
	// llm.ToolCapableClient), and the selector requires both.
	SupportsTools bool
	// Quality ranks reasoning strength, 1 (weakest) to 10 (strongest). Used to
	// meet the minimum bar a task kind demands.
	Quality int
	// Speed ranks responsiveness, 1 (slowest) to 10 (fastest). Used to break
	// ties between candidates that clear the quality bar at equal cost.
	Speed int
}

// DefaultCatalog returns the built-in candidate set, ordered strongest-first
// within each provider purely for readability — the selector sorts explicitly
// and does not depend on this order.
//
// Model ids must match the keys used in costengine's catalog, or cost-based
// filtering silently falls back to the provider wildcard and stops discriminating.
func DefaultCatalog() []Candidate {
	return []Candidate{
		// ── OpenAI ──────────────────────────────────────────────────────────
		{Provider: "openai", Model: "gpt-4o", ContextTokens: 128_000, SupportsTools: true, Quality: 8, Speed: 6},
		{Provider: "openai", Model: "gpt-4-turbo", ContextTokens: 128_000, SupportsTools: true, Quality: 8, Speed: 4},
		{Provider: "openai", Model: "gpt-4o-mini", ContextTokens: 128_000, SupportsTools: true, Quality: 5, Speed: 9},
		{Provider: "openai", Model: "gpt-3.5-turbo", ContextTokens: 16_385, SupportsTools: true, Quality: 3, Speed: 10},

		// ── Anthropic / Claude ──────────────────────────────────────────────
		{Provider: "claude", Model: "claude-opus-4-20250514", ContextTokens: 200_000, SupportsTools: true, Quality: 10, Speed: 3},
		{Provider: "claude", Model: "claude-sonnet-4-20250514", ContextTokens: 200_000, SupportsTools: true, Quality: 9, Speed: 6},
		{Provider: "claude", Model: "claude-3-5-sonnet-20241022", ContextTokens: 200_000, SupportsTools: true, Quality: 8, Speed: 6},
		{Provider: "claude", Model: "claude-3-5-haiku-20241022", ContextTokens: 200_000, SupportsTools: true, Quality: 6, Speed: 9},
		{Provider: "claude", Model: "claude-3-haiku-20240307", ContextTokens: 200_000, SupportsTools: true, Quality: 4, Speed: 10},

		// ── Ollama (local) ──────────────────────────────────────────────────
		// Free to run, so the selector reaches for these first whenever they
		// clear the quality bar. No native tool-calling (llm.OllamaClient
		// reports SupportsTools() == false), so tool tasks skip them.
		{Provider: "ollama", Model: "llama3:latest", ContextTokens: 8_192, SupportsTools: false, Quality: 4, Speed: 7},
	}
}

// TaskKind names the sort of work a request represents. Unknown kinds fall back
// to KindStep's requirements rather than erroring, so a new call site that
// forgets to classify itself still gets a reasonable model.
const (
	KindClassify  = "classify"
	KindSummarize = "summarize"
	KindChat      = "chat"
	KindStep      = "step"
	KindCode      = "code"
	KindPlan      = "plan"
	KindReflect   = "reflect"
)

// minQualityFor returns the lowest Quality a candidate may have and still be
// trusted with this kind of task.
//
// The bar is what makes Auto cheap: most work is classification, summarizing
// and single steps, which small models handle well. Planning and code get a
// higher bar because a bad plan costs more than the tokens saved making it,
// and chat sits one notch above a step because its output is read directly by
// a person rather than consumed by the next step.
func minQualityFor(kind string) int {
	switch kind {
	case KindClassify, KindSummarize:
		return 3
	case KindStep, KindReflect:
		return 4
	case KindChat:
		return 5
	case KindCode:
		return 7
	case KindPlan:
		return 8
	default:
		return 4
	}
}
