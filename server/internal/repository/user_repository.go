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

// UserRepository defines operations for user persistence.
type UserRepository interface {
	Create(ctx context.Context, user *model.User) error
	FindByEmail(ctx context.Context, email string) (*model.User, error)
	FindByID(ctx context.Context, id uuid.UUID) (*model.User, error)
	UpdateOrgID(ctx context.Context, userID uuid.UUID, orgID uuid.UUID) error
}

type userRepository struct {
	pool *pgxpool.Pool
}

// NewUserRepository creates a new UserRepository backed by PostgreSQL.
func NewUserRepository(pool *pgxpool.Pool) UserRepository {
	return &userRepository{pool: pool}
}

func (r *userRepository) Create(ctx context.Context, user *model.User) error {
	query := `
		INSERT INTO users (id, email, password, full_name, role, org_id, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, NOW(), NOW())
		RETURNING created_at, updated_at`

	return r.pool.QueryRow(ctx, query,
		user.ID, user.Email, user.Password, user.FullName, user.Role, user.OrgID,
	).Scan(&user.CreatedAt, &user.UpdatedAt)
}

func (r *userRepository) FindByEmail(ctx context.Context, email string) (*model.User, error) {
	query := `
		SELECT id, email, password, full_name, avatar_url, role, org_id, created_at, updated_at
		FROM users WHERE email = $1`

	user := &model.User{}
	err := r.pool.QueryRow(ctx, query, email).Scan(
		&user.ID, &user.Email, &user.Password, &user.FullName,
		&user.AvatarURL, &user.Role, &user.OrgID,
		&user.CreatedAt, &user.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("finding user by email: %w", err)
	}
	return user, nil
}

func (r *userRepository) FindByID(ctx context.Context, id uuid.UUID) (*model.User, error) {
	query := `
		SELECT id, email, password, full_name, avatar_url, role, org_id, created_at, updated_at
		FROM users WHERE id = $1`

	user := &model.User{}
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&user.ID, &user.Email, &user.Password, &user.FullName,
		&user.AvatarURL, &user.Role, &user.OrgID,
		&user.CreatedAt, &user.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("finding user by id: %w", err)
	}
	return user, nil
}

func (r *userRepository) UpdateOrgID(ctx context.Context, userID uuid.UUID, orgID uuid.UUID) error {
	query := `UPDATE users SET org_id = $1, updated_at = NOW() WHERE id = $2`
	_, err := r.pool.Exec(ctx, query, orgID, userID)
	if err != nil {
		return fmt.Errorf("updating user org_id: %w", err)
	}
	return nil
}
