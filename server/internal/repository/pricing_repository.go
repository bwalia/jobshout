package repository

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jobshout/server/internal/model"
)

// PricingRepository handles versioned pricing configuration persistence.
type PricingRepository interface {
	Create(ctx context.Context, cfg *model.PricingConfig) (*model.PricingConfig, error)
	// GetActive returns the active pricing for a provider/model, preferring tenant override.
	GetActive(ctx context.Context, orgID *uuid.UUID, provider, modelName string) (*model.PricingConfig, error)
	ListActive(ctx context.Context, orgID *uuid.UUID) ([]model.PricingConfig, error)
	Deactivate(ctx context.Context, id uuid.UUID) error
}

type pricingRepository struct {
	pool *pgxpool.Pool
}

func NewPricingRepository(pool *pgxpool.Pool) PricingRepository {
	return &pricingRepository{pool: pool}
}

func (r *pricingRepository) Create(ctx context.Context, cfg *model.PricingConfig) (*model.PricingConfig, error) {
	const sql = `
		INSERT INTO pricing_configs (org_id, provider, model, input_price_per_m_token,
		    output_price_per_m_token, compute_price_per_sec, version, effective_from, is_active)
		VALUES ($1,$2,$3,$4,$5,$6,
		    COALESCE((SELECT MAX(version) FROM pricing_configs WHERE provider=$2 AND model=$3 AND (org_id=$1 OR ($1 IS NULL AND org_id IS NULL))), 0) + 1,
		    COALESCE($7, NOW()), true)
		RETURNING id, org_id, provider, model, input_price_per_m_token, output_price_per_m_token,
		    compute_price_per_sec, version, effective_from, effective_until, is_active, created_at`

	out := &model.PricingConfig{}
	if err := r.pool.QueryRow(ctx, sql,
		cfg.OrgID, cfg.Provider, cfg.Model,
		cfg.InputPricePerMToken, cfg.OutputPricePerMToken, cfg.ComputePricePerSec,
		cfg.EffectiveFrom,
	).Scan(
		&out.ID, &out.OrgID, &out.Provider, &out.Model,
		&out.InputPricePerMToken, &out.OutputPricePerMToken, &out.ComputePricePerSec,
		&out.Version, &out.EffectiveFrom, &out.EffectiveUntil, &out.IsActive, &out.CreatedAt,
	); err != nil {
		return nil, fmt.Errorf("pricing_repo: create: %w", err)
	}
	return out, nil
}

// GetActive returns tenant-specific pricing if it exists, else system-wide.
func (r *pricingRepository) GetActive(ctx context.Context, orgID *uuid.UUID, provider, modelName string) (*model.PricingConfig, error) {
	const sql = `
		SELECT id, org_id, provider, model, input_price_per_m_token, output_price_per_m_token,
		    compute_price_per_sec, version, effective_from, effective_until, is_active, created_at
		FROM pricing_configs
		WHERE provider = $1 AND model = $2 AND is_active = true
		    AND (org_id = $3 OR org_id IS NULL)
		ORDER BY
		    CASE WHEN org_id IS NOT NULL THEN 0 ELSE 1 END,
		    version DESC
		LIMIT 1`

	out := &model.PricingConfig{}
	if err := r.pool.QueryRow(ctx, sql, provider, modelName, orgID).Scan(
		&out.ID, &out.OrgID, &out.Provider, &out.Model,
		&out.InputPricePerMToken, &out.OutputPricePerMToken, &out.ComputePricePerSec,
		&out.Version, &out.EffectiveFrom, &out.EffectiveUntil, &out.IsActive, &out.CreatedAt,
	); err != nil {
		return nil, nil // not found is OK — fall back to in-memory catalog
	}
	return out, nil
}

func (r *pricingRepository) ListActive(ctx context.Context, orgID *uuid.UUID) ([]model.PricingConfig, error) {
	const sql = `
		SELECT id, org_id, provider, model, input_price_per_m_token, output_price_per_m_token,
		    compute_price_per_sec, version, effective_from, effective_until, is_active, created_at
		FROM pricing_configs
		WHERE is_active = true AND (org_id = $1 OR org_id IS NULL)
		ORDER BY provider, model, version DESC`

	rows, err := r.pool.Query(ctx, sql, orgID)
	if err != nil {
		return nil, fmt.Errorf("pricing_repo: list: %w", err)
	}
	defer rows.Close()

	var configs []model.PricingConfig
	for rows.Next() {
		var c model.PricingConfig
		if err := rows.Scan(
			&c.ID, &c.OrgID, &c.Provider, &c.Model,
			&c.InputPricePerMToken, &c.OutputPricePerMToken, &c.ComputePricePerSec,
			&c.Version, &c.EffectiveFrom, &c.EffectiveUntil, &c.IsActive, &c.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("pricing_repo: scan: %w", err)
		}
		configs = append(configs, c)
	}
	return configs, rows.Err()
}

func (r *pricingRepository) Deactivate(ctx context.Context, id uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `UPDATE pricing_configs SET is_active = false, effective_until = NOW() WHERE id = $1`, id)
	return err
}
