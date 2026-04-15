package repository

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jobshout/server/internal/model"
)

// TelegramRepository manages Telegram user mappings, link tokens, and rate limits.
type TelegramRepository interface {
	CreateMapping(ctx context.Context, m *model.TelegramUserMapping) error
	FindByTelegramID(ctx context.Context, telegramUserID int64) (*model.TelegramUserMapping, error)
	FindByJobshoutUser(ctx context.Context, userID uuid.UUID) (*model.TelegramUserMapping, error)
	DeleteMapping(ctx context.Context, id uuid.UUID) error
	DeleteByTelegramID(ctx context.Context, telegramUserID int64) error

	StoreLinkToken(ctx context.Context, token *model.TelegramLinkToken) error
	ConsumeLinkToken(ctx context.Context, token string) (*model.TelegramLinkToken, error)

	CheckRateLimit(ctx context.Context, telegramUserID int64, maxTokens float64, refillPerSec float64) (bool, error)
}

type telegramRepository struct {
	pool *pgxpool.Pool
}

func NewTelegramRepository(pool *pgxpool.Pool) TelegramRepository {
	return &telegramRepository{pool: pool}
}

func (r *telegramRepository) CreateMapping(ctx context.Context, m *model.TelegramUserMapping) error {
	if m.ID == uuid.Nil {
		m.ID = uuid.New()
	}
	const sql = `
		INSERT INTO telegram_user_mappings (id, telegram_user_id, telegram_username, jobshout_user_id, org_id, verified)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (telegram_user_id) DO UPDATE
		SET jobshout_user_id = EXCLUDED.jobshout_user_id,
		    org_id = EXCLUDED.org_id,
		    telegram_username = EXCLUDED.telegram_username,
		    verified = EXCLUDED.verified,
		    linked_at = NOW()`

	_, err := r.pool.Exec(ctx, sql,
		m.ID, m.TelegramUserID, m.TelegramUsername,
		m.JobshoutUserID, m.OrgID, m.Verified,
	)
	if err != nil {
		return fmt.Errorf("telegram_repo: create_mapping: %w", err)
	}
	return nil
}

func (r *telegramRepository) FindByTelegramID(ctx context.Context, telegramUserID int64) (*model.TelegramUserMapping, error) {
	const sql = `
		SELECT id, telegram_user_id, telegram_username, jobshout_user_id, org_id, verified, linked_at
		FROM telegram_user_mappings WHERE telegram_user_id = $1`

	var m model.TelegramUserMapping
	err := r.pool.QueryRow(ctx, sql, telegramUserID).Scan(
		&m.ID, &m.TelegramUserID, &m.TelegramUsername,
		&m.JobshoutUserID, &m.OrgID, &m.Verified, &m.LinkedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("telegram_repo: find_by_telegram_id: %w", err)
	}
	return &m, nil
}

func (r *telegramRepository) FindByJobshoutUser(ctx context.Context, userID uuid.UUID) (*model.TelegramUserMapping, error) {
	const sql = `
		SELECT id, telegram_user_id, telegram_username, jobshout_user_id, org_id, verified, linked_at
		FROM telegram_user_mappings WHERE jobshout_user_id = $1`

	var m model.TelegramUserMapping
	err := r.pool.QueryRow(ctx, sql, userID).Scan(
		&m.ID, &m.TelegramUserID, &m.TelegramUsername,
		&m.JobshoutUserID, &m.OrgID, &m.Verified, &m.LinkedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("telegram_repo: find_by_jobshout_user: %w", err)
	}
	return &m, nil
}

func (r *telegramRepository) DeleteMapping(ctx context.Context, id uuid.UUID) error {
	const sql = `DELETE FROM telegram_user_mappings WHERE id = $1`
	_, err := r.pool.Exec(ctx, sql, id)
	if err != nil {
		return fmt.Errorf("telegram_repo: delete_mapping: %w", err)
	}
	return nil
}

func (r *telegramRepository) DeleteByTelegramID(ctx context.Context, telegramUserID int64) error {
	const sql = `DELETE FROM telegram_user_mappings WHERE telegram_user_id = $1`
	_, err := r.pool.Exec(ctx, sql, telegramUserID)
	if err != nil {
		return fmt.Errorf("telegram_repo: delete_by_telegram_id: %w", err)
	}
	return nil
}

func (r *telegramRepository) StoreLinkToken(ctx context.Context, token *model.TelegramLinkToken) error {
	if token.Token == "" {
		b := make([]byte, 16)
		if _, err := rand.Read(b); err != nil {
			return fmt.Errorf("telegram_repo: generate_token: %w", err)
		}
		token.Token = hex.EncodeToString(b)
	}
	if token.ExpiresAt.IsZero() {
		token.ExpiresAt = time.Now().Add(15 * time.Minute)
	}

	const sql = `
		INSERT INTO telegram_link_tokens (token, user_id, org_id, expires_at)
		VALUES ($1, $2, $3, $4)`

	_, err := r.pool.Exec(ctx, sql, token.Token, token.UserID, token.OrgID, token.ExpiresAt)
	if err != nil {
		return fmt.Errorf("telegram_repo: store_link_token: %w", err)
	}
	return nil
}

func (r *telegramRepository) ConsumeLinkToken(ctx context.Context, token string) (*model.TelegramLinkToken, error) {
	const sql = `
		DELETE FROM telegram_link_tokens
		WHERE token = $1 AND expires_at > NOW()
		RETURNING token, user_id, org_id, expires_at`

	var t model.TelegramLinkToken
	err := r.pool.QueryRow(ctx, sql, token).Scan(
		&t.Token, &t.UserID, &t.OrgID, &t.ExpiresAt,
	)
	if err != nil {
		return nil, fmt.Errorf("telegram_repo: consume_link_token: %w", err)
	}
	return &t, nil
}

// CheckRateLimit implements a token bucket rate limiter using Postgres.
// Returns true if the request is allowed, false if rate-limited.
func (r *telegramRepository) CheckRateLimit(ctx context.Context, telegramUserID int64, maxTokens float64, refillPerSec float64) (bool, error) {
	const sql = `
		INSERT INTO telegram_rate_limits (telegram_user_id, tokens_remaining, last_refill)
		VALUES ($1, $2, NOW())
		ON CONFLICT (telegram_user_id) DO UPDATE
		SET tokens_remaining = LEAST(
			$2,
			telegram_rate_limits.tokens_remaining +
			EXTRACT(EPOCH FROM NOW() - telegram_rate_limits.last_refill) * $3
		) - 1,
		last_refill = NOW()
		RETURNING tokens_remaining`

	var remaining float64
	err := r.pool.QueryRow(ctx, sql, telegramUserID, maxTokens, refillPerSec).Scan(&remaining)
	if err != nil {
		return false, fmt.Errorf("telegram_repo: check_rate_limit: %w", err)
	}
	return remaining >= 0, nil
}
