package modelselect

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/jobshout/server/internal/llm"
)

// AutoProvider is the sentinel stored in Agent.ModelProvider to request
// automatic selection. Anything else — including empty — keeps the existing
// behaviour, so turning Auto on is always an explicit act.
const AutoProvider = "auto"

// IsAuto reports whether a provider value requests automatic selection.
func IsAuto(provider string) bool {
	return strings.EqualFold(strings.TrimSpace(provider), AutoProvider)
}

// ErrNoCandidate is returned when every catalog entry was filtered out. The
// wrapped message names the last reason a candidate was rejected, because "no
// model available" on its own is close to undebuggable in production.
var ErrNoCandidate = errors.New("modelselect: no suitable model")

// responseHeadroomTokens is reserved on top of the prompt estimate when
// checking a model's context window, so a request that only just fits the
// prompt is not chosen and then truncated mid-answer.
const responseHeadroomTokens = 4096

// TaskSignals describes the work to be done. Everything here is cheap for a
// caller to supply; nothing requires calling a model to find out.
type TaskSignals struct {
	// Kind is one of the Kind* constants. Unknown values are treated as KindStep.
	Kind string
	// PromptTokens is an estimate of the prompt size. Zero means unknown, which
	// skips the context-window filter.
	PromptTokens int
	// NeedsTools is true when the request will carry llm.ToolDefs.
	NeedsTools bool
	// MaxOutputTokens is the cap the caller intends to set, used for cost
	// estimation. Zero falls back to responseHeadroomTokens.
	MaxOutputTokens int
	// PriorFailures counts attempts already made for this task. Each failure
	// raises the quality bar, so a retry escalates instead of re-picking the
	// model that just failed.
	PriorFailures int
}

// Constraints are the hard limits a decision must respect. They come from
// governance policy and configuration, never from the task itself.
type Constraints struct {
	// AllowedProviders, when non-empty, is an allowlist.
	AllowedProviders []string
	// AllowedModels, when non-empty, is an allowlist.
	AllowedModels []string
	// MaxUSDPerCall, when > 0, rejects candidates whose estimated cost exceeds it.
	MaxUSDPerCall float64
}

// Decision is the selector's answer: one model to try, plus the ordered
// alternatives to fall back through if it fails.
type Decision struct {
	Provider string
	Model    string
	// Chosen is the full catalog entry behind Provider/Model, so callers can
	// see its context window and capabilities without re-scanning the catalog.
	Chosen Candidate
	// Reason explains the choice in one line, for logs and for showing a user
	// why Auto picked what it did.
	Reason string
	// Fallbacks are the remaining viable candidates, best first.
	Fallbacks []Candidate
}

// Registry is the subset of llm.Router the selector needs. Declaring it here
// keeps the selector testable without constructing a real Router, and keeps the
// dependency pointing one way (modelselect -> llm).
type Registry interface {
	RegisteredProviders() []llm.ProviderInfo
	For(providerName string) (llm.Client, error)
}

// Pricer is the subset of costengine.Engine the selector needs.
type Pricer interface {
	Calculate(provider, model string, inputTokens, outputTokens, latencyMs int) float64
}

// Selector picks a model per task from a fixed catalog.
type Selector struct {
	catalog []Candidate
	reg     Registry
	pricer  Pricer
}

// New builds a Selector. A nil catalog uses DefaultCatalog. A nil pricer
// disables cost-based filtering and cost tie-breaking, which is the sensible
// degradation: better to pick a working model than to refuse for want of prices.
func New(reg Registry, pricer Pricer, catalog []Candidate) *Selector {
	if catalog == nil {
		catalog = DefaultCatalog()
	}
	return &Selector{catalog: catalog, reg: reg, pricer: pricer}
}

// Select returns the best candidate for the task, or ErrNoCandidate.
//
// Filtering is all-or-nothing: a candidate that fails any hard check is out.
// Ranking among survivors prefers the cheapest model that clears the quality
// bar, which is what makes Auto default to small local models for routine work
// and only reach for expensive ones when the task earns it.
func (s *Selector) Select(sig TaskSignals, cons Constraints) (Decision, error) {
	registered := s.registeredSet()
	minQuality := minQualityFor(sig.Kind) + sig.PriorFailures

	// Two buckets, because "no suitable model" is only useful if it explains
	// why the models we actually have were unsuitable. An unconfigured provider
	// is background noise next to "the ones you do have are too weak".
	unavailable := "catalog is empty"
	lastReject := ""
	reject := func(format string, args ...any) { lastReject = fmt.Sprintf(format, args...) }

	type scored struct {
		c    Candidate
		cost float64
	}
	var viable []scored

	for _, c := range s.catalog {
		if !registered[c.Provider] {
			unavailable = fmt.Sprintf("provider %q is not registered", c.Provider)
			continue
		}
		if sig.NeedsTools && !s.toolCapable(c) {
			reject("%s/%s cannot do native tool-calling", c.Provider, c.Model)
			continue
		}
		if c.Quality < minQuality {
			reject("%s/%s quality %d is below the bar of %d for kind %q",
				c.Provider, c.Model, c.Quality, minQuality, sig.Kind)
			continue
		}
		if !fitsContext(c, sig) {
			reject("%s/%s context %d cannot hold %d prompt tokens plus headroom",
				c.Provider, c.Model, c.ContextTokens, sig.PromptTokens)
			continue
		}
		if !allowed(cons.AllowedProviders, c.Provider) {
			reject("provider %q is not permitted by policy", c.Provider)
			continue
		}
		if !allowed(cons.AllowedModels, c.Model) {
			reject("model %q is not permitted by policy", c.Model)
			continue
		}

		cost := s.estimateCost(c, sig)
		if cons.MaxUSDPerCall > 0 && cost > cons.MaxUSDPerCall {
			reject("%s/%s estimated $%.4f exceeds the $%.4f cap",
				c.Provider, c.Model, cost, cons.MaxUSDPerCall)
			continue
		}

		viable = append(viable, scored{c: c, cost: cost})
	}

	if len(viable) == 0 {
		why := lastReject
		if why == "" {
			why = unavailable
		}
		return Decision{}, fmt.Errorf("%w for kind %q: %s", ErrNoCandidate, sig.Kind, why)
	}

	// Cheapest first; then faster; then stronger; then by name so the result is
	// deterministic for a given catalog rather than dependent on map ordering.
	sort.SliceStable(viable, func(i, j int) bool {
		a, b := viable[i], viable[j]
		if a.cost != b.cost {
			return a.cost < b.cost
		}
		if a.c.Speed != b.c.Speed {
			return a.c.Speed > b.c.Speed
		}
		if a.c.Quality != b.c.Quality {
			return a.c.Quality > b.c.Quality
		}
		if a.c.Provider != b.c.Provider {
			return a.c.Provider < b.c.Provider
		}
		return a.c.Model < b.c.Model
	})

	best := viable[0]
	fallbacks := make([]Candidate, 0, len(viable)-1)
	for _, v := range viable[1:] {
		fallbacks = append(fallbacks, v.c)
	}

	return Decision{
		Provider:  best.c.Provider,
		Model:     best.c.Model,
		Chosen:    best.c,
		Reason:    s.explain(best.c, best.cost, sig, minQuality, len(viable)),
		Fallbacks: fallbacks,
	}, nil
}

// SelectLargerContext returns the best candidate that can hold at least
// minContext tokens. It exists because a context-length failure should escalate
// to a bigger window specifically, rather than walking the ordinary fallback
// list, which is sorted by cost and may well offer another model just as small.
func (s *Selector) SelectLargerContext(sig TaskSignals, cons Constraints, minContext int) (Decision, error) {
	widened := sig
	widened.PromptTokens = minContext
	return s.Select(widened, cons)
}

func (s *Selector) explain(c Candidate, cost float64, sig TaskSignals, minQuality, viableCount int) string {
	kind := sig.Kind
	if kind == "" {
		kind = KindStep
	}
	var b strings.Builder
	fmt.Fprintf(&b, "auto: %s/%s for %q", c.Provider, c.Model, kind)
	fmt.Fprintf(&b, " (quality %d >= %d", c.Quality, minQuality)
	if cost > 0 {
		fmt.Fprintf(&b, ", est $%.4f", cost)
	} else {
		b.WriteString(", no metered cost")
	}
	if sig.NeedsTools {
		b.WriteString(", tool-capable")
	}
	if sig.PriorFailures > 0 {
		fmt.Fprintf(&b, ", escalated after %d failure(s)", sig.PriorFailures)
	}
	fmt.Fprintf(&b, ", cheapest of %d)", viableCount)
	return b.String()
}

// toolCapable requires the model to advertise tool support AND its client to
// implement llm.ToolCapableClient affirmatively. Either alone is insufficient:
// the catalog describes the model, the client describes our implementation, and
// a task needing tools must have both.
func (s *Selector) toolCapable(c Candidate) bool {
	if !c.SupportsTools {
		return false
	}
	client, err := s.reg.For(c.Provider)
	if err != nil {
		return false
	}
	tc, ok := client.(llm.ToolCapableClient)
	return ok && tc.SupportsTools()
}

func (s *Selector) estimateCost(c Candidate, sig TaskSignals) float64 {
	if s.pricer == nil {
		return 0
	}
	out := sig.MaxOutputTokens
	if out <= 0 {
		out = responseHeadroomTokens
	}
	// Latency is unknown before the call; pass 0 so compute-priced providers
	// contribute nothing rather than a fabricated number.
	return s.pricer.Calculate(c.Provider, c.Model, sig.PromptTokens, out, 0)
}

func (s *Selector) registeredSet() map[string]bool {
	set := make(map[string]bool)
	for _, p := range s.reg.RegisteredProviders() {
		set[p.Name] = true
	}
	return set
}

func fitsContext(c Candidate, sig TaskSignals) bool {
	if sig.PromptTokens <= 0 {
		return true
	}
	headroom := sig.MaxOutputTokens
	if headroom <= 0 {
		headroom = responseHeadroomTokens
	}
	return c.ContextTokens >= sig.PromptTokens+headroom
}

// allowed reports whether value passes an allowlist. An empty list means
// unrestricted, matching how governance policies treat empty AllowedProviders.
func allowed(list []string, value string) bool {
	if len(list) == 0 {
		return true
	}
	for _, v := range list {
		if strings.EqualFold(v, value) {
			return true
		}
	}
	return false
}
