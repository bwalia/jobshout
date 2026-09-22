package research

import (
	"context"
	"slices"
	"strings"
	"testing"
)

// The run that motivated these tests (int, 596b1b16): a trending brief on
// observing AI agents was researched from its wording alone, the planner turned
// that into efficient-inference searches, and the article came back about
// efficient inference.
var driftFocus = []string{"Observability for AI Agents and LLMs", "SRE", "Grafana"}

const driftTopic = "Efficiently Observing AI Agents with Prompt Caching"

// driftDocText is what the seed page says. Quotes below are taken from it.
const driftDocText = `Tracing every agent step with OpenTelemetry lets SRE teams see which prompts
hit the cache and which pay full price. Grafana dashboards built on those traces show
cache hit rates per agent, so a drop in hit rate appears before the bill does.`

const seedURL = "https://example.com/agent-observability"

func driftBackend() *fixedBackend {
	b := defaultBackend()
	// Search finds the same on-subject page, so a run with no seeds still has
	// something readable; a run with the page as a seed must not read it twice.
	b.sources = []Source{{URL: seedURL, Title: "Observing agents"}}
	b.docs[seedURL] = &Document{
		Source: Source{URL: seedURL, Title: "Observing agents", Site: "example.com"},
		Text:   driftDocText,
	}
	return b
}

func driftModel(plan string) *scriptedLLM {
	return &scriptedLLM{responses: []scriptedResponse{
		{trigger: "planning research", content: plan},
		{trigger: "choosing which search results", content: `{"selected": [0]}`},
		{trigger: "extracting citable facts", content: `{"findings": [
			{"claim": "Tracing agent steps shows cache hits.",
			 "quote": "Tracing every agent step with OpenTelemetry lets SRE teams see which prompts hit the cache"}
		]}`},
		{trigger: "fact-checking citations", content: `{"verdicts": [{"index": 0, "supported": true}]}`},
		{trigger: "Summarise the current state", content: "summary"},
	}}
}

func promptWith(prompts []string, marker string) string {
	for _, p := range prompts {
		if strings.Contains(p, marker) {
			return p
		}
	}
	return ""
}

// Seeds are read first and used as sources, without going through search
// relevance, and a seed that cannot be read is skipped rather than fatal.
func TestResearch_ReadsSeedsAsSources(t *testing.T) {
	backend := driftBackend()
	model := driftModel(`{"queries": ["agent tracing"]}`)
	agent := newTestAgent(t, backend, model)

	brief, err := agent.Research(context.Background(), Request{
		Topic: driftTopic,
		Seeds: []string{seedURL, "https://example.com/dead-seed"},
	}, nil)
	if err != nil {
		t.Fatalf("Research returned error: %v", err)
	}

	// The seed is also a search result; it must be read once, as a seed.
	fetched := backend.fetchURLs()
	if len(fetched) < 2 || !slices.Contains(fetched[:2], seedURL) {
		t.Errorf("seeds were not fetched first: %v", fetched)
	}
	if n := len(slices.DeleteFunc(slices.Clone(fetched), func(u string) bool { return u != seedURL })); n != 1 {
		t.Errorf("seed fetched %d times, want once: %v", n, fetched)
	}
	if !slices.ContainsFunc(brief.Sources, func(s Source) bool { return s.URL == seedURL }) {
		t.Errorf("seed was not used as a source: %+v", brief.Sources)
	}
	if !slices.ContainsFunc(brief.Warnings, func(w string) bool { return strings.Contains(w, "dead-seed") }) {
		t.Errorf("unreadable seed was not recorded as a warning: %v", brief.Warnings)
	}
	// The seed never went before the selector: it is already known to be on
	// the topic.
	if sel := promptWith(model.prompts, "choosing which search results"); strings.Contains(sel, seedURL) {
		t.Errorf("seed was put through search relevance:\n%s", sel)
	}
}

// Seeds count toward MaxSources ahead of search: when they fill it, no search
// runs at all.
func TestResearch_SeedsCountTowardMaxSources(t *testing.T) {
	backend := driftBackend()
	model := driftModel(`{"queries": ["agent tracing"]}`)
	agent := newTestAgent(t, backend, model)
	agent.cfg.MaxSources = 1

	if _, err := agent.Research(context.Background(), Request{Topic: driftTopic, Seeds: []string{seedURL}}, nil); err != nil {
		t.Fatalf("Research returned error: %v", err)
	}
	if n := backend.searchCalls(); n != 0 {
		t.Errorf("searched %d times with the source budget already filled by seeds", n)
	}
}

func TestResearch_PlannerPromptIncludesFocus(t *testing.T) {
	model := driftModel(`{"queries": ["observability ai agents"]}`)
	agent := newTestAgent(t, driftBackend(), model)

	if _, err := agent.Research(context.Background(), Request{Topic: driftTopic, Focus: driftFocus}, nil); err != nil {
		t.Fatalf("Research returned error: %v", err)
	}

	plan := promptWith(model.prompts, "planning research")
	for _, area := range driftFocus {
		if !strings.Contains(plan, area) {
			t.Errorf("planner prompt is missing focus area %q:\n%s", area, plan)
		}
	}
	if !strings.Contains(plan, "must stay inside the topic's subject area") {
		t.Errorf("planner prompt does not tell the model to stay inside the focus:\n%s", plan)
	}
}

// The evidence case: every planned query drifted to efficient inference, so a
// query tying the topic back to the closest focus area is added.
func TestResearch_AddsFocusQueryWhenPlanIgnoresFocus(t *testing.T) {
	model := driftModel(`{"queries": ["efficient inference", "prompt tuning"]}`)
	agent := newTestAgent(t, driftBackend(), model)

	brief, err := agent.Research(context.Background(), Request{Topic: driftTopic, Focus: driftFocus}, nil)
	if err != nil {
		t.Fatalf("Research returned error: %v", err)
	}

	want := []string{"efficient inference", "prompt tuning", "caching observability ai agents"}
	if !slices.Equal(brief.Queries, want) {
		t.Errorf("queries = %q, want %q", brief.Queries, want)
	}
}

func TestEnsureFocusQuery(t *testing.T) {
	tests := []struct {
		name    string
		queries []string
		focus   []string
		want    []string
	}{
		{"no focus leaves queries alone", []string{"prompt tuning"}, nil, []string{"prompt tuning"}},
		{"a query already on focus is enough", []string{"prompt tuning", "grafana agent dashboards"}, driftFocus,
			[]string{"prompt tuning", "grafana agent dashboards"}},
		{"closest focus area wins", []string{"prompt tuning"}, []string{"Grafana", "Observability for AI Agents and LLMs"},
			[]string{"prompt tuning", "caching observability ai agents"}},
		{"blank focus areas are ignored", []string{"prompt tuning"}, []string{" ", ""}, []string{"prompt tuning"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ensureFocusQuery(driftTopic, tt.queries, tt.focus); !slices.Equal(got, tt.want) {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

// Source selection judges candidates against the focus areas as well as the
// topic, so a page that shares the topic's words but not its subject is
// dropped.
func TestResearch_SelectionJudgesAgainstFocus(t *testing.T) {
	backend := driftBackend()
	backend.sources = append(backend.sources, Source{
		URL: "https://example.com/efficient-inference", Title: "Efficient inference with prompt caching",
	})
	model := driftModel(`{"queries": ["observability ai agents"]}`)
	agent := newTestAgent(t, backend, model)

	if _, err := agent.Research(context.Background(), Request{Topic: driftTopic, Focus: driftFocus}, nil); err != nil {
		t.Fatalf("Research returned error: %v", err)
	}

	sel := promptWith(model.prompts, "choosing which search results")
	if sel == "" {
		t.Fatal("source selection never ran")
	}
	for _, area := range driftFocus {
		if !strings.Contains(sel, area) {
			t.Errorf("selection prompt is missing focus area %q:\n%s", area, sel)
		}
	}
	if !strings.Contains(sel, "off-subject") {
		t.Errorf("selection prompt does not tell the model to reject off-focus sources:\n%s", sel)
	}
}

// Every other caller (research handler, platform tools, career, mail) sends
// neither field. For them nothing may change: the same prompts, the same
// queries, no seed fetches.
func TestResearch_NoFocusOrSeedsChangesNothing(t *testing.T) {
	backend := driftBackend()
	backend.sources = append(backend.sources, Source{URL: "https://example.com/other", Title: "Other"})
	model := driftModel(`{"queries": ["efficient inference"]}`)
	agent := newTestAgent(t, backend, model)

	brief, err := agent.Research(context.Background(), Request{Topic: driftTopic}, nil)
	if err != nil {
		t.Fatalf("Research returned error: %v", err)
	}

	if !slices.Equal(brief.Queries, []string{"efficient inference"}) {
		t.Errorf("queries = %q, want the planner's unchanged", brief.Queries)
	}
	if fetched := backend.fetchURLs(); len(fetched) != 1 || fetched[0] != seedURL {
		t.Errorf("fetched %v, want only the selected search result", fetched)
	}
	for _, p := range model.prompts {
		if strings.Contains(p, "FOCUS AREAS") {
			t.Errorf("a prompt mentions focus areas with none given:\n%s", p)
		}
	}
	if focusPlanBlock(nil) != "" || focusSelectBlock(nil) != "" {
		t.Error("focus prompt blocks are not empty without focus")
	}
}
