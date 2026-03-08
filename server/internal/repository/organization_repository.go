package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jobshout/server/internal/model"
)

// OrganizationRepository defines operations for organization persistence.
type OrganizationRepository interface {
	Create(ctx context.Context, org *model.Organization) error
	FindByID(ctx context.Context, id uuid.UUID) (*model.Organization, error)
	FindBySlug(ctx context.Context, slug string) (*model.Organization, error)
	Update(ctx context.Context, org *model.Organization) error
	UpdateChart(ctx context.Context, orgID uuid.UUID, entries []model.UpdateOrgChartEntry) error
}

type organizationRepository struct {
	pool *pgxpool.Pool
}

// NewOrganizationRepository creates a new OrganizationRepository backed by PostgreSQL.
func NewOrganizationRepository(pool *pgxpool.Pool) OrganizationRepository {
	return &organizationRepository{pool: pool}
}

func (r *organizationRepository) Create(ctx context.Context, org *model.Organization) error {
	query := `
		INSERT INTO organizations (id, name, slug, owner_id, created_at, updated_at)
		VALUES ($1, $2, $3, $4, NOW(), NOW())
		RETURNING created_at, updated_at`

	return r.pool.QueryRow(ctx, query,
		org.ID, org.Name, org.Slug, org.OwnerID,
	).Scan(&org.CreatedAt, &org.UpdatedAt)
}

func (r *organizationRepository) FindByID(ctx context.Context, id uuid.UUID) (*model.Organization, error) {
	query := `
		SELECT id, name, slug, owner_id, created_at, updated_at
		FROM organizations WHERE id = $1`

	org := &model.Organization{}
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&org.ID, &org.Name, &org.Slug, &org.OwnerID,
		&org.CreatedAt, &org.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("finding organization by id: %w", err)
	}
	return org, nil
}

func (r *organizationRepository) FindBySlug(ctx context.Context, slug string) (*model.Organization, error) {
	query := `
		SELECT id, name, slug, owner_id, created_at, updated_at
		FROM organizations WHERE slug = $1`

	org := &model.Organization{}
	err := r.pool.QueryRow(ctx, query, slug).Scan(
		&org.ID, &org.Name, &org.Slug, &org.OwnerID,
		&org.CreatedAt, &org.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("finding organization by slug: %w", err)
	}
	return org, nil
}

func (r *organizationRepository) Update(ctx context.Context, org *model.Organization) error {
	query := `
		UPDATE organizations SET name = $1, slug = $2, updated_at = NOW()
		WHERE id = $3
		RETURNING updated_at`

	return r.pool.QueryRow(ctx, query, org.Name, org.Slug, org.ID).Scan(&org.UpdatedAt)
}

func (r *organizationRepository) UpdateChart(ctx context.Context, orgID uuid.UUID, entries []model.UpdateOrgChartEntry) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	for _, entry := range entries {
		query := `UPDATE agents SET manager_id = $1, updated_at = NOW() WHERE id = $2 AND org_id = $3`
		_, err := tx.Exec(ctx, query, entry.ManagerID, entry.AgentID, orgID)
		if err != nil {
			return fmt.Errorf("updating agent manager: %w", err)
		}
	}

	return tx.Commit(ctx)
}
