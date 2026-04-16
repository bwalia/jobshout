package repository

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jobshout/server/internal/model"
)

// RBACRepository handles role and user_role persistence.
type RBACRepository interface {
	CreateRole(ctx context.Context, role *model.Role) (*model.Role, error)
	GetRole(ctx context.Context, id uuid.UUID) (*model.Role, error)
	GetRoleByName(ctx context.Context, orgID uuid.UUID, name string) (*model.Role, error)
	ListRoles(ctx context.Context, orgID uuid.UUID) ([]model.Role, error)
	DeleteRole(ctx context.Context, id uuid.UUID) error
	AssignRole(ctx context.Context, userRole *model.UserRole) error
	RemoveRole(ctx context.Context, userID, roleID, orgID uuid.UUID) error
	ListUserRoles(ctx context.Context, userID, orgID uuid.UUID) ([]model.Role, error)
	EnsureSystemRoles(ctx context.Context, orgID uuid.UUID) error
}

type rbacRepository struct {
	pool *pgxpool.Pool
}

// NewRBACRepository creates an RBACRepository.
func NewRBACRepository(pool *pgxpool.Pool) RBACRepository {
	return &rbacRepository{pool: pool}
}

func (r *rbacRepository) CreateRole(ctx context.Context, role *model.Role) (*model.Role, error) {
	const sql = `
		INSERT INTO roles (org_id, name, description, permissions, is_system)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, org_id, name, description, permissions, is_system, created_at, updated_at`

	out := &model.Role{}
	if err := r.pool.QueryRow(ctx, sql,
		role.OrgID, role.Name, role.Description, role.Permissions, role.IsSystem,
	).Scan(&out.ID, &out.OrgID, &out.Name, &out.Description, &out.Permissions,
		&out.IsSystem, &out.CreatedAt, &out.UpdatedAt,
	); err != nil {
		return nil, fmt.Errorf("rbac_repo: create role: %w", err)
	}
	return out, nil
}

func (r *rbacRepository) GetRole(ctx context.Context, id uuid.UUID) (*model.Role, error) {
	const sql = `
		SELECT id, org_id, name, description, permissions, is_system, created_at, updated_at
		FROM roles WHERE id = $1`

	out := &model.Role{}
	if err := r.pool.QueryRow(ctx, sql, id).Scan(
		&out.ID, &out.OrgID, &out.Name, &out.Description, &out.Permissions,
		&out.IsSystem, &out.CreatedAt, &out.UpdatedAt,
	); err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("rbac_repo: get role: %w", err)
	}
	return out, nil
}

func (r *rbacRepository) GetRoleByName(ctx context.Context, orgID uuid.UUID, name string) (*model.Role, error) {
	const sql = `
		SELECT id, org_id, name, description, permissions, is_system, created_at, updated_at
		FROM roles WHERE org_id = $1 AND name = $2`

	out := &model.Role{}
	if err := r.pool.QueryRow(ctx, sql, orgID, name).Scan(
		&out.ID, &out.OrgID, &out.Name, &out.Description, &out.Permissions,
		&out.IsSystem, &out.CreatedAt, &out.UpdatedAt,
	); err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("rbac_repo: get role by name: %w", err)
	}
	return out, nil
}

func (r *rbacRepository) ListRoles(ctx context.Context, orgID uuid.UUID) ([]model.Role, error) {
	const sql = `
		SELECT id, org_id, name, description, permissions, is_system, created_at, updated_at
		FROM roles WHERE org_id = $1 ORDER BY is_system DESC, name`

	rows, err := r.pool.Query(ctx, sql, orgID)
	if err != nil {
		return nil, fmt.Errorf("rbac_repo: list roles: %w", err)
	}
	defer rows.Close()

	var roles []model.Role
	for rows.Next() {
		var role model.Role
		if err := rows.Scan(&role.ID, &role.OrgID, &role.Name, &role.Description,
			&role.Permissions, &role.IsSystem, &role.CreatedAt, &role.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("rbac_repo: scan role: %w", err)
		}
		roles = append(roles, role)
	}
	return roles, rows.Err()
}

func (r *rbacRepository) DeleteRole(ctx context.Context, id uuid.UUID) error {
	const sql = `DELETE FROM roles WHERE id = $1 AND is_system = false`
	_, err := r.pool.Exec(ctx, sql, id)
	return err
}

func (r *rbacRepository) AssignRole(ctx context.Context, ur *model.UserRole) error {
	const sql = `
		INSERT INTO user_roles (user_id, role_id, org_id, granted_by)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (user_id, role_id, org_id) DO NOTHING`

	_, err := r.pool.Exec(ctx, sql, ur.UserID, ur.RoleID, ur.OrgID, ur.GrantedBy)
	if err != nil {
		return fmt.Errorf("rbac_repo: assign role: %w", err)
	}
	return nil
}

func (r *rbacRepository) RemoveRole(ctx context.Context, userID, roleID, orgID uuid.UUID) error {
	const sql = `DELETE FROM user_roles WHERE user_id = $1 AND role_id = $2 AND org_id = $3`
	_, err := r.pool.Exec(ctx, sql, userID, roleID, orgID)
	return err
}

func (r *rbacRepository) ListUserRoles(ctx context.Context, userID, orgID uuid.UUID) ([]model.Role, error) {
	const sql = `
		SELECT r.id, r.org_id, r.name, r.description, r.permissions, r.is_system, r.created_at, r.updated_at
		FROM roles r
		JOIN user_roles ur ON ur.role_id = r.id
		WHERE ur.user_id = $1 AND ur.org_id = $2
		ORDER BY r.name`

	rows, err := r.pool.Query(ctx, sql, userID, orgID)
	if err != nil {
		return nil, fmt.Errorf("rbac_repo: list user roles: %w", err)
	}
	defer rows.Close()

	var roles []model.Role
	for rows.Next() {
		var role model.Role
		if err := rows.Scan(&role.ID, &role.OrgID, &role.Name, &role.Description,
			&role.Permissions, &role.IsSystem, &role.CreatedAt, &role.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("rbac_repo: scan user role: %w", err)
		}
		roles = append(roles, role)
	}
	return roles, rows.Err()
}

// EnsureSystemRoles creates the four system roles for an org if they don't exist.
func (r *rbacRepository) EnsureSystemRoles(ctx context.Context, orgID uuid.UUID) error {
	for name, perms := range model.SystemRolePermissions {
		const sql = `
			INSERT INTO roles (org_id, name, permissions, is_system)
			VALUES ($1, $2, $3, true)
			ON CONFLICT (org_id, name) DO NOTHING`
		if _, err := r.pool.Exec(ctx, sql, orgID, name, perms); err != nil {
			return fmt.Errorf("rbac_repo: ensure system role %q: %w", name, err)
		}
	}
	return nil
}
