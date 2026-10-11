package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jobshout/server/internal/model"
)

// ErrKnowledgeNotFound is returned when a knowledge file or its agent does not
// exist OR belongs to another org — deliberately indistinguishable, so ids
// cannot be probed across the tenant boundary.
var ErrKnowledgeNotFound = errors.New("knowledge file not found")

// KnowledgeFileStore is the org-scoped persistence behind the agent knowledge
// endpoints. Every method takes the caller's org and the {agentID} of the
// route: a file is only visible through its own agent, and only to that
// agent's org (knowledge_files has no org_id; the boundary is agents.org_id).
type KnowledgeFileStore interface {
	// AgentInOrg reports whether agentID exists and belongs to orgID.
	AgentInOrg(ctx context.Context, orgID, agentID string) (bool, error)
	ListByAgent(ctx context.Context, orgID, agentID string) ([]model.KnowledgeFile, error)
	Get(ctx context.Context, orgID, agentID, fileID string) (*model.KnowledgeFile, error)
	Create(ctx context.Context, agentID, filename, content string) (string, error)
	UpdateContent(ctx context.Context, orgID, agentID, fileID, content string) error
	Delete(ctx context.Context, orgID, agentID, fileID string) error
}

type knowledgeFileStore struct {
	pool *pgxpool.Pool
}

// NewKnowledgeFileStore returns the Postgres KnowledgeFileStore.
func NewKnowledgeFileStore(pool *pgxpool.Pool) KnowledgeFileStore {
	return &knowledgeFileStore{pool: pool}
}

// The ownership predicate shared by every file query: the file's agent is the
// route's agent, and that agent is in the caller's org.
const knowledgeOwned = `kf.agent_id = $2::uuid AND a.org_id = $1::uuid`

func (s *knowledgeFileStore) AgentInOrg(ctx context.Context, orgID, agentID string) (bool, error) {
	var ok bool
	err := s.pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM agents WHERE id = $2::uuid AND org_id = $1::uuid)`,
		orgID, agentID).Scan(&ok)
	if err != nil {
		return false, fmt.Errorf("knowledge: agent in org: %w", err)
	}
	return ok, nil
}

func (s *knowledgeFileStore) ListByAgent(ctx context.Context, orgID, agentID string) ([]model.KnowledgeFile, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT kf.id, kf.agent_id, kf.filename, kf.content, kf.size_bytes, kf.created_at, kf.updated_at
			FROM knowledge_files kf JOIN agents a ON a.id = kf.agent_id
			WHERE `+knowledgeOwned+` ORDER BY kf.filename`, orgID, agentID)
	if err != nil {
		return nil, fmt.Errorf("knowledge: list: %w", err)
	}
	defer rows.Close()
	files := []model.KnowledgeFile{}
	for rows.Next() {
		var f model.KnowledgeFile
		if err := rows.Scan(&f.ID, &f.AgentID, &f.Filename, &f.Content, &f.SizeBytes, &f.CreatedAt, &f.UpdatedAt); err != nil {
			return nil, fmt.Errorf("knowledge: scan: %w", err)
		}
		files = append(files, f)
	}
	return files, rows.Err()
}

func (s *knowledgeFileStore) Get(ctx context.Context, orgID, agentID, fileID string) (*model.KnowledgeFile, error) {
	var f model.KnowledgeFile
	err := s.pool.QueryRow(ctx,
		`SELECT kf.id, kf.agent_id, kf.filename, kf.content, kf.size_bytes, kf.created_at, kf.updated_at
			FROM knowledge_files kf JOIN agents a ON a.id = kf.agent_id
			WHERE kf.id = $3::uuid AND `+knowledgeOwned, orgID, agentID, fileID,
	).Scan(&f.ID, &f.AgentID, &f.Filename, &f.Content, &f.SizeBytes, &f.CreatedAt, &f.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrKnowledgeNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("knowledge: get: %w", err)
	}
	return &f, nil
}

func (s *knowledgeFileStore) Create(ctx context.Context, agentID, filename, content string) (string, error) {
	var id string
	err := s.pool.QueryRow(ctx,
		`INSERT INTO knowledge_files (agent_id, filename, content, size_bytes)
			VALUES ($1, $2, $3, $4) RETURNING id`,
		agentID, filename, content, len([]byte(content)),
	).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("knowledge: create: %w", err)
	}
	return id, nil
}

func (s *knowledgeFileStore) UpdateContent(ctx context.Context, orgID, agentID, fileID, content string) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE knowledge_files kf SET content = $4, size_bytes = $5, updated_at = NOW()
			FROM agents a
			WHERE a.id = kf.agent_id AND kf.id = $3::uuid AND `+knowledgeOwned,
		orgID, agentID, fileID, content, len([]byte(content)))
	if err != nil {
		return fmt.Errorf("knowledge: update: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrKnowledgeNotFound
	}
	return nil
}

func (s *knowledgeFileStore) Delete(ctx context.Context, orgID, agentID, fileID string) error {
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM knowledge_files kf USING agents a
			WHERE a.id = kf.agent_id AND kf.id = $3::uuid AND `+knowledgeOwned,
		orgID, agentID, fileID)
	if err != nil {
		return fmt.Errorf("knowledge: delete: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrKnowledgeNotFound
	}
	return nil
}
