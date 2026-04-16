package handler

import (
	"net/http"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"

	"github.com/jobshout/server/internal/middleware"
	"github.com/jobshout/server/internal/selector"
)

// SelectorHandler exposes the cost-aware agent selection endpoint.
type SelectorHandler struct {
	sel      *selector.Selector
	validate *validator.Validate
}

// NewSelectorHandler creates a SelectorHandler.
func NewSelectorHandler(sel *selector.Selector) *SelectorHandler {
	return &SelectorHandler{sel: sel, validate: validator.New()}
}

// Select handles POST /agents/select
func (h *SelectorHandler) Select(w http.ResponseWriter, r *http.Request) {
	orgID, err := uuid.Parse(middleware.GetOrgID(r.Context()))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid org_id in token")
		return
	}

	var req selector.SelectionRequest
	if !DecodeJSON(w, r, &req) {
		return
	}
	if err := h.validate.Struct(req); err != nil {
		RespondError(w, http.StatusBadRequest, "validation failed: "+err.Error())
		return
	}

	result, err := h.sel.Select(r.Context(), orgID, req.TaskType, req.AgentIDs)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "agent selection failed: "+err.Error())
		return
	}
	RespondJSON(w, http.StatusOK, result)
}

// RefreshScores handles POST /agents/scores/refresh
func (h *SelectorHandler) RefreshScores(w http.ResponseWriter, r *http.Request) {
	orgID, err := uuid.Parse(middleware.GetOrgID(r.Context()))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid org_id in token")
		return
	}

	if err := h.sel.UpdateScores(r.Context(), orgID); err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to refresh agent scores: "+err.Error())
		return
	}
	RespondJSON(w, http.StatusOK, map[string]string{"status": "scores refreshed"})
}
