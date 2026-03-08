package repository

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// AgentToolPermission records which tool names an agent may use.
type AgentToolPermission struct {
	AgentID  uuid.UUID      `json:"agent_id"`
	ToolName string         `json:"tool_name"`
	Config   map[string]any `json:"config"`
}

// AgentToolRepository manages tool permission records for agents.
type AgentToolRepository interface {
	ListByAgent(ctx context.Context, agentID uuid.UUID) ([]string, error)
	Set(ctx context.Context, agentID uuid.UUID, toolNames []string) error
}

type agentToolRepository struct {
	pool *pgxpool.Pool
}

// NewAgentToolRepository creates a new AgentToolRepository.
func NewAgentToolRepository(pool *pgxpool.Pool) AgentToolRepository {
	return &agentToolRepository{pool: pool}
}

func (r *agentToolRepository) ListByAgent(ctx context.Context, agentID uuid.UUID) ([]string, error) {
	const sql = `SELECT tool_name FROM agent_tool_permissions WHERE agent_id = $1 ORDER BY tool_name`
	rows, err := r.pool.Query(ctx, sql, agentID)
	if err != nil {
		return nil, fmt.Errorf("agent_tool_repo: list tools: %w", err)
	}
	defer rows.Close()

	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("agent_tool_repo: scan tool name: %w", err)
		}
		names = append(names, name)
	}
	return names, rows.Err()
}

func (r *agentToolRepository) Set(ctx context.Context, agentID uuid.UUID, toolNames []string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("agent_tool_repo: begin transaction: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if _, err = tx.Exec(ctx, `DELETE FROM agent_tool_permissions WHERE agent_id = $1`, agentID); err != nil {
		return fmt.Errorf("agent_tool_repo: clear tools: %w", err)
	}

	emptyConfig, _ := json.Marshal(map[string]any{})
	for _, name := range toolNames {
		const sql = `INSERT INTO agent_tool_permissions (agent_id, tool_name, config) VALUES ($1, $2, $3)`
		if _, err = tx.Exec(ctx, sql, agentID, name, emptyConfig); err != nil {
			return fmt.Errorf("agent_tool_repo: insert tool %q: %w", name, err)
		}
	}

	return tx.Commit(ctx)
}
