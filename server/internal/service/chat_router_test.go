package service

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/llm"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/repository"
)

// ── test doubles ────────────────────────────────────────────────────────────

// fakeLLMClient returns a pre-seeded response for each call. When exhausted it
// returns an empty response so the router exercises its fallbacks.
type fakeLLMClient struct {
	responses []string
	calls     int
	lastReq   llm.GenerateRequest
}

func (f *fakeLLMClient) Generate(_ context.Context, req llm.GenerateRequest) (*llm.GenerateResponse, error) {
	f.lastReq = req
	var content string
	if f.calls < len(f.responses) {
		content = f.responses[f.calls]
	}
	f.calls++
	return &llm.GenerateResponse{Content: content, FinishReason: "stop"}, nil
}
func (f *fakeLLMClient) ProviderName() string { return "fake" }

type fakeAgentSvc struct {
	agents []model.Agent
}

func (f *fakeAgentSvc) Create(context.Context, uuid.UUID, uuid.UUID, model.CreateAgentRequest) (*model.Agent, error) {
	return nil, nil
}
func (f *fakeAgentSvc) GetByID(_ context.Context, id uuid.UUID) (*model.Agent, error) {
	for i := range f.agents {
		if f.agents[i].ID == id {
			return &f.agents[i], nil
		}
	}
	return nil, ErrAgentNotFound
}
func (f *fakeAgentSvc) List(_ context.Context, _ uuid.UUID, _ model.PaginationParams, filter repository.AgentListFilter) (*model.PaginatedResponse[model.Agent], error) {
	data := make([]model.Agent, 0, len(f.agents))
	if filter.Search == "" {
		data = append(data, f.agents...)
	} else {
		needle := strings.ToLower(filter.Search)
		for _, a := range f.agents {
			if strings.Contains(strings.ToLower(a.Name), needle) {
				data = append(data, a)
			}
		}
	}
	return &model.PaginatedResponse[model.Agent]{Data: data, Total: len(data)}, nil
}
func (f *fakeAgentSvc) Update(context.Context, uuid.UUID, model.UpdateAgentRequest) (*model.Agent, error) {
	return nil, nil
}
func (f *fakeAgentSvc) Delete(context.Context, uuid.UUID) error            { return nil }
func (f *fakeAgentSvc) UpdateStatus(context.Context, uuid.UUID, string) error { return nil }

type fakeExecSvc struct {
	exec *model.AgentExecution
	err  error

	gotAgentID uuid.UUID
	gotPrompt  string
	calls      int
}

func (f *fakeExecSvc) Execute(_ context.Context, _ uuid.UUID, agentID uuid.UUID, req model.ExecuteAgentRequest) (*model.AgentExecution, error) {
	f.calls++
	f.gotAgentID = agentID
	f.gotPrompt = req.Prompt
	if f.err != nil {
		return nil, f.err
	}
	if f.exec != nil {
		return f.exec, nil
	}
	out := "ok"
	return &model.AgentExecution{ID: uuid.New(), AgentID: agentID, Output: &out, Status: model.ExecutionStatusCompleted}, nil
}
func (f *fakeExecSvc) GetByID(context.Context, uuid.UUID) (*model.AgentExecution, error) {
	return nil, nil
}
func (f *fakeExecSvc) ListByAgent(context.Context, uuid.UUID, model.PaginationParams) (*model.PaginatedResponse[model.AgentExecution], error) {
	return nil, nil
}
func (f *fakeExecSvc) ListLangChainTraces(context.Context, uuid.UUID) ([]model.LangChainRunTrace, error) {
	return nil, nil
}
func (f *fakeExecSvc) ListLangGraphSnapshots(context.Context, uuid.UUID) ([]model.LangGraphStateSnapshot, error) {
	return nil, nil
}

type fakeWorkflowSvc struct {
	workflows []model.Workflow
	run       *model.WorkflowRun
	runErr    error

	gotWfID uuid.UUID
	calls   int
}

func (f *fakeWorkflowSvc) Create(context.Context, uuid.UUID, uuid.UUID, model.CreateWorkflowRequest) (*model.Workflow, error) {
	return nil, nil
}
func (f *fakeWorkflowSvc) GetByID(context.Context, uuid.UUID) (*model.Workflow, error) { return nil, nil }
func (f *fakeWorkflowSvc) ListByOrg(_ context.Context, _ uuid.UUID, _ model.PaginationParams) (*model.PaginatedResponse[model.Workflow], error) {
	return &model.PaginatedResponse[model.Workflow]{Data: f.workflows, Total: len(f.workflows)}, nil
}
func (f *fakeWorkflowSvc) Update(context.Context, uuid.UUID, model.UpdateWorkflowRequest) (*model.Workflow, error) {
	return nil, nil
}
func (f *fakeWorkflowSvc) Delete(context.Context, uuid.UUID) error { return nil }
func (f *fakeWorkflowSvc) Execute(_ context.Context, wfID uuid.UUID, _ uuid.UUID, _ uuid.UUID, _ model.ExecuteWorkflowRequest) (*model.WorkflowRun, error) {
	f.calls++
	f.gotWfID = wfID
	if f.runErr != nil {
		return nil, f.runErr
	}
	if f.run != nil {
		return f.run, nil
	}
	return &model.WorkflowRun{ID: uuid.New(), WorkflowID: wfID, Status: "running"}, nil
}
func (f *fakeWorkflowSvc) GetRunByID(context.Context, uuid.UUID) (*model.WorkflowRun, error) {
	return nil, nil
}
func (f *fakeWorkflowSvc) ListRuns(context.Context, uuid.UUID, model.PaginationParams) (*model.PaginatedResponse[model.WorkflowRun], error) {
	return nil, nil
}

// fakeTaskRepo is a no-op repository satisfying the interface for tests that
// don't exercise list_tasks or get_status paths.
type fakeTaskRepo struct {
	tasks []model.Task
}

func (f *fakeTaskRepo) Create(context.Context, *model.Task) error              { return nil }
func (f *fakeTaskRepo) FindByID(context.Context, uuid.UUID) (*model.Task, error) { return nil, nil }
func (f *fakeTaskRepo) ListByProject(context.Context, uuid.UUID, model.PaginationParams) (*model.PaginatedResponse[model.Task], error) {
	return nil, nil
}
func (f *fakeTaskRepo) ListByOrg(_ context.Context, _ uuid.UUID, _ model.PaginationParams) (*model.PaginatedResponse[model.Task], error) {
	return &model.PaginatedResponse[model.Task]{Data: f.tasks, Total: len(f.tasks)}, nil
}
func (f *fakeTaskRepo) ListComments(context.Context, uuid.UUID) ([]model.TaskComment, error) {
	return nil, nil
}
func (f *fakeTaskRepo) AddComment(context.Context, *model.TaskComment) error            { return nil }
func (f *fakeTaskRepo) Update(context.Context, *model.Task) error                       { return nil }
func (f *fakeTaskRepo) Delete(context.Context, uuid.UUID) error                         { return nil }
func (f *fakeTaskRepo) TransitionStatus(context.Context, uuid.UUID, string) error       { return nil }
func (f *fakeTaskRepo) Reorder(context.Context, uuid.UUID, string, int) error           { return nil }

// ── helpers ─────────────────────────────────────────────────────────────────

func newRouter(t *testing.T, responses []string, agents []model.Agent, workflows []model.Workflow, tasks []model.Task) (*chatRouterService, *fakeLLMClient, *fakeExecSvc, *fakeWorkflowSvc) {
	t.Helper()
	client := &fakeLLMClient{responses: responses}
	llmRouter := llm.NewTestRouter("fake", map[string]llm.Client{"fake": client})
	agentSvc := &fakeAgentSvc{agents: agents}
	execSvc := &fakeExecSvc{}
	workflowSvc := &fakeWorkflowSvc{workflows: workflows}
	taskRepo := &fakeTaskRepo{tasks: tasks}
	logger := zap.NewNop()

	svc := NewChatRouterService(llmRouter, agentSvc, execSvc, workflowSvc, taskRepo, nil, logger).(*chatRouterService)
	return svc, client, execSvc, workflowSvc
}

// ── tests ───────────────────────────────────────────────────────────────────

func TestChatRouter_RunTask_DispatchesExecution(t *testing.T) {
	agentID := uuid.New()
	agents := []model.Agent{{ID: agentID, Name: "devops-agent", Role: "ops"}}

	responses := []string{
		// intent
		`{"intent":"run_task","agent":"devops-agent","task":"debug payment","params":{},"confidence":0.9}`,
		// task plan (used to clean up prompt)
		`{"task_name":"debug-payment","description":"investigate payment failure","inputs":{},"priority":"high"}`,
		// formatter
		`Execution completed: payment service healthy`,
	}

	svc, _, execSvc, _ := newRouter(t, responses, agents, nil, nil)

	res, err := svc.Route(context.Background(), uuid.New(), uuid.New(), uuid.New(), "debug payment service", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Intent != llm.ChatIntentRunTask {
		t.Errorf("intent = %q; want run_task", res.Intent)
	}
	if execSvc.calls != 1 {
		t.Errorf("execSvc.calls = %d; want 1", execSvc.calls)
	}
	if execSvc.gotAgentID != agentID {
		t.Errorf("wrong agent: %v", execSvc.gotAgentID)
	}
	if !strings.Contains(execSvc.gotPrompt, "investigate payment failure") {
		t.Errorf("expected task-plan description fed to executor; got %q", execSvc.gotPrompt)
	}
	if !strings.Contains(res.Message, "Execution completed") {
		t.Errorf("expected formatted message, got %q", res.Message)
	}
}

func TestChatRouter_RunTask_AgentNotFound_AsksForClarification(t *testing.T) {
	agents := []model.Agent{{ID: uuid.New(), Name: "devops-agent"}}

	responses := []string{
		`{"intent":"run_task","agent":"ghost-agent","task":"x","params":{},"confidence":0.8}`,
	}

	svc, _, execSvc, _ := newRouter(t, responses, agents, nil, nil)

	res, err := svc.Route(context.Background(), uuid.New(), uuid.New(), uuid.New(), "x", nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Intent != llm.ChatIntentClarify {
		t.Errorf("intent = %q; want clarify", res.Intent)
	}
	if execSvc.calls != 0 {
		t.Errorf("executor should not have run for unknown agent")
	}
	if !strings.Contains(res.Message, "ghost-agent") {
		t.Errorf("expected message to mention the unknown agent; got %q", res.Message)
	}
}

func TestChatRouter_Clarify_ReturnsQuestion(t *testing.T) {
	responses := []string{
		`{"intent":"clarify","params":{},"confidence":0.1}`,
		`{"intent":"clarify","question":"which agent should handle this?"}`,
	}

	svc, _, _, _ := newRouter(t, responses, nil, nil, nil)

	res, err := svc.Route(context.Background(), uuid.New(), uuid.New(), uuid.New(), "do the thing", nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Intent != llm.ChatIntentClarify {
		t.Errorf("intent = %q; want clarify", res.Intent)
	}
	if res.ClarifyQuestion != "which agent should handle this?" {
		t.Errorf("unexpected question: %q", res.ClarifyQuestion)
	}
}

func TestChatRouter_ListAgents_DoesNotCallLLMBeyondIntent(t *testing.T) {
	agents := []model.Agent{{ID: uuid.New(), Name: "a"}, {ID: uuid.New(), Name: "b"}}
	responses := []string{
		`{"intent":"list_agents","params":{},"confidence":1.0}`,
	}
	svc, client, _, _ := newRouter(t, responses, agents, nil, nil)

	res, err := svc.Route(context.Background(), uuid.New(), uuid.New(), uuid.New(), "show agents", nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Intent != llm.ChatIntentListAgents {
		t.Errorf("intent = %q; want list_agents", res.Intent)
	}
	if !strings.Contains(res.Message, "a") || !strings.Contains(res.Message, "b") {
		t.Errorf("expected both agents in message; got %q", res.Message)
	}
	if client.calls != 1 {
		t.Errorf("expected exactly 1 LLM call for list_agents; got %d", client.calls)
	}
}

func TestChatRouter_GetStatus_RunsStatusPrompt(t *testing.T) {
	taskID := uuid.New()
	tasks := []model.Task{{ID: taskID, Title: "investigate latency"}}

	taskIDStr := taskID.String()
	responses := []string{
		`{"intent":"get_status","params":{},"confidence":0.9}`,
		`{"intent":"get_status","task_id":"` + taskIDStr + `"}`,
	}

	svc, _, _, _ := newRouter(t, responses, nil, nil, tasks)

	res, err := svc.Route(context.Background(), uuid.New(), uuid.New(), uuid.New(), "how's the latency task going?", nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Intent != llm.ChatIntentGetStatus {
		t.Errorf("intent = %q; want get_status", res.Intent)
	}
	if !strings.Contains(res.Message, taskIDStr) {
		t.Errorf("expected task id in message; got %q", res.Message)
	}
}

func TestChatRouter_RunWorkflow_StartsRun(t *testing.T) {
	wfID := uuid.New()
	workflows := []model.Workflow{{ID: wfID, Name: "incident-triage"}}

	responses := []string{
		`{"intent":"run_workflow","workflow":"incident-triage","params":{"severity":"high"},"confidence":0.95}`,
	}
	svc, _, _, wfSvc := newRouter(t, responses, nil, workflows, nil)

	res, err := svc.Route(context.Background(), uuid.New(), uuid.New(), uuid.New(), "run incident triage", nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Intent != llm.ChatIntentRunWorkflow {
		t.Errorf("intent = %q; want run_workflow", res.Intent)
	}
	if wfSvc.calls != 1 || wfSvc.gotWfID != wfID {
		t.Errorf("workflow not executed: calls=%d wf=%v", wfSvc.calls, wfSvc.gotWfID)
	}
	if res.WorkflowRun == nil {
		t.Error("expected workflow run in result")
	}
}

func TestChatRouter_PolicyDenied_SkipsIntent(t *testing.T) {
	responses := []string{
		`{"allowed":false,"reason":"destructive operation"}`,
	}
	client := &fakeLLMClient{responses: responses}
	llmRouter := llm.NewTestRouter("fake", map[string]llm.Client{"fake": client})
	svc := NewChatRouterService(
		llmRouter,
		&fakeAgentSvc{}, &fakeExecSvc{}, &fakeWorkflowSvc{}, &fakeTaskRepo{},
		[]string{"no destructive prod operations"},
		zap.NewNop(),
	)

	res, err := svc.Route(context.Background(), uuid.New(), uuid.New(), uuid.New(), "drop all tables", nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Intent != llm.ChatIntentClarify {
		t.Errorf("intent = %q; want clarify on policy denial", res.Intent)
	}
	if !strings.Contains(res.Message, "destructive operation") {
		t.Errorf("expected denial reason in message; got %q", res.Message)
	}
	if client.calls != 1 {
		t.Errorf("expected exactly 1 LLM call (policy check only); got %d", client.calls)
	}
}

func TestChatRouter_EmptyMessage_ShortCircuits(t *testing.T) {
	client := &fakeLLMClient{}
	llmRouter := llm.NewTestRouter("fake", map[string]llm.Client{"fake": client})
	svc := NewChatRouterService(llmRouter, &fakeAgentSvc{}, &fakeExecSvc{}, &fakeWorkflowSvc{}, &fakeTaskRepo{}, nil, zap.NewNop())

	res, err := svc.Route(context.Background(), uuid.New(), uuid.New(), uuid.New(), "   ", nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Intent != llm.ChatIntentClarify {
		t.Errorf("intent = %q; want clarify for empty message", res.Intent)
	}
	if client.calls != 0 {
		t.Errorf("empty message should not trigger any LLM calls; got %d", client.calls)
	}
}
