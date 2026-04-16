package handler

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"

	"github.com/jobshout/server/internal/middleware"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/service"
)

// RBACHandler exposes role and permission management endpoints.
type RBACHandler struct {
	svc      service.RBACService
	validate *validator.Validate
}

// NewRBACHandler creates a RBACHandler.
func NewRBACHandler(svc service.RBACService) *RBACHandler {
	return &RBACHandler{svc: svc, validate: validator.New()}
}

// ListRoles handles GET /rbac/roles
func (h *RBACHandler) ListRoles(w http.ResponseWriter, r *http.Request) {
	orgID, err := uuid.Parse(middleware.GetOrgID(r.Context()))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid org_id in token")
		return
	}

	roles, err := h.svc.ListRoles(r.Context(), orgID)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to list roles")
		return
	}
	if roles == nil {
		roles = []model.Role{}
	}
	RespondJSON(w, http.StatusOK, roles)
}

// CreateRole handles POST /rbac/roles
func (h *RBACHandler) CreateRole(w http.ResponseWriter, r *http.Request) {
	orgID, err := uuid.Parse(middleware.GetOrgID(r.Context()))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid org_id in token")
		return
	}

	var req model.CreateRoleRequest
	if !DecodeJSON(w, r, &req) {
		return
	}
	if err := h.validate.Struct(req); err != nil {
		RespondError(w, http.StatusBadRequest, "validation failed: "+err.Error())
		return
	}

	role, err := h.svc.CreateRole(r.Context(), orgID, req)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to create role: "+err.Error())
		return
	}
	RespondJSON(w, http.StatusCreated, role)
}

// DeleteRole handles DELETE /rbac/roles/{roleID}
func (h *RBACHandler) DeleteRole(w http.ResponseWriter, r *http.Request) {
	roleID, err := uuid.Parse(chi.URLParam(r, "roleID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid role ID")
		return
	}

	if err := h.svc.DeleteRole(r.Context(), roleID); err != nil {
		RespondError(w, http.StatusBadRequest, err.Error())
		return
	}
	RespondJSON(w, http.StatusNoContent, nil)
}

// AssignRole handles POST /rbac/assignments
func (h *RBACHandler) AssignRole(w http.ResponseWriter, r *http.Request) {
	orgID, err := uuid.Parse(middleware.GetOrgID(r.Context()))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid org_id in token")
		return
	}
	grantedBy, err := uuid.Parse(middleware.GetUserID(r.Context()))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid user_id in token")
		return
	}

	var req model.AssignRoleRequest
	if !DecodeJSON(w, r, &req) {
		return
	}
	if err := h.validate.Struct(req); err != nil {
		RespondError(w, http.StatusBadRequest, "validation failed: "+err.Error())
		return
	}

	if err := h.svc.AssignRole(r.Context(), orgID, grantedBy, req); err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to assign role: "+err.Error())
		return
	}
	RespondJSON(w, http.StatusOK, map[string]string{"status": "assigned"})
}

// RemoveRole handles DELETE /rbac/assignments/{userID}/{roleID}
func (h *RBACHandler) RemoveRole(w http.ResponseWriter, r *http.Request) {
	orgID, err := uuid.Parse(middleware.GetOrgID(r.Context()))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid org_id in token")
		return
	}
	userID, err := uuid.Parse(chi.URLParam(r, "userID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid user ID")
		return
	}
	roleID, err := uuid.Parse(chi.URLParam(r, "roleID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid role ID")
		return
	}

	if err := h.svc.RemoveRole(r.Context(), userID, roleID, orgID); err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to remove role: "+err.Error())
		return
	}
	RespondJSON(w, http.StatusNoContent, nil)
}

// ListUserRoles handles GET /rbac/users/{userID}/roles
func (h *RBACHandler) ListUserRoles(w http.ResponseWriter, r *http.Request) {
	orgID, err := uuid.Parse(middleware.GetOrgID(r.Context()))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid org_id in token")
		return
	}
	userID, err := uuid.Parse(chi.URLParam(r, "userID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid user ID")
		return
	}

	roles, err := h.svc.ListUserRoles(r.Context(), userID, orgID)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to list user roles")
		return
	}
	if roles == nil {
		roles = []model.Role{}
	}
	RespondJSON(w, http.StatusOK, roles)
}

// MyPermissions handles GET /rbac/me/permissions
func (h *RBACHandler) MyPermissions(w http.ResponseWriter, r *http.Request) {
	orgID, err := uuid.Parse(middleware.GetOrgID(r.Context()))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid org_id in token")
		return
	}
	userID, err := uuid.Parse(middleware.GetUserID(r.Context()))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid user_id in token")
		return
	}

	perms, err := h.svc.UserPermissions(r.Context(), userID, orgID)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to get permissions")
		return
	}
	if perms == nil {
		perms = []string{}
	}
	RespondJSON(w, http.StatusOK, map[string]any{"permissions": perms})
}

// parseLimit extracts a limit query param with a default.
func parseLimit(r *http.Request, defaultLimit int) int {
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return defaultLimit
}
