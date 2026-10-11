package model

import (
	"time"

	"github.com/google/uuid"
)

// OrgLLMRouting is an org's model-routing settings (table org_llm_routing).
// Absent row = route through every active provider config of the org, the
// default one first. Enabled=false = ignore the org's configs and use the
// server's env providers.
type OrgLLMRouting struct {
	OrgID     uuid.UUID      `json:"org_id"`
	Enabled   bool           `json:"enabled"`
	Chain     []LLMRouteStep `json:"chain"`
	UpdatedBy *uuid.UUID     `json:"updated_by,omitempty"`
	UpdatedAt time.Time      `json:"updated_at"`
}

// LLMRouteStep is one hop of an org's ordered fallback chain: a provider
// config of the org and, optionally, the model to ask it for (empty = the
// config's default_model).
type LLMRouteStep struct {
	ProviderConfigID uuid.UUID `json:"provider_config_id" validate:"required"`
	Model            string    `json:"model,omitempty"`
}

// PutOrgLLMRoutingRequest is the body of PUT /api/v1/llm-routing.
type PutOrgLLMRoutingRequest struct {
	Enabled *bool          `json:"enabled"`
	Chain   []LLMRouteStep `json:"chain" validate:"dive"`
}
