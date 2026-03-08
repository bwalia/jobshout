package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/jobshout/server/internal/service"
)

// RequireAuth returns middleware that validates JWT access tokens
// and injects user claims into the request context.
func RequireAuth(jwtSvc service.JWTService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tokenStr := extractBearerToken(r)
			if tokenStr == "" {
				respondUnauthorized(w, "missing authorization token")
				return
			}

			claims, err := jwtSvc.ParseAccessToken(tokenStr)
			if err != nil {
				respondUnauthorized(w, "invalid or expired token")
				return
			}

			ctx := r.Context()
			ctx = context.WithValue(ctx, ContextKeyUserID, claims.UserID)
			ctx = context.WithValue(ctx, ContextKeyEmail, claims.Email)
			ctx = context.WithValue(ctx, ContextKeyOrgID, claims.OrgID)
			ctx = context.WithValue(ctx, ContextKeyRole, claims.Role)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// GetUserID extracts the user ID from the request context.
func GetUserID(ctx context.Context) string {
	val, _ := ctx.Value(ContextKeyUserID).(string)
	return val
}

// GetOrgID extracts the organization ID from the request context.
func GetOrgID(ctx context.Context) string {
	val, _ := ctx.Value(ContextKeyOrgID).(string)
	return val
}

func extractBearerToken(r *http.Request) string {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return ""
	}
	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
		return ""
	}
	return parts[1]
}

// respondUnauthorized writes a 401 JSON error without importing the handler package.
func respondUnauthorized(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
