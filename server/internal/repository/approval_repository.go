package repository

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jobshout/server/internal/model"
)

// ApprovalRepository persists human-in-the-loop approvals for gated agent tool
// calls, plus the per-agent gate rules that decide which tools require approval.
type ApprovalRepository interface {
	Create(ctx context.Context, a *model.Approval) error
	FindByID(ctx context.Context, id uuid.UUID) (*model.Approval, error)
	// ListByOrg returns approvals for an org, optionally filtered by status
	// (pass "" for all statuses), newest first.
	ListByOrg(ctx context.Context, orgID uuid.UUID, status string) ([]model.Approval, error)
	// UpdateDecision records a human's decision (status must be approved or
	// rejected) together with the reason and decider.
	UpdateDecision(ctx context.Context, id uuid.UUID, status string, reason string, decidedBy uuid.UUID) error

	// IsGated reports whether the given tool requires approval for this agent.
	IsGated(ctx context.Context, agentID uuid.UUID, toolName string) (bool, error)
	// ListRules returns the tool names gated for an agent.
	ListRules(ctx context.Context, agentID uuid.UUID) ([]string, error)
	// SetRules replaces the full set of gated tools for an agent.
	SetRules(ctx context.Context, agentID uuid.UUID, toolNames []string) error
}

type approvalRepository struct {
	pool *pgxpool.Pool
}

// NewApprovalRepository creates an ApprovalRepository backed by pgxpool.
func NewApprovalRepository(pool *pgxpool.Pool) ApprovalRepository {
	return &approvalRepository{pool: pool}
}

const approvalColumns = `id, org_id, execution_id, agent_id, tool_name, tool_input,
	status, reason, resume_state, requested_at, decided_by, decided_at`

func scanApproval(s rowScanner, out *model.Approval) error {
	var inputRaw, resumeRaw []byte
	if err := s.Scan(
		&out.ID, &out.OrgID, &out.ExecutionID, &out.AgentID, &out.ToolName, &inputRaw,
		&out.Status, &out.Reason, &resumeRaw, &out.RequestedAt, &out.DecidedBy, &out.DecidedAt,
	); err != nil {
		return err
	}
	if len(inputRaw) > 0 {
		if err := json.Unmarshal(inputRaw, &out.ToolInput); err != nil {
			out.ToolInput = map[string]any{}
		}
	}
	if out.ToolInput == nil {
		out.ToolInput = map[string]any{}
	}
	out.ResumeState = resumeRaw
	return nil
}

func (r *approvalRepository) Create(ctx context.Context, a *model.Approval) error {
	if a.ID == uuid.Nil {
		a.ID = uuid.New()
	}
	if a.Status == "" {
		a.Status = model.ApprovalStatusPending
	}
	inputJSON, err := json.Marshal(a.ToolInput)
	if err != nil || inputJSON == nil {
		inputJSON = []byte("{}")
	}
	var resumeArg any // nil => NULL
	if len(a.ResumeState) > 0 {
		resumeArg = a.ResumeState
	}

	const sql = `
		INSERT INTO approvals (id, org_id, execution_id, agent_id, tool_name, tool_input, status, reason, resume_state)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING requested_at`
	if err := r.pool.QueryRow(ctx, sql,
		a.ID, a.OrgID, a.ExecutionID, a.AgentID, a.ToolName, inputJSON, a.Status, a.Reason, resumeArg,
	).Scan(&a.RequestedAt); err != nil {
		return fmt.Errorf("approval_repo: create: %w", err)
	}
	return nil
}

func (r *approvalRepository) FindByID(ctx context.Context, id uuid.UUID) (*model.Approval, error) {
	sql := `SELECT ` + approvalColumns + ` FROM approvals WHERE id = $1`
	out := &model.Approval{}
	if err := scanApproval(r.pool.QueryRow(ctx, sql, id), out); err != nil {
		return nil, fmt.Errorf("approval_repo: find by id: %w", err)
	}
	return out, nil
}

// buildListByOrgQuery assembles the ListByOrg SQL and its args. The status
// predicate is parameterised (never interpolated), so it is injection-safe. It
// is a pure function so the query-building logic can be unit-tested without a DB.
func buildListByOrgQuery(orgID uuid.UUID, status string) (string, []any) {
	sql := `SELECT ` + approvalColumns + ` FROM approvals WHERE org_id = $1`
	args := []any{orgID}
	if status != "" {
		sql += ` AND status = $2`
		args = append(args, status)
	}
	sql += ` ORDER BY requested_at DESC`
	return sql, args
}

func (r *approvalRepository) ListByOrg(ctx context.Context, orgID uuid.UUID, status string) ([]model.Approval, error) {
	sql, args := buildListByOrgQuery(orgID, status)

	rows, err := r.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("approval_repo: list by org: %w", err)
	}
	defer rows.Close()

	out := []model.Approval{}
	for rows.Next() {
		var a model.Approval
		if err := scanApproval(rows, &a); err != nil {
			return nil, fmt.Errorf("approval_repo: list scan: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (r *approvalRepository) UpdateDecision(ctx context.Context, id uuid.UUID, status string, reason string, decidedBy uuid.UUID) error {
	var reasonArg any
	if reason != "" {
		reasonArg = reason
	}
	const sql = `
		UPDATE approvals
		SET status = $2, reason = $3, decided_by = $4, decided_at = NOW()
		WHERE id = $1 AND status = 'pending'`
	tag, err := r.pool.Exec(ctx, sql, id, status, reasonArg, decidedBy)
	if err != nil {
		return fmt.Errorf("approval_repo: update decision: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("approval_repo: approval %s not found or already decided", id)
	}
	return nil
}

func (r *approvalRepository) IsGated(ctx context.Context, agentID uuid.UUID, toolName string) (bool, error) {
	const sql = `SELECT EXISTS (SELECT 1 FROM agent_approval_rules WHERE agent_id = $1 AND tool_name = $2)`
	var exists bool
	if err := r.pool.QueryRow(ctx, sql, agentID, toolName).Scan(&exists); err != nil {
		return false, fmt.Errorf("approval_repo: is gated: %w", err)
	}
	return exists, nil
}

func (r *approvalRepository) ListRules(ctx context.Context, agentID uuid.UUID) ([]string, error) {
	const sql = `SELECT tool_name FROM agent_approval_rules WHERE agent_id = $1 ORDER BY tool_name`
	rows, err := r.pool.Query(ctx, sql, agentID)
	if err != nil {
		return nil, fmt.Errorf("approval_repo: list rules: %w", err)
	}
	defer rows.Close()

	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("approval_repo: scan rule: %w", err)
		}
		names = append(names, name)
	}
	return names, rows.Err()
}

func (r *approvalRepository) SetRules(ctx context.Context, agentID uuid.UUID, toolNames []string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("approval_repo: begin transaction: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if _, err = tx.Exec(ctx, `DELETE FROM agent_approval_rules WHERE agent_id = $1`, agentID); err != nil {
		return fmt.Errorf("approval_repo: clear rules: %w", err)
	}
	for _, name := range toolNames {
		const sql = `INSERT INTO agent_approval_rules (agent_id, tool_name) VALUES ($1, $2)
			ON CONFLICT (agent_id, tool_name) DO NOTHING`
		if _, err = tx.Exec(ctx, sql, agentID, name); err != nil {
			return fmt.Errorf("approval_repo: insert rule %q: %w", name, err)
		}
	}
	return tx.Commit(ctx)
}
