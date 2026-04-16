package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/repository"
)

// RBACService manages roles and permission checks.
type RBACService interface {
	// UserHasPermission checks whether a user has a specific permission in the org.
	UserHasPermission(ctx context.Context, userID, orgID uuid.UUID, permission string) (bool, error)
	// UserPermissions returns all permissions the user holds in the org.
	UserPermissions(ctx context.Context, userID, orgID uuid.UUID) ([]string, error)
	CreateRole(ctx context.Context, orgID uuid.UUID, req model.CreateRoleRequest) (*model.Role, error)
	ListRoles(ctx context.Context, orgID uuid.UUID) ([]model.Role, error)
	DeleteRole(ctx context.Context, id uuid.UUID) error
	AssignRole(ctx context.Context, orgID uuid.UUID, grantedBy uuid.UUID, req model.AssignRoleRequest) error
	RemoveRole(ctx context.Context, userID, roleID, orgID uuid.UUID) error
	ListUserRoles(ctx context.Context, userID, orgID uuid.UUID) ([]model.Role, error)
	EnsureSystemRoles(ctx context.Context, orgID uuid.UUID) error
}

type rbacService struct {
	repo   repository.RBACRepository
	logger *zap.Logger
}

// NewRBACService creates an RBACService.
func NewRBACService(repo repository.RBACRepository, logger *zap.Logger) RBACService {
	return &rbacService{repo: repo, logger: logger}
}

func (s *rbacService) UserHasPermission(ctx context.Context, userID, orgID uuid.UUID, permission string) (bool, error) {
	roles, err := s.repo.ListUserRoles(ctx, userID, orgID)
	if err != nil {
		return false, err
	}
	for _, role := range roles {
		for _, perm := range role.Permissions {
			if perm == permission {
				return true, nil
			}
		}
	}
	return false, nil
}

func (s *rbacService) UserPermissions(ctx context.Context, userID, orgID uuid.UUID) ([]string, error) {
	roles, err := s.repo.ListUserRoles(ctx, userID, orgID)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var perms []string
	for _, role := range roles {
		for _, perm := range role.Permissions {
			if !seen[perm] {
				seen[perm] = true
				perms = append(perms, perm)
			}
		}
	}
	return perms, nil
}

func (s *rbacService) CreateRole(ctx context.Context, orgID uuid.UUID, req model.CreateRoleRequest) (*model.Role, error) {
	role := &model.Role{
		OrgID:       orgID,
		Name:        req.Name,
		Description: req.Description,
		Permissions: req.Permissions,
		IsSystem:    false,
	}
	return s.repo.CreateRole(ctx, role)
}

func (s *rbacService) ListRoles(ctx context.Context, orgID uuid.UUID) ([]model.Role, error) {
	return s.repo.ListRoles(ctx, orgID)
}

func (s *rbacService) DeleteRole(ctx context.Context, id uuid.UUID) error {
	role, err := s.repo.GetRole(ctx, id)
	if err != nil {
		return err
	}
	if role != nil && role.IsSystem {
		return fmt.Errorf("cannot delete system role")
	}
	return s.repo.DeleteRole(ctx, id)
}

func (s *rbacService) AssignRole(ctx context.Context, orgID, grantedBy uuid.UUID, req model.AssignRoleRequest) error {
	ur := &model.UserRole{
		UserID:    req.UserID,
		RoleID:    req.RoleID,
		OrgID:     orgID,
		GrantedBy: &grantedBy,
	}
	return s.repo.AssignRole(ctx, ur)
}

func (s *rbacService) RemoveRole(ctx context.Context, userID, roleID, orgID uuid.UUID) error {
	return s.repo.RemoveRole(ctx, userID, roleID, orgID)
}

func (s *rbacService) ListUserRoles(ctx context.Context, userID, orgID uuid.UUID) ([]model.Role, error) {
	return s.repo.ListUserRoles(ctx, userID, orgID)
}

func (s *rbacService) EnsureSystemRoles(ctx context.Context, orgID uuid.UUID) error {
	return s.repo.EnsureSystemRoles(ctx, orgID)
}
