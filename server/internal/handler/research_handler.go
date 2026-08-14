package handler

import (
	"net/http"
	"strconv"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"

	"github.com/jobshout/server/internal/middleware"
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
