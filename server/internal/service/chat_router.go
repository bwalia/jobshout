package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/llm"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/repository"
)

// ChatRouteResult is what the chat router returns for a single inbound message.
// At most one of Execution or WorkflowRun is populated, depending on the
// matched intent; the Message field is always set and is the string the
// adapter should surface to the user.
type ChatRouteResult struct {
	Intent       string               `json:"intent"`
	Confidence   float64              `json:"confidence"`
	Message      string               `json:"message"`
	Agent        *model.Agent         `json:"agent,omitempty"`
	Execution    *model.AgentExecution `json:"execution,omitempty"`
	Workflow     *model.Workflow      `json:"workflow,omitempty"`
	WorkflowRun  *model.WorkflowRun   `json:"workflow_run,omitempty"`
	ClarifyQuestion string            `json:"clarify_question,omitempty"`
}

// ChatRouterService converts natural-language chat into concrete agent/
// workflow actions using a 12-stage LLM prompt pipeline. It reuses the
// existing execution/workflow/agent/task repositories so behaviour stays
// consistent with the REST surface.
type ChatRouterService interface {
	Route(ctx context.Context, orgID, userID, sessionID uuid.UUID, message string, history []model.ChatMessage) (*ChatRouteResult, error)
}

type chatRouterService struct {
	llmRouter    *llm.Router
	agentSvc     AgentService
	execSvc      ExecutionService
	workflowSvc  WorkflowService
	taskRepo     repository.TaskRepository
	policies     []string
	defaultTemp  float64
	listPageSize int
	logger       *zap.Logger
}

// NewChatRouterService constructs the router. policies is an optional list of
// plain-English rules the policy check prompt evaluates each incoming message
// against; pass nil to skip the policy check entirely.
func NewChatRouterService(
	llmRouter *llm.Router,
	agentSvc AgentService,
	execSvc ExecutionService,
	workflowSvc WorkflowService,
	taskRepo repository.TaskRepository,
	policies []string,
	logger *zap.Logger,
) ChatRouterService {
	return &chatRouterService{
		llmRouter:    llmRouter,
		agentSvc:     agentSvc,
		execSvc:      execSvc,
		workflowSvc:  workflowSvc,
		taskRepo:     taskRepo,
		policies:     policies,
		defaultTemp:  0.3,
		listPageSize: 25,
		logger:       logger,
	}
}

func (s *chatRouterService) Route(ctx context.Context, orgID, userID, sessionID uuid.UUID, message string, history []model.ChatMessage) (*ChatRouteResult, error) {
	message = strings.TrimSpace(message)
	if message == "" {
		return &ChatRouteResult{
			Intent:  llm.ChatIntentClarify,
			Message: "Please send a non-empty message.",
		}, nil
	}

	// 9. Policy enforcement first — refuse the message before spending tokens
	//    on anything else.
	if len(s.policies) > 0 {
		allowed, reason := s.policyCheck(ctx, message)
		if !allowed {
			return &ChatRouteResult{
				Intent:  llm.ChatIntentClarify,
				Message: fmt.Sprintf("I can't process that request: %s", reason),
			}, nil
		}
	}

	// Load the agents and workflows the downstream prompts need. Failures here
	// aren't fatal — we fall back to empty lists so the LLM still answers with
	// "clarify" or "help".
	agents := s.loadAgents(ctx, orgID)
	workflows := s.loadWorkflows(ctx, orgID)

	// 1,2,8. Intent detection (conversational when there's history).
	intent, err := s.detectIntent(ctx, message, agents, workflows, history)
	if err != nil {
		s.logger.Warn("chat_router: intent detection failed", zap.Error(err))
		return &ChatRouteResult{
			Intent:  llm.ChatIntentClarify,
			Message: "I had trouble understanding that. Could you rephrase?",
		}, nil
	}

	result := &ChatRouteResult{
		Intent:     intent.Intent,
		Confidence: intent.Confidence,
	}

	switch intent.Intent {
	case llm.ChatIntentHelp:
		result.Message = helpMessage()

	case llm.ChatIntentListAgents:
		result.Message = formatAgentList(agents)

	case llm.ChatIntentListTasks:
		result.Message = s.formatTaskList(ctx, orgID)

	case llm.ChatIntentGetStatus:
		result.Message = s.resolveStatus(ctx, orgID, message)

	case llm.ChatIntentClarify:
		// 6. Clarification prompt — produces the actual follow-up question.
		q := s.clarify(ctx, message)
		result.ClarifyQuestion = q
		result.Message = q

	case llm.ChatIntentRunTask:
		return s.handleRunTask(ctx, orgID, userID, message, intent, agents, result)

	case llm.ChatIntentCreateTask:
		return s.handleCreateTask(ctx, message, intent, result)

	case llm.ChatIntentRunWorkflow:
		return s.handleRunWorkflow(ctx, orgID, userID, intent, workflows, result)

	default:
		// The parser rejects unknown intents already, but keep a safe fallback.
		result.Intent = llm.ChatIntentClarify
		result.Message = "I'm not sure how to help with that — could you be more specific?"
	}

	return result, nil
}

// ── intent detection ────────────────────────────────────────────────────────

func (s *chatRouterService) detectIntent(ctx context.Context, message string, agents []llm.AgentSummary, workflows []llm.WorkflowSummary, history []model.ChatMessage) (*llm.ChatIntent, error) {
	var user string
	if len(history) > 0 {
		user = llm.BuildConversationalPrompt(toLLMHistory(history), message, agents, workflows)
	} else {
		user = llm.BuildChatIntentPrompt(message, agents, workflows)
	}

	raw, err := s.callLLM(ctx, user, 600)
	if err != nil {
		return nil, err
	}
	return llm.ParseChatIntent(raw)
}

// ── run_task: pick agent → plan task → execute ──────────────────────────────

func (s *chatRouterService) handleRunTask(ctx context.Context, orgID, userID uuid.UUID, message string, intent *llm.ChatIntent, agents []llm.AgentSummary, result *ChatRouteResult) (*ChatRouteResult, error) {
	agentName := ""
	if intent.Agent != nil {
		agentName = *intent.Agent
	}

	// If the intent prompt didn't nominate an agent, 4. agent selector picks one.
	if agentName == "" {
		selected, err := s.selectAgent(ctx, intent.Task, agents)
		if err != nil {
			result.Intent = llm.ChatIntentClarify
			result.Message = "I couldn't pick an agent for that request. Which agent should handle it?"
			return result, nil
		}
		agentName = selected
	}

	agent, err := s.findAgentByName(ctx, orgID, agentName)
	if err != nil {
		result.Intent = llm.ChatIntentClarify
		result.Message = fmt.Sprintf("I couldn't find an agent named %q. Try 'list agents' to see the options.", agentName)
		return result, nil
	}
	result.Agent = agent

	// 3. Convert the raw message into a structured task payload. We mostly use
	//    this for the description so the prompt fed to the agent is cleaner
	//    than the raw chat text.
	taskPrompt := message
	if plan, err := s.planTask(ctx, message, agent.Name); err == nil && plan.Description != "" {
		taskPrompt = plan.Description
	} else if err != nil {
		s.logger.Debug("chat_router: task plan failed, using raw message", zap.Error(err))
	}

	exec, err := s.execSvc.Execute(ctx, orgID, agent.ID, model.ExecuteAgentRequest{Prompt: taskPrompt})
	if err != nil {
		s.logger.Warn("chat_router: execution failed", zap.Error(err))
		result.Intent = llm.ChatIntentClarify
		result.Message = fmt.Sprintf("Failed to start %s: %v", agent.Name, err)
		return result, nil
	}
	result.Execution = exec

	// 10. Format the final answer as a short chat-friendly string.
	result.Message = s.formatExecutionResponse(ctx, exec, agent)
	return result, nil
}

// ── create_task: surface the structured plan so the caller persists it ──────

func (s *chatRouterService) handleCreateTask(ctx context.Context, message string, intent *llm.ChatIntent, result *ChatRouteResult) (*ChatRouteResult, error) {
	plan, err := s.planTask(ctx, message, "")
	if err != nil {
		result.Intent = llm.ChatIntentClarify
		result.Message = "I couldn't extract task details from that message. Could you add a title and a short description?"
		return result, nil
	}

	// The router does not own task persistence because tasks in this codebase
	// require a project_id that the chat context doesn't carry. We return the
	// parsed plan so the caller (or a future handler) can prompt for the
	// missing project and create the task via POST /api/v1/tasks.
	result.Message = fmt.Sprintf("I parsed a task for you:\n• %s\n• Priority: %s\n\nCreate it via POST /api/v1/tasks with a project_id to finalise.",
		plan.TaskName, plan.Priority)
	if planSummary := strings.TrimSpace(plan.Description); planSummary != "" {
		result.Message += "\n\nDescription: " + planSummary
	}
	return result, nil
}

// ── run_workflow: look up the workflow and kick off an async run ────────────

func (s *chatRouterService) handleRunWorkflow(ctx context.Context, orgID, userID uuid.UUID, intent *llm.ChatIntent, workflows []llm.WorkflowSummary, result *ChatRouteResult) (*ChatRouteResult, error) {
	wfName := ""
	if intent.Workflow != nil {
		wfName = *intent.Workflow
	}
	if wfName == "" {
		result.Intent = llm.ChatIntentClarify
		result.Message = "Which workflow should I run? Try 'list workflows' to see the options."
		return result, nil
	}

	wf, err := s.findWorkflowByName(ctx, orgID, wfName)
	if err != nil {
		result.Intent = llm.ChatIntentClarify
		result.Message = fmt.Sprintf("I couldn't find a workflow named %q.", wfName)
		return result, nil
	}
	result.Workflow = wf

	run, err := s.workflowSvc.Execute(ctx, wf.ID, orgID, userID, model.ExecuteWorkflowRequest{Input: intent.Params})
	if err != nil {
		result.Intent = llm.ChatIntentClarify
		result.Message = fmt.Sprintf("Failed to start workflow %s: %v", wf.Name, err)
		return result, nil
	}
	result.WorkflowRun = run
	result.Message = fmt.Sprintf("Workflow %s started. Poll GET /api/v1/workflow-runs/%s for status.", wf.Name, run.ID)
	return result, nil
}

// ── helper: sub-prompts ─────────────────────────────────────────────────────

func (s *chatRouterService) policyCheck(ctx context.Context, message string) (bool, string) {
	raw, err := s.callLLM(ctx, llm.BuildPolicyCheckPrompt(message, s.policies), 200)
	if err != nil {
		s.logger.Debug("chat_router: policy check failed, allowing", zap.Error(err))
		return true, ""
	}
	res, err := llm.ParsePolicyCheck(raw)
	if err != nil {
		s.logger.Debug("chat_router: policy parse failed, allowing", zap.Error(err))
		return true, ""
	}
	return res.Allowed, res.Reason
}

func (s *chatRouterService) selectAgent(ctx context.Context, taskDesc string, agents []llm.AgentSummary) (string, error) {
	if len(agents) == 0 {
		return "", fmt.Errorf("no agents available")
	}
	withMetrics := make([]llm.AgentWithMetrics, 0, len(agents))
	for _, a := range agents {
		withMetrics = append(withMetrics, llm.AgentWithMetrics{AgentSummary: a})
	}
	raw, err := s.callLLM(ctx, llm.BuildAgentSelectorPrompt(taskDesc, withMetrics), 200)
	if err != nil {
		return "", err
	}
	sel, err := llm.ParseAgentSelection(raw)
	if err != nil {
		return "", err
	}
	return sel.Agent, nil
}

func (s *chatRouterService) planTask(ctx context.Context, message, agentName string) (*llm.ChatTaskPlan, error) {
	raw, err := s.callLLM(ctx, llm.BuildChatToTaskPrompt(message, agentName), 400)
	if err != nil {
		return nil, err
	}
	return llm.ParseChatTaskPlan(raw)
}

func (s *chatRouterService) clarify(ctx context.Context, message string) string {
	raw, err := s.callLLM(ctx, llm.BuildClarificationPrompt(message), 200)
	if err != nil {
		return "Could you rephrase your request with more detail?"
	}
	c, err := llm.ParseClarification(raw)
	if err != nil {
		return "Could you rephrase your request with more detail?"
	}
	return c.Question
}

func (s *chatRouterService) resolveStatus(ctx context.Context, orgID uuid.UUID, message string) string {
	tasks := s.listTaskSummaries(ctx, orgID)
	raw, err := s.callLLM(ctx, llm.BuildStatusQueryPrompt(message, tasks), 200)
	if err != nil {
		return "I couldn't read the status lookup. Try specifying the task ID."
	}
	res, err := llm.ParseStatusQuery(raw)
	if err != nil {
		return "I couldn't read the status lookup. Try specifying the task ID."
	}
	if res.TaskID == nil || *res.TaskID == "" {
		return "I couldn't find a matching task. Share the task ID and I'll look it up."
	}
	return fmt.Sprintf("Fetch status via GET /api/v1/tasks/%s or GET /api/v1/executions/%s.", *res.TaskID, *res.TaskID)
}

func (s *chatRouterService) formatExecutionResponse(ctx context.Context, exec *model.AgentExecution, agent *model.Agent) string {
	// Prefer a short LLM-formatted summary when we have a final answer.
	if exec.Output != nil && *exec.Output != "" {
		raw, err := s.callLLM(ctx, llm.BuildResponseFormatterPrompt(*exec.Output), 300)
		if err == nil {
			cleaned := strings.TrimSpace(raw)
			if cleaned != "" {
				return cleaned
			}
		}
		return *exec.Output
	}
	if exec.ErrorMessage != nil && *exec.ErrorMessage != "" {
		return fmt.Sprintf(":x: %s failed: %s", agent.Name, *exec.ErrorMessage)
	}
	return fmt.Sprintf(":arrow_forward: %s started (execution %s).", agent.Name, exec.ID)
}

// ── helper: LLM call ────────────────────────────────────────────────────────

func (s *chatRouterService) callLLM(ctx context.Context, userPrompt string, maxTokens int) (string, error) {
	client := s.llmRouter.Default()
	resp, err := client.Generate(ctx, llm.GenerateRequest{
		Messages: []llm.Message{
			{Role: llm.RoleSystem, Content: llm.BuildChatSystemPrompt()},
			{Role: llm.RoleUser, Content: userPrompt},
		},
		MaxTokens:   maxTokens,
		Temperature: s.defaultTemp,
	})
	if err != nil {
		return "", err
	}
	return resp.Content, nil
}

// ── helper: data loads ──────────────────────────────────────────────────────

func (s *chatRouterService) loadAgents(ctx context.Context, orgID uuid.UUID) []llm.AgentSummary {
	page, err := s.agentSvc.List(ctx, orgID, model.PaginationParams{Page: 1, PerPage: s.listPageSize}, repository.AgentListFilter{})
	if err != nil {
		s.logger.Debug("chat_router: agent list failed", zap.Error(err))
		return nil
	}
	out := make([]llm.AgentSummary, 0, len(page.Data))
	for _, a := range page.Data {
		desc := ""
		if a.Description != nil {
			desc = *a.Description
		}
		out = append(out, llm.AgentSummary{
			Name:        a.Name,
			Role:        a.Role,
			Description: desc,
		})
	}
	return out
}

func (s *chatRouterService) loadWorkflows(ctx context.Context, orgID uuid.UUID) []llm.WorkflowSummary {
	page, err := s.workflowSvc.ListByOrg(ctx, orgID, model.PaginationParams{Page: 1, PerPage: s.listPageSize})
	if err != nil {
		s.logger.Debug("chat_router: workflow list failed", zap.Error(err))
		return nil
	}
	out := make([]llm.WorkflowSummary, 0, len(page.Data))
	for _, w := range page.Data {
		desc := ""
		if w.Description != nil {
			desc = *w.Description
		}
		out = append(out, llm.WorkflowSummary{Name: w.Name, Description: desc})
	}
	return out
}

func (s *chatRouterService) listTaskSummaries(ctx context.Context, orgID uuid.UUID) []llm.TaskSummary {
	if s.taskRepo == nil {
		return nil
	}
	page, err := s.taskRepo.ListByOrg(ctx, orgID, model.PaginationParams{Page: 1, PerPage: s.listPageSize})
	if err != nil {
		s.logger.Debug("chat_router: task list failed", zap.Error(err))
		return nil
	}
	out := make([]llm.TaskSummary, 0, len(page.Data))
	for _, t := range page.Data {
		out = append(out, llm.TaskSummary{ID: t.ID.String(), Title: t.Title})
	}
	return out
}

func (s *chatRouterService) findAgentByName(ctx context.Context, orgID uuid.UUID, name string) (*model.Agent, error) {
	page, err := s.agentSvc.List(ctx, orgID, model.PaginationParams{Page: 1, PerPage: 100}, repository.AgentListFilter{Search: name})
	if err != nil {
		return nil, err
	}
	lower := strings.ToLower(name)
	for i := range page.Data {
		if strings.EqualFold(page.Data[i].Name, name) {
			return &page.Data[i], nil
		}
	}
	// Loose fallback: first agent whose name contains the query.
	for i := range page.Data {
		if strings.Contains(strings.ToLower(page.Data[i].Name), lower) {
			return &page.Data[i], nil
		}
	}
	return nil, fmt.Errorf("agent %q not found", name)
}

func (s *chatRouterService) findWorkflowByName(ctx context.Context, orgID uuid.UUID, name string) (*model.Workflow, error) {
	page, err := s.workflowSvc.ListByOrg(ctx, orgID, model.PaginationParams{Page: 1, PerPage: 100})
	if err != nil {
		return nil, err
	}
	for i := range page.Data {
		if strings.EqualFold(page.Data[i].Name, name) {
			return &page.Data[i], nil
		}
	}
	lower := strings.ToLower(name)
	for i := range page.Data {
		if strings.Contains(strings.ToLower(page.Data[i].Name), lower) {
			return &page.Data[i], nil
		}
	}
	return nil, fmt.Errorf("workflow %q not found", name)
}

// ── formatting helpers ──────────────────────────────────────────────────────

func formatAgentList(agents []llm.AgentSummary) string {
	if len(agents) == 0 {
		return "No agents are configured for this organisation yet."
	}
	var sb strings.Builder
	sb.WriteString("Available agents:\n")
	for _, a := range agents {
		sb.WriteString("• ")
		sb.WriteString(a.Name)
		if a.Role != "" {
			sb.WriteString(" — ")
			sb.WriteString(a.Role)
		}
		sb.WriteString("\n")
	}
	return strings.TrimRight(sb.String(), "\n")
}

func (s *chatRouterService) formatTaskList(ctx context.Context, orgID uuid.UUID) string {
	tasks := s.listTaskSummaries(ctx, orgID)
	if len(tasks) == 0 {
		return "No tasks found."
	}
	var sb strings.Builder
	sb.WriteString("Recent tasks:\n")
	for _, t := range tasks {
		sb.WriteString(fmt.Sprintf("• %s — %s\n", t.ID, t.Title))
	}
	return strings.TrimRight(sb.String(), "\n")
}

func helpMessage() string {
	return `I can help you with:
• *run task* — kick off an agent with a request (e.g. "debug my payment service in kubernetes")
• *run workflow <name>* — start a multi-agent workflow
• *create task* — turn a request into a task draft
• *list agents* / *list tasks* — show what's available
• *status* — check the status of a task or run

Type naturally; I'll figure out the rest or ask a follow-up.`
}

func toLLMHistory(history []model.ChatMessage) []llm.Message {
	out := make([]llm.Message, 0, len(history))
	for _, m := range history {
		role := llm.RoleUser
		switch m.Role {
		case model.ChatRoleAgent:
			role = llm.RoleAssistant
		case model.ChatRoleSystem:
			role = llm.RoleSystem
		}
		out = append(out, llm.Message{Role: role, Content: m.Content})
	}
	return out
}
