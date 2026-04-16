package model

import (
	"time"

	"github.com/google/uuid"
)

// SSOConfig holds OIDC configuration for a provider bound to an org.
type SSOConfig struct {
	ID            uuid.UUID      `json:"id"`
	OrgID         uuid.UUID      `json:"org_id"`
	Provider      string         `json:"provider"`
	ClientID      string         `json:"client_id"`
	ClientSecret  string         `json:"-"`
	IssuerURL     string         `json:"issuer_url"`
	RedirectURL   string         `json:"redirect_url"`
	Scopes        []string       `json:"scopes"`
	AutoProvision bool           `json:"auto_provision"`
	DefaultRole   string         `json:"default_role"`
	DomainFilter  *string        `json:"domain_filter"`
	Enabled       bool           `json:"enabled"`
	Metadata      map[string]any `json:"metadata"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
}

// SSO provider constants.
const (
	SSOProviderAzureAD = "azure_ad"
	SSOProviderOkta    = "okta"
	SSOProviderGoogle  = "google"
)

// LoginAuditLog records authentication events.
type LoginAuditLog struct {
	ID        uuid.UUID      `json:"id"`
	UserID    *uuid.UUID     `json:"user_id"`
	OrgID     *uuid.UUID     `json:"org_id"`
	Email     string         `json:"email"`
	Provider  string         `json:"provider"`
	IPAddress *string        `json:"ip_address"`
	UserAgent *string        `json:"user_agent"`
	Status    string         `json:"status"`
	ErrorMsg  *string        `json:"error_message"`
	Metadata  map[string]any `json:"metadata"`
	CreatedAt time.Time      `json:"created_at"`
}

// Login audit statuses.
const (
	LoginStatusSuccess = "success"
	LoginStatusFailed  = "failed"
	LoginStatusBlocked = "blocked"
)

// ─── Requests ───────────────────────────────────────────────────────────────

// CreateSSOConfigRequest creates an SSO config for an org.
type CreateSSOConfigRequest struct {
	Provider      string   `json:"provider" validate:"required,oneof=azure_ad okta google"`
	ClientID      string   `json:"client_id" validate:"required"`
	ClientSecret  string   `json:"client_secret" validate:"required"`
	IssuerURL     string   `json:"issuer_url" validate:"required,url"`
	RedirectURL   string   `json:"redirect_url" validate:"required,url"`
	Scopes        []string `json:"scopes"`
	AutoProvision *bool    `json:"auto_provision"`
	DefaultRole   string   `json:"default_role" validate:"omitempty,oneof=admin operator viewer finance"`
	DomainFilter  *string  `json:"domain_filter"`
}

// SSOLoginRequest initiates an SSO login.
type SSOLoginRequest struct {
	Provider string `json:"provider" validate:"required"`
	OrgSlug  string `json:"org_slug" validate:"required"`
}

// SSOCallbackRequest completes an SSO login.
type SSOCallbackRequest struct {
	Code  string `json:"code" validate:"required"`
	State string `json:"state" validate:"required"`
}
