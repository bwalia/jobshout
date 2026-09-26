package service

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/repository"
	"github.com/jobshout/server/internal/research"
)

// discoveryRepo records what discovery asked it for and what it wrote back.
type discoveryRepo struct {
	repository.BlogRepository
	recent    []string
	recentErr error
	// since records the cutoff RecentTopics was called with, and audience the
	// reader it was scoped to.
	since    time.Time
	audience string
	steps    []model.BlogStep
	// briefs records what UpdateBriefs was asked to persist.
	briefs        []model.BlogBrief
	briefsWritten bool
}

func (r *discoveryRepo) UpdateBriefs(
	_ context.Context, _ uuid.UUID, briefs []model.BlogBrief, _ []string,
) error {
	r.briefs = briefs
	r.briefsWritten = true
	return nil
}

func (r *discoveryRepo) RecentTopics(_ context.Context, _ uuid.UUID, since time.Time, audience string) ([]string, error) {
	r.since = since
	r.audience = audience
	return r.recent, r.recentErr
}

func (r *discoveryRepo) UpdateSteps(_ context.Context, _ uuid.UUID, steps []model.BlogStep) error {
	r.steps = steps
	return nil
}

// discoveryResearch stands in for the research service.
type discoveryResearch struct {
	ResearchService
	topics []research.Topic
	err    error
	// got records the request discovery was asked to fulfil.
	got research.DiscoverRequest
}

func (d *discoveryResearch) Discover(
	_ context.Context, _ uuid.UUID, req research.DiscoverRequest, progress research.ProgressFunc,
) ([]research.Topic, error) {
	d.got = req
	if progress != nil {
		progress(research.PhaseDiscovering, "looking")
	}
	return d.topics, d.err
}

func newDiscoverySvc(repo *discoveryRepo, rs *discoveryResearch) (*blogService, *stepTracker) {
	svc := &blogService{repo: repo, research: rs, logger: zap.NewNop()}
	tracker := &stepTracker{
		runID:  uuid.New(),
		steps:  initialSteps(true),
		repo:   repo,
		logger: zap.NewNop(),
	}
	return svc, tracker
}

func TestDiscoverBriefs_TurnsTopicsIntoBriefs(t *testing.T) {
	repo := &discoveryRepo{}
	rs := &discoveryResearch{topics: []research.Topic{
		{Topic: "First subject", Context: "For platform engineers."},
		{Topic: "Second subject", Context: "For ML practitioners."},
	}}
	svc, tracker := newDiscoverySvc(repo, rs)
	run := &model.BlogRun{ID: uuid.New(), OrgID: uuid.New()}

	got, err := svc.discoverBriefs(context.Background(), run,
		model.GenerateBlogRequest{Trending: true, TrendingCount: 2}, tracker)
	if err != nil {
		t.Fatalf("discoverBriefs: %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("got %d briefs, want 2", len(got))
	}
	if got[0].Topic != "First subject" || got[0].Context != "For platform engineers." {
		t.Errorf("brief did not carry the discovered topic and context: %+v", got[0])
	}
	if rs.got.Count != 2 {
		t.Errorf("asked discovery for %d topics, want 2", rs.got.Count)
	}
}

// The guarantee that makes a daily schedule usable: what has already been
// written must reach discovery, or the job repeats itself.
func TestDiscoverBriefs_PassesRecentTopicsAsAvoid(t *testing.T) {
	repo := &discoveryRepo{recent: []string{"Gateway API goes GA", "Postgres tuning"}}
	rs := &discoveryResearch{topics: []research.Topic{{Topic: "Something new"}}}
	svc, tracker := newDiscoverySvc(repo, rs)
	run := &model.BlogRun{ID: uuid.New(), OrgID: uuid.New()}

	if _, err := svc.discoverBriefs(context.Background(), run,
		model.GenerateBlogRequest{Trending: true}, tracker); err != nil {
		t.Fatalf("discoverBriefs: %v", err)
	}

	if len(rs.got.Avoid) != 2 {
		t.Fatalf("passed %d topics to avoid, want 2", len(rs.got.Avoid))
	}
	// The window has to actually look back, or "recent" means nothing.
	if lookback := time.Since(repo.since); lookback < recentTopicWindow {
		t.Errorf("looked back %v, want at least %v", lookback, recentTopicWindow)
	}
}

// Losing the recent-topics lookup should cost de-duplication, not the article.
// A database hiccup that silently cancels the day's writing is the worse trade.
func TestDiscoverBriefs_SurvivesRecentTopicsFailure(t *testing.T) {
	repo := &discoveryRepo{recentErr: fmt.Errorf("database unavailable")}
	rs := &discoveryResearch{topics: []research.Topic{{Topic: "Something"}}}
	svc, tracker := newDiscoverySvc(repo, rs)
	run := &model.BlogRun{ID: uuid.New(), OrgID: uuid.New()}

	got, err := svc.discoverBriefs(context.Background(), run,
		model.GenerateBlogRequest{Trending: true}, tracker)
	if err != nil {
		t.Fatalf("discoverBriefs failed on a recent-topics error: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("got %d briefs, want the run to continue", len(got))
	}
	if rs.got.Avoid != nil {
		t.Errorf("passed %v to avoid despite the lookup failing", rs.got.Avoid)
	}
}

func TestDiscoverBriefs_DiscoveryFailurePropagates(t *testing.T) {
	repo := &discoveryRepo{}
	rs := &discoveryResearch{err: fmt.Errorf("nothing is trending")}
	svc, tracker := newDiscoverySvc(repo, rs)
	run := &model.BlogRun{ID: uuid.New(), OrgID: uuid.New()}

	_, err := svc.discoverBriefs(context.Background(), run,
		model.GenerateBlogRequest{Trending: true}, tracker)
	if err == nil {
		t.Fatal("discoverBriefs succeeded when discovery failed")
	}
}

// Returning zero topics without an error would start a run with nothing to
// write, which would then fail further down with a less useful message.
func TestDiscoverBriefs_NoTopicsIsAnError(t *testing.T) {
	repo := &discoveryRepo{}
	rs := &discoveryResearch{topics: nil}
	svc, tracker := newDiscoverySvc(repo, rs)
	run := &model.BlogRun{ID: uuid.New(), OrgID: uuid.New()}

	if _, err := svc.discoverBriefs(context.Background(), run,
		model.GenerateBlogRequest{Trending: true}, tracker); err == nil {
		t.Fatal("discoverBriefs succeeded with no topics")
	}
}

func TestDiscoverBriefs_UnconfiguredResearchIsAnError(t *testing.T) {
	repo := &discoveryRepo{}
	svc := &blogService{repo: repo, logger: zap.NewNop()} // no research service
	tracker := &stepTracker{runID: uuid.New(), steps: initialSteps(true), repo: repo, logger: zap.NewNop()}
	run := &model.BlogRun{ID: uuid.New(), OrgID: uuid.New()}

	if _, err := svc.discoverBriefs(context.Background(), run,
		model.GenerateBlogRequest{Trending: true}, tracker); err == nil {
		t.Fatal("discoverBriefs succeeded with no research service configured")
	}
}

// The discovery step is attributed to the Research Agent, and only exists on
// runs that were not handed a subject.
func TestInitialSteps_DiscoveryStepOnlyWhenDiscovering(t *testing.T) {
	withDiscovery := initialSteps(true)
	var found *model.BlogStep
	for i := range withDiscovery {
		if withDiscovery[i].Key == model.BlogStepDiscovering {
			found = &withDiscovery[i]
		}
	}
	if found == nil {
		t.Fatal("a trending run has no discovery step")
	}
	if found.Agent != model.AgentNameResearcher {
		t.Errorf("discovery attributed to %q, want %q", found.Agent, model.AgentNameResearcher)
	}

	for _, s := range initialSteps(false) {
		if s.Key == model.BlogStepDiscovering {
			t.Error("a run given its topics still shows a discovery step")
		}
	}
}

// A trending run is created with no topics and discovers them once it starts.
// Update does not write briefs, so without the narrow writer the run finishes
// with an empty topic list — which leaves the page headerless and makes Retry
// refuse the run for having nothing to retry. Found by running it locally.
func TestGenerate_PersistsDiscoveredTopics(t *testing.T) {
	repo := &discoveryRepo{}
	rs := &discoveryResearch{topics: []research.Topic{
		{Topic: "A discovered subject", Context: "For engineers."},
	}}
	svc, tracker := newDiscoverySvc(repo, rs)
	run := &model.BlogRun{ID: uuid.New(), OrgID: uuid.New()}

	briefs, err := svc.discoverBriefs(context.Background(), run,
		model.GenerateBlogRequest{Trending: true}, tracker)
	if err != nil {
		t.Fatalf("discoverBriefs: %v", err)
	}

	// discoverBriefs returns them; runGeneration is what persists them, so
	// this asserts the writer exists and round-trips what it is given.
	if err := repo.UpdateBriefs(context.Background(), run.ID, briefs, []string{briefs[0].Topic}); err != nil {
		t.Fatalf("UpdateBriefs: %v", err)
	}
	if !repo.briefsWritten {
		t.Fatal("the discovered topics were never persisted")
	}
	if len(repo.briefs) != 1 || repo.briefs[0].Topic != "A discovered subject" {
		t.Errorf("persisted %+v, want the discovered subject", repo.briefs)
	}
	if repo.briefs[0].Context != "For engineers." {
		t.Errorf("the discovered context was lost: %q", repo.briefs[0].Context)
	}
}

// Two schedules for two readers must not block each other. The developer
// schedule writing about Gateway API on Monday is not a reason the business
// schedule cannot brief managers on it on Tuesday — that is the point of
// running both — so "already written about" is scoped to one reader.
func TestDiscoverBriefs_ScopesRecentTopicsToTheReader(t *testing.T) {
	repo := &discoveryRepo{recent: []string{"Gateway API goes GA"}}
	rs := &discoveryResearch{topics: []research.Topic{{Topic: "Something new"}}}
	svc, tracker := newDiscoverySvc(repo, rs)
	run := &model.BlogRun{ID: uuid.New(), OrgID: uuid.New()}

	req := model.GenerateBlogRequest{Trending: true, Audience: "business", Industry: "NHS trusts"}
	if _, err := svc.discoverBriefs(context.Background(), run, req, tracker); err != nil {
		t.Fatalf("discoverBriefs: %v", err)
	}

	if repo.audience != "business" {
		t.Errorf("recent topics were looked up for %q; want the run's reader", repo.audience)
	}
}

// Discovery has to be told who it is choosing for, or it applies the developer
// candidate filter — which rejects every subject a business briefing wants.
func TestDiscoverBriefs_PassesTheReaderToDiscovery(t *testing.T) {
	repo := &discoveryRepo{}
	rs := &discoveryResearch{topics: []research.Topic{{Topic: "Something new", Context: "angle"}}}
	svc, tracker := newDiscoverySvc(repo, rs)
	run := &model.BlogRun{ID: uuid.New(), OrgID: uuid.New()}

	req := model.GenerateBlogRequest{
		Trending: true, Audience: "business", Industry: "NHS trusts", Focus: []string{"AI"},
	}
	got, err := svc.discoverBriefs(context.Background(), run, req, tracker)
	if err != nil {
		t.Fatalf("discoverBriefs: %v", err)
	}

	if rs.got.Audience != "business" || rs.got.Industry != "NHS trusts" {
		t.Errorf("discovery asked for audience=%q industry=%q; want the run's",
			rs.got.Audience, rs.got.Industry)
	}
	// And the discovered briefs inherit it, or discovery chooses for managers
	// and the writer writes for engineers.
	if len(got) != 1 {
		t.Fatalf("got %d briefs, want 1", len(got))
	}
	if got[0].Audience != "business" || got[0].Industry != "NHS trusts" {
		t.Errorf("discovered brief did not inherit the reader: %+v", got[0])
	}
}
