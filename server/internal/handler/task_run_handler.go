package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"

	"github.com/jobshout/server/internal/middleware"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/service"
)

type TaskRunHandler struct {
	svc      service.TaskRunService
	validate *validator.Validate
}

func NewTaskRunHandler(svc service.TaskRunService) *TaskRunHandler {
	return &TaskRunHandler{
		svc:      svc,
		validate: validator.New(),
	}
}

// CreateRun POST /api/v1/tasks/{taskID}/run
func (h *TaskRunHandler) CreateRun(w http.ResponseWriter, r *http.Request) {
	taskID, err := uuid.Parse(chi.URLParam(r, "taskID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid task ID")
		return
	}

	var req model.CreateTaskRunRequest
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

	var requestedBy *uuid.UUID
	if userID, err := uuid.Parse(middleware.GetUserID(r.Context())); err == nil {
		requestedBy = &userID
	}

	run, err := h.svc.CreateRun(r.Context(), taskID, req, orgID, requestedBy)
	if err != nil {
		if errors.Is(err, service.ErrTaskRunNotFound) {
			RespondError(w, http.StatusNotFound, "task not found")
			return
		}
		RespondError(w, http.StatusBadRequest, err.Error())
		return
	}

	RespondJSON(w, http.StatusAccepted, run)
}

// ListRuns GET /api/v1/tasks/{taskID}/runs
func (h *TaskRunHandler) ListRuns(w http.ResponseWriter, r *http.Request) {
	taskID, err := uuid.Parse(chi.URLParam(r, "taskID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid task ID")
		return
	}

	orgID, err := uuid.Parse(middleware.GetOrgID(r.Context()))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid org_id in token")
		return
	}

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	perPage, _ := strconv.Atoi(r.URL.Query().Get("per_page"))
	params := model.PaginationParams{Page: page, PerPage: perPage}

	result, err := h.svc.ListRuns(r.Context(), taskID, orgID, params)
	if err != nil {
		if errors.Is(err, service.ErrTaskRunNotFound) {
			RespondError(w, http.StatusNotFound, "task not found")
			return
		}
		RespondError(w, http.StatusInternalServerError, "failed to list task runs: "+err.Error())
		return
	}

	RespondJSON(w, http.StatusOK, result)
}

// GetRun GET /api/v1/task-runs/{runID}
func (h *TaskRunHandler) GetRun(w http.ResponseWriter, r *http.Request) {
	runID, err := uuid.Parse(chi.URLParam(r, "runID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid run ID")
		return
	}

	orgID, err := uuid.Parse(middleware.GetOrgID(r.Context()))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid org_id in token")
		return
	}

	run, err := h.svc.GetRun(r.Context(), runID, orgID)
	if err != nil {
		RespondError(w, http.StatusNotFound, "run not found")
		return
	}

	RespondJSON(w, http.StatusOK, run)
}
