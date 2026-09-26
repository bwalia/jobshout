package handler

import (
	"net/http"

	"github.com/jobshout/server/internal/agentmodule"
	"github.com/jobshout/server/internal/agentschema"
)

// AgentSchemaHandler exposes the agent input contract so the Task Manager
// consumes one schema (GET /api/v1/agent-schemas) instead of a TypeScript copy.
type AgentSchemaHandler struct{}

func NewAgentSchemaHandler() *AgentSchemaHandler { return &AgentSchemaHandler{} }

// List handles GET /api/v1/agent-schemas.
//
// All specialists are wired this way: schema on the module, this API, web
// consumes it. A new agent does not need a TypeScript SCHEMAS map — register it.
func (h *AgentSchemaHandler) List(w http.ResponseWriter, r *http.Request) {
	out := agentschema.All()
	for i, ws := range out {
		m, ok := agentmodule.Lookup(ws.Builtin)
		if !ok {
			continue
		}
		out[i].Label = m.Label
		out[i].Icon = m.Icon
		out[i].TabSlug = m.TabSlug
		out[i].StayOnTab = m.StayOnTab
		if m.PrefillMailbox && out[i].Prefill == "" {
			out[i].Prefill = "mailbox"
		}
	}
	RespondJSON(w, http.StatusOK, out)
}
