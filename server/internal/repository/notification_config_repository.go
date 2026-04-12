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

type NotificationConfigRepository interface {
	Create(ctx context.Context, cfg *model.NotificationConfig) error
	FindByID(ctx context.Context, id uuid.UUID) (*model.NotificationConfig, error)
	ListByOrg(ctx context.Context, orgID uuid.UUID) ([]model.NotificationConfig, error)
	ListByOrgAndEvent(ctx context.Context, orgID uuid.UUID, eventType string) ([]model.NotificationConfig, error)
	Update(ctx context.Context, cfg *model.NotificationConfig) error
	Delete(ctx context.Context, id uuid.UUID) error
}

type notificationConfigRepository struct {
	pool *pgxpool.Pool
}

func NewNotificationConfigRepository(pool *pgxpool.Pool) NotificationConfigRepository {
	return &notificationConfigRepository{pool: pool}
}

func (r *notificationConfigRepository) Create(ctx context.Context, cfg *model.NotificationConfig) error {
	cfg.ID = uuid.New()
	cfg.CreatedAt = time.Now()
	cfg.UpdatedAt = cfg.CreatedAt
	configJSON, _ := json.Marshal(cfg.Config)

	_, err := r.pool.Exec(ctx, `
		INSERT INTO notification_configs (id, org_id, name, channel_type, webhook_url, config, enabled, events, created_by, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		cfg.ID, cfg.OrgID, cfg.Name, cfg.ChannelType, cfg.WebhookURL, configJSON, cfg.Enabled, cfg.Events, cfg.CreatedBy, cfg.CreatedAt, cfg.UpdatedAt,
	)
	return err
}

func (r *notificationConfigRepository) FindByID(ctx context.Context, id uuid.UUID) (*model.NotificationConfig, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, org_id, name, channel_type, webhook_url, config, enabled, events, created_by, created_at, updated_at
		FROM notification_configs WHERE id = $1`, id)
	return scanNotificationConfig(row)
}

func (r *notificationConfigRepository) ListByOrg(ctx context.Context, orgID uuid.UUID) ([]model.NotificationConfig, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, org_id, name, channel_type, webhook_url, config, enabled, events, created_by, created_at, updated_at
		FROM notification_configs WHERE org_id = $1 ORDER BY created_at DESC`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanNotificationConfigs(rows)
}

func (r *notificationConfigRepository) ListByOrgAndEvent(ctx context.Context, orgID uuid.UUID, eventType string) ([]model.NotificationConfig, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, org_id, name, channel_type, webhook_url, config, enabled, events, created_by, created_at, updated_at
		FROM notification_configs WHERE org_id = $1 AND enabled = true AND $2 = ANY(events)
		ORDER BY created_at DESC`, orgID, eventType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanNotificationConfigs(rows)
}

func (r *notificationConfigRepository) Update(ctx context.Context, cfg *model.NotificationConfig) error {
	cfg.UpdatedAt = time.Now()
	configJSON, _ := json.Marshal(cfg.Config)

	_, err := r.pool.Exec(ctx, `
		UPDATE notification_configs SET name=$1, webhook_url=$2, config=$3, enabled=$4, events=$5, updated_at=$6
		WHERE id=$7`,
		cfg.Name, cfg.WebhookURL, configJSON, cfg.Enabled, cfg.Events, cfg.UpdatedAt, cfg.ID,
	)
	return err
}

func (r *notificationConfigRepository) Delete(ctx context.Context, id uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM notification_configs WHERE id = $1`, id)
	return err
}

func scanNotificationConfig(s scannable) (*model.NotificationConfig, error) {
	var c model.NotificationConfig
	var configJSON []byte
	err := s.Scan(&c.ID, &c.OrgID, &c.Name, &c.ChannelType, &c.WebhookURL, &configJSON, &c.Enabled, &c.Events, &c.CreatedBy, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	_ = json.Unmarshal(configJSON, &c.Config)
	return &c, nil
}

func scanNotificationConfigs(rows pgx.Rows) ([]model.NotificationConfig, error) {
	var items []model.NotificationConfig
	for rows.Next() {
		var c model.NotificationConfig
		var configJSON []byte
		if err := rows.Scan(&c.ID, &c.OrgID, &c.Name, &c.ChannelType, &c.WebhookURL, &configJSON, &c.Enabled, &c.Events, &c.CreatedBy, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(configJSON, &c.Config)
		items = append(items, c)
	}
	return items, nil
}
