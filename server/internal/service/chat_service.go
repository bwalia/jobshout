package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/repository"
)

// ChatService manages chat sessions and dispatches messages to appropriate handlers.
type ChatService interface {
	StartSession(ctx context.Context, orgID, userID uuid.UUID, req model.StartChatSessionRequest) (*model.ChatSession, error)
	SendMessage(ctx context.Context, orgID, userID, sessionID uuid.UUID, content, source string) (*model.ChatMessage, *model.ChatMessage, error)
	GetHistory(ctx context.Context, sessionID uuid.UUID, limit int) ([]model.ChatMessage, error)
	ListSessions(ctx context.Context, orgID, userID uuid.UUID, params model.PaginationParams) (*model.PaginatedResponse[model.ChatSession], error)
}

type chatService struct {
	chatRepo  repository.ChatRepository
	intentSvc IntentService
	memorySvc MemoryService
	goalSvc   GoalService
	logger    *zap.Logger
}

func NewChatService(
	chatRepo repository.ChatRepository,
	intentSvc IntentService,
	memorySvc MemoryService,
	goalSvc GoalService,
	logger *zap.Logger,
) ChatService {
	return &chatService{
		chatRepo:  chatRepo,
		intentSvc: intentSvc,
		memorySvc: memorySvc,
		goalSvc:   goalSvc,
		logger:    logger,
	}
}

func (s *chatService) StartSession(ctx context.Context, orgID, userID uuid.UUID, req model.StartChatSessionRequest) (*model.ChatSession, error) {
	source := req.Source
	if source == "" {
		source = model.ChatSourceWeb
	}

	session := &model.ChatSession{
		ID:       uuid.New(),
		OrgID:    orgID,
		UserID:   userID,
		AgentID:  req.AgentID,
		Source:   source,
		Metadata: map[string]any{},
	}

	if err := s.chatRepo.CreateSession(ctx, session); err != nil {
		return nil, fmt.Errorf("chat_svc: create session: %w", err)
	}
	return session, nil
}

func (s *chatService) SendMessage(ctx context.Context, orgID, userID, sessionID uuid.UUID, content, source string) (*model.ChatMessage, *model.ChatMessage, error) {
	if source == "" {
		source = model.ChatSourceWeb
	}

	// Persist the user message.
	userMsg := &model.ChatMessage{
		ID:        uuid.New(),
		SessionID: sessionID,
		OrgID:     orgID,
		Role:      model.ChatRoleUser,
		Source:    source,
		Content:   content,
		Metadata:  map[string]any{},
	}
	if err := s.chatRepo.AppendMessage(ctx, userMsg); err != nil {
		return nil, nil, fmt.Errorf("chat_svc: persist user message: %w", err)
	}

	// Parse intent.
	intent, err := s.intentSvc.Parse(ctx, content)
	if err != nil {
		s.logger.Warn("intent parsing failed, defaulting to chat", zap.Error(err))
		intent = &ParsedIntent{Action: IntentChat, Parameters: map[string]any{}}
	}

	// Dispatch based on intent and build response.
	responseContent := s.dispatch(ctx, orgID, userID, sessionID, intent, content)

	// Persist the agent response.
	agentMsg := &model.ChatMessage{
		ID:        uuid.New(),
		SessionID: sessionID,
		OrgID:     orgID,
		Role:      model.ChatRoleAgent,
		Source:    source,
		Content:   responseContent,
		Metadata: map[string]any{
			"intent":     intent.Action,
			"confidence": intent.Confidence,
		},
	}
	if err := s.chatRepo.AppendMessage(ctx, agentMsg); err != nil {
		s.logger.Warn("failed to persist agent response", zap.Error(err))
	}

	return userMsg, agentMsg, nil
}

func (s *chatService) GetHistory(ctx context.Context, sessionID uuid.UUID, limit int) ([]model.ChatMessage, error) {
	return s.chatRepo.ListMessages(ctx, sessionID, limit)
}

func (s *chatService) ListSessions(ctx context.Context, orgID, userID uuid.UUID, params model.PaginationParams) (*model.PaginatedResponse[model.ChatSession], error) {
	return s.chatRepo.ListSessions(ctx, orgID, userID, params)
}

func (s *chatService) dispatch(ctx context.Context, orgID, userID, sessionID uuid.UUID, intent *ParsedIntent, rawMessage string) string {
	switch intent.Action {
	case IntentRunGoal:
		return s.handleRunGoal(ctx, orgID, intent)
	case IntentHelp:
		return s.handleHelp()
	case IntentListAgents:
		return "To view agents, visit the Agents page in the dashboard or use GET /api/v1/agents."
	case IntentListTasks:
		return "To view tasks, visit the Tasks page in the dashboard or use GET /api/v1/tasks."
	case IntentListWorkflows:
		return "To view workflows, visit the Workflows page or use GET /api/v1/workflows."
	case IntentStatus:
		return "To check status, provide the resource type (task/goal/execution) and ID. Use GET /api/v1/goals/{id} or GET /api/v1/executions/{id}."
	case IntentCreateTask:
		return s.handleCreateTask(intent)
	case IntentExecuteAgent:
		return "To execute an agent, provide the agent name and a prompt. Use POST /api/v1/agents/{id}/execute."
	default:
		return fmt.Sprintf("I understood your message. Intent detected: %s (confidence: %.0f%%). "+
			"I can help you run goals, create tasks, list agents, and more. Type 'help' for available commands.",
			intent.Action, intent.Confidence*100)
	}
}

func (s *chatService) handleRunGoal(ctx context.Context, orgID uuid.UUID, intent *ParsedIntent) string {
	agentName, _ := intent.Parameters["agent_name"].(string)
	goal, _ := intent.Parameters["goal"].(string)
	if goal == "" {
		goal, _ = intent.Parameters["prompt"].(string)
	}

	if goal == "" {
		return "Please provide a goal description. Example: 'Run goal: investigate the API latency spike'"
	}

	if agentName == "" {
		return fmt.Sprintf("Goal received: %q. Please specify which agent should work on this, "+
			"or create a goal via POST /api/v1/agents/{agentID}/goals with the goal text.", goal)
	}

	return fmt.Sprintf("Goal %q will be assigned to agent %q. "+
		"Use POST /api/v1/agents/{agentID}/goals with {\"goal_text\": %q} to start execution.", goal, agentName, goal)
}

func (s *chatService) handleCreateTask(intent *ParsedIntent) string {
	title, _ := intent.Parameters["title"].(string)
	if title == "" {
		return "Please provide a task title. Example: 'Create task: Fix payment timeout issue'"
	}
	return fmt.Sprintf("Task %q noted. Create it via POST /api/v1/tasks with the full details.", title)
}

func (s *chatService) handleHelp() string {
	return `Available commands:
- "run goal: <description>" — Start an autonomous agent goal
- "create task: <title>" — Create a new task
- "list agents" — Show available agents
- "list tasks" — Show recent tasks
- "list workflows" — Show workflows
- "status <resource> <id>" — Check status
- "help" — Show this message

You can also type naturally and I'll try to understand your intent.`
}
