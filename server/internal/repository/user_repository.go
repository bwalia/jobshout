package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jobshout/server/internal/model"
)

// UserRepository defines operations for user persistence.
type UserRepository interface {
	Create(ctx context.Context, user *model.User) error
	FindByEmail(ctx context.Context, email string) (*model.User, error)
	FindByEmailFold(ctx context.Context, email string) (*model.User, error)
	FindByID(ctx context.Context, id uuid.UUID) (*model.User, error)
	FindByGoogleSub(ctx context.Context, googleSub string) (*model.User, error)
	UpdateOrgID(ctx context.Context, userID uuid.UUID, orgID uuid.UUID) error
	UpdateProfile(ctx context.Context, user *model.User) error
	LinkGoogleSub(ctx context.Context, userID uuid.UUID, googleSub string, avatarURL *string) error
	PutGoogleOAuthState(ctx context.Context, st *model.GoogleOAuthState) error
	ConsumeGoogleOAuthState(ctx context.Context, state string) (*model.GoogleOAuthState, error)
	PutGoogleOAuthTicket(ctx context.Context, t *model.GoogleOAuthTicket) error
	ConsumeGoogleOAuthTicket(ctx context.Context, ticket string) (*model.GoogleOAuthTicket, error)
}

type userRepository struct {
	pool *pgxpool.Pool
}

// NewUserRepository creates a new UserRepository backed by PostgreSQL.
func NewUserRepository(pool *pgxpool.Pool) UserRepository {
	return &userRepository{pool: pool}
}

const userColumns = `id, email, password, full_name, avatar_url, role, org_id, google_sub, created_at, updated_at`

func scanUser(row pgx.Row) (*model.User, error) {
	user := &model.User{}
	err := row.Scan(
		&user.ID, &user.Email, &user.Password, &user.FullName,
		&user.AvatarURL, &user.Role, &user.OrgID, &user.GoogleSub,
		&user.CreatedAt, &user.UpdatedAt,
	)
	return user, err
}

func (r *userRepository) Create(ctx context.Context, user *model.User) error {
	query := `
		INSERT INTO users (id, email, password, full_name, avatar_url, role, org_id, google_sub, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW(), NOW())
		RETURNING created_at, updated_at`

	return r.pool.QueryRow(ctx, query,
		user.ID, user.Email, user.Password, user.FullName, user.AvatarURL,
		user.Role, user.OrgID, user.GoogleSub,
	).Scan(&user.CreatedAt, &user.UpdatedAt)
}

func (r *userRepository) FindByEmail(ctx context.Context, email string) (*model.User, error) {
	query := `SELECT ` + userColumns + ` FROM users WHERE email = $1`
	user, err := scanUser(r.pool.QueryRow(ctx, query, email))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("finding user by email: %w", err)
	}
	return user, nil
}

func (r *userRepository) FindByEmailFold(ctx context.Context, email string) (*model.User, error) {
	query := `SELECT ` + userColumns + ` FROM users WHERE LOWER(email) = LOWER($1)`
	user, err := scanUser(r.pool.QueryRow(ctx, query, email))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("finding user by email fold: %w", err)
	}
	return user, nil
}

func (r *userRepository) FindByID(ctx context.Context, id uuid.UUID) (*model.User, error) {
	query := `SELECT ` + userColumns + ` FROM users WHERE id = $1`
	user, err := scanUser(r.pool.QueryRow(ctx, query, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("finding user by id: %w", err)
	}
	return user, nil
}

func (r *userRepository) FindByGoogleSub(ctx context.Context, googleSub string) (*model.User, error) {
	query := `SELECT ` + userColumns + ` FROM users WHERE google_sub = $1`
	user, err := scanUser(r.pool.QueryRow(ctx, query, googleSub))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("finding user by google_sub: %w", err)
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

func (r *userRepository) UpdateProfile(ctx context.Context, user *model.User) error {
	query := `UPDATE users SET full_name = $1, avatar_url = $2, updated_at = NOW()
		WHERE id = $3 RETURNING updated_at`
	return r.pool.QueryRow(ctx, query, user.FullName, user.AvatarURL, user.ID).Scan(&user.UpdatedAt)
}

func (r *userRepository) LinkGoogleSub(ctx context.Context, userID uuid.UUID, googleSub string, avatarURL *string) error {
	query := `
		UPDATE users
		SET google_sub = $1,
		    avatar_url = COALESCE(NULLIF(avatar_url, ''), $2),
		    updated_at = NOW()
		WHERE id = $3`
	_, err := r.pool.Exec(ctx, query, googleSub, avatarURL, userID)
	if err != nil {
		return fmt.Errorf("linking google_sub: %w", err)
	}
	return nil
}

func (r *userRepository) PutGoogleOAuthState(ctx context.Context, st *model.GoogleOAuthState) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO auth_oauth_states (state, intent, org_name, expires_at)
		VALUES ($1, $2, $3, $4)`, st.State, st.Intent, st.OrgName, st.ExpiresAt)
	if err != nil {
		return fmt.Errorf("put google oauth state: %w", err)
	}
	return nil
}

func (r *userRepository) ConsumeGoogleOAuthState(ctx context.Context, state string) (*model.GoogleOAuthState, error) {
	st := &model.GoogleOAuthState{}
	err := r.pool.QueryRow(ctx, `
		DELETE FROM auth_oauth_states WHERE state = $1
		RETURNING state, intent, org_name, expires_at`, state).Scan(
		&st.State, &st.Intent, &st.OrgName, &st.ExpiresAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("consume google oauth state: %w", err)
	}
	if time.Now().After(st.ExpiresAt) {
		return nil, nil
	}
	return st, nil
}

func (r *userRepository) PutGoogleOAuthTicket(ctx context.Context, t *model.GoogleOAuthTicket) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO auth_oauth_tickets (ticket, user_id, expires_at)
		VALUES ($1, $2, $3)`, t.Ticket, t.UserID, t.ExpiresAt)
	if err != nil {
		return fmt.Errorf("put google oauth ticket: %w", err)
	}
	return nil
}

func (r *userRepository) ConsumeGoogleOAuthTicket(ctx context.Context, ticket string) (*model.GoogleOAuthTicket, error) {
	t := &model.GoogleOAuthTicket{}
	err := r.pool.QueryRow(ctx, `
		DELETE FROM auth_oauth_tickets WHERE ticket = $1
		RETURNING ticket, user_id, expires_at`, ticket).Scan(
		&t.Ticket, &t.UserID, &t.ExpiresAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("consume google oauth ticket: %w", err)
	}
	if time.Now().After(t.ExpiresAt) {
		return nil, nil
	}
	return t, nil
}
