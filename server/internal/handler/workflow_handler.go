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

// WorkflowHandler exposes workflow CRUD and execution endpoints.
type WorkflowHandler struct {
	svc      service.WorkflowService
	validate *validator.Validate
}

// NewWorkflowHandler creates a WorkflowHandler.
func NewWorkflowHandler(svc service.WorkflowService) *WorkflowHandler {
	return &WorkflowHandler{svc: svc, validate: validator.New()}
}

// Create handles POST /workflows
func (h *WorkflowHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req model.CreateWorkflowRequest
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
	userID, err := uuid.Parse(middleware.GetUserID(r.Context()))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid user_id in token")
		return
	}

	wf, err := h.svc.Create(r.Context(), orgID, userID, req)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to create workflow: "+err.Error())
		return
	}
	RespondJSON(w, http.StatusCreated, wf)
}

// GetByID handles GET /workflows/{workflowID}
func (h *WorkflowHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "workflowID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid workflow ID")
		return
	}

	wf, err := h.svc.GetByID(r.Context(), id)
	if err != nil {
		RespondError(w, http.StatusNotFound, "workflow not found")
		return
	}
	RespondJSON(w, http.StatusOK, wf)
}

// List handles GET /workflows
func (h *WorkflowHandler) List(w http.ResponseWriter, r *http.Request) {
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
		RespondError(w, http.StatusInternalServerError, "failed to list workflows")
		return
	}
	RespondJSON(w, http.StatusOK, result)
}

// Update handles PUT /workflows/{workflowID}
func (h *WorkflowHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "workflowID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid workflow ID")
		return
	}

	var req model.UpdateWorkflowRequest
	if !DecodeJSON(w, r, &req) {
		return
	}
	if err := h.validate.Struct(req); err != nil {
		RespondError(w, http.StatusBadRequest, "validation failed: "+err.Error())
		return
	}

	wf, err := h.svc.Update(r.Context(), id, req)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to update workflow: "+err.Error())
		return
	}
	RespondJSON(w, http.StatusOK, wf)
}

// Delete handles DELETE /workflows/{workflowID}
func (h *WorkflowHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "workflowID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid workflow ID")
		return
	}

	if err := h.svc.Delete(r.Context(), id); err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to delete workflow")
		return
	}
	RespondJSON(w, http.StatusNoContent, nil)
}

// ExecuteWorkflow handles POST /workflows/{workflowID}/execute
// It starts a workflow run asynchronously and returns the run record immediately.
func (h *WorkflowHandler) ExecuteWorkflow(w http.ResponseWriter, r *http.Request) {
	wfID, err := uuid.Parse(chi.URLParam(r, "workflowID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid workflow ID")
		return
	}

	var req model.ExecuteWorkflowRequest
	// Body is optional — caller may send {} or omit entirely.
	_ = DecodeJSON(w, r, &req)

	orgID, err := uuid.Parse(middleware.GetOrgID(r.Context()))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid org_id in token")
		return
	}
	userID, err := uuid.Parse(middleware.GetUserID(r.Context()))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid user_id in token")
		return
	}

	run, err := h.svc.Execute(r.Context(), wfID, orgID, userID, req)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to start workflow: "+err.Error())
		return
	}

	// 202 Accepted — the run is in progress; poll GET /runs/{id} for status.
	RespondJSON(w, http.StatusAccepted, run)
}

// GetRun handles GET /workflow-runs/{runID}
func (h *WorkflowHandler) GetRun(w http.ResponseWriter, r *http.Request) {
	runID, err := uuid.Parse(chi.URLParam(r, "runID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid run ID")
		return
	}

	run, err := h.svc.GetRunByID(r.Context(), runID)
	if err != nil {
		RespondError(w, http.StatusNotFound, "workflow run not found")
		return
	}
	RespondJSON(w, http.StatusOK, run)
}

// ListRuns handles GET /workflows/{workflowID}/runs
func (h *WorkflowHandler) ListRuns(w http.ResponseWriter, r *http.Request) {
	wfID, err := uuid.Parse(chi.URLParam(r, "workflowID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid workflow ID")
		return
	}

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	perPage, _ := strconv.Atoi(r.URL.Query().Get("per_page"))
	params := model.PaginationParams{Page: page, PerPage: perPage}

	result, err := h.svc.ListRuns(r.Context(), wfID, params)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to list runs")
		return
	}
	RespondJSON(w, http.StatusOK, result)
}
