package repository

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
	"github.com/jobshout/server/internal/secretbox"
)

// ErrWebhookNotFound covers a missing endpoint and another org's alike.
var ErrWebhookNotFound = errors.New("webhook endpoint not found")

// WebhookRepository persists org webhook endpoints and their delivery log.
// Endpoint secrets are sealed with the box (when configured) and returned
// opened; every endpoint read is org-scoped.
type WebhookRepository interface {
	CreateEndpoint(ctx context.Context, e *model.WebhookEndpoint) error
	GetEndpoint(ctx context.Context, orgID, id uuid.UUID) (*model.WebhookEndpoint, error)
	ListEndpoints(ctx context.Context, orgID uuid.UUID) ([]model.WebhookEndpoint, error)
	UpdateEndpoint(ctx context.Context, e *model.WebhookEndpoint) error
	DeleteEndpoint(ctx context.Context, orgID, id uuid.UUID) error
	// ListSubscribed returns the org's enabled endpoints subscribed to eventType.
	ListSubscribed(ctx context.Context, orgID uuid.UUID, eventType string) ([]model.WebhookEndpoint, error)

	// CreateDelivery inserts a pending delivery leased until nextAttempt.
	CreateDelivery(ctx context.Context, d *model.WebhookDelivery) error
	// ClaimDue leases up to limit pending deliveries whose next attempt is
	// due, pushing their next_attempt_at out by lease so no other worker (or
	// replica) picks them up meanwhile.
	ClaimDue(ctx context.Context, limit int, lease time.Duration) ([]model.WebhookDelivery, error)
	// RecordAttempt stores the outcome of one attempt.
	RecordAttempt(ctx context.Context, id uuid.UUID, status string, attempts int, code *int, errMsg *string, next *time.Time) error
	ListDeliveries(ctx context.Context, orgID, endpointID uuid.UUID, limit int) ([]model.WebhookDelivery, error)
	// GetEndpointForDelivery loads an endpoint by id regardless of org (the
	// worker side; the delivery row already carries the org).
	GetEndpointForDelivery(ctx context.Context, id uuid.UUID) (*model.WebhookEndpoint, error)
}

type webhookRepository struct {
	pool *pgxpool.Pool
	box  *secretbox.Box
}

// NewWebhookRepository builds the repository. box may be nil (plaintext).
func NewWebhookRepository(pool *pgxpool.Pool, box *secretbox.Box) WebhookRepository {
	return &webhookRepository{pool: pool, box: box}
}

const webhookEndpointColumns = `id, org_id, url, secret, events, description, enabled, created_by, created_at, updated_at`

func (r *webhookRepository) scanEndpoint(row pgx.Row) (*model.WebhookEndpoint, error) {
	e := &model.WebhookEndpoint{}
	if err := row.Scan(&e.ID, &e.OrgID, &e.URL, &e.Secret, &e.Events, &e.Description,
		&e.Enabled, &e.CreatedBy, &e.CreatedAt, &e.UpdatedAt); err != nil {
		return nil, err
	}
	plain, err := r.box.Open(e.Secret)
	if err != nil {
		return nil, fmt.Errorf("webhook_repo: open secret: %w", err)
	}
	e.Secret = plain
	return e, nil
}

func (r *webhookRepository) CreateEndpoint(ctx context.Context, e *model.WebhookEndpoint) error {
	if e.ID == uuid.Nil {
		e.ID = uuid.New()
	}
	sealed, err := r.box.Seal(e.Secret)
	if err != nil {
		return fmt.Errorf("webhook_repo: seal secret: %w", err)
	}
	return r.pool.QueryRow(ctx, `
		INSERT INTO org_webhook_endpoints (id, org_id, url, secret, events, description, enabled, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING created_at, updated_at`,
		e.ID, e.OrgID, e.URL, sealed, e.Events, e.Description, e.Enabled, e.CreatedBy,
	).Scan(&e.CreatedAt, &e.UpdatedAt)
}

func (r *webhookRepository) GetEndpoint(ctx context.Context, orgID, id uuid.UUID) (*model.WebhookEndpoint, error) {
	e, err := r.scanEndpoint(r.pool.QueryRow(ctx,
		`SELECT `+webhookEndpointColumns+` FROM org_webhook_endpoints WHERE id = $1 AND org_id = $2`, id, orgID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrWebhookNotFound
	}
	return e, err
}

func (r *webhookRepository) GetEndpointForDelivery(ctx context.Context, id uuid.UUID) (*model.WebhookEndpoint, error) {
	e, err := r.scanEndpoint(r.pool.QueryRow(ctx,
		`SELECT `+webhookEndpointColumns+` FROM org_webhook_endpoints WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrWebhookNotFound
	}
	return e, err
}

func (r *webhookRepository) listEndpoints(ctx context.Context, sql string, args ...any) ([]model.WebhookEndpoint, error) {
	rows, err := r.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("webhook_repo: list endpoints: %w", err)
	}
	defer rows.Close()
	out := []model.WebhookEndpoint{}
	for rows.Next() {
		e, err := r.scanEndpoint(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *e)
	}
	return out, rows.Err()
}

func (r *webhookRepository) ListEndpoints(ctx context.Context, orgID uuid.UUID) ([]model.WebhookEndpoint, error) {
	return r.listEndpoints(ctx,
		`SELECT `+webhookEndpointColumns+` FROM org_webhook_endpoints WHERE org_id = $1 ORDER BY created_at`, orgID)
}

func (r *webhookRepository) ListSubscribed(ctx context.Context, orgID uuid.UUID, eventType string) ([]model.WebhookEndpoint, error) {
	return r.listEndpoints(ctx, `SELECT `+webhookEndpointColumns+` FROM org_webhook_endpoints
		WHERE org_id = $1 AND enabled AND ($2 = ANY(events) OR '*' = ANY(events)) ORDER BY created_at`, orgID, eventType)
}

func (r *webhookRepository) UpdateEndpoint(ctx context.Context, e *model.WebhookEndpoint) error {
	sealed, err := r.box.Seal(e.Secret)
	if err != nil {
		return fmt.Errorf("webhook_repo: seal secret: %w", err)
	}
	err = r.pool.QueryRow(ctx, `
		UPDATE org_webhook_endpoints
		SET url = $3, secret = $4, events = $5, description = $6, enabled = $7, updated_at = NOW()
		WHERE id = $1 AND org_id = $2
		RETURNING updated_at`,
		e.ID, e.OrgID, e.URL, sealed, e.Events, e.Description, e.Enabled,
	).Scan(&e.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrWebhookNotFound
	}
	return err
}

func (r *webhookRepository) DeleteEndpoint(ctx context.Context, orgID, id uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM org_webhook_endpoints WHERE id = $1 AND org_id = $2`, id, orgID)
	if err != nil {
		return fmt.Errorf("webhook_repo: delete: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrWebhookNotFound
	}
	return nil
}

const webhookDeliveryColumns = `id, endpoint_id, org_id, event_id, event_type, payload, status, attempts,
	last_status_code, last_error, next_attempt_at, delivered_at, created_at, updated_at`

func scanDelivery(row pgx.Row) (*model.WebhookDelivery, error) {
	d := &model.WebhookDelivery{}
	var payload []byte
	if err := row.Scan(&d.ID, &d.EndpointID, &d.OrgID, &d.EventID, &d.EventType, &payload, &d.Status,
		&d.Attempts, &d.LastStatusCode, &d.LastError, &d.NextAttemptAt, &d.DeliveredAt, &d.CreatedAt, &d.UpdatedAt); err != nil {
		return nil, err
	}
	d.Payload = json.RawMessage(payload)
	return d, nil
}

func (r *webhookRepository) CreateDelivery(ctx context.Context, d *model.WebhookDelivery) error {
	if d.ID == uuid.Nil {
		d.ID = uuid.New()
	}
	if d.Status == "" {
		d.Status = model.WebhookDeliveryPending
	}
	return r.pool.QueryRow(ctx, `
		INSERT INTO org_webhook_deliveries (id, endpoint_id, org_id, event_id, event_type, payload, status, next_attempt_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING created_at, updated_at`,
		d.ID, d.EndpointID, d.OrgID, d.EventID, d.EventType, []byte(d.Payload), d.Status, d.NextAttemptAt,
	).Scan(&d.CreatedAt, &d.UpdatedAt)
}

func (r *webhookRepository) ClaimDue(ctx context.Context, limit int, lease time.Duration) ([]model.WebhookDelivery, error) {
	rows, err := r.pool.Query(ctx, `
		UPDATE org_webhook_deliveries d
		SET next_attempt_at = NOW() + make_interval(secs => $2), updated_at = NOW()
		WHERE d.id IN (
			SELECT id FROM org_webhook_deliveries
			WHERE status = 'pending' AND next_attempt_at <= NOW()
			ORDER BY next_attempt_at
			LIMIT $1
			FOR UPDATE SKIP LOCKED
		)
		RETURNING `+webhookDeliveryColumns, limit, lease.Seconds())
	if err != nil {
		return nil, fmt.Errorf("webhook_repo: claim due: %w", err)
	}
	defer rows.Close()
	out := []model.WebhookDelivery{}
	for rows.Next() {
		d, err := scanDelivery(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *d)
	}
	return out, rows.Err()
}

func (r *webhookRepository) RecordAttempt(ctx context.Context, id uuid.UUID, status string, attempts int, code *int, errMsg *string, next *time.Time) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE org_webhook_deliveries
		SET status = $2::varchar, attempts = $3, last_status_code = $4, last_error = $5, next_attempt_at = $6,
		    delivered_at = CASE WHEN $2::varchar = 'succeeded' THEN NOW() ELSE delivered_at END,
		    updated_at = NOW()
		WHERE id = $1`, id, status, attempts, code, errMsg, next)
	if err != nil {
		return fmt.Errorf("webhook_repo: record attempt: %w", err)
	}
	return nil
}

func (r *webhookRepository) ListDeliveries(ctx context.Context, orgID, endpointID uuid.UUID, limit int) ([]model.WebhookDelivery, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := r.pool.Query(ctx, `SELECT `+webhookDeliveryColumns+` FROM org_webhook_deliveries
		WHERE org_id = $1 AND endpoint_id = $2 ORDER BY created_at DESC LIMIT $3`, orgID, endpointID, limit)
	if err != nil {
		return nil, fmt.Errorf("webhook_repo: list deliveries: %w", err)
	}
	defer rows.Close()
	out := []model.WebhookDelivery{}
	for rows.Next() {
		d, err := scanDelivery(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *d)
	}
	return out, rows.Err()
}
