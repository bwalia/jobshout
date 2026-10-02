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

// DeviceRepository persists signed-in app installs. Every method that takes a
// userID scopes to it, so one user can never read or revoke another's device.
type DeviceRepository interface {
	Create(ctx context.Context, d *model.Device) error
	FindForUser(ctx context.Context, id, userID uuid.UUID) (*model.Device, error)
	// Touch updates the client-reported fields and last_seen_at.
	Touch(ctx context.Context, d *model.Device) error
	ListByUser(ctx context.Context, userID uuid.UUID) ([]model.Device, error)
	// Delete removes the device and, by cascade, its refresh tokens. It
	// reports whether a row was removed.
	Delete(ctx context.Context, id, userID uuid.UUID) (bool, error)
	DeleteAllForUser(ctx context.Context, userID uuid.UUID) error
	// SetPush stores (or clears, with an empty token) the device's push
	// token. A token moves with the install: it is cleared from any other
	// row first, so a phone that switches account stops notifying the old one.
	SetPush(ctx context.Context, id, userID uuid.UUID, token, environment string) (bool, error)
}

type deviceRepository struct {
	pool *pgxpool.Pool
}

// NewDeviceRepository creates a DeviceRepository backed by PostgreSQL.
func NewDeviceRepository(pool *pgxpool.Pool) DeviceRepository {
	return &deviceRepository{pool: pool}
}

const deviceColumns = `id, user_id, platform, name, app_version, push_token, push_environment, created_at, last_seen_at`

func scanDevice(row pgx.Row) (*model.Device, error) {
	d := &model.Device{}
	if err := row.Scan(&d.ID, &d.UserID, &d.Platform, &d.Name, &d.AppVersion,
		&d.PushToken, &d.PushEnvironment, &d.CreatedAt, &d.LastSeenAt); err != nil {
		return nil, err
	}
	d.HasPush = d.PushToken != nil && *d.PushToken != ""
	return d, nil
}

func (r *deviceRepository) Create(ctx context.Context, d *model.Device) error {
	query := `
		INSERT INTO devices (id, user_id, platform, name, app_version)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING created_at, last_seen_at`
	if err := r.pool.QueryRow(ctx, query, d.ID, d.UserID, d.Platform, d.Name, d.AppVersion).
		Scan(&d.CreatedAt, &d.LastSeenAt); err != nil {
		return fmt.Errorf("creating device: %w", err)
	}
	return nil
}

func (r *deviceRepository) FindForUser(ctx context.Context, id, userID uuid.UUID) (*model.Device, error) {
	query := `SELECT ` + deviceColumns + ` FROM devices WHERE id = $1 AND user_id = $2`
	d, err := scanDevice(r.pool.QueryRow(ctx, query, id, userID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("finding device: %w", err)
	}
	return d, nil
}

func (r *deviceRepository) Touch(ctx context.Context, d *model.Device) error {
	query := `
		UPDATE devices SET platform = $1, name = $2, app_version = $3, last_seen_at = NOW()
		WHERE id = $4 AND user_id = $5
		RETURNING last_seen_at`
	if err := r.pool.QueryRow(ctx, query, d.Platform, d.Name, d.AppVersion, d.ID, d.UserID).
		Scan(&d.LastSeenAt); err != nil {
		return fmt.Errorf("touching device: %w", err)
	}
	return nil
}

func (r *deviceRepository) ListByUser(ctx context.Context, userID uuid.UUID) ([]model.Device, error) {
	query := `SELECT ` + deviceColumns + ` FROM devices WHERE user_id = $1 ORDER BY last_seen_at DESC`
	rows, err := r.pool.Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("listing devices: %w", err)
	}
	defer rows.Close()
	out := []model.Device{}
	for rows.Next() {
		d, err := scanDevice(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning device: %w", err)
		}
		out = append(out, *d)
	}
	return out, rows.Err()
}

func (r *deviceRepository) Delete(ctx context.Context, id, userID uuid.UUID) (bool, error) {
	tag, err := r.pool.Exec(ctx, `DELETE FROM devices WHERE id = $1 AND user_id = $2`, id, userID)
	if err != nil {
		return false, fmt.Errorf("deleting device: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

func (r *deviceRepository) DeleteAllForUser(ctx context.Context, userID uuid.UUID) error {
	if _, err := r.pool.Exec(ctx, `DELETE FROM devices WHERE user_id = $1`, userID); err != nil {
		return fmt.Errorf("deleting devices: %w", err)
	}
	return nil
}

func (r *deviceRepository) SetPush(ctx context.Context, id, userID uuid.UUID, token, environment string) (bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var tokenArg, envArg *string
	if token != "" {
		tokenArg, envArg = &token, &environment
		if _, err := tx.Exec(ctx,
			`UPDATE devices SET push_token = NULL, push_environment = NULL WHERE push_token = $1 AND id <> $2`,
			token, id); err != nil {
			return false, fmt.Errorf("releasing push token: %w", err)
		}
	}
	tag, err := tx.Exec(ctx, `
		UPDATE devices SET push_token = $1, push_environment = $2, last_seen_at = NOW()
		WHERE id = $3 AND user_id = $4`, tokenArg, envArg, id, userID)
	if err != nil {
		return false, fmt.Errorf("setting push token: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("commit: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}
