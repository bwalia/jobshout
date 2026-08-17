package blog

// The Article Writer makes two kinds of call, and a benchmark found that
// different models are better at each. Rather than leave that knowledge in a
// commit message, it is published to the UI so the model picker can say which
// choice the evidence supports — and so the reason travels with the
// recommendation instead of being folklore.
//
// A recommendation is advice, never a default. The picker labels the suggested
// model; it does not select it, and choosing anything else is a normal thing to
// do. Speed, cost and what a given box has pulled are all reasons to differ.

// ModelRole is a kind of call the writing pipeline makes.
type ModelRole string

const (
	// RoleProse covers drafting, revising and expanding — the article's text,
	// and the diagrams, which are written in the same call.
	RoleProse ModelRole = "prose"
	// RoleStructured covers planning and review, which must come back as JSON.
	RoleStructured ModelRole = "structured"
)

// ModelRecommendation is the suggested model for one role and why.
type ModelRecommendation struct {
	Role ModelRole `json:"role"`
	// Label is what the setting is called in the UI.
	Label string `json:"label"`
	// Model is the suggested model name. Empty means there is no evidence-backed
	// suggestion, and the UI should not invent one.
	Model string `json:"model"`
	// Reason is one short sentence a person can act on.
	Reason string `json:"reason"`
	// Caveat is the cost of taking the advice, where there is one. Shown
	// alongside the reason so the trade-off is visible at the point of choosing
	// rather than discovered afterwards.
	Caveat string `json:"caveat,omitempty"`
	// Describes which calls the setting governs.
	Covers string `json:"covers"`
}

// EffectiveModels reports which model each role uses when neither the agent nor
// the run names one — that is, what the server was started with.
//
// The UI shows this behind an unset picker, so "no choice made" reads as the
// model it will actually use rather than as an empty box.
func (r *Runner) EffectiveModels() map[string]string {
	return map[string]string{
		string(RoleProse):      r.proseModel(GenerateRequest{}),
		string(RoleStructured): r.structuredModel(GenerateRequest{}),
	}
}

// ProviderName is the LLM provider the writing pipeline is bound to.
func (r *Runner) ProviderName() string {
	if r == nil || r.llm == nil {
		return ""
	}
	return r.llm.ProviderName()
}

// RecommendedModels reports the measured recommendation for each role.
//
// The models named here are the ones the benchmark actually ran. On a
// deployment that has never pulled them the UI simply shows no badge, which is
// the honest outcome — a recommendation for a model you do not have is noise.
func RecommendedModels() []ModelRecommendation {
	return []ModelRecommendation{
		{
			Role:   RoleProse,
			Label:  "Writing model",
			Covers: "Writes the article, revises it, and draws the diagrams.",
			Model:  "muse-glimmer:latest",
			Reason: "Scored higher on clarity, accuracy and depth across three runs, " +
				"and did not invent code the way the alternative did.",
			Caveat: "Roughly seven minutes per article instead of one.",
		},
		{
			Role:   RoleStructured,
			Label:  "Structured model",
			Covers: "Chooses the title and outline, and reviews the draft.",
			Model:  "qwen3-coder:30b",
			Reason: "Returned valid JSON on every one of six attempts, and is five " +
				"to eight times faster on these calls.",
		},
	}
}
