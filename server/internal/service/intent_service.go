package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"go.uber.org/zap"

	"github.com/jobshout/server/internal/llm"
)

// Supported intent actions.
const (
	IntentCreateTask    = "create_task"
	IntentExecuteAgent  = "execute_agent"
	IntentRunGoal       = "run_goal"
	IntentRunWorkflow   = "run_workflow"
	IntentListAgents    = "list_agents"
	IntentListTasks     = "list_tasks"
	IntentListWorkflows = "list_workflows"
	IntentStatus        = "status"
	IntentHelp          = "help"
	IntentChat          = "chat"
)

// AllIntentActions is the full set of actions the intent classifier can return.
var AllIntentActions = []string{
	IntentCreateTask + " — create a new task (params: title, description)",
	IntentExecuteAgent + " — run an agent on a prompt (params: agent_name, prompt)",
	IntentRunGoal + " — start an autonomous goal (params: agent_name, goal)",
	IntentRunWorkflow + " — execute a workflow (params: workflow_name)",
	IntentListAgents + " — list available agents",
	IntentListTasks + " — list recent tasks",
	IntentListWorkflows + " — list workflows",
	IntentStatus + " — check status of a task/execution/goal (params: resource_type, id)",
	IntentHelp + " — show help information",
	IntentChat + " — general conversation or unrecognised intent",
}

// ParsedIntent is the result of classifying a user message.
type ParsedIntent struct {
	Action     string         `json:"action"`
	Parameters map[string]any `json:"parameters"`
	Confidence float64        `json:"confidence"`
}

// IntentService parses natural language messages into structured intents.
type IntentService interface {
	Parse(ctx context.Context, userMessage string) (*ParsedIntent, error)
}

type intentService struct {
	llmRouter *llm.Router
	logger    *zap.Logger
}

func NewIntentService(llmRouter *llm.Router, logger *zap.Logger) IntentService {
	return &intentService{llmRouter: llmRouter, logger: logger}
}

func (s *intentService) Parse(ctx context.Context, userMessage string) (*ParsedIntent, error) {
	client := s.llmRouter.Default()

	prompt := llm.BuildIntentPrompt(userMessage, AllIntentActions)

	resp, err := client.Generate(ctx, llm.GenerateRequest{
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: prompt},
		},
		MaxTokens:   512,
		Temperature: 0.1,
	})
	if err != nil {
		return nil, fmt.Errorf("intent_svc: generate: %w", err)
	}

	content := strings.TrimSpace(resp.Content)
	content = extractIntentJSON(content)

	var parsed ParsedIntent
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		s.logger.Warn("failed to parse intent JSON, defaulting to chat",
			zap.String("raw", resp.Content), zap.Error(err))
		return &ParsedIntent{
			Action:     IntentChat,
			Parameters: map[string]any{"raw": userMessage},
			Confidence: 0.0,
		}, nil
	}

	if parsed.Parameters == nil {
		parsed.Parameters = map[string]any{}
	}
	return &parsed, nil
}

func extractIntentJSON(s string) string {
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	s = strings.TrimSpace(s)

	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start == -1 || end == -1 || end < start {
		return s
	}
	return s[start : end+1]
}
