package model

import (
	"time"

	"github.com/google/uuid"
)

// Transport constants for MCP servers. Only the Streamable HTTP transport is
// supported today; the CHECK constraint in the migration enforces this.
const (
	MCPTransportHTTP = "http"
)

// MCPServer represents a Model Context Protocol server an organization has
// configured. Agents connect to the org's enabled servers at execution time to
// discover and invoke their tools.
type MCPServer struct {
	ID         uuid.UUID  `json:"id"`
	OrgID      uuid.UUID  `json:"org_id"`
	Name       string     `json:"name"`
	Transport  string     `json:"transport"`
	URL        string     `json:"url"`
	AuthHeader string     `json:"auth_header,omitempty"`
	Enabled    bool       `json:"enabled"`
	CreatedBy  *uuid.UUID `json:"created_by"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

// CreateMCPServerRequest is the payload for registering an MCP server.
type CreateMCPServerRequest struct {
	Name       string `json:"name" validate:"required"`
	Transport  string `json:"transport" validate:"omitempty,oneof=http"`
	URL        string `json:"url" validate:"required,url"`
	AuthHeader string `json:"auth_header"`
}

// UpdateMCPServerRequest is the payload for updating an MCP server.
type UpdateMCPServerRequest struct {
	Name       *string `json:"name,omitempty"`
	URL        *string `json:"url,omitempty"`
	AuthHeader *string `json:"auth_header,omitempty"`
	Enabled    *bool   `json:"enabled,omitempty"`
}
