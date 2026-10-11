package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"regexp"
	"strings"

	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/service"
)

// Context keys set for API-key (service principal) requests.
const (
	// ContextKeyAPIKeyID holds the authenticating key's id (string).
	ContextKeyAPIKeyID contextKey = "api_key_id"
	// ContextKeyPrincipal is "user" for a JWT, "api_key" for an API key.
	ContextKeyPrincipal contextKey = "principal"
)

// RoleService is the role placed on the context for API-key requests.
const RoleService = "service"

// APIKeyAuthenticator resolves a raw API key to the principal it acts as.
type APIKeyAuthenticator interface {
	Authenticate(ctx context.Context, raw string) (*model.APIKeyPrincipal, error)
}

// IsAPIKey reports whether the request was authenticated with an API key.
func IsAPIKey(ctx context.Context) bool {
	v, _ := ctx.Value(ContextKeyPrincipal).(string)
	return v == "api_key"
}

// GetAPIKeyID returns the authenticating API key's id, or "".
func GetAPIKeyID(ctx context.Context) string {
	v, _ := ctx.Value(ContextKeyAPIKeyID).(string)
	return v
}

// apiKeyRoute maps a method + path pattern to the scope it needs.
type apiKeyRoute struct {
	methods string // space-separated
	path    *regexp.Regexp
	scope   string
}

const uuidRe = `[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`

func route(methods, path, scope string) apiKeyRoute {
	p := strings.ReplaceAll(path, "{id}", uuidRe)
	return apiKeyRoute{methods: " " + methods + " ", path: regexp.MustCompile(`^/api/v1` + p + `/?$`), scope: scope}
}

// apiKeyRoutes is the complete allow-list of what an API key may call — the
// routes OpsAPI needs. Every other route answers 403 to an API key, so adding
// a route to the server never silently widens what keys can do.
var apiKeyRoutes = []apiKeyRoute{
	route("POST", "/tasks/launch", model.ScopeRuns),
	route("POST", "/tasks/{id}/run", model.ScopeRuns),
	route("GET", "/tasks/{id}/runs", model.ScopeRuns),
	route("GET", "/task-runs/{id}", model.ScopeRuns),
	route("GET", "/approvals", model.ScopeApprovals),
	route("GET", "/approvals/{id}", model.ScopeApprovals),
	route("POST", "/approvals/{id}/decide", model.ScopeApprovals),
	route("GET", "/agents", model.ScopeAgentsRead),
	route("GET", "/agents/{id}", model.ScopeAgentsRead),
	route("GET", "/agent-schemas", model.ScopeAgentsRead),
	route("GET", "/agent-schemas/generic", model.ScopeAgentsRead),
	route("GET POST", "/agents/{id}/knowledge", model.ScopeKnowledge),
	route("GET PUT DELETE", "/agents/{id}/knowledge/{id}", model.ScopeKnowledge),
	route("PUT DELETE", "/agents/{id}/knowledge/external/[A-Za-z0-9][A-Za-z0-9._-]{0,99}", model.ScopeKnowledge),
	route("GET", "/property-pa/agents", model.ScopeAgentsRead),
}

// APIKeyRouteScope returns the scope an API key needs for method+path, and
// false when API keys may not call it at all.
func APIKeyRouteScope(method, path string) (string, bool) {
	for _, rt := range apiKeyRoutes {
		if strings.Contains(rt.methods, " "+method+" ") && rt.path.MatchString(path) {
			return rt.scope, true
		}
	}
	return "", false
}

// RequireAuthOrAPIKey is RequireAuth that additionally accepts an org API key
// ("Authorization: Bearer jsk_...") as a service principal. A key acts in its
// org as the user who created it, may only call the allow-listed routes its
// scopes cover, and gets role "service". keys may be nil (JWT only).
func RequireAuthOrAPIKey(jwtSvc service.JWTService, keys APIKeyAuthenticator) func(http.Handler) http.Handler {
	jwtOnly := RequireAuth(jwtSvc)
	return func(next http.Handler) http.Handler {
		jwtNext := jwtOnly(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := extractBearerToken(r)
			if keys == nil || !strings.HasPrefix(token, model.APIKeyPrefix) {
				jwtNext.ServeHTTP(w, r)
				return
			}
			p, err := keys.Authenticate(r.Context(), token)
			if err != nil || p == nil {
				respondUnauthorized(w, "invalid, revoked or expired api key")
				return
			}
			scope, allowed := APIKeyRouteScope(r.Method, r.URL.Path)
			if !allowed {
				respondJSONStatus(w, http.StatusForbidden, "api keys cannot access this endpoint")
				return
			}
			if !p.HasScope(scope) {
				respondJSONStatus(w, http.StatusForbidden, "api key lacks scope: "+scope)
				return
			}
			ctx := r.Context()
			ctx = context.WithValue(ctx, ContextKeyUserID, p.UserID.String())
			ctx = context.WithValue(ctx, ContextKeyEmail, "")
			ctx = context.WithValue(ctx, ContextKeyOrgID, p.OrgID.String())
			ctx = context.WithValue(ctx, ContextKeyRole, RoleService)
			ctx = context.WithValue(ctx, ContextKeyAPIKeyID, p.KeyID.String())
			ctx = context.WithValue(ctx, ContextKeyPrincipal, "api_key")
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func respondJSONStatus(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
