package seo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jobshout/server/internal/mail"
)

// GitHubConnection is a named, org-scoped GitHub credential (token sealed at rest).
type GitHubConnection struct {
	ID             uuid.UUID  `json:"id"`
	OrgID          uuid.UUID  `json:"org_id"`
	Name           string     `json:"name"`
	Repo           string     `json:"repo"`
	DefaultBranch  string     `json:"default_branch"`
	ConnectedBy    *uuid.UUID `json:"connected_by,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	AccessTokenEnc []byte     `json:"-"`
}

// ConnectionPublic is the API shape (never includes the token).
type ConnectionPublic struct {
	ID            uuid.UUID  `json:"id"`
	Name          string     `json:"name"`
	Repo          string     `json:"repo"`
	DefaultBranch string     `json:"default_branch"`
	ConnectedBy   *uuid.UUID `json:"connected_by,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

// Public strips sealed material.
func (c *GitHubConnection) Public() ConnectionPublic {
	return ConnectionPublic{
		ID: c.ID, Name: c.Name, Repo: c.Repo, DefaultBranch: c.DefaultBranch,
		ConnectedBy: c.ConnectedBy, CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt,
	}
}

// ConnStore persists named GitHub connections.
type ConnStore struct {
	pool *pgxpool.Pool
}

// NewConnStore constructs a Postgres-backed connection store.
func NewConnStore(pool *pgxpool.Pool) *ConnStore {
	return &ConnStore{pool: pool}
}

// Upsert inserts or updates a named connection for the org.
func (s *ConnStore) Upsert(ctx context.Context, c *GitHubConnection) error {
	if c.ID == uuid.Nil {
		c.ID = uuid.New()
	}
	if c.DefaultBranch == "" {
		c.DefaultBranch = "main"
	}
	return s.pool.QueryRow(ctx, `
		INSERT INTO seo_github_connections (
			id, org_id, name, repo, default_branch, access_token_enc, connected_by
		) VALUES ($1,$2,$3,$4,$5,$6,$7)
		ON CONFLICT (org_id, name) DO UPDATE SET
			repo = EXCLUDED.repo,
			default_branch = EXCLUDED.default_branch,
			access_token_enc = EXCLUDED.access_token_enc,
			connected_by = COALESCE(EXCLUDED.connected_by, seo_github_connections.connected_by),
			updated_at = NOW()
		RETURNING id, created_at, updated_at`,
		c.ID, c.OrgID, c.Name, c.Repo, c.DefaultBranch, c.AccessTokenEnc, c.ConnectedBy,
	).Scan(&c.ID, &c.CreatedAt, &c.UpdatedAt)
}

// GetByName loads a connection by org + name.
func (s *ConnStore) GetByName(ctx context.Context, orgID uuid.UUID, name string) (*GitHubConnection, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, org_id, name, repo, default_branch, access_token_enc, connected_by, created_at, updated_at
		FROM seo_github_connections WHERE org_id = $1 AND name = $2`, orgID, name)
	return scanConn(row)
}

// ListByOrg returns connections for an org (newest first).
func (s *ConnStore) ListByOrg(ctx context.Context, orgID uuid.UUID) ([]GitHubConnection, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, org_id, name, repo, default_branch, access_token_enc, connected_by, created_at, updated_at
		FROM seo_github_connections WHERE org_id = $1 ORDER BY name`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []GitHubConnection
	for rows.Next() {
		c, err := scanConn(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *c)
	}
	return out, rows.Err()
}

// Delete removes a named connection.
func (s *ConnStore) Delete(ctx context.Context, orgID uuid.UUID, name string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM seo_github_connections WHERE org_id=$1 AND name=$2`, orgID, name)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("connection not found")
	}
	return nil
}

func scanConn(row pgx.Row) (*GitHubConnection, error) {
	c := &GitHubConnection{}
	if err := row.Scan(
		&c.ID, &c.OrgID, &c.Name, &c.Repo, &c.DefaultBranch, &c.AccessTokenEnc,
		&c.ConnectedBy, &c.CreatedAt, &c.UpdatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("github connection not found")
		}
		return nil, err
	}
	return c, nil
}

// SealToken encrypts a PAT with the configured key material.
func SealToken(tokenKey, token string) ([]byte, error) {
	key, err := mail.KeyFromSecret(tokenKey)
	if err != nil {
		return nil, err
	}
	return mail.Encrypt(key, []byte(token))
}

// OpenToken decrypts a sealed PAT.
func OpenToken(tokenKey string, enc []byte) (string, error) {
	key, err := mail.KeyFromSecret(tokenKey)
	if err != nil {
		return "", err
	}
	plain, err := mail.Decrypt(key, enc)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}
