package chatsvc

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/chatagent"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/platformtools"
)

// ChatRouteResult is what POST /chat/route returns. Surfaces that still
// expect the pre-upgrade shape keep working; the structured envelope is the
// source of truth.
type ChatRouteResult struct {
	// Intent is derived from the envelope, not classified: "confirm" when a
	// confirmation is pending, "clarify" when a question is being asked, the
	// first executed tool's name when tools ran, else "chat". Confidence was
	// an invention of the pre-upgrade classifier and is no longer reported.
	Intent          string                 `json:"intent"`
	Confidence      float64                `json:"confidence,omitempty"`
	Message         string                 `json:"message"`
	Agent           *model.Agent           `json:"agent,omitempty"`
	Execution       *model.AgentExecution  `json:"execution,omitempty"`
	Workflow        *model.Workflow        `json:"workflow,omitempty"`
	WorkflowRun     *model.WorkflowRun     `json:"workflow_run,omitempty"`
	ClarifyQuestion string                 `json:"clarify_question,omitempty"`
	Response        model.ChatResponse     `json:"response"`
}

// ChatRouterService is the stateless chat entrypoint (POST /chat/route).
type ChatRouterService interface {
	Route(ctx context.Context, orgID, userID, sessionID uuid.UUID, message string, history []model.ChatMessage) (*ChatRouteResult, error)
}

type chatRouterAdapter struct {
	agent  *chatagent.Agent
	logger *zap.Logger
}

// NewChatRouterService wraps the tool-calling agent as a stateless router so
// existing REST callers keep the same URL and response envelope.
func NewChatRouterService(agent *chatagent.Agent, logger *zap.Logger) ChatRouterService {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &chatRouterAdapter{agent: agent, logger: logger}
}

func (a *chatRouterAdapter) Route(ctx context.Context, orgID, userID, sessionID uuid.UUID, message string, history []model.ChatMessage) (*ChatRouteResult, error) {
	message = strings.TrimSpace(message)
	if message == "" {
		return &ChatRouteResult{
			Intent:   "clarify",
			Message:  "Please send a non-empty message.",
			Response: model.ChatResponse{Message: "Please send a non-empty message."},
		}, nil
	}
	if a.agent == nil {
		msg := "Chat is not configured."
		return &ChatRouteResult{Intent: "chat", Message: msg, Response: model.ChatResponse{Message: msg}}, nil
	}

	tr, err := a.agent.Run(ctx, chatagent.TurnRequest{
		Ident:    platformtools.Identity{OrgID: orgID, UserID: userID, SessionID: sessionID},
		Message:  message,
		History:  history,
		Metadata: map[string]any{},
	})
	if err != nil {
		msg := chatagent.HumaniseError(err)
		return &ChatRouteResult{Intent: "chat", Message: msg, Response: model.ChatResponse{Message: msg}}, nil
	}

	resp := tr.Response
	out := &ChatRouteResult{
		Intent:   intentFrom(resp),
		Message:  resp.Message,
		Response: resp,
	}
	if resp.Clarify != nil {
		out.ClarifyQuestion = resp.Clarify.Question
	}
	return out, nil
}

// intentFrom reads the intent off what actually happened this turn.
func intentFrom(resp model.ChatResponse) string {
	switch {
	case resp.Confirmation != nil:
		return "confirm"
	case resp.Clarify != nil:
		return "clarify"
	case len(resp.Actions) > 0:
		return resp.Actions[0].Tool
	default:
		return "chat"
	}
}
