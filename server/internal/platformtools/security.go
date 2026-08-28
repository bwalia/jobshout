package platformtools

import (
	"context"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/tools"
)

func registerSecurity(reg *Registry, d Deps) {
	if d.RBAC == nil {
		return
	}

	reg.Register(newTool(
		"my_permissions",
		"Report the current user's permissions and roles in this organisation.",
		"security", "", false, true,
		tools.ObjectSchema(map[string]any{}),
		func(ctx context.Context, input map[string]any) (*Result, error) {
			ident := MustIdentity(ctx)
			perms, err := d.RBAC.UserPermissions(ctx, ident.UserID, ident.OrgID)
			if err != nil {
				return nil, err
			}
			roles, err := d.RBAC.ListUserRoles(ctx, ident.UserID, ident.OrgID)
			if err != nil {
				return nil, err
			}
			names := make([]string, 0, len(roles))
			for _, r := range roles {
				names = append(names, r.Name)
			}
			return &Result{Data: map[string]any{"permissions": perms, "roles": names}}, nil
		},
	))

	reg.Register(newTool(
		"role_list",
		"List roles and who holds them.",
		"security", "", false, true,
		tools.ObjectSchema(map[string]any{}),
		func(ctx context.Context, input map[string]any) (*Result, error) {
			ident := MustIdentity(ctx)
			roles, err := d.RBAC.ListRoles(ctx, ident.OrgID)
			if err != nil {
				return nil, err
			}
			type row struct {
				Name        string   `json:"name"`
				Permissions []string `json:"permissions"`
				System      bool     `json:"system"`
			}
			rows := make([]row, 0, len(roles))
			for _, r := range roles {
				rows = append(rows, row{Name: r.Name, Permissions: r.Permissions, System: r.IsSystem})
			}
			return &Result{Data: map[string]any{"roles": rows}}, nil
		},
	))

	reg.Register(newTool(
		"role_assign",
		"Assign a role to a user by user id and role name. Requires confirmation.",
		"security", model.PermRolesManage, true, false,
		tools.ObjectSchema(map[string]any{
			"user_id": map[string]any{"type": "string"},
			"role":    map[string]any{"type": "string"},
		}, "user_id", "role"),
		func(ctx context.Context, input map[string]any) (*Result, error) {
			ident := MustIdentity(ctx)
			uid, err := uuid.Parse(strArg(input, "user_id"))
			if err != nil {
				return &Result{Missing: []string{"user_id"}, Question: "Which user should I assign the role to?"}, nil
			}
			roles, err := d.RBAC.ListRoles(ctx, ident.OrgID)
			if err != nil {
				return nil, err
			}
			m := ByName(roles, strArg(input, "role"), func(r model.Role) string { return r.Name })
			if !m.Found {
				return clarifyFromMatch("role", strArg(input, "role"), "role", m.Candidates, func(r model.Role) string { return r.Name }), nil
			}
			if err := d.RBAC.AssignRole(ctx, ident.OrgID, ident.UserID, model.AssignRoleRequest{UserID: uid, RoleID: m.Exact.ID}); err != nil {
				return nil, err
			}
			return &Result{
				Data:   map[string]any{"role": m.Exact.Name, "assigned": true},
				Effect: "assign the " + m.Exact.Name + " role to that user",
			}, nil
		},
	))
}
