package modelselect

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/jobshout/server/internal/llm"
)

// ModelSource is the subset of llm.Router the dynamic catalog needs.
//
// It is deliberately the NON-BLOCKING accessor: auto-selection runs on the
// execution path, where a network round trip per call would cost far more than
// a slightly stale model list is worth.
type ModelSource interface {
	CachedModels() []llm.ProviderModels
}

// LiveCatalog builds a dynamic catalog source from a router's discovery cache.
//
// maxContext is the ceiling the Ollama client will actually request via num_ctx.
// Applying it here as well is what keeps the selector's belief about a context
// window equal to what the call will really be given — without it the selector
// would approve a 200k prompt for a model that gets silently truncated.
func LiveCatalog(src ModelSource, maxContext int) func() []Candidate {
	return func() []Candidate {
		var out []Candidate
		for _, pm := range src.CachedModels() {
			for _, m := range pm.Models {
				if c, ok := FromModelInfo(m, maxContext); ok {
					out = append(out, c)
				}
			}
		}
		return out
	}
}

// FromModelInfo converts a discovered model into a Candidate.
//
// It reports false for models that cannot be selected at all: embedding-only
// models, and anything with no usable name.
func FromModelInfo(m llm.ModelInfo, maxContext int) (Candidate, bool) {
	if m.Name == "" || m.IsEmbeddingOnly() {
		return Candidate{}, false
	}

	ctx := m.ContextTokens
	if ctx <= 0 {
		// The provider did not say. Assume small, so an unknown model only wins
		// tiny prompts — the right direction to be wrong in.
		ctx = 4096
	}
	// Local models are subject to the num_ctx ceiling; cloud models are not,
	// because their context window is whatever the API grants.
	if m.Provider == "ollama" && maxContext > 0 && ctx > maxContext {
		ctx = maxContext
	}

	quality, speed := qualitySpeedFor(m)

	return Candidate{
		Provider:      m.Provider,
		Model:         m.Name,
		ContextTokens: ctx,
		SupportsTools: m.SupportsTools(),
		Quality:       quality,
		Speed:         speed,
	}, true
}

// mergeCatalogs combines the static catalog with a dynamic one. On a
// provider+model collision the dynamic entry wins, because a live probe knows
// more about a model than a literal written months ago.
func mergeCatalogs(static, dynamic []Candidate) []Candidate {
	if len(dynamic) == 0 {
		return static
	}

	key := func(c Candidate) string { return c.Provider + ":" + c.Model }
	seen := make(map[string]bool, len(dynamic))
	out := make([]Candidate, 0, len(static)+len(dynamic))

	for _, c := range dynamic {
		seen[key(c)] = true
		out = append(out, c)
	}
	for _, c := range static {
		if seen[key(c)] {
			continue
		}
		// A provider with live discovery has spoken for itself; keeping stale
		// static entries for it would resurrect models that are not installed.
		if hasProvider(dynamic, c.Provider) {
			continue
		}
		out = append(out, c)
	}
	return out
}

func hasProvider(cs []Candidate, provider string) bool {
	for _, c := range cs {
		if c.Provider == provider {
			return true
		}
	}
	return false
}

// ─── Quality / Speed heuristic ──────────────────────────────────────────────

// maxLocalQuality caps what the size heuristic may claim for a locally
// discovered model.
//
// Parameter count is a weak proxy for reasoning, and over-trusting it buys a bad
// plan or bad code — which costs far more than the tokens a local model saves.
// At 6 a local model can take classify (bar 3), summarize (3), step (4),
// reflect (4) and chat (5), but never code (7) or plan (8). That is exactly the
// split where being wrong is cheap.
const maxLocalQuality = 6

// qualitySpeedFor scores a discovered model.
//
// For providers with a hand-maintained catalog entry (OpenAI, Claude) the known
// values win. For everything else the score comes from parameter count.
//
// BE HONEST ABOUT THIS TABLE: it has no measurement behind it and is known to be
// wrong in at least three ways.
//  1. Parameter count ignores model age and training quality — a 2026 8B model
//     beats a 2023 70B on most benchmarks and this cannot see that.
//  2. It ignores quantization, though Quantization is right there on ModelInfo.
//  3. It misreads mixture-of-experts models badly, which is why Speed is
//     adjusted for them below.
//
// What makes it defensible rather than reckless: maxLocalQuality means a wrong
// score can only ever misroute cheap work, and this is a pure function with a
// table-driven test, so it is falsifiable. Tune it here and nowhere else.
func qualitySpeedFor(m llm.ModelInfo) (quality, speed int) {
	if known, ok := knownQualitySpeed[m.Provider+":"+m.Name]; ok {
		return known.quality, known.speed
	}

	billions, ok := parseParameterSize(m.ParameterSize)
	if !ok {
		// Unknown size: assume small and fast rather than strong and slow, so an
		// unidentified model is only trusted with routine work.
		return 3, 8
	}

	switch {
	case billions < 1:
		quality, speed = 1, 10
	case billions < 3:
		quality, speed = 2, 10
	case billions < 8:
		quality, speed = 3, 9
	case billions < 15:
		// Calibrated so llama3:latest and llama3.1:8b score the same 4 the
		// hand-written catalog entry already gave them — no behaviour change
		// for the model that was already there.
		quality, speed = 4, 8
	case billions < 35:
		quality, speed = 6, 6
	case billions < 80:
		quality, speed = 7, 4
	default:
		quality, speed = 8, 2
	}

	// Mixture-of-experts models activate a fraction of their parameters, so they
	// run far closer to their active size than their total. qwen3:30b-a3b is
	// 30.5B total but ~3B active — scoring it at 30B speed is simply wrong.
	if isMoE(m) {
		speed = min(speed+2, 10)
	}

	if quality > maxLocalQuality {
		quality = maxLocalQuality
	}
	return quality, speed
}

// knownQualitySpeed carries the judgements for models we have a considered
// opinion about. Local models are deliberately absent: populating this from
// vibes is how a catalog becomes unmaintainable. Add an entry only with a
// reason.
var knownQualitySpeed = map[string]struct{ quality, speed int }{
	"openai:gpt-4o":                     {8, 6},
	"openai:gpt-4-turbo":                {8, 4},
	"openai:gpt-4o-mini":                {5, 9},
	"openai:gpt-3.5-turbo":              {3, 10},
	"claude:claude-opus-4-20250514":     {10, 3},
	"claude:claude-sonnet-4-20250514":   {9, 6},
	"claude:claude-3-5-sonnet-20241022": {8, 6},
	"claude:claude-3-5-haiku-20241022":  {6, 9},
	"claude:claude-3-haiku-20240307":    {4, 10},
}

// moeSuffix matches Ollama's active-parameter naming, e.g. "qwen3:30b-a3b".
var moeSuffix = regexp.MustCompile(`-a\d+(\.\d+)?b`)

func isMoE(m llm.ModelInfo) bool {
	return strings.Contains(strings.ToLower(m.Family), "moe") ||
		moeSuffix.MatchString(strings.ToLower(m.Name))
}

// parseParameterSize converts Ollama's "30.5B" / "23M" into billions.
func parseParameterSize(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}

	multiplier := 1.0
	switch {
	case strings.HasSuffix(strings.ToUpper(s), "B"):
		s = s[:len(s)-1]
	case strings.HasSuffix(strings.ToUpper(s), "M"):
		s = s[:len(s)-1]
		multiplier = 1.0 / 1000.0
	case strings.HasSuffix(strings.ToUpper(s), "K"):
		s = s[:len(s)-1]
		multiplier = 1.0 / 1_000_000.0
	}

	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0, false
	}
	return v * multiplier, true
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
