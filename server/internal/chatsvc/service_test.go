package chatsvc

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/chatagent"
	"github.com/jobshout/server/internal/llm"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/platformtools"
	"github.com/jobshout/server/internal/repository"
)

// memChatRepo is an in-memory ChatRepository that fails if ctx is already
// canceled — the same failure mode Postgres would show after a reload.
type memChatRepo struct {
	mu       sync.Mutex
	sessions map[uuid.UUID]*model.ChatSession
	messages map[uuid.UUID][]model.ChatMessage
}

func newMemChatRepo() *memChatRepo {
	return &memChatRepo{
		sessions: map[uuid.UUID]*model.ChatSession{},
		messages: map[uuid.UUID][]model.ChatMessage{},
	}
}

func requireLive(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("repo: %w", err)
	}
	return nil
}

func (r *memChatRepo) CreateSession(ctx context.Context, session *model.ChatSession) error {
	if err := requireLive(ctx); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := *session
	if cp.Metadata == nil {
		cp.Metadata = map[string]any{}
	}
	r.sessions[cp.ID] = &cp
	return nil
}

func (r *memChatRepo) GetSession(ctx context.Context, id uuid.UUID) (*model.ChatSession, error) {
	if err := requireLive(ctx); err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.sessions[id]
	if !ok {
		return nil, fmt.Errorf("not found")
	}
	cp := *s
	meta := map[string]any{}
	for k, v := range s.Metadata {
		meta[k] = v
	}
	cp.Metadata = meta
	return &cp, nil
}

func (r *memChatRepo) UpdateSession(ctx context.Context, id uuid.UUID, agentID *uuid.UUID) error {
	if err := requireLive(ctx); err != nil {
		return err
	}
	return nil
}

func (r *memChatRepo) ListSessions(ctx context.Context, orgID, userID uuid.UUID, params model.PaginationParams) (*model.PaginatedResponse[model.ChatSession], error) {
	if err := requireLive(ctx); err != nil {
		return nil, err
	}
	return &model.PaginatedResponse[model.ChatSession]{}, nil
}

func (r *memChatRepo) AppendMessage(ctx context.Context, msg *model.ChatMessage) error {
	if err := requireLive(ctx); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := *msg
	if cp.CreatedAt.IsZero() {
		cp.CreatedAt = time.Now()
	}
	r.messages[cp.SessionID] = append(r.messages[cp.SessionID], cp)
	return nil
}

func (r *memChatRepo) ListMessages(ctx context.Context, sessionID uuid.UUID, limit int) ([]model.ChatMessage, error) {
	if err := requireLive(ctx); err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	msgs := r.messages[sessionID]
	out := make([]model.ChatMessage, len(msgs))
	copy(out, msgs)
	if limit > 0 && len(out) > limit {
		out = out[len(out)-limit:]
	}
	return out, nil
}

func (r *memChatRepo) UpdateSessionMetadata(ctx context.Context, id uuid.UUID, metadata map[string]any) error {
	if err := requireLive(ctx); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.sessions[id]
	if !ok {
		return fmt.Errorf("not found")
	}
	cp := map[string]any{}
	for k, v := range metadata {
		cp[k] = v
	}
	s.Metadata = cp
	s.UpdatedAt = time.Now()
	return nil
}

func (r *memChatRepo) DeleteSession(ctx context.Context, id uuid.UUID) error {
	if err := requireLive(ctx); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.sessions, id)
	delete(r.messages, id)
	return nil
}

var _ repository.ChatRepository = (*memChatRepo)(nil)

type gatedLLM struct {
	started chan struct{}
	release chan struct{}
}

func (g *gatedLLM) Generate(ctx context.Context, _ llm.GenerateRequest) (*llm.GenerateResponse, error) {
	select {
	case <-g.started:
	default:
		close(g.started)
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-g.release:
		return &llm.GenerateResponse{Content: "Here is the reply.", FinishReason: "stop"}, nil
	}
}
func (g *gatedLLM) ProviderName() string { return "gated" }
func (g *gatedLLM) SupportsTools() bool  { return true }

func TestSendTurn_SurvivesCanceledRequest(t *testing.T) {
	repo := newMemChatRepo()
	org, user, sid := uuid.New(), uuid.New(), uuid.New()
	if err := repo.CreateSession(context.Background(), &model.ChatSession{
		ID: sid, OrgID: org, UserID: user, Source: model.ChatSourceWeb, Metadata: map[string]any{},
	}); err != nil {
		t.Fatal(err)
	}

	gated := &gatedLLM{started: make(chan struct{}), release: make(chan struct{})}
	reg := platformtools.NewRegistry()
	agent := chatagent.New(gated, reg, platformtools.NewGuard(nil, nil), nil, zap.NewNop())
	svc := NewChatService(repo, agent, zap.NewNop())

	reqCtx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	var turn *model.ChatTurnResult
	go func() {
		var err error
		turn, err = svc.SendTurn(reqCtx, org, user, sid, "hello", "web", "", nil)
		errCh <- err
	}()

	select {
	case <-gated.started:
	case <-time.After(2 * time.Second):
		t.Fatal("agent never started")
	}

	cancel() // reload: the HTTP request context dies
	close(gated.release)

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("SendTurn after cancel: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("SendTurn did not return")
	}

	if turn == nil || turn.AgentMessage == nil || turn.AgentMessage.Content != "Here is the reply." {
		t.Fatalf("want persisted reply, got %+v", turn)
	}
	msgs, err := repo.ListMessages(context.Background(), sid, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 2 {
		t.Fatalf("messages = %d; want 2 (user+agent)", len(msgs))
	}
	if msgs[1].Role != model.ChatRoleAgent || msgs[1].Content != "Here is the reply." {
		t.Fatalf("agent message = %+v", msgs[1])
	}
	got, _ := repo.GetSession(context.Background(), sid)
	if _, ok := got.Metadata[model.ChatMetaTurnStartedAt]; ok {
		t.Fatalf("turn_started_at should be cleared: %+v", got.Metadata)
	}
}

func TestSendTurn_CanceledBeforeStartStillPersists(t *testing.T) {
	repo := newMemChatRepo()
	org, user, sid := uuid.New(), uuid.New(), uuid.New()
	if err := repo.CreateSession(context.Background(), &model.ChatSession{
		ID: sid, OrgID: org, UserID: user, Source: model.ChatSourceWeb, Metadata: map[string]any{},
	}); err != nil {
		t.Fatal(err)
	}
	client := &scriptedOK{}
	agent := chatagent.New(client, platformtools.NewRegistry(), platformtools.NewGuard(nil, nil), nil, zap.NewNop())
	svc := NewChatService(repo, agent, zap.NewNop())

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	turn, err := svc.SendTurn(ctx, org, user, sid, "hello", "web", "", nil)
	if err != nil {
		t.Fatalf("already-canceled request must still complete: %v", err)
	}
	if turn.AgentMessage == nil || turn.AgentMessage.Content == "" {
		t.Fatal("missing agent reply")
	}
}

type scriptedOK struct{}

func (s *scriptedOK) Generate(_ context.Context, _ llm.GenerateRequest) (*llm.GenerateResponse, error) {
	return &llm.GenerateResponse{Content: "Okay.", FinishReason: "stop"}, nil
}
func (s *scriptedOK) ProviderName() string { return "ok" }
func (s *scriptedOK) SupportsTools() bool  { return true }
