package blog

import (
	"fmt"
	"strings"
	"testing"

	"github.com/jobshout/server/eval/harness"
)

// This is the evaluation that proves the cover upgrade landed: covers must read
// as one publication (the pinned axes) while being visibly different articles
// (the varying axes).
//
// It lives in package blog rather than under server/eval because coverPrompt is
// unexported, and exporting it purely so an eval could reach it would widen the
// package's API for a test's convenience. The report is still written through
// the shared harness, so it lands in the same place as the other suites.

// evalTopics are the subjects the diversity suite draws covers for. Twelve
// real-sounding topics rather than "topic 1".."topic 12": the variant is keyed
// on the topic string, so degenerate inputs would measure the hash rather than
// the behaviour.
var evalTopics = []string{
	"kubernetes networking",
	"postgres query planning",
	"llm inference cost control",
	"terraform state management",
	"observability with opentelemetry",
	"rust memory safety",
	"ci pipeline caching",
	"event driven architecture",
	"zero downtime database migrations",
	"webassembly at the edge",
	"secrets management in ci",
	"graph databases for fraud detection",
}

// brandMarkers must appear in every cover prompt. These are the axes that make
// the covers a set; a change that drops one is a regression even if the covers
// become more varied.
var brandMarkers = []string{
	"charcoal navy",
	"flat vector",
	"16:9",
	"dark-mode",
	"No logos",
}

func TestCoverDiversityEval(t *testing.T) {
	suite := harness.NewSuite(t, "article-covers", ".")

	prompts := make([]string, 0, len(evalTopics))
	variants := make([]coverVariant, 0, len(evalTopics))
	for _, topic := range evalTopics {
		prompts = append(prompts, coverPrompt(titleFor(topic), topic))
		variants = append(variants, variantFor(topic))
	}

	rep := suite.Case(t, "cover_prompts", []harness.Check{
		{Name: "covers_share_the_brand", Fatal: true, Fn: func() error {
			for i, p := range prompts {
				if err := harness.RequireContains(
					fmt.Sprintf("cover for %q", evalTopics[i]), p, brandMarkers...); err != nil {
					return err
				}
			}
			return nil
		}},
		{Name: "covers_differ_from_each_other", Fatal: true, Fn: func() error {
			return harness.RequireAtLeast("distinct cover treatments",
				distinctVariants(variants), 8)
		}},
		{Name: "same_topic_is_stable", Fatal: true, Fn: func() error {
			a := coverPrompt("Kubernetes costs", "kubernetes networking")
			b := coverPrompt("Kubernetes costs", "kubernetes networking")
			return harness.RequireEqual("repeat cover prompt", a, b)
		}},
		{Name: "every_axis_actually_varies", Fatal: true, Fn: func() error {
			for name, vals := range axisValues(variants) {
				if err := harness.RequireAtLeast("distinct "+name, len(vals), 2); err != nil {
					return err
				}
			}
			return nil
		}},
		// Non-fatal: with twelve samples this is a quality signal rather than a
		// contract. It exists to catch the specific mistake of deriving every
		// axis from the same hash without dividing, which collapses the variety.
		{Name: "axes_are_not_perfectly_correlated", Fn: func() error {
			return checkUncorrelated(variants)
		}},
		// The drift guard for the in-body upgrade: the writing prompt must
		// advertise the same budget the pipeline enforces.
		{Name: "prompt_budget_matches_pipeline_cap", Fatal: true, Fn: func() error {
			return harness.RequireContains("illustration rules", illustrationRules,
				fmt.Sprintf("up to %d generated illustrations", maxInlineIllustrations))
		}},
		{Name: "prompt_still_forbids_lettering_in_illustrations", Fatal: true, Fn: func() error {
			return harness.RequireContains("illustration rules", illustrationRules,
				"Do not ask for text, labels, diagrams, charts or UI")
		}},
	})

	rep.Note("Drew %d cover prompts across %d distinct treatments.",
		len(prompts), distinctVariants(variants))
	for i, topic := range evalTopics {
		rep.Note("%s → accent %q, %s, %s", topic,
			variants[i].accent, variants[i].composition, variants[i].lighting)
	}
}

func titleFor(topic string) string {
	return strings.ToUpper(topic[:1]) + topic[1:]
}

func distinctVariants(vs []coverVariant) int {
	seen := map[coverVariant]struct{}{}
	for _, v := range vs {
		seen[v] = struct{}{}
	}
	return len(seen)
}

func axisValues(vs []coverVariant) map[string]map[string]struct{} {
	out := map[string]map[string]struct{}{
		"accent": {}, "composition": {}, "arrangement": {}, "lighting": {},
	}
	for _, v := range vs {
		out["accent"][v.accent] = struct{}{}
		out["composition"][v.composition] = struct{}{}
		out["arrangement"][v.arrangement] = struct{}{}
		out["lighting"][v.lighting] = struct{}{}
	}
	return out
}

// checkUncorrelated reports axis pairs where one value perfectly predicts the
// other. When that happens the pair contributes no more variety than a single
// axis would, which is the symptom of reusing one hash for both.
func checkUncorrelated(vs []coverVariant) error {
	axes := map[string]func(coverVariant) string{
		"accent":      func(v coverVariant) string { return v.accent },
		"composition": func(v coverVariant) string { return v.composition },
		"arrangement": func(v coverVariant) string { return v.arrangement },
		"lighting":    func(v coverVariant) string { return v.lighting },
	}
	names := []string{"accent", "composition", "arrangement", "lighting"}
	for i := 0; i < len(names); i++ {
		for j := i + 1; j < len(names); j++ {
			a, b := axes[names[i]], axes[names[j]]
			pairs, av, bv := map[string]struct{}{}, map[string]struct{}{}, map[string]struct{}{}
			for _, v := range vs {
				pairs[a(v)+"|"+b(v)] = struct{}{}
				av[a(v)] = struct{}{}
				bv[b(v)] = struct{}{}
			}
			if len(av) < 2 || len(bv) < 2 {
				continue
			}
			if len(pairs) <= max(len(av), len(bv)) {
				return fmt.Errorf("%s and %s move together: %d distinct pairs for %d×%d values",
					names[i], names[j], len(pairs), len(av), len(bv))
			}
		}
	}
	return nil
}
