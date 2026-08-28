package chatsvc

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/chatagent"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/platformtools"
	"github.com/jobshout/server/internal/repository"
)

// chatTurnTimeout bounds one agent loop after it is detached from the HTTP
// request. A reload cancels r.Context(); this deadline is independent of that.
const chatTurnTimeout = 10 * time.Minute

// ChatService manages chat sessions and dispatches messages through the
// tool-calling chat agent.
type ChatService interface {
	StartSession(ctx context.Context, orgID, userID uuid.UUID, req model.StartChatSessionRequest) (*model.ChatSession, error)
	GetSession(ctx context.Context, orgID, userID, sessionID uuid.UUID) (*model.ChatSession, error)
	// CreateSession inserts a session with a caller-chosen ID (Telegram uses a
	// deterministic UUID). Prefer StartSession for the web/API.
	CreateSession(ctx context.Context, session *model.ChatSession) error
	DeleteSession(ctx context.Context, orgID, userID, sessionID uuid.UUID) error

	SendMessage(ctx context.Context, orgID, userID, sessionID uuid.UUID, content, source string) (*model.ChatMessage, *model.ChatMessage, error)
	SendTurn(ctx context.Context, orgID, userID, sessionID uuid.UUID, content, source, confirmToken string, stream chatagent.StreamFunc) (*model.ChatTurnResult, error)
	SendWithConfirm(ctx context.Context, orgID, userID, sessionID uuid.UUID, content, source, confirmToken string) (*model.ChatTurnResult, error)
	GetHistory(ctx context.Context, orgID, userID, sessionID uuid.UUID, limit int) ([]model.ChatMessage, error)
	ListSessions(ctx context.Context, orgID, userID uuid.UUID, params model.PaginationParams) (*model.PaginatedResponse[model.ChatSession], error)
}

type chatService struct {
	chatRepo  repository.ChatRepository
	agent     *chatagent.Agent
	logger    *zap.Logger
	mu        sync.Mutex
	sessionMu map[uuid.UUID]*sync.Mutex
}

func NewChatService(
	chatRepo repository.ChatRepository,
	agent *chatagent.Agent,
	logger *zap.Logger,
) ChatService {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &chatService{
		chatRepo:  chatRepo,
		agent:     agent,
		logger:    logger,
		sessionMu: map[uuid.UUID]*sync.Mutex{},
	}
}

func (s *chatService) lockSession(id uuid.UUID) func() {
	s.mu.Lock()
	m, ok := s.sessionMu[id]
	if !ok {
		m = &sync.Mutex{}
		s.sessionMu[id] = m
	}
	s.mu.Unlock()
	m.Lock()
	return m.Unlock
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

func (s *chatService) GetSession(ctx context.Context, orgID, userID, sessionID uuid.UUID) (*model.ChatSession, error) {
	session, err := s.chatRepo.GetSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if session.OrgID != orgID || session.UserID != userID {
		return nil, fmt.Errorf("chat_svc: session not found")
	}
	return session, nil
}

func (s *chatService) CreateSession(ctx context.Context, session *model.ChatSession) error {
	if session.Metadata == nil {
		session.Metadata = map[string]any{}
	}
	return s.chatRepo.CreateSession(ctx, session)
}

func (s *chatService) DeleteSession(ctx context.Context, orgID, userID, sessionID uuid.UUID) error {
	if _, err := s.GetSession(ctx, orgID, userID, sessionID); err != nil {
		return err
	}
	return s.chatRepo.DeleteSession(ctx, sessionID)
}

func (s *chatService) SendMessage(ctx context.Context, orgID, userID, sessionID uuid.UUID, content, source string) (*model.ChatMessage, *model.ChatMessage, error) {
	turn, err := s.SendTurn(ctx, orgID, userID, sessionID, content, source, "", nil)
	if err != nil {
		return nil, nil, err
	}
	return turn.UserMessage, turn.AgentMessage, nil
}

func (s *chatService) SendWithConfirm(ctx context.Context, orgID, userID, sessionID uuid.UUID, content, source, confirmToken string) (*model.ChatTurnResult, error) {
	return s.SendTurn(ctx, orgID, userID, sessionID, content, source, confirmToken, nil)
}

func (s *chatService) SendTurn(ctx context.Context, orgID, userID, sessionID uuid.UUID, content, source, confirmToken string, stream chatagent.StreamFunc) (*model.ChatTurnResult, error) {
	if source == "" {
		source = model.ChatSourceWeb
	}

	// Reloading (or the 30s request-timeout middleware) cancels r.Context().
	// The turn has to outlive that: tools already ran, and the reply must
	// still be persisted so the client can catch up on the next load.
	runCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), chatTurnTimeout)
	defer cancel()

	unlock := s.lockSession(sessionID)
	defer unlock()

	session, err := s.GetSession(runCtx, orgID, userID, sessionID)
	if err != nil {
		return nil, err
	}

	userMsg := &model.ChatMessage{
		ID:        uuid.New(),
		SessionID: sessionID,
		OrgID:     orgID,
		Role:      model.ChatRoleUser,
		Source:    source,
		Content:   content,
		Metadata:  map[string]any{},
	}
	if err := s.chatRepo.AppendMessage(runCtx, userMsg); err != nil {
		return nil, fmt.Errorf("chat_svc: persist user message: %w", err)
	}

	history, err := s.chatRepo.ListMessages(runCtx, sessionID, chatagent.MaxHistoryLoad)
	if err != nil {
		s.logger.Warn("chat_svc: list history", zap.Error(err))
		history = nil
	}
	// The user message we just wrote is included; the agent should not see it
	// twice. Drop the last user turn if it matches.
	history = dropTrailingUser(history, userMsg.ID)

	meta := session.Metadata
	if meta == nil {
		meta = map[string]any{}
	}
	if _, ok := meta["title"]; !ok {
		meta["title"] = titleFrom(content)
	}
	meta[model.ChatMetaTurnStartedAt] = time.Now().UTC().Format(time.RFC3339)
	if err := s.chatRepo.UpdateSessionMetadata(runCtx, sessionID, meta); err != nil {
		s.logger.Warn("chat_svc: mark turn in flight", zap.Error(err))
	}

	clearInFlight := func(m map[string]any) {
		if m == nil {
			return
		}
		delete(m, model.ChatMetaTurnStartedAt)
		if err := s.chatRepo.UpdateSessionMetadata(runCtx, sessionID, m); err != nil {
			s.logger.Warn("chat_svc: persist session metadata", zap.Error(err))
		}
	}

	if s.agent == nil {
		clearInFlight(meta)
		return s.fallbackTurn(userMsg, source, "Chat is not configured.")
	}

	tr, err := s.agent.Run(runCtx, chatagent.TurnRequest{
		Ident: platformtools.Identity{
			OrgID: orgID, UserID: userID, SessionID: sessionID,
		},
		Message:           content,
		ConfirmationToken: confirmToken,
		History:           history,
		Metadata:          meta,
		Stream:            stream,
		Source:            source,
	})
	if err != nil {
		s.logger.Warn("chat agent failed", zap.Error(err))
		clearInFlight(meta)
		return s.fallbackTurn(userMsg, source, chatagent.HumaniseError(err))
	}

	resp := tr.Response
	resp.Message = chatagent.SanitiseMessage(resp.Message)
	if tr.Metadata == nil {
		tr.Metadata = meta
	}
	delete(tr.Metadata, model.ChatMetaTurnStartedAt)
	if err := s.chatRepo.UpdateSessionMetadata(runCtx, sessionID, tr.Metadata); err != nil {
		s.logger.Warn("chat_svc: persist session metadata", zap.Error(err))
	}

	for i := range tr.Transcript {
		row := tr.Transcript[i]
		if row.ID == uuid.Nil {
			row.ID = uuid.New()
		}
		row.SessionID = sessionID
		row.OrgID = orgID
		if row.Source == "" {
			row.Source = source
		}
		if err := s.chatRepo.AppendMessage(runCtx, &row); err != nil {
			s.logger.Warn("chat_svc: persist tool transcript", zap.Error(err))
		}
	}

	agentMeta := map[string]any{
		"response": resp,
		"actions":  resp.Actions,
	}
	agentMsg := &model.ChatMessage{
		ID:        uuid.New(),
		SessionID: sessionID,
		OrgID:     orgID,
		Role:      model.ChatRoleAgent,
		Source:    source,
		Content:   resp.Message,
		Metadata:  agentMeta,
	}
	if err := s.chatRepo.AppendMessage(runCtx, agentMsg); err != nil {
		s.logger.Warn("failed to persist agent response", zap.Error(err))
	}

	return &model.ChatTurnResult{
		UserMessage:  userMsg,
		AgentMessage: agentMsg,
		Response:     resp,
	}, nil
}

func (s *chatService) fallbackTurn(userMsg *model.ChatMessage, source, message string) (*model.ChatTurnResult, error) {
	agentMsg := &model.ChatMessage{
		ID:        uuid.New(),
		SessionID: userMsg.SessionID,
		OrgID:     userMsg.OrgID,
		Role:      model.ChatRoleAgent,
		Source:    source,
		Content:   message,
		Metadata:  map[string]any{},
	}
	_ = s.chatRepo.AppendMessage(context.Background(), agentMsg)
	resp := model.ChatResponse{Message: message}
	return &model.ChatTurnResult{UserMessage: userMsg, AgentMessage: agentMsg, Response: resp}, nil
}

func (s *chatService) GetHistory(ctx context.Context, orgID, userID, sessionID uuid.UUID, limit int) ([]model.ChatMessage, error) {
	if _, err := s.GetSession(ctx, orgID, userID, sessionID); err != nil {
		return nil, err
	}
	return s.chatRepo.ListMessages(ctx, sessionID, limit)
}

func (s *chatService) ListSessions(ctx context.Context, orgID, userID uuid.UUID, params model.PaginationParams) (*model.PaginatedResponse[model.ChatSession], error) {
	return s.chatRepo.ListSessions(ctx, orgID, userID, params)
}

func dropTrailingUser(history []model.ChatMessage, justWrote uuid.UUID) []model.ChatMessage {
	if len(history) == 0 {
		return history
	}
	last := history[len(history)-1]
	if last.ID == justWrote {
		return history[:len(history)-1]
	}
	return history
}

func titleFrom(content string) string {
	s := strings.TrimSpace(strings.ReplaceAll(content, "\n", " "))
	if s == "" {
		return "New chat"
	}
	if len(s) > 80 {
		return strings.TrimSpace(s[:80]) + "…"
	}
	return s
}
