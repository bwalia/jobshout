package repository

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jobshout/server/internal/model"
)

type SyncLogRepository interface {
	Append(ctx context.Context, log *model.IntegrationSyncLog) error
	ListByIntegration(ctx context.Context, integrationID uuid.UUID, params model.PaginationParams) (*model.PaginatedResponse[model.IntegrationSyncLog], error)
}

type syncLogRepository struct {
	pool *pgxpool.Pool
}

func NewSyncLogRepository(pool *pgxpool.Pool) SyncLogRepository {
	return &syncLogRepository{pool: pool}
}

func (r *syncLogRepository) Append(ctx context.Context, log *model.IntegrationSyncLog) error {
	log.ID = uuid.New()
	reqJSON, _ := json.Marshal(log.RequestBody)
	respJSON, _ := json.Marshal(log.ResponseBody)

	_, err := r.pool.Exec(ctx, `
		INSERT INTO integration_sync_logs (id, integration_id, task_link_id, direction, status, error_message, request_body, response_body, duration_ms)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		log.ID, log.IntegrationID, log.TaskLinkID, log.Direction, log.Status, log.ErrorMessage, reqJSON, respJSON, log.DurationMs,
	)
	return err
}

func (r *syncLogRepository) ListByIntegration(ctx context.Context, integrationID uuid.UUID, params model.PaginationParams) (*model.PaginatedResponse[model.IntegrationSyncLog], error) {
	page := params.Page
	if page < 1 {
		page = 1
	}
	perPage := params.PerPage
	if perPage < 1 {
		perPage = 20
	}

	var total int
	err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM integration_sync_logs WHERE integration_id = $1`, integrationID).Scan(&total)
	if err != nil {
		return nil, err
	}

	rows, err := r.pool.Query(ctx, `
		SELECT id, integration_id, task_link_id, direction, status, error_message, request_body, response_body, duration_ms, created_at
		FROM integration_sync_logs WHERE integration_id = $1 ORDER BY created_at DESC
		LIMIT $2 OFFSET $3`, integrationID, perPage, (page-1)*perPage)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []model.IntegrationSyncLog
	for rows.Next() {
		var l model.IntegrationSyncLog
		var reqJSON, respJSON []byte
		if err := rows.Scan(&l.ID, &l.IntegrationID, &l.TaskLinkID, &l.Direction, &l.Status, &l.ErrorMessage, &reqJSON, &respJSON, &l.DurationMs, &l.CreatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(reqJSON, &l.RequestBody)
		_ = json.Unmarshal(respJSON, &l.ResponseBody)
		items = append(items, l)
	}

	return &model.PaginatedResponse[model.IntegrationSyncLog]{
		Data:  items,
		Total: total,
		Page:  page,
	}, nil
}
