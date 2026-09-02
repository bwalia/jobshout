package platformtools

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/agentmodules"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/research"
	"github.com/jobshout/server/internal/tasklaunch"
)

type launchResearch struct {
	n    int
	last research.Request
}

func (s *launchResearch) Research(_ context.Context, _ uuid.UUID, req research.Request, _ research.ProgressFunc) (*research.Brief, error) {
	s.n++
	s.last = req
	return &research.Brief{
		Topic:    req.Topic,
		Summary:  "usable brief",
		Findings: []research.Finding{{Claim: "one", SourceURL: "https://ex.example", Quote: "q"}},
		Sources:  []research.Source{{URL: "https://ex.example", Title: "ex"}},
	}, nil
}
func (s *launchResearch) Trending(context.Context, int) ([]research.TrendingItem, error) {
	return nil, nil
}
func (s *launchResearch) Discover(context.Context, uuid.UUID, research.DiscoverRequest, research.ProgressFunc) ([]research.Topic, error) {
	return nil, nil
}
func (s *launchResearch) EnsureResearcher(context.Context, uuid.UUID) (*model.Agent, error) {
	return nil, nil
}
func (s *launchResearch) Available() bool { return true }

func TestEval_ResearchTwoProjectsAsksThenLaunches(t *testing.T) {
	org := uuid.New()
	researcher := builtinAgent("Research Agent", model.BuiltinResearcher)
	researcher.OrgID = org
	agents := &stubAgents{items: []model.Agent{researcher}}
	website := model.Project{ID: uuid.New(), OrgID: org, Name: "Website"}
	projects := &fakeProjects{items: []model.Project{
		{ID: uuid.New(), OrgID: org, Name: "Platform"},
		website,
	}}
	tasks := &fakeTasks{}
	rs := &launchResearch{}
	agentmodules.Register(agentmodules.Deps{Research: rs})
	launch := &tasklaunch.Service{
		Agents: agents, Tasks: tasks, Projects: projects,
	}
	reg := NewRegistryWithTools(Deps{Launch: launch, Agents: agents, Research: rs})
	tool, ok := reg.Get("research_run")
	if !ok {
		t.Fatal("research_run missing")
	}
	ctx := WithIdentity(context.Background(), Identity{OrgID: org, UserID: uuid.New()})

	res, err := tool.Run(ctx, map[string]any{"topic": "kubernetes cost optimisation"})
	if err != nil {
		t.Fatal(err)
	}
	if res == nil || len(res.Missing) == 0 || res.Missing[0] != "project" {
		t.Fatalf("expected project interview, got %+v", res)
	}
	if rs.n != 0 {
		t.Fatal("research must not run before a project is chosen")
	}

	res, err = tool.Run(ctx, map[string]any{
		"topic": "kubernetes cost optimisation", "project": "Website",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res == nil || res.Entity == nil || res.Entity.Kind != model.EntityTask {
		t.Fatalf("expected board task entity, got %+v", res)
	}
	if len(tasks.items) != 1 {
		t.Fatalf("created %d tasks; want 1", len(tasks.items))
	}
	if rs.n != 1 || rs.last.Topic != "kubernetes cost optimisation" {
		t.Fatalf("research calls=%d topic=%q", rs.n, rs.last.Topic)
	}
}

func TestEval_LaunchInjectsMemoryIntoContext(t *testing.T) {
	org := uuid.New()
	researcher := builtinAgent("Research Agent", model.BuiltinResearcher)
	researcher.OrgID = org
	agents := &stubAgents{items: []model.Agent{researcher}}
	proj := model.Project{ID: uuid.New(), OrgID: org, Name: "Inbox"}
	projects := &fakeProjects{items: []model.Project{proj}}
	tasks := &fakeTasks{}
	rs := &launchResearch{}
	agentmodules.Register(agentmodules.Deps{Research: rs})
	launch := &tasklaunch.Service{
		Agents: agents, Tasks: tasks, Projects: projects,
	}
	reg := NewRegistryWithTools(Deps{Launch: launch, Agents: agents, Research: rs})
	tool, _ := reg.Get("research_run")
	ctx := WithIdentity(context.Background(), Identity{OrgID: org, UserID: uuid.New()})
	ctx = WithMemories(ctx, []string{"we only write about rust"})

	res, err := tool.Run(ctx, map[string]any{"topic": "async runtimes"})
	if err != nil {
		t.Fatal(err)
	}
	if res == nil || res.Entity == nil {
		t.Fatalf("expected launch result, got %+v", res)
	}
	if !strings.Contains(rs.last.Context, "we only write about rust") {
		t.Fatalf("memory not injected into specialist context: %q", rs.last.Context)
	}
}

func TestEval_InjectMemoryContextMerges(t *testing.T) {
	vals := map[string]string{"context": "audience: operators"}
	ctx := WithMemories(context.Background(), []string{"prefer rust"})
	injectMemoryContext(ctx, vals)
	if !strings.Contains(vals["context"], "audience: operators") {
		t.Fatal("original context dropped")
	}
	if !strings.Contains(vals["context"], "prefer rust") {
		t.Fatal("memory not merged")
	}
}
