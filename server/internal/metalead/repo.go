package metalead

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jobshout/server/internal/model"
)

// Store persists connections, runs and leads.
type Store interface {
	UpsertConnection(ctx context.Context, orgID uuid.UUID, name, pageID, pageName string, tokenEnc []byte, connectedBy *uuid.UUID) (*model.MetaLeadConnection, error)
	GetConnectionByName(ctx context.Context, orgID uuid.UUID, name string) (*connRow, error)
	GetConnectionByPageID(ctx context.Context, pageID string) (*connRow, error)
	ListConnections(ctx context.Context, orgID uuid.UUID) ([]model.MetaLeadConnection, error)
	CreateRun(ctx context.Context, run *model.MetaLeadRun) error
	UpdateRun(ctx context.Context, run *model.MetaLeadRun) error
	GetRun(ctx context.Context, orgID, runID uuid.UUID) (*model.MetaLeadRun, error)
	ListRuns(ctx context.Context, orgID uuid.UUID, limit int) ([]model.MetaLeadRun, error)
	UpsertLead(ctx context.Context, lead *model.MetaLead) error
	UpdateLeadAnalysis(ctx context.Context, orgID, leadID uuid.UUID, analysis json.RawMessage, status, errMsg string) error
	ListLeadsByRun(ctx context.Context, orgID, runID uuid.UUID) ([]model.MetaLead, error)
	ListRecentLeads(ctx context.Context, orgID uuid.UUID, limit int) ([]model.MetaLead, error)
	GetLeadByLeadgenID(ctx context.Context, orgID uuid.UUID, leadgenID string) (*model.MetaLead, error)
}

type connRow struct {
	model.MetaLeadConnection
	AccessTokenEnc []byte
}

// Repo is a Postgres Store.
type Repo struct{ pool *pgxpool.Pool }

// NewRepo builds a Store on pool.
func NewRepo(pool *pgxpool.Pool) *Repo { return &Repo{pool: pool} }

func (r *Repo) UpsertConnection(ctx context.Context, orgID uuid.UUID, name, pageID, pageName string, tokenEnc []byte, connectedBy *uuid.UUID) (*model.MetaLeadConnection, error) {
	id := uuid.New()
	row := r.pool.QueryRow(ctx, `
		INSERT INTO meta_lead_connections (id, org_id, name, page_id, page_name, access_token_enc, connected_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (org_id, name) DO UPDATE SET
			page_id = EXCLUDED.page_id,
			page_name = EXCLUDED.page_name,
			access_token_enc = EXCLUDED.access_token_enc,
			connected_by = COALESCE(EXCLUDED.connected_by, meta_lead_connections.connected_by),
			updated_at = NOW()
		RETURNING id, org_id, name, page_id, page_name, connected_by, created_at, updated_at`,
		id, orgID, name, pageID, pageName, tokenEnc, connectedBy)
	var c model.MetaLeadConnection
	if err := row.Scan(&c.ID, &c.OrgID, &c.Name, &c.PageID, &c.PageName, &c.ConnectedBy, &c.CreatedAt, &c.UpdatedAt); err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *Repo) GetConnectionByName(ctx context.Context, orgID uuid.UUID, name string) (*connRow, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, org_id, name, page_id, page_name, access_token_enc, connected_by, created_at, updated_at
		FROM meta_lead_connections WHERE org_id = $1 AND name = $2`, orgID, name)
	var c connRow
	err := row.Scan(&c.ID, &c.OrgID, &c.Name, &c.PageID, &c.PageName, &c.AccessTokenEnc, &c.ConnectedBy, &c.CreatedAt, &c.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("meta connection %q not found", name)
	}
	return &c, err
}

func (r *Repo) GetConnectionByPageID(ctx context.Context, pageID string) (*connRow, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, org_id, name, page_id, page_name, access_token_enc, connected_by, created_at, updated_at
		FROM meta_lead_connections WHERE page_id = $1
		ORDER BY updated_at DESC LIMIT 1`, pageID)
	var c connRow
	err := row.Scan(&c.ID, &c.OrgID, &c.Name, &c.PageID, &c.PageName, &c.AccessTokenEnc, &c.ConnectedBy, &c.CreatedAt, &c.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("no meta connection for page %s", pageID)
	}
	return &c, err
}

func (r *Repo) ListConnections(ctx context.Context, orgID uuid.UUID) ([]model.MetaLeadConnection, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, org_id, name, page_id, page_name, connected_by, created_at, updated_at
		FROM meta_lead_connections WHERE org_id = $1 ORDER BY name`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.MetaLeadConnection
	for rows.Next() {
		var c model.MetaLeadConnection
		if err := rows.Scan(&c.ID, &c.OrgID, &c.Name, &c.PageID, &c.PageName, &c.ConnectedBy, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *Repo) CreateRun(ctx context.Context, run *model.MetaLeadRun) error {
	if run.ID == uuid.Nil {
		run.ID = uuid.New()
	}
	now := time.Now().UTC()
	run.CreatedAt = now
	run.UpdatedAt = now
	_, err := r.pool.Exec(ctx, `
		INSERT INTO meta_lead_runs (id, org_id, task_id, connection_id, form_id, form_name, focus, status, lead_count, rollup, error_message, source, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`,
		run.ID, run.OrgID, run.TaskID, run.ConnectionID, run.FormID, run.FormName, run.Focus, run.Status,
		run.LeadCount, nullJSON(run.Rollup), run.ErrorMessage, run.Source, run.CreatedAt, run.UpdatedAt)
	return err
}

func (r *Repo) UpdateRun(ctx context.Context, run *model.MetaLeadRun) error {
	run.UpdatedAt = time.Now().UTC()
	_, err := r.pool.Exec(ctx, `
		UPDATE meta_lead_runs SET status=$2, lead_count=$3, rollup=$4, error_message=$5, updated_at=$6
		WHERE id=$1 AND org_id=$7`,
		run.ID, run.Status, run.LeadCount, nullJSON(run.Rollup), run.ErrorMessage, run.UpdatedAt, run.OrgID)
	return err
}

func (r *Repo) GetRun(ctx context.Context, orgID, runID uuid.UUID) (*model.MetaLeadRun, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, org_id, task_id, connection_id, form_id, form_name, focus, status, lead_count, rollup, error_message, source, created_at, updated_at
		FROM meta_lead_runs WHERE id=$1 AND org_id=$2`, runID, orgID)
	return scanRun(row)
}

func (r *Repo) ListRuns(ctx context.Context, orgID uuid.UUID, limit int) ([]model.MetaLeadRun, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id, org_id, task_id, connection_id, form_id, form_name, focus, status, lead_count, rollup, error_message, source, created_at, updated_at
		FROM meta_lead_runs WHERE org_id=$1 ORDER BY created_at DESC LIMIT $2`, orgID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.MetaLeadRun
	for rows.Next() {
		run, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *run)
	}
	return out, rows.Err()
}

type scannable interface {
	Scan(dest ...any) error
}

func scanRun(row scannable) (*model.MetaLeadRun, error) {
	var run model.MetaLeadRun
	var rollup []byte
	err := row.Scan(&run.ID, &run.OrgID, &run.TaskID, &run.ConnectionID, &run.FormID, &run.FormName, &run.Focus,
		&run.Status, &run.LeadCount, &rollup, &run.ErrorMessage, &run.Source, &run.CreatedAt, &run.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("meta lead run not found")
	}
	if err != nil {
		return nil, err
	}
	if len(rollup) > 0 {
		run.Rollup = json.RawMessage(rollup)
	}
	return &run, nil
}

func (r *Repo) UpsertLead(ctx context.Context, lead *model.MetaLead) error {
	if lead.ID == uuid.Nil {
		lead.ID = uuid.New()
	}
	now := time.Now().UTC()
	lead.CreatedAt = now
	lead.UpdatedAt = now
	err := r.pool.QueryRow(ctx, `
		INSERT INTO meta_leads (id, org_id, connection_id, run_id, leadgen_id, form_id, field_data, created_time, analysis, status, error_message, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
		ON CONFLICT (org_id, leadgen_id) DO UPDATE SET
			run_id = COALESCE(EXCLUDED.run_id, meta_leads.run_id),
			form_id = COALESCE(NULLIF(EXCLUDED.form_id,''), meta_leads.form_id),
			field_data = COALESCE(EXCLUDED.field_data, meta_leads.field_data),
			created_time = COALESCE(EXCLUDED.created_time, meta_leads.created_time),
			updated_at = NOW()
		RETURNING id, created_at, updated_at`,
		lead.ID, lead.OrgID, lead.ConnectionID, lead.RunID, lead.LeadgenID, lead.FormID,
		nullJSON(lead.FieldData), lead.CreatedTime, nullJSON(lead.Analysis), lead.Status, lead.ErrorMessage,
		lead.CreatedAt, lead.UpdatedAt).Scan(&lead.ID, &lead.CreatedAt, &lead.UpdatedAt)
	return err
}

func (r *Repo) UpdateLeadAnalysis(ctx context.Context, orgID, leadID uuid.UUID, analysis json.RawMessage, status, errMsg string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE meta_leads SET analysis=$2, status=$3, error_message=$4, updated_at=NOW()
		WHERE id=$1 AND org_id=$5`, leadID, nullJSON(analysis), status, errMsg, orgID)
	return err
}

func (r *Repo) ListLeadsByRun(ctx context.Context, orgID, runID uuid.UUID) ([]model.MetaLead, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, org_id, connection_id, run_id, leadgen_id, form_id, field_data, created_time, analysis, status, error_message, created_at, updated_at
		FROM meta_leads WHERE org_id=$1 AND run_id=$2 ORDER BY created_time DESC NULLS LAST, created_at DESC`, orgID, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanLeads(rows)
}

func (r *Repo) ListRecentLeads(ctx context.Context, orgID uuid.UUID, limit int) ([]model.MetaLead, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id, org_id, connection_id, run_id, leadgen_id, form_id, field_data, created_time, analysis, status, error_message, created_at, updated_at
		FROM meta_leads WHERE org_id=$1 ORDER BY created_at DESC LIMIT $2`, orgID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanLeads(rows)
}

func (r *Repo) GetLeadByLeadgenID(ctx context.Context, orgID uuid.UUID, leadgenID string) (*model.MetaLead, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, org_id, connection_id, run_id, leadgen_id, form_id, field_data, created_time, analysis, status, error_message, created_at, updated_at
		FROM meta_leads WHERE org_id=$1 AND leadgen_id=$2`, orgID, leadgenID)
	l, err := scanLead(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("lead not found")
	}
	return l, err
}

type leadRows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
}

func scanLead(row scannable) (*model.MetaLead, error) {
	var l model.MetaLead
	var fieldData, analysis []byte
	err := row.Scan(&l.ID, &l.OrgID, &l.ConnectionID, &l.RunID, &l.LeadgenID, &l.FormID,
		&fieldData, &l.CreatedTime, &analysis, &l.Status, &l.ErrorMessage, &l.CreatedAt, &l.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if len(fieldData) > 0 {
		l.FieldData = json.RawMessage(fieldData)
	}
	if len(analysis) > 0 {
		l.Analysis = json.RawMessage(analysis)
	}
	return &l, nil
}

func scanLeads(rows leadRows) ([]model.MetaLead, error) {
	var out []model.MetaLead
	for rows.Next() {
		l, err := scanLead(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *l)
	}
	return out, rows.Err()
}

func nullJSON(b json.RawMessage) any {
	if len(b) == 0 {
		return nil
	}
	return []byte(b)
}
