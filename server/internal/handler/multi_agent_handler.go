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

// MultiAgentHandler handles multi-agent collaboration job endpoints.
type MultiAgentHandler struct {
	svc      service.MultiAgentService
	validate *validator.Validate
}

// NewMultiAgentHandler creates a new MultiAgentHandler.
func NewMultiAgentHandler(svc service.MultiAgentService) *MultiAgentHandler {
	return &MultiAgentHandler{svc: svc, validate: validator.New()}
}

// RunJob starts a new multi-agent collaboration job.
func (h *MultiAgentHandler) RunJob(w http.ResponseWriter, r *http.Request) {
	orgID, _ := uuid.Parse(middleware.GetOrgID(r.Context()))

	var req model.RunMultiAgentRequest
	if !DecodeJSON(w, r, &req) {
		return
	}
	if err := h.validate.Struct(req); err != nil {
		RespondError(w, http.StatusBadRequest, err.Error())
		return
	}

	job, err := h.svc.RunJob(r.Context(), orgID, req)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	RespondJSON(w, http.StatusAccepted, job)
}

// GetJob returns a multi-agent job by ID.
func (h *MultiAgentHandler) GetJob(w http.ResponseWriter, r *http.Request) {
	jobID, err := uuid.Parse(chi.URLParam(r, "jobID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid job ID")
		return
	}

	job, err := h.svc.GetJob(r.Context(), jobID)
	if err != nil {
		RespondError(w, http.StatusNotFound, "job not found")
		return
	}
	RespondJSON(w, http.StatusOK, job)
}

// ListJobs lists multi-agent jobs for the org.
func (h *MultiAgentHandler) ListJobs(w http.ResponseWriter, r *http.Request) {
	orgID, _ := uuid.Parse(middleware.GetOrgID(r.Context()))

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	perPage, _ := strconv.Atoi(r.URL.Query().Get("per_page"))
	params := model.PaginationParams{Page: page, PerPage: perPage}

	result, err := h.svc.ListJobs(r.Context(), orgID, params)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	RespondJSON(w, http.StatusOK, result)
}
