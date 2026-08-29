package handler

import (
	"net/http"

	"github.com/jobshout/server/internal/agentschema"
)

// AgentSchemaHandler exposes the agent input contract.
//
// The Task Manager keeps its own copy of this contract in TypeScript
// (web/nextjs/lib/agents/input-schemas.ts) because it renders the form before
// any request is made. This endpoint is what makes that copy checkable: a
// contract test compares the two, so drift becomes a red test rather than an
// agent that behaves differently depending on which surface launched it.
type AgentSchemaHandler struct{}

func NewAgentSchemaHandler() *AgentSchemaHandler { return &AgentSchemaHandler{} }

// List handles GET /api/v1/agent-schemas.
func (h *AgentSchemaHandler) List(w http.ResponseWriter, r *http.Request) {
	RespondJSON(w, http.StatusOK, agentschema.All())
}
