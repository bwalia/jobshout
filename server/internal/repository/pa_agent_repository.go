package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jobshout/server/internal/model"
)

// PAAgentStore is the agent persistence for the Property PA installer
// (propertypa.AgentStore). Agents are found by a metadata marker rather than
// by name, so an admin may rename them.
type PAAgentStore struct {
	pool   *pgxpool.Pool
	agents AgentRepository
}

// NewPAAgentStore builds a PAAgentStore.
func NewPAAgentStore(pool *pgxpool.Pool, agents AgentRepository) *PAAgentStore {
	return &PAAgentStore{pool: pool, agents: agents}
}

// FindByPAKey returns the org's oldest agent with metadata.property_pa = key.
func (s *PAAgentStore) FindByPAKey(ctx context.Context, orgID uuid.UUID, key string) (*model.Agent, error) {
	a, err := scanAgent(s.pool.QueryRow(ctx, `SELECT `+agentColumns+`
		FROM agents WHERE org_id = $1 AND metadata->>'property_pa' = $2
		ORDER BY created_at ASC LIMIT 1`, orgID, key))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("pa_agent: find %q: %w", key, err)
	}
	return a, nil
}

// NameTaken reports whether the org has an agent with this name.
func (s *PAAgentStore) NameTaken(ctx context.Context, orgID uuid.UUID, name string) (bool, error) {
	var taken bool
	err := s.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM agents WHERE org_id = $1 AND lower(name) = lower($2))`,
		orgID, strings.TrimSpace(name)).Scan(&taken)
	if err != nil {
		return false, fmt.Errorf("pa_agent: name taken: %w", err)
	}
	return taken, nil
}

// Create inserts the agent (metadata included).
func (s *PAAgentStore) Create(ctx context.Context, a *model.Agent) error {
	return s.agents.Create(ctx, a)
}

// UpdateDefinition rewrites the definition fields and merges metadata. The
// org check is in the WHERE clause.
func (s *PAAgentStore) UpdateDefinition(ctx context.Context, orgID, agentID uuid.UUID, role, description, systemPrompt string, metadata map[string]any) error {
	md, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("pa_agent: encode metadata: %w", err)
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE agents SET role = $3, description = $4, system_prompt = $5, engine_type = $6,
			metadata = COALESCE(metadata, '{}'::jsonb) || $7::jsonb, updated_at = NOW()
		WHERE id = $1 AND org_id = $2`,
		agentID, orgID, role, description, systemPrompt, model.EngineGoNative, md)
	if err != nil {
		return fmt.Errorf("pa_agent: update: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("pa_agent: agent %s not found in org", agentID)
	}
	return nil
}
