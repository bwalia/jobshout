package handler

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"

	"github.com/jobshout/server/internal/middleware"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/research"
	"github.com/jobshout/server/internal/service"
)

// ResearchHandler exposes the Research Agent directly, not only through the
// article pipeline. Research is a general capability — anything that needs
// current, cited material about a subject can call it — and an endpoint is what
// makes that true in practice rather than in principle.
type ResearchHandler struct {
	svc      service.ResearchService
	validate *validator.Validate
}

// NewResearchHandler creates a ResearchHandler.
func NewResearchHandler(svc service.ResearchService) *ResearchHandler {
	return &ResearchHandler{svc: svc, validate: validator.New()}
}

// ResearchRequest is the body of POST /api/v1/research.
type ResearchRequest struct {
	// Topic is the subject to research. It is not a title.
	Topic string `json:"topic" validate:"required,min=3"`
	// Context is optional guidance: angle, audience, points to cover, things to
	// avoid.
	Context string `json:"context,omitempty"`
	// Model optionally overrides the LLM used.
	Model string `json:"model,omitempty"`
}

// Research handles POST /api/v1/research.
//
// This runs synchronously rather than returning a run to poll. A brief takes
// several LLM calls and a handful of page fetches, which is slow for a request
// but not unboundedly so, and the caller wants the brief itself — there is no
// artefact left behind to come back for later. Callers that need it in the
// background already have one: the article pipeline.
func (h *ResearchHandler) Research(w http.ResponseWriter, r *http.Request) {
	var req ResearchRequest
	if !DecodeJSON(w, r, &req) {
		return
	}
	if err := h.validate.Struct(req); err != nil {
		RespondError(w, http.StatusBadRequest, "validation failed: "+err.Error())
		return
	}

	orgID, err := uuid.Parse(middleware.GetOrgID(r.Context()))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid org_id in token")
		return
	}

	if !h.svc.Available() {
		RespondError(w, http.StatusServiceUnavailable,
			"research is not configured (an LLM provider must be reachable)")
		return
	}

	brief, err := h.svc.Research(r.Context(), orgID, research.Request{
		Topic:   req.Topic,
		Context: req.Context,
		Model:   req.Model,
	}, nil)
	if err != nil {
		// Research failures are about the subject, not the server: nothing
		// found, nothing retrievable, nothing verifiable. The message says
		// which, and the caller can act on it by rephrasing.
		RespondError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	RespondJSON(w, http.StatusOK, brief)
}

// Trending handles GET /api/v1/research/trending.
//
// Takes no query: it reports what is currently being published across
// technology, AI and infrastructure. This is the endpoint topic discovery is
// built on.
func (h *ResearchHandler) Trending(w http.ResponseWriter, r *http.Request) {
	limit := research.DefaultLimit
	if raw := r.URL.Query().Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			RespondError(w, http.StatusBadRequest, "limit must be an integer")
			return
		}
		limit = parsed
	}

	items, err := h.svc.Trending(r.Context(), limit)
	if err != nil {
		RespondError(w, http.StatusBadGateway, err.Error())
		return
	}
	RespondJSON(w, http.StatusOK, map[string]any{"items": items})
}

// StartRun handles POST /api/v1/research/runs.
//
// Returns 202 with the run to poll rather than the brief itself. This is the
// endpoint that replaces holding a request open for the length of a research
// run: the Task Manager used to wait three minutes in the browser and then
// write the findings into the task's description, because there was no run row
// to come back to.
func (h *ResearchHandler) StartRun(w http.ResponseWriter, r *http.Request) {
	var req model.CreateResearchRunRequest
	if !DecodeJSON(w, r, &req) {
		return
	}
	if len(strings.TrimSpace(req.Topic)) < 3 {
		RespondError(w, http.StatusBadRequest, "topic is required")
		return
	}

	orgID, err := uuid.Parse(middleware.GetOrgID(r.Context()))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid org_id in token")
		return
	}
	if !h.svc.Available() {
		RespondError(w, http.StatusServiceUnavailable,
			"research is not configured (an LLM provider must be reachable)")
		return
	}

	opts := service.ResearchRunOptions{
		Source: model.ResearchSourceTaskManager,
		TaskID: req.TaskID,
	}
	if uid, err := uuid.Parse(middleware.GetUserID(r.Context())); err == nil {
		opts.RequestedBy = &uid
	}
	agentReq := research.Request{Topic: req.Topic, Context: req.Context, URLs: req.URLs}

	// wait=true keeps the synchronous shape for callers that have not moved to
	// polling yet. It is deliberately opt-in: the default is the one that does
	// not depend on a browser staying open.
	if req.Wait {
		out, err := h.svc.Run(r.Context(), orgID, agentReq, nil, opts)
		if err != nil {
			RespondError(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
		RespondJSON(w, http.StatusOK, out.Run)
		return
	}

	run, err := h.svc.StartAsync(r.Context(), orgID, agentReq, opts)
	if err != nil {
		RespondError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	RespondJSON(w, http.StatusAccepted, run)
}

// GetRun handles GET /api/v1/research/runs/{runID}.
func (h *ResearchHandler) GetRun(w http.ResponseWriter, r *http.Request) {
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
		RespondError(w, http.StatusNotFound, "research run not found")
		return
	}
	RespondJSON(w, http.StatusOK, run)
}

// ListRuns handles GET /api/v1/research/runs.
func (h *ResearchHandler) ListRuns(w http.ResponseWriter, r *http.Request) {
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
