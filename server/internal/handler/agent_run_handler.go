package handler

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/jobshout/server/internal/middleware"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/service"
)

// AgentRunHandler is the one front door for running an agent.
//
// Before it existed, "run agent X with inputs Y" was implemented three times —
// a switch in the browser, a switch in the chat tools, and a builtin-unaware
// generic loop — so the same agent could behave differently depending on which
// surface launched it, and most run types never reached the agent board.
// Callers now supply an agent id and inputs; the server owns validation,
// dispatch, the run row and the board entry.
type AgentRunHandler struct {
	svc service.AgentRunService
}

func NewAgentRunHandler(svc service.AgentRunService) *AgentRunHandler {
	return &AgentRunHandler{svc: svc}
}

// Create handles POST /api/v1/agent-runs.
//
// Returns 202 with the run to poll. A missing required input is a 400 carrying
// the slot and the question, which is the same shape chat renders as a
// clarifying question — so both surfaces ask the user the same thing in their
// own idiom.
func (h *AgentRunHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req model.CreateAgentRunRequest
	if !DecodeJSON(w, r, &req) {
		return
	}
	if req.AgentID == uuid.Nil {
		RespondError(w, http.StatusBadRequest, "agent_id is required")
		return
	}

	orgID, err := uuid.Parse(middleware.GetOrgID(r.Context()))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid org_id in token")
		return
	}
	var requestedBy *uuid.UUID
	if uid, err := uuid.Parse(middleware.GetUserID(r.Context())); err == nil {
		requestedBy = &uid
	}

	run, agent, err := h.svc.Start(r.Context(), orgID, req, requestedBy, model.AgentRunSourceTaskManager)
	if err != nil {
		if miss, ok := service.AsMissingInput(err); ok {
			RespondJSON(w, http.StatusBadRequest, model.AgentRunMissingInput{
				Missing:  miss.Missing,
				Question: miss.Question,
				Options:  miss.Options,
			})
			return
		}
		RespondError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}

	RespondJSON(w, http.StatusAccepted, model.AgentRunAccepted{
		Run:   run,
		Agent: agent.Name,
		Kind:  run.ExternalKind,
	})
}

// Get handles GET /api/v1/agent-runs/{runID}.
func (h *AgentRunHandler) Get(w http.ResponseWriter, r *http.Request) {
	orgID, err := uuid.Parse(middleware.GetOrgID(r.Context()))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid org_id in token")
		return
	}
	runID, err := uuid.Parse(chi.URLParam(r, "runID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid run id")
		return
	}
	run, err := h.svc.GetRun(r.Context(), orgID, runID)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if run == nil {
		RespondError(w, http.StatusNotFound, "agent run not found")
		return
	}
	RespondJSON(w, http.StatusOK, run)
}

// List handles GET /api/v1/agent-runs.
func (h *AgentRunHandler) List(w http.ResponseWriter, r *http.Request) {
	orgID, err := uuid.Parse(middleware.GetOrgID(r.Context()))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid org_id in token")
		return
	}
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	perPage, _ := strconv.Atoi(r.URL.Query().Get("per_page"))
	params := model.PaginationParams{Page: page, PerPage: perPage}
	params.Normalize()

	runs, err := h.svc.ListRuns(r.Context(), orgID, params)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	RespondJSON(w, http.StatusOK, runs)
}
