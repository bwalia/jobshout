package model

import (
	"time"

	"github.com/google/uuid"
)

// API key scopes. A key may call only the routes its scopes cover (see
// middleware.APIKeyRouteScope); every other route rejects API keys.
const (
	ScopeRuns       = "runs"        // launch/run tasks, read runs
	ScopeApprovals  = "approvals"   // list/get/decide approvals
	ScopeAgentsRead = "agents:read" // list/get agents and agent schemas
	ScopeKnowledge  = "knowledge"   // agent knowledge CRUD
)

// AllAPIKeyScopes lists every valid scope.
var AllAPIKeyScopes = []string{ScopeRuns, ScopeApprovals, ScopeAgentsRead, ScopeKnowledge}

// APIKeyPrefix starts every key, so the auth middleware can tell a key from a JWT.
const APIKeyPrefix = "jsk_"

// APIKey is an org-scoped service credential (table org_api_keys). The secret
// itself is never stored or returned after creation.
type APIKey struct {
	ID         uuid.UUID  `json:"id"`
	OrgID      uuid.UUID  `json:"org_id"`
	Name       string     `json:"name"`
	Prefix     string     `json:"prefix"`
	Scopes     []string   `json:"scopes"`
	CreatedBy  *uuid.UUID `json:"created_by"`
	LastUsedAt *time.Time `json:"last_used_at"`
	ExpiresAt  *time.Time `json:"expires_at"`
	RevokedAt  *time.Time `json:"revoked_at"`
	CreatedAt  time.Time  `json:"created_at"`
}

// CreateAPIKeyRequest is the body of POST /api/v1/api-keys.
type CreateAPIKeyRequest struct {
	Name      string     `json:"name" validate:"required,min=2,max=100"`
	Scopes    []string   `json:"scopes" validate:"required,min=1,dive,oneof=runs approvals agents:read knowledge"`
	ExpiresAt *time.Time `json:"expires_at"`
}

// CreatedAPIKey is the one response that carries the full key.
type CreatedAPIKey struct {
	APIKey
	Key string `json:"key"`
}

// APIKeyPrincipal is the identity an authenticated API key acts as.
type APIKeyPrincipal struct {
	KeyID  uuid.UUID
	OrgID  uuid.UUID
	UserID uuid.UUID // the key's creator; requests act as this user
	Scopes []string
}

// HasScope reports whether the principal holds scope.
func (p *APIKeyPrincipal) HasScope(scope string) bool {
	for _, s := range p.Scopes {
		if s == scope {
			return true
		}
	}
	return false
}
