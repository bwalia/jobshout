package handler

import (
	"net/http"

	"github.com/jobshout/server/internal/agentschema"
)

// AgentSchemaHandler exposes the agent input contract so the Task Manager
// TypeScript copy can be checked against the server.
type AgentSchemaHandler struct{}

func NewAgentSchemaHandler() *AgentSchemaHandler { return &AgentSchemaHandler{} }

// List handles GET /api/v1/agent-schemas.
func (h *AgentSchemaHandler) List(w http.ResponseWriter, r *http.Request) {
	RespondJSON(w, http.StatusOK, agentschema.All())
}
