package repository

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jobshout/server/internal/model"
)

// SkillRepository persists the skills catalog and agent enablement.
type SkillRepository interface {
	Create(ctx context.Context, s *model.Skill) error
	Get(ctx context.Context, id uuid.UUID) (*model.Skill, error)
	// List returns every skill visible to orgID — that is, every built-in
	// skill (org_id IS NULL) plus org-private skills owned by orgID.
	List(ctx context.Context, orgID uuid.UUID) ([]model.Skill, error)
	Delete(ctx context.Context, id uuid.UUID) error

	EnableForAgent(ctx context.Context, agentID, skillID uuid.UUID, override map[string]any) error
	DisableForAgent(ctx context.Context, agentID, skillID uuid.UUID) error
	ListForAgent(ctx context.Context, agentID uuid.UUID) ([]model.Skill, error)
}

type skillRepository struct {
	pool *pgxpool.Pool
}

func NewSkillRepository(pool *pgxpool.Pool) SkillRepository {
	return &skillRepository{pool: pool}
}

const skillColumns = `id, org_id, slug, name, description, kind, config_json, version, status, created_by, created_at, updated_at`

func scanSkill(s rowScanner, out *model.Skill) error {
	var raw []byte
	if err := s.Scan(
		&out.ID, &out.OrgID, &out.Slug, &out.Name, &out.Description,
		&out.Kind, &raw, &out.Version, &out.Status, &out.CreatedBy,
		&out.CreatedAt, &out.UpdatedAt,
	); err != nil {
		return err
	}
	_ = json.Unmarshal(raw, &out.ConfigJSON)
	return nil
}

func (r *skillRepository) Create(ctx context.Context, s *model.Skill) error {
	cfg, _ := json.Marshal(s.ConfigJSON)
	if cfg == nil {
		cfg = []byte("{}")
	}
	if s.Kind == "" {
		s.Kind = "tool"
	}
	if s.Version == "" {
		s.Version = "0.1.0"
	}
	if s.Status == "" {
		s.Status = "published"
	}

	const sql = `
		INSERT INTO skills (id, org_id, slug, name, description, kind, config_json, version, status, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING created_at, updated_at`
	return r.pool.QueryRow(ctx, sql,
		s.ID, s.OrgID, s.Slug, s.Name, s.Description, s.Kind, cfg, s.Version, s.Status, s.CreatedBy,
	).Scan(&s.CreatedAt, &s.UpdatedAt)
}

func (r *skillRepository) Get(ctx context.Context, id uuid.UUID) (*model.Skill, error) {
	sql := `SELECT ` + skillColumns + ` FROM skills WHERE id = $1`
	out := &model.Skill{}
	if err := scanSkill(r.pool.QueryRow(ctx, sql, id), out); err != nil {
		return nil, fmt.Errorf("skill_repo: get: %w", err)
	}
	return out, nil
}

func (r *skillRepository) List(ctx context.Context, orgID uuid.UUID) ([]model.Skill, error) {
	sql := `SELECT ` + skillColumns + ` FROM skills
		WHERE (org_id IS NULL OR org_id = $1) AND status = 'published'
		ORDER BY (org_id IS NULL) DESC, name`

	rows, err := r.pool.Query(ctx, sql, orgID)
	if err != nil {
		return nil, fmt.Errorf("skill_repo: list: %w", err)
	}
	defer rows.Close()

	out := []model.Skill{}
	for rows.Next() {
		var s model.Skill
		if err := scanSkill(rows, &s); err != nil {
			return nil, fmt.Errorf("skill_repo: list scan: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *skillRepository) Delete(ctx context.Context, id uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM skills WHERE id = $1`, id)
	return err
}

func (r *skillRepository) EnableForAgent(ctx context.Context, agentID, skillID uuid.UUID, override map[string]any) error {
	cfg, _ := json.Marshal(override)
	if cfg == nil {
		cfg = []byte("{}")
	}
	const sql = `
		INSERT INTO agent_skills (agent_id, skill_id, config_override, enabled)
		VALUES ($1, $2, $3, true)
		ON CONFLICT (agent_id, skill_id) DO UPDATE SET
		    config_override = EXCLUDED.config_override,
		    enabled         = true,
		    enabled_at      = NOW()`
	_, err := r.pool.Exec(ctx, sql, agentID, skillID, cfg)
	return err
}

func (r *skillRepository) DisableForAgent(ctx context.Context, agentID, skillID uuid.UUID) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE agent_skills SET enabled = false WHERE agent_id = $1 AND skill_id = $2`,
		agentID, skillID)
	return err
}

func (r *skillRepository) ListForAgent(ctx context.Context, agentID uuid.UUID) ([]model.Skill, error) {
	sql := `SELECT ` + qualifiedSkillColumns("s") + ` FROM skills s
		JOIN agent_skills a ON a.skill_id = s.id
		WHERE a.agent_id = $1 AND a.enabled
		ORDER BY s.name`

	rows, err := r.pool.Query(ctx, sql, agentID)
	if err != nil {
		return nil, fmt.Errorf("skill_repo: list for agent: %w", err)
	}
	defer rows.Close()

	out := []model.Skill{}
	for rows.Next() {
		var s model.Skill
		if err := scanSkill(rows, &s); err != nil {
			return nil, fmt.Errorf("skill_repo: list for agent scan: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// qualifiedSkillColumns prefixes each column in skillColumns with the given
// table alias. Spelled out so the JOIN above stays readable.
func qualifiedSkillColumns(alias string) string {
	cols := []string{
		"id", "org_id", "slug", "name", "description", "kind",
		"config_json", "version", "status", "created_by", "created_at", "updated_at",
	}
	out := ""
	for i, c := range cols {
		if i > 0 {
			out += ", "
		}
		out += alias + "." + c
	}
	return out
}
