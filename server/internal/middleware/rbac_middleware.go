package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/google/uuid"
)

// RBACService is the interface required by the RBAC middleware.
type RBACService interface {
	UserHasPermission(ctx context.Context, userID, orgID uuid.UUID, permission string) (bool, error)
}

// RequirePermission returns middleware that checks the authenticated user has the given permission.
func RequirePermission(rbacSvc RBACService, permission string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userIDStr := GetUserID(r.Context())
			orgIDStr := GetOrgID(r.Context())

			userID, err := uuid.Parse(userIDStr)
			if err != nil {
				respondForbidden(w, "invalid user identity")
				return
			}
			orgID, err := uuid.Parse(orgIDStr)
			if err != nil {
				respondForbidden(w, "invalid org identity")
				return
			}

			allowed, err := rbacSvc.UserHasPermission(r.Context(), userID, orgID, permission)
			if err != nil {
				respondForbidden(w, "permission check failed")
				return
			}
			if !allowed {
				respondForbidden(w, "insufficient permissions: "+permission)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// RequireAnyPermission checks that the user has at least one of the given permissions.
func RequireAnyPermission(rbacSvc RBACService, permissions ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userIDStr := GetUserID(r.Context())
			orgIDStr := GetOrgID(r.Context())

			userID, err := uuid.Parse(userIDStr)
			if err != nil {
				respondForbidden(w, "invalid user identity")
				return
			}
			orgID, err := uuid.Parse(orgIDStr)
			if err != nil {
				respondForbidden(w, "invalid org identity")
				return
			}

			for _, perm := range permissions {
				allowed, err := rbacSvc.UserHasPermission(r.Context(), userID, orgID, perm)
				if err == nil && allowed {
					next.ServeHTTP(w, r)
					return
				}
			}

			respondForbidden(w, "insufficient permissions: "+strings.Join(permissions, " or "))
		})
	}
}

func respondForbidden(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusForbidden)
	w.Write([]byte(`{"error":"` + msg + `"}`))
}
