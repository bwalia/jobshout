package repository

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jobshout/server/internal/model"
)

type IntegrationRepository interface {
	Create(ctx context.Context, i *model.Integration) error
	FindByID(ctx context.Context, id uuid.UUID) (*model.Integration, error)
	ListByOrg(ctx context.Context, orgID uuid.UUID) ([]model.Integration, error)
	Update(ctx context.Context, i *model.Integration) error
	Delete(ctx context.Context, id uuid.UUID) error
}

type integrationRepository struct {
	pool *pgxpool.Pool
}

func NewIntegrationRepository(pool *pgxpool.Pool) IntegrationRepository {
	return &integrationRepository{pool: pool}
}

func (r *integrationRepository) Create(ctx context.Context, i *model.Integration) error {
	i.ID = uuid.New()
	i.CreatedAt = time.Now()
	i.UpdatedAt = i.CreatedAt
	if i.Status == "" {
		i.Status = "active"
	}

	credsJSON, _ := json.Marshal(i.Credentials)
	configJSON, _ := json.Marshal(i.Config)

	_, err := r.pool.Exec(ctx, `
		INSERT INTO integrations (id, org_id, name, provider, base_url, credentials, config, status, created_by, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		i.ID, i.OrgID, i.Name, i.Provider, i.BaseURL, credsJSON, configJSON, i.Status, i.CreatedBy, i.CreatedAt, i.UpdatedAt,
	)
	return err
}

func (r *integrationRepository) FindByID(ctx context.Context, id uuid.UUID) (*model.Integration, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, org_id, name, provider, base_url, credentials, config, status, last_synced_at, created_by, created_at, updated_at
		FROM integrations WHERE id = $1`, id)
	return scanIntegration(row)
}

func (r *integrationRepository) ListByOrg(ctx context.Context, orgID uuid.UUID) ([]model.Integration, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, org_id, name, provider, base_url, credentials, config, status, last_synced_at, created_by, created_at, updated_at
		FROM integrations WHERE org_id = $1 ORDER BY created_at DESC`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []model.Integration
	for rows.Next() {
		i, err := scanIntegration(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *i)
	}
	return items, nil
}

func (r *integrationRepository) Update(ctx context.Context, i *model.Integration) error {
	i.UpdatedAt = time.Now()
	credsJSON, _ := json.Marshal(i.Credentials)
	configJSON, _ := json.Marshal(i.Config)

	_, err := r.pool.Exec(ctx, `
		UPDATE integrations SET name=$1, base_url=$2, credentials=$3, config=$4, status=$5, updated_at=$6
		WHERE id=$7`,
		i.Name, i.BaseURL, credsJSON, configJSON, i.Status, i.UpdatedAt, i.ID,
	)
	return err
}

func (r *integrationRepository) Delete(ctx context.Context, id uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM integrations WHERE id = $1`, id)
	return err
}

type scannable interface {
	Scan(dest ...any) error
}

func scanIntegration(s scannable) (*model.Integration, error) {
	var i model.Integration
	var credsJSON, configJSON []byte

	err := s.Scan(&i.ID, &i.OrgID, &i.Name, &i.Provider, &i.BaseURL, &credsJSON, &configJSON, &i.Status, &i.LastSyncedAt, &i.CreatedBy, &i.CreatedAt, &i.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	_ = json.Unmarshal(credsJSON, &i.Credentials)
	_ = json.Unmarshal(configJSON, &i.Config)
	return &i, nil
}
