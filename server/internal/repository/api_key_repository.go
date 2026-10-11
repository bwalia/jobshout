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

// ErrAPIKeyNotFound covers a missing key and another org's key alike.
var ErrAPIKeyNotFound = errors.New("api key not found")

// APIKeyRepository persists org API keys (hashes only).
type APIKeyRepository interface {
	Create(ctx context.Context, k *model.APIKey, keyHash string) error
	ListByOrg(ctx context.Context, orgID uuid.UUID) ([]model.APIKey, error)
	// Revoke marks the org's key revoked. ErrAPIKeyNotFound when the key is
	// not this org's, or already revoked.
	Revoke(ctx context.Context, orgID, keyID uuid.UUID) error
	// Authenticate resolves an active key by hash: not revoked, not expired,
	// and its creator still a user of the key's org. It also stamps
	// last_used_at (at most once a minute).
	Authenticate(ctx context.Context, keyHash string) (*model.APIKeyPrincipal, error)
}

type apiKeyRepository struct {
	pool *pgxpool.Pool
}

func NewAPIKeyRepository(pool *pgxpool.Pool) APIKeyRepository {
	return &apiKeyRepository{pool: pool}
}

func (r *apiKeyRepository) Create(ctx context.Context, k *model.APIKey, keyHash string) error {
	if k.ID == uuid.Nil {
		k.ID = uuid.New()
	}
	return r.pool.QueryRow(ctx, `
		INSERT INTO org_api_keys (id, org_id, name, prefix, key_hash, scopes, created_by, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING created_at`,
		k.ID, k.OrgID, k.Name, k.Prefix, keyHash, k.Scopes, k.CreatedBy, k.ExpiresAt,
	).Scan(&k.CreatedAt)
}

func (r *apiKeyRepository) ListByOrg(ctx context.Context, orgID uuid.UUID) ([]model.APIKey, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, org_id, name, prefix, scopes, created_by, last_used_at, expires_at, revoked_at, created_at
		FROM org_api_keys WHERE org_id = $1 ORDER BY created_at DESC`, orgID)
	if err != nil {
		return nil, fmt.Errorf("api_key_repo: list: %w", err)
	}
	defer rows.Close()
	out := []model.APIKey{}
	for rows.Next() {
		var k model.APIKey
		if err := rows.Scan(&k.ID, &k.OrgID, &k.Name, &k.Prefix, &k.Scopes, &k.CreatedBy,
			&k.LastUsedAt, &k.ExpiresAt, &k.RevokedAt, &k.CreatedAt); err != nil {
			return nil, fmt.Errorf("api_key_repo: scan: %w", err)
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

func (r *apiKeyRepository) Revoke(ctx context.Context, orgID, keyID uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE org_api_keys SET revoked_at = NOW()
		WHERE id = $1 AND org_id = $2 AND revoked_at IS NULL`, keyID, orgID)
	if err != nil {
		return fmt.Errorf("api_key_repo: revoke: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrAPIKeyNotFound
	}
	return nil
}

func (r *apiKeyRepository) Authenticate(ctx context.Context, keyHash string) (*model.APIKeyPrincipal, error) {
	p := &model.APIKeyPrincipal{}
	err := r.pool.QueryRow(ctx, `
		SELECT k.id, k.org_id, u.id, k.scopes
		FROM org_api_keys k
		JOIN users u ON u.id = k.created_by AND u.org_id = k.org_id
		WHERE k.key_hash = $1
		  AND k.revoked_at IS NULL
		  AND (k.expires_at IS NULL OR k.expires_at > NOW())`, keyHash,
	).Scan(&p.KeyID, &p.OrgID, &p.UserID, &p.Scopes)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrAPIKeyNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("api_key_repo: authenticate: %w", err)
	}
	_, _ = r.pool.Exec(ctx, `
		UPDATE org_api_keys SET last_used_at = NOW()
		WHERE id = $1 AND (last_used_at IS NULL OR last_used_at < NOW() - INTERVAL '1 minute')`, p.KeyID)
	return p, nil
}
