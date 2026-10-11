package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jobshout/server/internal/model"
)

// OrgLLMRoutingRepository persists per-org routing settings.
type OrgLLMRoutingRepository interface {
	// Get returns the org's routing row, or (nil, nil) when it has none.
	Get(ctx context.Context, orgID uuid.UUID) (*model.OrgLLMRouting, error)
	Upsert(ctx context.Context, r *model.OrgLLMRouting) error
}

type orgLLMRoutingRepository struct {
	pool *pgxpool.Pool
}

func NewOrgLLMRoutingRepository(pool *pgxpool.Pool) OrgLLMRoutingRepository {
	return &orgLLMRoutingRepository{pool: pool}
}

func (r *orgLLMRoutingRepository) Get(ctx context.Context, orgID uuid.UUID) (*model.OrgLLMRouting, error) {
	out := &model.OrgLLMRouting{}
	var chainRaw []byte
	err := r.pool.QueryRow(ctx,
		`SELECT org_id, enabled, chain, updated_by, updated_at FROM org_llm_routing WHERE org_id = $1`, orgID,
	).Scan(&out.OrgID, &out.Enabled, &chainRaw, &out.UpdatedBy, &out.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("org_llm_routing: get: %w", err)
	}
	if err := json.Unmarshal(chainRaw, &out.Chain); err != nil {
		return nil, fmt.Errorf("org_llm_routing: decode chain: %w", err)
	}
	if out.Chain == nil {
		out.Chain = []model.LLMRouteStep{}
	}
	return out, nil
}

func (r *orgLLMRoutingRepository) Upsert(ctx context.Context, in *model.OrgLLMRouting) error {
	if in.Chain == nil {
		in.Chain = []model.LLMRouteStep{}
	}
	chain, err := json.Marshal(in.Chain)
	if err != nil {
		return fmt.Errorf("org_llm_routing: encode chain: %w", err)
	}
	return r.pool.QueryRow(ctx, `
		INSERT INTO org_llm_routing (org_id, enabled, chain, updated_by)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (org_id) DO UPDATE
		SET enabled = EXCLUDED.enabled, chain = EXCLUDED.chain,
		    updated_by = EXCLUDED.updated_by, updated_at = NOW()
		RETURNING updated_at`,
		in.OrgID, in.Enabled, chain, in.UpdatedBy,
	).Scan(&in.UpdatedAt)
}
