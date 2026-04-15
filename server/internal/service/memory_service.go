package service

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/llm"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/repository"
)

// MemoryService provides short-term and long-term memory operations for agents.
type MemoryService interface {
	LoadShortTerm(ctx context.Context, agentID, sessionID uuid.UUID) ([]llm.Message, error)
	SaveShortTerm(ctx context.Context, agentID, sessionID uuid.UUID, messages []llm.Message) error
	Append(ctx context.Context, agentID, orgID uuid.UUID, content, summary string) error
	Recall(ctx context.Context, agentID uuid.UUID, query string, limit int) ([]string, error)
}

type memoryService struct {
	repo   repository.MemoryRepository
	logger *zap.Logger
}

func NewMemoryService(repo repository.MemoryRepository, logger *zap.Logger) MemoryService {
	return &memoryService{repo: repo, logger: logger}
}

func (s *memoryService) LoadShortTerm(ctx context.Context, agentID, sessionID uuid.UUID) ([]llm.Message, error) {
	mem, err := s.repo.GetShortTerm(ctx, agentID, sessionID)
	if err != nil {
		return nil, nil // Not found is not an error — empty context.
	}
	var messages []llm.Message
	if err := json.Unmarshal(mem.Messages, &messages); err != nil {
		s.logger.Warn("failed to unmarshal short-term memory", zap.Error(err))
		return nil, nil
	}
	return messages, nil
}

func (s *memoryService) SaveShortTerm(ctx context.Context, agentID, sessionID uuid.UUID, messages []llm.Message) error {
	// Keep only the last 20 turns to prevent unbounded growth.
	const maxTurns = 20
	if len(messages) > maxTurns {
		messages = messages[len(messages)-maxTurns:]
	}

	data, err := json.Marshal(messages)
	if err != nil {
		return fmt.Errorf("memory_svc: marshal short-term: %w", err)
	}

	return s.repo.UpsertShortTerm(ctx, &model.AgentMemoryShortTerm{
		AgentID:   agentID,
		SessionID: sessionID,
		Messages:  data,
	})
}

func (s *memoryService) Append(ctx context.Context, agentID, orgID uuid.UUID, content, summary string) error {
	return s.repo.AppendLongTerm(ctx, &model.AgentMemoryLongTerm{
		AgentID: agentID,
		OrgID:   orgID,
		Content: content,
		Summary: summary,
	})
}

func (s *memoryService) Recall(ctx context.Context, agentID uuid.UUID, query string, limit int) ([]string, error) {
	mems, err := s.repo.SearchLongTerm(ctx, agentID, query, limit)
	if err != nil {
		return nil, err
	}
	results := make([]string, 0, len(mems))
	for _, m := range mems {
		if m.Summary != "" {
			results = append(results, m.Summary)
		} else {
			results = append(results, m.Content)
		}
	}
	return results, nil
}
