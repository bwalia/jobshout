package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// IdentityRepository links external sign-in subjects (provider, subject) to
// users.
type IdentityRepository interface {
	// FindUserID returns the linked user, or uuid.Nil when none is.
	FindUserID(ctx context.Context, provider, subject string) (uuid.UUID, error)
	// Link is idempotent: re-linking the same subject to the same user is a
	// no-op, and a subject already linked to someone else is left alone.
	Link(ctx context.Context, provider, subject string, userID uuid.UUID, email string) error
}

type identityRepository struct {
	pool *pgxpool.Pool
}

// NewIdentityRepository creates an IdentityRepository backed by PostgreSQL.
func NewIdentityRepository(pool *pgxpool.Pool) IdentityRepository {
	return &identityRepository{pool: pool}
}

func (r *identityRepository) FindUserID(ctx context.Context, provider, subject string) (uuid.UUID, error) {
	var id uuid.UUID
	err := r.pool.QueryRow(ctx,
		`SELECT user_id FROM user_identities WHERE provider = $1 AND subject = $2`,
		provider, subject).Scan(&id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, nil
		}
		return uuid.Nil, fmt.Errorf("finding identity: %w", err)
	}
	return id, nil
}

func (r *identityRepository) Link(ctx context.Context, provider, subject string, userID uuid.UUID, email string) error {
	var emailArg *string
	if email != "" {
		emailArg = &email
	}
	if _, err := r.pool.Exec(ctx, `
		INSERT INTO user_identities (provider, subject, user_id, email)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (provider, subject) DO NOTHING`,
		provider, subject, userID, emailArg); err != nil {
		return fmt.Errorf("linking identity: %w", err)
	}
	return nil
}
