package repository

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jobshout/server/internal/model"
)

type TaskLinkRepository interface {
	Create(ctx context.Context, link *model.IntegrationTaskLink) error
	FindByTaskID(ctx context.Context, taskID uuid.UUID) ([]model.IntegrationTaskLink, error)
	FindByExternalID(ctx context.Context, integrationID uuid.UUID, externalID string) (*model.IntegrationTaskLink, error)
	ListByIntegration(ctx context.Context, integrationID uuid.UUID) ([]model.IntegrationTaskLink, error)
	Update(ctx context.Context, link *model.IntegrationTaskLink) error
	Delete(ctx context.Context, id uuid.UUID) error
	DeleteByTaskAndIntegration(ctx context.Context, integrationID, taskID uuid.UUID) error
}

type taskLinkRepository struct {
	pool *pgxpool.Pool
}

func NewTaskLinkRepository(pool *pgxpool.Pool) TaskLinkRepository {
	return &taskLinkRepository{pool: pool}
}

func (r *taskLinkRepository) Create(ctx context.Context, link *model.IntegrationTaskLink) error {
	link.ID = uuid.New()
	link.CreatedAt = time.Now()
	link.UpdatedAt = link.CreatedAt
	if link.SyncDirection == "" {
		link.SyncDirection = "bidirectional"
	}
	if link.SyncStatus == "" {
		link.SyncStatus = "pending"
	}

	_, err := r.pool.Exec(ctx, `
		INSERT INTO integration_task_links (id, integration_id, task_id, external_id, external_url, sync_direction, sync_status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		link.ID, link.IntegrationID, link.TaskID, link.ExternalID, link.ExternalURL, link.SyncDirection, link.SyncStatus, link.CreatedAt, link.UpdatedAt,
	)
	return err
}

func (r *taskLinkRepository) FindByTaskID(ctx context.Context, taskID uuid.UUID) ([]model.IntegrationTaskLink, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, integration_id, task_id, external_id, external_url, sync_direction, last_synced_at, sync_status, created_at, updated_at
		FROM integration_task_links WHERE task_id = $1`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanLinks(rows)
}

func (r *taskLinkRepository) FindByExternalID(ctx context.Context, integrationID uuid.UUID, externalID string) (*model.IntegrationTaskLink, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, integration_id, task_id, external_id, external_url, sync_direction, last_synced_at, sync_status, created_at, updated_at
		FROM integration_task_links WHERE integration_id = $1 AND external_id = $2`, integrationID, externalID)
	return scanLink(row)
}

func (r *taskLinkRepository) ListByIntegration(ctx context.Context, integrationID uuid.UUID) ([]model.IntegrationTaskLink, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, integration_id, task_id, external_id, external_url, sync_direction, last_synced_at, sync_status, created_at, updated_at
		FROM integration_task_links WHERE integration_id = $1 ORDER BY created_at DESC`, integrationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanLinks(rows)
}

func (r *taskLinkRepository) Update(ctx context.Context, link *model.IntegrationTaskLink) error {
	link.UpdatedAt = time.Now()
	_, err := r.pool.Exec(ctx, `
		UPDATE integration_task_links SET external_id=$1, external_url=$2, sync_direction=$3, last_synced_at=$4, sync_status=$5, updated_at=$6
		WHERE id=$7`,
		link.ExternalID, link.ExternalURL, link.SyncDirection, link.LastSyncedAt, link.SyncStatus, link.UpdatedAt, link.ID,
	)
	return err
}

func (r *taskLinkRepository) Delete(ctx context.Context, id uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM integration_task_links WHERE id = $1`, id)
	return err
}

func (r *taskLinkRepository) DeleteByTaskAndIntegration(ctx context.Context, integrationID, taskID uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM integration_task_links WHERE integration_id = $1 AND task_id = $2`, integrationID, taskID)
	return err
}

func scanLink(s scannable) (*model.IntegrationTaskLink, error) {
	var l model.IntegrationTaskLink
	err := s.Scan(&l.ID, &l.IntegrationID, &l.TaskID, &l.ExternalID, &l.ExternalURL, &l.SyncDirection, &l.LastSyncedAt, &l.SyncStatus, &l.CreatedAt, &l.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &l, nil
}

func scanLinks(rows pgx.Rows) ([]model.IntegrationTaskLink, error) {
	var items []model.IntegrationTaskLink
	for rows.Next() {
		var l model.IntegrationTaskLink
		if err := rows.Scan(&l.ID, &l.IntegrationID, &l.TaskID, &l.ExternalID, &l.ExternalURL, &l.SyncDirection, &l.LastSyncedAt, &l.SyncStatus, &l.CreatedAt, &l.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, l)
	}
	return items, nil
}
