package repository

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jobshout/server/internal/model"
)

type MCPRepository interface {
	Create(ctx context.Context, m *model.MCPServer) error
	FindByID(ctx context.Context, id uuid.UUID) (*model.MCPServer, error)
	ListByOrg(ctx context.Context, orgID uuid.UUID) ([]model.MCPServer, error)
	Update(ctx context.Context, m *model.MCPServer) error
	Delete(ctx context.Context, id uuid.UUID) error
}

type mcpRepository struct {
	pool *pgxpool.Pool
}

func NewMCPRepository(pool *pgxpool.Pool) MCPRepository {
	return &mcpRepository{pool: pool}
}

func (r *mcpRepository) Create(ctx context.Context, m *model.MCPServer) error {
	m.ID = uuid.New()
	m.CreatedAt = time.Now()
	m.UpdatedAt = m.CreatedAt
	if m.Transport == "" {
		m.Transport = model.MCPTransportHTTP
	}

	_, err := r.pool.Exec(ctx, `
		INSERT INTO mcp_servers (id, org_id, name, transport, url, auth_header, enabled, created_by, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		m.ID, m.OrgID, m.Name, m.Transport, m.URL, nullableString(m.AuthHeader), m.Enabled, m.CreatedBy, m.CreatedAt, m.UpdatedAt,
	)
	return err
}

func (r *mcpRepository) FindByID(ctx context.Context, id uuid.UUID) (*model.MCPServer, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, org_id, name, transport, url, auth_header, enabled, created_by, created_at, updated_at
		FROM mcp_servers WHERE id = $1`, id)
	return scanMCPServer(row)
}

func (r *mcpRepository) ListByOrg(ctx context.Context, orgID uuid.UUID) ([]model.MCPServer, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, org_id, name, transport, url, auth_header, enabled, created_by, created_at, updated_at
		FROM mcp_servers WHERE org_id = $1 ORDER BY created_at DESC`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []model.MCPServer
	for rows.Next() {
		m, err := scanMCPServer(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *m)
	}
	return items, nil
}

func (r *mcpRepository) Update(ctx context.Context, m *model.MCPServer) error {
	m.UpdatedAt = time.Now()
	_, err := r.pool.Exec(ctx, `
		UPDATE mcp_servers SET name=$1, url=$2, auth_header=$3, enabled=$4, updated_at=$5
		WHERE id=$6`,
		m.Name, m.URL, nullableString(m.AuthHeader), m.Enabled, m.UpdatedAt, m.ID,
	)
	return err
}

func (r *mcpRepository) Delete(ctx context.Context, id uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM mcp_servers WHERE id = $1`, id)
	return err
}

func scanMCPServer(s scannable) (*model.MCPServer, error) {
	var m model.MCPServer
	var authHeader *string

	err := s.Scan(&m.ID, &m.OrgID, &m.Name, &m.Transport, &m.URL, &authHeader, &m.Enabled, &m.CreatedBy, &m.CreatedAt, &m.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	if authHeader != nil {
		m.AuthHeader = *authHeader
	}
	return &m, nil
}

// nullableString maps an empty string to a SQL NULL so the nullable auth_header
// column stays NULL rather than an empty string when no header is configured.
func nullableString(s string) any {
	if s == "" {
		return nil
	}
	return s
}
