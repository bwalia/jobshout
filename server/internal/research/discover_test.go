package research

import (
	"context"
	"strings"
	"testing"

	"go.uber.org/zap"
)

const promptDiscover = "choosing what a technical blog should write about"

// trendingBackend serves canned trending items.
type trendingBackend struct {
	items []TrendingItem
	err   error
}

func (b *trendingBackend) Name() string { return "trending-stub" }
func (b *trendingBackend) List(context.Context, int) ([]TrendingItem, error) {
	return b.items, b.err
}

func newDiscoverAgent(t *testing.T, items []TrendingItem, model *scriptedLLM) *Agent {
	t.Helper()
	client := NewWith(nil, nil, []Lister{&trendingBackend{items: items}}, zap.NewNop())
	return NewAgent(client, model, DefaultAgentConfig(), zap.NewNop())
}

func sampleTrending() []TrendingItem {
	return []TrendingItem{
		{Source: Source{URL: "https://cilium.io/blog/1", Title: "Cilium 1.18 released", Site: "cilium.io"}, Score: 300, Channel: "hackernews"},
		{Source: Source{URL: "https://example.com/2", Title: "Series B funding for a startup", Site: "example.com"}, Score: 200, Channel: "hackernews"},
	}
}

func TestDiscover_TurnsTrendingItemsIntoTopics(t *testing.T) {
	model := &scriptedLLM{responses: []scriptedResponse{
		{trigger: promptDiscover, content: `{"topics":[
			{"topic":"What a kube-proxy-free datapath changes for cluster operators",
			 "context":"For platform engineers running production clusters.",
			 "rationale":"Cilium 1.18 makes this the default.",
			 "seeds":[0]}
		]}`},
	}}
	agent := newDiscoverAgent(t, sampleTrending(), model)

	got, err := agent.Discover(context.Background(), DiscoverRequest{Count: 1}, nil)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d topics, want 1", len(got))
	}

	// The topic must be a subject, not the headline it came from.
	if strings.Contains(got[0].Topic, "1.18 released") {
		t.Errorf("returned the headline rather than a topic: %q", got[0].Topic)
	}
	if got[0].Context == "" {
		t.Error("topic has no context to brief the writer with")
	}
	// Seeds record where the idea came from, resolved back to real URLs.
	if len(got[0].Seeds) != 1 || got[0].Seeds[0] != "https://cilium.io/blog/1" {
		t.Errorf("seeds = %v, want the trending URL it came from", got[0].Seeds)
	}
}

// The guarantee that makes a daily schedule usable: a story stays on the front
// page for days, and the job must not write about it every day.
func TestDiscover_DropsTopicsAlreadyWrittenAbout(t *testing.T) {
	model := &scriptedLLM{responses: []scriptedResponse{
		// The model ignores the instruction and proposes a duplicate anyway.
		{trigger: promptDiscover, content: `{"topics":[
			{"topic":"Gateway API reaches GA in Kubernetes","context":"c","rationale":"r","seeds":[]},
			{"topic":"Writing eBPF programs for network observability","context":"c","rationale":"r","seeds":[]}
		]}`},
	}}
	agent := newDiscoverAgent(t, sampleTrending(), model)

	got, err := agent.Discover(context.Background(), DiscoverRequest{
		Count: 2,
		Avoid: []string{"Kubernetes Gateway API goes GA"},
	}, nil)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}

	for _, topic := range got {
		if matchesAny(topic.Topic, []string{"Kubernetes Gateway API goes GA"}) {
			t.Errorf("returned a topic already written about: %q", topic.Topic)
		}
	}
	if len(got) != 1 {
		t.Fatalf("got %d topics, want 1 after de-duplication", len(got))
	}
}

// Trending items whose headline restates something already covered are removed
// before the model ever sees them, so they do not waste a slot.
func TestDiscover_FiltersSeenHeadlinesBeforePrompting(t *testing.T) {
	model := &scriptedLLM{responses: []scriptedResponse{
		{trigger: promptDiscover, content: `{"topics":[{"topic":"Something else entirely","context":"c","rationale":"r","seeds":[]}]}`},
	}}
	agent := newDiscoverAgent(t, sampleTrending(), model)

	if _, err := agent.Discover(context.Background(), DiscoverRequest{
		Count: 1,
		Avoid: []string{"Cilium 1.18 released"},
	}, nil); err != nil {
		t.Fatalf("Discover: %v", err)
	}

	// The headline legitimately appears further down in the "already written"
	// list, so only the candidate section above it is checked.
	prompt := model.prompts[0]
	candidates, _, found := strings.Cut(prompt, "ALREADY WRITTEN ABOUT RECENTLY")
	if !found {
		t.Fatalf("prompt does not have the expected sections:\n%s", prompt)
	}
	if strings.Contains(candidates, "Cilium 1.18 released") {
		t.Error("an already-covered headline was still offered as a candidate")
	}
	// And it must still be named in the avoid list, or the model has no way to
	// steer away from close variants the word filter did not catch.
	if !strings.Contains(prompt, "Cilium 1.18 released") {
		t.Error("the avoid list did not reach the prompt at all")
	}
}

func TestDiscover_RespectsCount(t *testing.T) {
	model := &scriptedLLM{responses: []scriptedResponse{
		{trigger: promptDiscover, content: `{"topics":[
			{"topic":"First subject","context":"c","rationale":"r","seeds":[]},
			{"topic":"Second subject","context":"c","rationale":"r","seeds":[]},
			{"topic":"Third subject","context":"c","rationale":"r","seeds":[]}
		]}`},
	}}
	agent := newDiscoverAgent(t, sampleTrending(), model)

	got, err := agent.Discover(context.Background(), DiscoverRequest{Count: 2}, nil)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("got %d topics, want the requested 2", len(got))
	}
}

func TestDiscover_NothingTrendingIsAnError(t *testing.T) {
	agent := newDiscoverAgent(t, nil, &scriptedLLM{})
	if _, err := agent.Discover(context.Background(), DiscoverRequest{Count: 1}, nil); err == nil {
		t.Fatal("Discover succeeded with nothing trending")
	}
}

// Everything trending having been covered is a real state, and must be an
// error rather than an empty success — a scheduled run that silently produces
// nothing looks identical to one that is broken.
func TestDiscover_EverythingAlreadyCoveredIsAnError(t *testing.T) {
	agent := newDiscoverAgent(t, sampleTrending(), &scriptedLLM{})

	_, err := agent.Discover(context.Background(), DiscoverRequest{
		Count: 1,
		Avoid: []string{"Cilium 1.18 released", "Series B funding for a startup"},
	}, nil)
	if err == nil {
		t.Fatal("Discover succeeded when everything trending was already covered")
	}
	if !strings.Contains(err.Error(), "written about recently") {
		t.Errorf("error %q does not explain why nothing was returned", err)
	}
}

func TestDiscover_ReportsProgress(t *testing.T) {
	model := &scriptedLLM{responses: []scriptedResponse{
		{trigger: promptDiscover, content: `{"topics":[{"topic":"A subject","context":"c","rationale":"r","seeds":[]}]}`},
	}}
	agent := newDiscoverAgent(t, sampleTrending(), model)

	var phases []string
	if _, err := agent.Discover(context.Background(), DiscoverRequest{Count: 1},
		func(phase, _ string) { phases = append(phases, phase) }); err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(phases) == 0 || phases[0] != PhaseDiscovering {
		t.Errorf("got phases %v, want discovery reported", phases)
	}
}

func TestMatchesAny(t *testing.T) {
	tests := []struct {
		name  string
		s     string
		avoid []string
		want  bool
	}{
		{
			// The case that actually recurs: the same story, reworded.
			name:  "same story reworded",
			s:     "Gateway API reaches GA in Kubernetes",
			avoid: []string{"Kubernetes Gateway API goes GA"},
			want:  true,
		},
		{
			name:  "identical",
			s:     "eBPF for Kubernetes networking",
			avoid: []string{"eBPF for Kubernetes networking"},
			want:  true,
		},
		{
			// Shares "kubernetes" and little else — a different subject.
			name:  "same technology different subject",
			s:     "Debugging Kubernetes CrashLoopBackOff",
			avoid: []string{"Kubernetes Gateway API goes GA"},
			want:  false,
		},
		{
			name:  "entirely unrelated",
			s:     "Postgres index-only scans",
			avoid: []string{"Kubernetes Gateway API goes GA"},
			want:  false,
		},
		{
			name:  "nothing to avoid",
			s:     "Anything at all",
			avoid: nil,
			want:  false,
		},
		{
			// Stop words alone must not make two headlines look related.
			name:  "only stop words in common",
			s:     "How to use the new thing",
			avoid: []string{"What is the new guide for"},
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := matchesAny(tt.s, tt.avoid); got != tt.want {
				t.Errorf("matchesAny(%q, %v) = %v, want %v", tt.s, tt.avoid, got, tt.want)
			}
		})
	}
}

func TestSignificantWords(t *testing.T) {
	got := significantWords("How to use the new Kubernetes Gateway API")

	for _, want := range []string{"kubernetes", "gateway", "api"} {
		if _, ok := got[want]; !ok {
			t.Errorf("missing significant word %q in %v", want, got)
		}
	}
	for _, unwanted := range []string{"how", "the", "new", "use", "to"} {
		if _, ok := got[unwanted]; ok {
			t.Errorf("kept stop word %q", unwanted)
		}
	}
}
