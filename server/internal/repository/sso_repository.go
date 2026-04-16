package repository

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jobshout/server/internal/model"
)

// SSORepository handles SSO configuration persistence.
type SSORepository interface {
	Create(ctx context.Context, cfg *model.SSOConfig) (*model.SSOConfig, error)
	GetByOrgAndProvider(ctx context.Context, orgID uuid.UUID, provider string) (*model.SSOConfig, error)
	ListByOrg(ctx context.Context, orgID uuid.UUID) ([]model.SSOConfig, error)
	Delete(ctx context.Context, id uuid.UUID) error
}

type ssoRepository struct {
	pool *pgxpool.Pool
}

func NewSSORepository(pool *pgxpool.Pool) SSORepository {
	return &ssoRepository{pool: pool}
}

func (r *ssoRepository) Create(ctx context.Context, cfg *model.SSOConfig) (*model.SSOConfig, error) {
	metaJSON, _ := json.Marshal(cfg.Metadata)
	const sql = `
		INSERT INTO sso_configs (org_id, provider, client_id, client_secret, issuer_url, redirect_url,
		    scopes, auto_provision, default_role, domain_filter, enabled, metadata)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
		ON CONFLICT (org_id, provider) DO UPDATE SET
		    client_id = EXCLUDED.client_id, client_secret = EXCLUDED.client_secret,
		    issuer_url = EXCLUDED.issuer_url, redirect_url = EXCLUDED.redirect_url,
		    scopes = EXCLUDED.scopes, auto_provision = EXCLUDED.auto_provision,
		    default_role = EXCLUDED.default_role, domain_filter = EXCLUDED.domain_filter,
		    enabled = EXCLUDED.enabled, metadata = EXCLUDED.metadata, updated_at = NOW()
		RETURNING id, org_id, provider, client_id, issuer_url, redirect_url, scopes,
		    auto_provision, default_role, domain_filter, enabled, metadata, created_at, updated_at`

	out := &model.SSOConfig{}
	var metaRaw []byte
	if err := r.pool.QueryRow(ctx, sql,
		cfg.OrgID, cfg.Provider, cfg.ClientID, cfg.ClientSecret, cfg.IssuerURL, cfg.RedirectURL,
		cfg.Scopes, cfg.AutoProvision, cfg.DefaultRole, cfg.DomainFilter, cfg.Enabled, metaJSON,
	).Scan(&out.ID, &out.OrgID, &out.Provider, &out.ClientID, &out.IssuerURL, &out.RedirectURL,
		&out.Scopes, &out.AutoProvision, &out.DefaultRole, &out.DomainFilter,
		&out.Enabled, &metaRaw, &out.CreatedAt, &out.UpdatedAt,
	); err != nil {
		return nil, fmt.Errorf("sso_repo: create: %w", err)
	}
	_ = json.Unmarshal(metaRaw, &out.Metadata)
	return out, nil
}

func (r *ssoRepository) GetByOrgAndProvider(ctx context.Context, orgID uuid.UUID, provider string) (*model.SSOConfig, error) {
	const sql = `
		SELECT id, org_id, provider, client_id, client_secret, issuer_url, redirect_url, scopes,
		    auto_provision, default_role, domain_filter, enabled, metadata, created_at, updated_at
		FROM sso_configs WHERE org_id = $1 AND provider = $2 AND enabled = true`

	out := &model.SSOConfig{}
	var metaRaw []byte
	if err := r.pool.QueryRow(ctx, sql, orgID, provider).Scan(
		&out.ID, &out.OrgID, &out.Provider, &out.ClientID, &out.ClientSecret,
		&out.IssuerURL, &out.RedirectURL, &out.Scopes,
		&out.AutoProvision, &out.DefaultRole, &out.DomainFilter,
		&out.Enabled, &metaRaw, &out.CreatedAt, &out.UpdatedAt,
	); err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("sso_repo: get by org and provider: %w", err)
	}
	_ = json.Unmarshal(metaRaw, &out.Metadata)
	return out, nil
}

func (r *ssoRepository) ListByOrg(ctx context.Context, orgID uuid.UUID) ([]model.SSOConfig, error) {
	const sql = `
		SELECT id, org_id, provider, client_id, issuer_url, redirect_url, scopes,
		    auto_provision, default_role, domain_filter, enabled, metadata, created_at, updated_at
		FROM sso_configs WHERE org_id = $1 ORDER BY provider`

	rows, err := r.pool.Query(ctx, sql, orgID)
	if err != nil {
		return nil, fmt.Errorf("sso_repo: list: %w", err)
	}
	defer rows.Close()

	var configs []model.SSOConfig
	for rows.Next() {
		var c model.SSOConfig
		var metaRaw []byte
		if err := rows.Scan(&c.ID, &c.OrgID, &c.Provider, &c.ClientID, &c.IssuerURL, &c.RedirectURL,
			&c.Scopes, &c.AutoProvision, &c.DefaultRole, &c.DomainFilter, &c.Enabled,
			&metaRaw, &c.CreatedAt, &c.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("sso_repo: scan: %w", err)
		}
		_ = json.Unmarshal(metaRaw, &c.Metadata)
		configs = append(configs, c)
	}
	return configs, rows.Err()
}

func (r *ssoRepository) Delete(ctx context.Context, id uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM sso_configs WHERE id = $1`, id)
	return err
}
