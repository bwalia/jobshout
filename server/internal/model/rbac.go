package model

import (
	"time"

	"github.com/google/uuid"
)

// ─── RBAC ───────────────────────────────────────────────────────────────────

// Role defines a named set of permissions within an organization.
type Role struct {
	ID          uuid.UUID `json:"id"`
	OrgID       uuid.UUID `json:"org_id"`
	Name        string    `json:"name"`
	Description *string   `json:"description"`
	Permissions []string  `json:"permissions"`
	IsSystem    bool      `json:"is_system"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// UserRole is a many-to-many link between users and roles.
type UserRole struct {
	ID        uuid.UUID  `json:"id"`
	UserID    uuid.UUID  `json:"user_id"`
	RoleID    uuid.UUID  `json:"role_id"`
	OrgID     uuid.UUID  `json:"org_id"`
	GrantedBy *uuid.UUID `json:"granted_by"`
	GrantedAt time.Time  `json:"granted_at"`
}

// System role names.
const (
	RoleAdmin    = "admin"
	RoleOperator = "operator"
	RoleViewer   = "viewer"
	RoleFinance  = "finance"
)

// Permission constants — granular actions.
const (
	PermAgentsCreate    = "agents:create"
	PermAgentsRead      = "agents:read"
	PermAgentsUpdate    = "agents:update"
	PermAgentsDelete    = "agents:delete"
	PermAgentsExecute   = "agents:execute"
	PermTasksCreate     = "tasks:create"
	PermTasksRead       = "tasks:read"
	PermTasksUpdate     = "tasks:update"
	PermTasksDelete     = "tasks:delete"
	PermProjectsCreate  = "projects:create"
	PermProjectsRead    = "projects:read"
	PermProjectsUpdate  = "projects:update"
	PermProjectsDelete  = "projects:delete"
	PermWorkflowsCreate = "workflows:create"
	PermWorkflowsRead   = "workflows:read"
	PermWorkflowsUpdate = "workflows:update"
	PermWorkflowsDelete = "workflows:delete"
	PermWorkflowsExec   = "workflows:execute"
	PermBudgetsRead     = "budgets:read"
	PermBudgetsManage   = "budgets:manage"
	PermPoliciesRead    = "policies:read"
	PermPoliciesManage  = "policies:manage"
	PermAnalyticsRead   = "analytics:read"
	PermCostRead        = "cost:read"
	PermAuditRead       = "audit:read"
	PermUsersManage     = "users:manage"
	PermRolesManage     = "roles:manage"
	PermSSOManage       = "sso:manage"
	PermOrgManage       = "org:manage"
)

// SystemRolePermissions maps system roles to their permissions.
var SystemRolePermissions = map[string][]string{
	RoleAdmin: {
		PermAgentsCreate, PermAgentsRead, PermAgentsUpdate, PermAgentsDelete, PermAgentsExecute,
		PermTasksCreate, PermTasksRead, PermTasksUpdate, PermTasksDelete,
		PermProjectsCreate, PermProjectsRead, PermProjectsUpdate, PermProjectsDelete,
		PermWorkflowsCreate, PermWorkflowsRead, PermWorkflowsUpdate, PermWorkflowsDelete, PermWorkflowsExec,
		PermBudgetsRead, PermBudgetsManage, PermPoliciesRead, PermPoliciesManage,
		PermAnalyticsRead, PermCostRead, PermAuditRead,
		PermUsersManage, PermRolesManage, PermSSOManage, PermOrgManage,
	},
	RoleOperator: {
		PermAgentsCreate, PermAgentsRead, PermAgentsUpdate, PermAgentsExecute,
		PermTasksCreate, PermTasksRead, PermTasksUpdate,
		PermProjectsCreate, PermProjectsRead, PermProjectsUpdate,
		PermWorkflowsCreate, PermWorkflowsRead, PermWorkflowsUpdate, PermWorkflowsExec,
		PermAnalyticsRead,
	},
	RoleViewer: {
		PermAgentsRead, PermTasksRead, PermProjectsRead, PermWorkflowsRead,
		PermAnalyticsRead,
	},
	RoleFinance: {
		PermBudgetsRead, PermBudgetsManage,
		PermAnalyticsRead, PermCostRead, PermAuditRead,
		PermPoliciesRead,
	},
}

// ─── Requests ───────────────────────────────────────────────────────────────

// CreateRoleRequest creates a custom role.
type CreateRoleRequest struct {
	Name        string   `json:"name" validate:"required,min=2,max=50"`
	Description *string  `json:"description"`
	Permissions []string `json:"permissions" validate:"required,min=1"`
}

// AssignRoleRequest assigns a role to a user.
type AssignRoleRequest struct {
	UserID uuid.UUID `json:"user_id" validate:"required"`
	RoleID uuid.UUID `json:"role_id" validate:"required"`
}
