package service

import (
	"context"
	"slices"
	"testing"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/research"
)

// A discovered brief carries the pages it came from and the run's focus
// areas, so research starts from them rather than from the topic's wording
// alone. Dropping them here is how an observability brief was written up as an
// article on efficient inference.
func TestDiscoverBriefs_CarriesSeedsAndFocus(t *testing.T) {
	repo := &discoveryRepo{}
	rs := &discoveryResearch{topics: []research.Topic{{
		Topic:   "Efficiently Observing AI Agents with Prompt Caching",
		Context: "For SREs.",
		Seeds:   []string{"https://example.com/agent-tracing", "https://example.com/prompt-cache"},
		InFocus: true,
	}}}
	svc, tracker := newDiscoverySvc(repo, rs)
	run := &model.BlogRun{ID: uuid.New(), OrgID: uuid.New()}
	focus := []string{"Observability for AI Agents and LLMs", "SRE", "Grafana"}

	got, err := svc.discoverBriefs(context.Background(), run,
		model.GenerateBlogRequest{Trending: true, TrendingCount: 1, Focus: focus}, tracker)
	if err != nil {
		t.Fatalf("discoverBriefs: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d briefs, want 1", len(got))
	}
	if !slices.Equal(got[0].Seeds, rs.topics[0].Seeds) {
		t.Errorf("brief seeds = %v, want the discovered topic's %v", got[0].Seeds, rs.topics[0].Seeds)
	}
	if !slices.Equal(got[0].Focus, focus) {
		t.Errorf("brief focus = %v, want the run's %v", got[0].Focus, focus)
	}

	// runGeneration normalises the discovered briefs before persisting and
	// writing them, and Retry replays stored briefs through Normalize too.
	// Neither may strip what discovery attached.
	req := model.GenerateBlogRequest{Briefs: got}
	req.Normalize()
	if !slices.Equal(req.Briefs[0].Seeds, rs.topics[0].Seeds) || !slices.Equal(req.Briefs[0].Focus, focus) {
		t.Errorf("Normalize dropped seeds or focus: %+v", req.Briefs[0])
	}
}
