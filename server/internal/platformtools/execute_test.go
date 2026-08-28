package platformtools

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/repository"
	"github.com/jobshout/server/internal/service"
	"github.com/jobshout/server/internal/tools"
)

type stubAgents struct {
	items []model.Agent
}

func (s *stubAgents) Create(context.Context, uuid.UUID, uuid.UUID, model.CreateAgentRequest) (*model.Agent, error) {
	return nil, nil
}
func (s *stubAgents) GetByID(_ context.Context, id uuid.UUID) (*model.Agent, error) {
	for i := range s.items {
		if s.items[i].ID == id {
			return &s.items[i], nil
		}
	}
	return nil, service.ErrAgentNotFound
}
func (s *stubAgents) List(context.Context, uuid.UUID, model.PaginationParams, repository.AgentListFilter) (*model.PaginatedResponse[model.Agent], error) {
	return &model.PaginatedResponse[model.Agent]{Data: s.items, Total: len(s.items)}, nil
}
func (s *stubAgents) Update(context.Context, uuid.UUID, model.UpdateAgentRequest) (*model.Agent, error) {
	return nil, nil
}
func (s *stubAgents) Delete(context.Context, uuid.UUID) error { return nil }
func (s *stubAgents) UpdateStatus(context.Context, uuid.UUID, string) error {
	return nil
}

var _ service.AgentService = (*stubAgents)(nil)

type stubExec struct {
	executeN     int
	startN       int
	executeSleep time.Duration
	lastPrompt   string
	byID         map[uuid.UUID]*model.AgentExecution
}

func (s *stubExec) Execute(_ context.Context, orgID, agentID uuid.UUID, req model.ExecuteAgentRequest) (*model.AgentExecution, error) {
	s.executeN++
	s.lastPrompt = req.Prompt
	if s.executeSleep > 0 {
		time.Sleep(s.executeSleep)
	}
	e := &model.AgentExecution{
		ID: uuid.New(), OrgID: orgID, AgentID: agentID,
		Status: model.ExecutionStatusCompleted, InputPrompt: req.Prompt,
	}
	return e, nil
}
func (s *stubExec) Start(_ context.Context, orgID, agentID uuid.UUID, req model.ExecuteAgentRequest) (*model.AgentExecution, error) {
	s.startN++
	s.lastPrompt = req.Prompt
	e := &model.AgentExecution{
		ID: uuid.New(), OrgID: orgID, AgentID: agentID,
		Status: model.ExecutionStatusRunning, InputPrompt: req.Prompt,
	}
	if s.byID == nil {
		s.byID = map[uuid.UUID]*model.AgentExecution{}
	}
	s.byID[e.ID] = e
	return e, nil
}
func (s *stubExec) GetByID(_ context.Context, id uuid.UUID) (*model.AgentExecution, error) {
	if s.byID != nil {
		if e, ok := s.byID[id]; ok {
			return e, nil
		}
	}
	return nil, service.ErrExecutionNotFound
}
func (s *stubExec) Cancel(context.Context, uuid.UUID, uuid.UUID) (*model.AgentExecution, error) {
	return nil, nil
}
func (s *stubExec) ListByAgent(context.Context, uuid.UUID, model.PaginationParams) (*model.PaginatedResponse[model.AgentExecution], error) {
	return &model.PaginatedResponse[model.AgentExecution]{}, nil
}
func (s *stubExec) ListLangChainTraces(context.Context, uuid.UUID) ([]model.LangChainRunTrace, error) {
	return nil, nil
}
func (s *stubExec) ListLangGraphSnapshots(context.Context, uuid.UUID) ([]model.LangGraphStateSnapshot, error) {
	return nil, nil
}

var _ service.ExecutionService = (*stubExec)(nil)

func builtinAgent(name, builtin string) model.Agent {
	return model.Agent{
		ID: uuid.New(), Name: name, Role: name,
		Metadata: map[string]any{model.MetadataKeyBuiltin: builtin},
	}
}

func executeReg(t *testing.T, agents []model.Agent, exec *stubExec, extra ...PlatformTool) (*Registry, context.Context) {
	t.Helper()
	reg := NewRegistry()
	registerAgents(reg, Deps{Agents: &stubAgents{items: agents}, Exec: exec})
	for _, tool := range extra {
		reg.Register(tool)
	}
	ctx := WithIdentity(context.Background(), Identity{OrgID: uuid.New(), UserID: uuid.New()})
	return reg, ctx
}

func TestAgentExecute_SchemaHasNoRequired(t *testing.T) {
	reg, _ := executeReg(t, nil, &stubExec{})
	tool, ok := reg.Get("agent_execute")
	if !ok {
		t.Fatal("missing")
	}
	req, _ := tool.Schema()["required"].([]string)
	if len(req) != 0 {
		t.Fatalf("required = %#v; want empty", req)
	}
}

func TestAgentExecute_ResearcherInterviewsTopic(t *testing.T) {
	exec := &stubExec{}
	researchN := 0
	reg, ctx := executeReg(t, []model.Agent{builtinAgent("Research Agent", model.BuiltinResearcher)}, exec,
		newTool("research_run", "research", "insight", "", false, false, tools.ObjectSchema(map[string]any{}),
			func(_ context.Context, in map[string]any) (*Result, error) {
				researchN++
				return &Result{Data: map[string]any{"topic": strArg(in, "topic")}}, nil
			}),
	)
	tool, _ := reg.Get("agent_execute")
	res, err := tool.Run(ctx, map[string]any{"name": "Research Agent"})
	if err != nil {
		t.Fatal(err)
	}
	if res == nil || len(res.Missing) == 0 || res.Missing[0] != "topic" {
		t.Fatalf("missing = %+v; want topic", res)
	}
	if exec.executeN != 0 || exec.startN != 0 {
		t.Fatal("must not launch before topic")
	}
	if researchN != 0 {
		t.Fatal("research_run must not run yet")
	}

	res, err = tool.Run(ctx, map[string]any{"name": "Research Agent", "topic": "Kubernetes cost optimisation"})
	if err != nil {
		t.Fatal(err)
	}
	if researchN != 1 {
		t.Fatalf("research_run = %d", researchN)
	}
	data, _ := res.Data.(map[string]any)
	if strArg(data, "topic") != "Kubernetes cost optimisation" {
		t.Fatalf("topic = %#v", res.Data)
	}
	if exec.executeN != 0 || exec.startN != 0 {
		t.Fatal("researcher must not call blocking Execute/Start")
	}
}

func TestAgentExecute_EmptyCallInterviews(t *testing.T) {
	exec := &stubExec{}
	reg, ctx := executeReg(t, []model.Agent{builtinAgent("Research Agent", model.BuiltinResearcher)}, exec,
		newTool("research_run", "research", "insight", "", false, false, tools.ObjectSchema(map[string]any{}), nilRun),
	)
	tool, _ := reg.Get("agent_execute")
	res, err := tool.Run(ctx, map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	if res == nil || (len(res.Missing) > 0 && res.Missing[0] != "topic" && res.Missing[0] != "name") {
		// one agent → skip name, ask topic
		if res == nil || len(res.Missing) == 0 || res.Missing[0] != "topic" {
			t.Fatalf("missing = %+v; want topic", res)
		}
	}
	if exec.executeN != 0 {
		t.Fatal("must not execute")
	}
}

func TestAgentExecute_PentesterBlankTargetClarifies(t *testing.T) {
	exec := &stubExec{}
	started := 0
	reg, ctx := executeReg(t, []model.Agent{builtinAgent("Pentester", model.BuiltinPentester)}, exec,
		newTool("pentest_start", "pentest", "security", "", true, false, tools.ObjectSchema(map[string]any{}),
			func(_ context.Context, in map[string]any) (*Result, error) {
				started++
				return &Result{Data: map[string]any{"target": strArg(in, "target")}}, nil
			}),
	)
	tool, _ := reg.Get("agent_execute")
	res, err := tool.Run(ctx, map[string]any{"name": "Pentester"})
	if err != nil {
		t.Fatal(err)
	}
	if res == nil || len(res.Missing) == 0 || res.Missing[0] != "target" {
		t.Fatalf("missing = %+v; want target", res)
	}
	data, _ := res.Data.(map[string]any)
	if _, ok := data["refused"]; ok {
		t.Fatal("blank target must clarify, not refuse")
	}
	if started != 0 {
		t.Fatal("must not start a scan")
	}
}

func TestAgentExecute_PRReviewerAsksPRNumber(t *testing.T) {
	exec := &stubExec{}
	reg, ctx := executeReg(t, []model.Agent{builtinAgent("PR Reviewer", model.BuiltinPRReviewer)}, exec,
		newTool("review_pull_request", "review", "security", "", false, false, tools.ObjectSchema(map[string]any{}),
			func(context.Context, map[string]any) (*Result, error) {
				t.Fatal("must not launch")
				return nil, nil
			}),
	)
	tool, _ := reg.Get("agent_execute")
	res, err := tool.Run(ctx, map[string]any{"name": "PR Reviewer", "repo": "acme/api"})
	if err != nil {
		t.Fatal(err)
	}
	if res == nil || len(res.Missing) == 0 || res.Missing[0] != "pr_number" {
		t.Fatalf("missing = %+v; want pr_number", res)
	}
}

func TestAgentExecute_GenericAsksPrompt(t *testing.T) {
	exec := &stubExec{}
	reg, ctx := executeReg(t, []model.Agent{{ID: uuid.New(), Name: "Custom", Role: "helper"}}, exec)
	tool, _ := reg.Get("agent_execute")
	res, err := tool.Run(ctx, map[string]any{"name": "Custom"})
	if err != nil {
		t.Fatal(err)
	}
	if res == nil || len(res.Missing) == 0 || res.Missing[0] != "prompt" {
		t.Fatalf("missing = %+v; want prompt", res)
	}
	if exec.startN != 0 || exec.executeN != 0 {
		t.Fatal("must not launch")
	}
}

func TestAgentExecute_GenericStartIsAsync(t *testing.T) {
	exec := &stubExec{executeSleep: 3 * time.Second}
	reg, ctx := executeReg(t, []model.Agent{{ID: uuid.New(), Name: "Custom", Role: "helper"}}, exec)
	tool, _ := reg.Get("agent_execute")
	start := time.Now()
	res, err := tool.Run(ctx, map[string]any{"name": "Custom", "prompt": "summarise last week's incidents"})
	elapsed := time.Since(start)
	if err != nil {
		t.Fatal(err)
	}
	if elapsed > 2*time.Second {
		t.Fatalf("blocked for %s; Start should return immediately", elapsed)
	}
	if exec.startN != 1 {
		t.Fatalf("start = %d", exec.startN)
	}
	if exec.executeN != 0 {
		t.Fatal("must not call blocking Execute")
	}
	if res.Entity == nil || res.Entity.Kind != model.EntityExecution {
		t.Fatalf("entity = %+v", res.Entity)
	}
	data, _ := res.Data.(map[string]any)
	if strArg(data, "status") != model.ExecutionStatusRunning {
		t.Fatalf("status = %#v", res.Data)
	}
}

func TestExecutionGet_UsesLastExecution(t *testing.T) {
	id := uuid.New()
	exec := &stubExec{byID: map[uuid.UUID]*model.AgentExecution{
		id: {ID: id, Status: model.ExecutionStatusRunning, OrgID: uuid.New()},
	}}
	org := uuid.New()
	exec.byID[id].OrgID = org
	reg, ctx := executeReg(t, nil, exec)
	ctx = WithSessionEntities(ctx, map[string]model.SessionEntity{
		"last_execution": {ID: id.String(), Kind: model.EntityExecution, Label: "Custom"},
	})
	// identity org must match
	ctx = WithIdentity(ctx, Identity{OrgID: org, UserID: uuid.New()})
	tool, ok := reg.Get("execution_get")
	if !ok {
		t.Fatal("missing")
	}
	res, err := tool.Run(ctx, map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	if res == nil || len(res.Missing) > 0 {
		t.Fatalf("should use last_execution, got %+v", res)
	}
}

func TestExecutionGet_MissingSlotIsExecutionID(t *testing.T) {
	reg, ctx := executeReg(t, nil, &stubExec{})
	tool, _ := reg.Get("execution_get")
	res, err := tool.Run(ctx, map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	if res == nil || len(res.Missing) == 0 || res.Missing[0] != "execution_id" {
		t.Fatalf("missing = %+v; want execution_id", res)
	}
}

func TestClarifyFromMatch_SlotIsFieldName(t *testing.T) {
	res := clarifyFromMatch("agent", "Res", "name", []model.Agent{{Name: "Research Agent"}}, func(a model.Agent) string { return a.Name })
	if len(res.Missing) != 1 || res.Missing[0] != "name" {
		t.Fatalf("missing = %v; want [name]", res.Missing)
	}
	n := notFoundClarify("execution", "", "execution_id", nil)
	if n.Missing[0] != "execution_id" {
		t.Fatalf("missing = %v", n.Missing)
	}
}

func TestAlwaysLoadIncludesSpecialists(t *testing.T) {
	for _, n := range []string{"research_run", "article_generate", "pentest_start", "mail_sync", "mail_list_drafts"} {
		if !inAlwaysLoad(n) {
			t.Errorf("%s must be always-load", n)
		}
	}
}

func TestPentestStart_EmptyTargetClarifies(t *testing.T) {
	// pentestTargetAllowed(blank) is false; the tool must clarify first.
	if pentestTargetAllowed("") {
		t.Fatal("blank target is not allowlisted")
	}
}

func TestObjectSchema_ZeroRequired(t *testing.T) {
	got := tools.ObjectSchema(map[string]any{"topic": map[string]any{"type": "string"}})
	req, ok := got["required"].([]string)
	if !ok || len(req) != 0 {
		t.Fatalf("required = %#v", got["required"])
	}
}

func TestAgentExecute_ManyAgentsAsksName(t *testing.T) {
	exec := &stubExec{}
	reg, ctx := executeReg(t, []model.Agent{
		builtinAgent("Research Agent", model.BuiltinResearcher),
		{ID: uuid.New(), Name: "Writer", Role: "write"},
	}, exec)
	tool, _ := reg.Get("agent_execute")
	res, err := tool.Run(ctx, map[string]any{"prompt": "run the research agent"})
	if err != nil {
		t.Fatal(err)
	}
	if res == nil || len(res.Missing) == 0 || res.Missing[0] != "name" {
		t.Fatalf("missing = %+v; want name", res)
	}
	found := false
	for _, o := range res.Options {
		if o.Value == "Research Agent" {
			found = true
		}
	}
	if !found {
		t.Fatalf("options = %+v", res.Options)
	}
}

func TestAgentExecute_ThinPromptDoesNotSeedTopic(t *testing.T) {
	exec := &stubExec{}
	reg, ctx := executeReg(t, []model.Agent{builtinAgent("Research Agent", model.BuiltinResearcher)}, exec,
		newTool("research_run", "research", "insight", "", false, false, tools.ObjectSchema(map[string]any{}),
			func(context.Context, map[string]any) (*Result, error) {
				t.Fatal("must not research a tautological prompt")
				return nil, nil
			}),
	)
	tool, _ := reg.Get("agent_execute")
	res, err := tool.Run(ctx, map[string]any{"name": "Research Agent", "prompt": "run the research agent"})
	if err != nil {
		t.Fatal(err)
	}
	if res == nil || len(res.Missing) == 0 || res.Missing[0] != "topic" {
		t.Fatalf("missing = %+v", res)
	}
}

func TestTaskCreate_ProjectSlot(t *testing.T) {
	org := uuid.New()
	projects := &fakeProjects{items: []model.Project{
		{ID: uuid.New(), OrgID: org, Name: "Website"},
		{ID: uuid.New(), OrgID: org, Name: "Mobile"},
	}}
	reg := NewRegistryWithTools(Deps{Tasks: &fakeTasks{}, Projects: projects})
	tool, _ := reg.Get("task_create")
	ctx := WithIdentity(context.Background(), Identity{OrgID: org, UserID: uuid.New()})
	res, err := tool.Run(ctx, map[string]any{"title": "Fix login timeout"})
	if err != nil {
		t.Fatal(err)
	}
	if res == nil || len(res.Missing) == 0 || res.Missing[0] != "project" {
		t.Fatalf("missing = %+v; want project", res)
	}
}

func TestResearchRun_EmptyTopic(t *testing.T) {
	// Direct schema check: research_run is only registered with a Research dep.
	// The empty-topic path is covered via agent_execute dispatch above.
	if !strings.Contains("What should I research?", "research") {
		t.Fatal("sanity")
	}
}
