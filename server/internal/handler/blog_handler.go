package handler

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"

	"github.com/jobshout/server/internal/middleware"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/service"
)

// BlogHandler exposes the automated blog-generator endpoints.
type BlogHandler struct {
	svc      service.BlogService
	validate *validator.Validate
}

// NewBlogHandler creates a BlogHandler.
func NewBlogHandler(svc service.BlogService) *BlogHandler {
	return &BlogHandler{svc: svc, validate: validator.New()}
}

// Generate handles POST /api/v1/blogs/generate — runs the full pipeline
// (LLM → git → PR) synchronously and returns the persisted run record.
func (h *BlogHandler) Generate(w http.ResponseWriter, r *http.Request) {
	var req model.GenerateBlogRequest
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
	userIDStr := middleware.GetUserID(r.Context())
	var triggeredBy *uuid.UUID
	if parsed, err := uuid.Parse(userIDStr); err == nil {
		triggeredBy = &parsed
	}

	run, err := h.svc.Generate(r.Context(), orgID, triggeredBy, "api", req)
	if err != nil {
		// Still return the run record so the caller can see what was
		// persisted, but surface the error status.
		RespondJSON(w, http.StatusInternalServerError, map[string]any{
			"error": err.Error(),
			"run":   run,
		})
		return
	}
	RespondJSON(w, http.StatusCreated, run)
}

// GetRun handles GET /api/v1/blogs/runs/{runID}.
func (h *BlogHandler) GetRun(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "runID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid run ID")
		return
	}
	run, err := h.svc.GetByID(r.Context(), id)
	if err != nil {
		RespondError(w, http.StatusNotFound, "blog run not found")
		return
	}
	RespondJSON(w, http.StatusOK, run)
}

// ListRuns handles GET /api/v1/blogs/runs.
func (h *BlogHandler) ListRuns(w http.ResponseWriter, r *http.Request) {
	orgID, err := uuid.Parse(middleware.GetOrgID(r.Context()))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid org_id in token")
		return
	}
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	perPage, _ := strconv.Atoi(r.URL.Query().Get("per_page"))
	params := model.PaginationParams{Page: page, PerPage: perPage}

	result, err := h.svc.ListByOrg(r.Context(), orgID, params)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to list blog runs")
		return
	}
	RespondJSON(w, http.StatusOK, result)
}
