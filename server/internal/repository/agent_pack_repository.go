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

	"github.com/jobshout/server/internal/agentpack"
	"github.com/jobshout/server/internal/model"
)

// ErrAgentPackNotFound is a missing agent. Wrong-org loads use the same
// error so existence is not leaked.
var ErrAgentPackNotFound = errors.New("agent not found")

// ErrAgentPackInUse is undo after the imported agent has already run.
var ErrAgentPackInUse = errors.New("imported agent has executions and cannot be undone")

// ErrAgentPackNotUndoable is undo of a specialist or other non-create import.
var ErrAgentPackNotUndoable = errors.New("this agent cannot be undone from import")

// AgentPackStore loads and applies portable agent packages.
type AgentPackStore interface {
	LoadBundle(ctx context.Context, orgID, agentID uuid.UUID) (*AgentBundle, error)
	NameTaken(ctx context.Context, orgID uuid.UUID, name string, except *uuid.UUID) (bool, error)
	ApplyCreate(ctx context.Context, orgID, userID uuid.UUID, pkg *agentpack.Package, name, provider, modelName string, tools []string) (*model.Agent, error)
	ApplyOverlay(ctx context.Context, orgID, agentID uuid.UUID, pkg *agentpack.Package, provider, modelName string, tools []string) (*model.Agent, error)
	ExecutionCount(ctx context.Context, agentID uuid.UUID) (int, error)
	KnowledgeFiles(ctx context.Context, agentID uuid.UUID) ([]KnowledgeFileRef, error)
	UndoCreate(ctx context.Context, orgID, agentID uuid.UUID) error
}

// KnowledgeFileRef is enough to re-ingest embeddings after import.
type KnowledgeFileRef struct {
	ID      uuid.UUID
	Content string
}

// AgentBundle is the origin snapshot used to pack.
type AgentBundle struct {
	Agent     *model.Agent
	Tools     []string
	Skills    []agentpack.Skill
	Knowledge []agentpack.File
}

type agentPackStore struct {
	pool *pgxpool.Pool
}

func NewAgentPackStore(pool *pgxpool.Pool) AgentPackStore {
	return &agentPackStore{pool: pool}
}

func (s *agentPackStore) LoadBundle(ctx context.Context, orgID, agentID uuid.UUID) (*AgentBundle, error) {
	a, err := scanAgent(s.pool.QueryRow(ctx, `SELECT `+agentColumns+` FROM agents WHERE id = $1`, agentID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrAgentPackNotFound
		}
		return nil, fmt.Errorf("agent_pack: load agent: %w", err)
	}
	if a.OrgID != orgID {
		return nil, ErrAgentPackNotFound
	}

	tools, err := listToolNames(ctx, s.pool, agentID)
	if err != nil {
		return nil, err
	}
	skills, err := listPackedSkills(ctx, s.pool, agentID)
	if err != nil {
		return nil, err
	}
	files, err := listKnowledge(ctx, s.pool, agentID)
	if err != nil {
		return nil, err
	}
	return &AgentBundle{Agent: a, Tools: tools, Skills: skills, Knowledge: files}, nil
}

func listToolNames(ctx context.Context, q querier, agentID uuid.UUID) ([]string, error) {
	rows, err := q.Query(ctx, `SELECT tool_name FROM agent_tool_permissions WHERE agent_id = $1 ORDER BY tool_name`, agentID)
	if err != nil {
		return nil, fmt.Errorf("agent_pack: list tools: %w", err)
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		names = append(names, n)
	}
	if names == nil {
		names = []string{}
	}
	return names, rows.Err()
}

func listPackedSkills(ctx context.Context, q querier, agentID uuid.UUID) ([]agentpack.Skill, error) {
	rows, err := q.Query(ctx, `
		SELECT s.slug, s.org_id, s.name, s.description, s.kind, s.config_json, s.version, a.config_override
		FROM skills s
		JOIN agent_skills a ON a.skill_id = s.id
		WHERE a.agent_id = $1 AND a.enabled
		ORDER BY s.name`, agentID)
	if err != nil {
		return nil, fmt.Errorf("agent_pack: list skills: %w", err)
	}
	defer rows.Close()
	var out []agentpack.Skill
	for rows.Next() {
		var s agentpack.Skill
		var orgID *uuid.UUID
		var desc *string
		var cfg, over []byte
		if err := rows.Scan(&s.Slug, &orgID, &s.Name, &desc, &s.Kind, &cfg, &s.Version, &over); err != nil {
			return nil, err
		}
		if desc != nil {
			s.Description = *desc
		}
		_ = json.Unmarshal(cfg, &s.ConfigJSON)
		_ = json.Unmarshal(over, &s.ConfigOverride)
		if orgID == nil {
			s.Origin = "builtin"
			s.ConfigJSON = nil
		} else {
			s.Origin = "org"
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func listKnowledge(ctx context.Context, q querier, agentID uuid.UUID) ([]agentpack.File, error) {
	rows, err := q.Query(ctx, `
		SELECT filename, content FROM knowledge_files WHERE agent_id = $1 ORDER BY filename`, agentID)
	if err != nil {
		return nil, fmt.Errorf("agent_pack: list knowledge: %w", err)
	}
	defer rows.Close()
	var out []agentpack.File
	for rows.Next() {
		var f agentpack.File
		if err := rows.Scan(&f.Filename, &f.Content); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

func (s *agentPackStore) NameTaken(ctx context.Context, orgID uuid.UUID, name string, except *uuid.UUID) (bool, error) {
	var exists bool
	var err error
	name = strings.TrimSpace(name)
	if except == nil {
		err = s.pool.QueryRow(ctx, `
			SELECT EXISTS(SELECT 1 FROM agents WHERE org_id = $1 AND lower(name) = lower($2))`,
			orgID, name).Scan(&exists)
	} else {
		err = s.pool.QueryRow(ctx, `
			SELECT EXISTS(SELECT 1 FROM agents WHERE org_id = $1 AND lower(name) = lower($2) AND id <> $3)`,
			orgID, name, *except).Scan(&exists)
	}
	if err != nil {
		return false, fmt.Errorf("agent_pack: name taken: %w", err)
	}
	return exists, nil
}

func (s *agentPackStore) ExecutionCount(ctx context.Context, agentID uuid.UUID) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM agent_executions WHERE agent_id = $1`, agentID).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("agent_pack: execution count: %w", err)
	}
	return n, nil
}

func (s *agentPackStore) ApplyCreate(ctx context.Context, orgID, userID uuid.UUID, pkg *agentpack.Package, name, provider, modelName string, tools []string) (*model.Agent, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	agent, err := insertPackedAgent(ctx, tx, orgID, userID, pkg, name, provider, modelName)
	if err != nil {
		return nil, err
	}
	if err := replaceTools(ctx, tx, agent.ID, tools); err != nil {
		return nil, err
	}
	if err := applySkills(ctx, tx, orgID, userID, agent.ID, pkg.Skills); err != nil {
		return nil, err
	}
	if err := replaceKnowledge(ctx, tx, agent.ID, pkg.Knowledge); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return agent, nil
}

func (s *agentPackStore) ApplyOverlay(ctx context.Context, orgID, agentID uuid.UUID, pkg *agentpack.Package, provider, modelName string, tools []string) (*model.Agent, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	a, err := scanAgent(tx.QueryRow(ctx, `SELECT `+agentColumns+` FROM agents WHERE id = $1`, agentID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrAgentPackNotFound
		}
		return nil, err
	}
	if a.OrgID != orgID {
		return nil, ErrAgentPackNotFound
	}

	desc := pkg.Agent.Description
	prompt := pkg.Agent.SystemPrompt
	a.Description = strPtr(desc)
	a.SystemPrompt = strPtr(prompt)
	a.ModelProvider = strPtr(provider)
	a.ModelName = strPtr(modelName)
	if pkg.Agent.EngineType != "" {
		a.EngineType = pkg.Agent.EngineType
	}
	a.EngineConfig = pkg.Agent.EngineConfig

	engineJSON, _ := json.Marshal(a.EngineConfig)
	err = tx.QueryRow(ctx, `
		UPDATE agents SET description = $1, system_prompt = $2,
			model_provider = $3, model_name = $4, engine_type = $5, engine_config = $6, updated_at = NOW()
		WHERE id = $7
		RETURNING updated_at`,
		a.Description, a.SystemPrompt, a.ModelProvider, a.ModelName, a.EngineType, engineJSON, a.ID,
	).Scan(&a.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("agent_pack: overlay update: %w", err)
	}

	if err := replaceTools(ctx, tx, a.ID, tools); err != nil {
		return nil, err
	}
	// Empty skills/knowledge in the package leave the destination set
	// (same idea as skipped tools). A non-empty list still replaces.
	if overlayReplaceSkills(pkg) {
		if _, err := tx.Exec(ctx, `DELETE FROM agent_skills WHERE agent_id = $1`, a.ID); err != nil {
			return nil, err
		}
		if err := applySkills(ctx, tx, orgID, uuid.Nil, a.ID, pkg.Skills); err != nil {
			return nil, err
		}
	}
	if overlayReplaceKnowledge(pkg) {
		if err := replaceKnowledge(ctx, tx, a.ID, pkg.Knowledge); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return a, nil
}

func insertPackedAgent(ctx context.Context, tx pgx.Tx, orgID, userID uuid.UUID, pkg *agentpack.Package, name, provider, modelName string) (*model.Agent, error) {
	id := uuid.New()
	desc := pkg.Agent.Description
	prompt := pkg.Agent.SystemPrompt
	engineType := pkg.Agent.EngineType
	if engineType == "" {
		engineType = model.EngineGoNative
	}
	engineJSON, _ := json.Marshal(pkg.Agent.EngineConfig)
	if pkg.Agent.EngineConfig == nil {
		engineJSON = []byte("{}")
	}
	meta := map[string]any{}
	metaJSON, _ := json.Marshal(meta)
	createdBy := userID
	a := &model.Agent{
		ID:            id,
		OrgID:         orgID,
		Name:          name,
		Role:          pkg.Agent.Role,
		Description:   strPtr(desc),
		Status:        "idle",
		ModelProvider: strPtr(provider),
		ModelName:     strPtr(modelName),
		SystemPrompt:  strPtr(prompt),
		CreatedBy:     &createdBy,
		EngineType:    engineType,
		EngineConfig:  pkg.Agent.EngineConfig,
		Metadata:      meta,
	}
	err := tx.QueryRow(ctx, `
		INSERT INTO agents (id, org_id, name, role, description, status,
			model_provider, model_name, system_prompt, created_by,
			engine_type, engine_config, metadata, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,'idle',$6,$7,$8,$9,$10,$11,$12,NOW(),NOW())
		RETURNING created_at, updated_at`,
		a.ID, a.OrgID, a.Name, a.Role, a.Description,
		a.ModelProvider, a.ModelName, a.SystemPrompt, a.CreatedBy,
		a.EngineType, engineJSON, metaJSON,
	).Scan(&a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("agent_pack: insert agent: %w", err)
	}
	if a.EngineConfig == nil {
		a.EngineConfig = map[string]any{}
	}
	return a, nil
}

func replaceTools(ctx context.Context, tx pgx.Tx, agentID uuid.UUID, tools []string) error {
	if _, err := tx.Exec(ctx, `DELETE FROM agent_tool_permissions WHERE agent_id = $1`, agentID); err != nil {
		return fmt.Errorf("agent_pack: clear tools: %w", err)
	}
	empty, _ := json.Marshal(map[string]any{})
	for _, name := range tools {
		if strings.TrimSpace(name) == "" {
			continue
		}
		if _, err := tx.Exec(ctx, `INSERT INTO agent_tool_permissions (agent_id, tool_name, config) VALUES ($1,$2,$3)`,
			agentID, name, empty); err != nil {
			return fmt.Errorf("agent_pack: insert tool %q: %w", name, err)
		}
	}
	return nil
}

func applySkills(ctx context.Context, tx pgx.Tx, orgID, userID, agentID uuid.UUID, skills []agentpack.Skill) error {
	for _, sk := range skills {
		slug := strings.TrimSpace(sk.Slug)
		if slug == "" {
			continue
		}
		var skillID uuid.UUID
		switch sk.Origin {
		case "org":
			id, err := upsertOrgSkill(ctx, tx, orgID, userID, sk)
			if err != nil {
				return err
			}
			skillID = id
		default:
			err := tx.QueryRow(ctx, `
				SELECT id FROM skills
				WHERE slug = $1 AND org_id IS NULL AND status <> 'deprecated'
				ORDER BY created_at ASC LIMIT 1`, slug).Scan(&skillID)
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					continue
				}
				return fmt.Errorf("agent_pack: builtin skill %q: %w", slug, err)
			}
		}
		over, _ := json.Marshal(sk.ConfigOverride)
		if sk.ConfigOverride == nil {
			over = []byte("{}")
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO agent_skills (agent_id, skill_id, config_override, enabled)
			VALUES ($1,$2,$3,true)
			ON CONFLICT (agent_id, skill_id) DO UPDATE SET
				config_override = EXCLUDED.config_override,
				enabled = true,
				enabled_at = NOW()`, agentID, skillID, over); err != nil {
			return fmt.Errorf("agent_pack: enable skill %q: %w", slug, err)
		}
	}
	return nil
}

func upsertOrgSkill(ctx context.Context, tx pgx.Tx, orgID, userID uuid.UUID, sk agentpack.Skill) (uuid.UUID, error) {
	var id uuid.UUID
	err := tx.QueryRow(ctx, `SELECT id FROM skills WHERE org_id = $1 AND slug = $2`, orgID, sk.Slug).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, err
	}
	id = uuid.New()
	cfg, _ := json.Marshal(sk.ConfigJSON)
	if sk.ConfigJSON == nil {
		cfg = []byte("{}")
	}
	kind := sk.Kind
	if kind == "" {
		kind = "prompt"
	}
	version := sk.Version
	if version == "" {
		version = "0.1.0"
	}
	var createdBy *uuid.UUID
	if userID != uuid.Nil {
		createdBy = &userID
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO skills (id, org_id, slug, name, description, kind, config_json, version, status, created_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,'published',$9)`,
		id, orgID, sk.Slug, nonEmpty(sk.Name, sk.Slug), strPtr(sk.Description), kind, cfg, version, createdBy)
	if err != nil {
		return uuid.Nil, fmt.Errorf("agent_pack: create skill %q: %w", sk.Slug, err)
	}
	return id, nil
}

func replaceKnowledge(ctx context.Context, tx pgx.Tx, agentID uuid.UUID, files []agentpack.File) error {
	if _, err := tx.Exec(ctx, `DELETE FROM knowledge_files WHERE agent_id = $1`, agentID); err != nil {
		return fmt.Errorf("agent_pack: clear knowledge: %w", err)
	}
	for _, f := range files {
		name := agentpack.SafeFilename(f.Filename)
		if name == "" {
			continue
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO knowledge_files (agent_id, filename, content, size_bytes)
			VALUES ($1,$2,$3,$4)`, agentID, name, f.Content, len(f.Content)); err != nil {
			return fmt.Errorf("agent_pack: insert knowledge %q: %w", name, err)
		}
	}
	return nil
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	p := s
	return &p
}

func nonEmpty(s, fallback string) string {
	if strings.TrimSpace(s) == "" {
		return fallback
	}
	return s
}

func (s *agentPackStore) UndoCreate(ctx context.Context, orgID, agentID uuid.UUID) error {
	a, err := scanAgent(s.pool.QueryRow(ctx, `SELECT `+agentColumns+` FROM agents WHERE id = $1`, agentID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrAgentPackNotFound
		}
		return fmt.Errorf("agent_pack: undo load: %w", err)
	}
	if a.OrgID != orgID {
		return ErrAgentPackNotFound
	}
	if builtinOf(a) != "" {
		return ErrAgentPackNotUndoable
	}
	n, err := s.ExecutionCount(ctx, agentID)
	if err != nil {
		return err
	}
	if n > 0 {
		return ErrAgentPackInUse
	}
	tag, err := s.pool.Exec(ctx, `DELETE FROM agents WHERE id = $1 AND org_id = $2`, agentID, orgID)
	if err != nil {
		return fmt.Errorf("agent_pack: undo delete: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrAgentPackNotFound
	}
	return nil
}

func overlayReplaceSkills(pkg *agentpack.Package) bool {
	return pkg != nil && len(pkg.Skills) > 0
}

func overlayReplaceKnowledge(pkg *agentpack.Package) bool {
	return pkg != nil && len(pkg.Knowledge) > 0
}

func builtinOf(a *model.Agent) string {
	if a == nil || a.Metadata == nil {
		return ""
	}
	s, _ := a.Metadata[model.MetadataKeyBuiltin].(string)
	return strings.TrimSpace(s)
}

func (s *agentPackStore) KnowledgeFiles(ctx context.Context, agentID uuid.UUID) ([]KnowledgeFileRef, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, content FROM knowledge_files WHERE agent_id = $1`, agentID)
	if err != nil {
		return nil, fmt.Errorf("agent_pack: knowledge files: %w", err)
	}
	defer rows.Close()
	var out []KnowledgeFileRef
	for rows.Next() {
		var f KnowledgeFileRef
		if err := rows.Scan(&f.ID, &f.Content); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

type querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}
