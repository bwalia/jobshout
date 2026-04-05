package handler

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"

	"github.com/jobshout/server/internal/middleware"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/repository"
)

type SchedulerHandler struct {
	repo     repository.SchedulerRepository
	validate *validator.Validate
}

func NewSchedulerHandler(repo repository.SchedulerRepository) *SchedulerHandler {
	return &SchedulerHandler{repo: repo, validate: validator.New()}
}

func (h *SchedulerHandler) List(w http.ResponseWriter, r *http.Request) {
	orgID, err := uuid.Parse(middleware.GetOrgID(r.Context()))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid org_id")
		return
	}

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	perPage, _ := strconv.Atoi(r.URL.Query().Get("per_page"))
	params := model.PaginationParams{Page: page, PerPage: perPage}

	result, err := h.repo.ListTasks(r.Context(), orgID, params)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to list scheduled tasks")
		return
	}
	RespondJSON(w, http.StatusOK, result)
}

func (h *SchedulerHandler) Create(w http.ResponseWriter, r *http.Request) {
	orgID, err := uuid.Parse(middleware.GetOrgID(r.Context()))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid org_id")
		return
	}
	userID, _ := uuid.Parse(middleware.GetUserID(r.Context()))

	var req model.CreateScheduledTaskRequest
	if !DecodeJSON(w, r, &req) {
		return
	}
	if err := h.validate.Struct(req); err != nil {
		RespondError(w, http.StatusBadRequest, "validation: "+err.Error())
		return
	}

	t := &model.ScheduledTask{
		ID:             uuid.New(),
		OrgID:          orgID,
		Name:           req.Name,
		Description:    req.Description,
		TaskType:       req.TaskType,
		InputPrompt:    req.InputPrompt,
		InputJSON:      req.InputJSON,
		ModelOverride:  req.ModelOverride,
		ScheduleType:   req.ScheduleType,
		CronExpression: req.CronExpression,
		IntervalSeconds: req.IntervalSeconds,
		Status:         "active",
		MaxRuns:        req.MaxRuns,
		RetryOnFailure: req.RetryOnFailure,
		MaxRetries:     req.MaxRetries,
		Priority:       req.Priority,
		Tags:           req.Tags,
		CreatedBy:      &userID,
	}

	if t.Priority == "" {
		t.Priority = "medium"
	}
	if t.Tags == nil {
		t.Tags = []string{}
	}
	if t.InputJSON == nil {
		t.InputJSON = map[string]any{}
	}

	if req.AgentID != nil {
		id, _ := uuid.Parse(*req.AgentID)
		t.AgentID = &id
	}
	if req.WorkflowID != nil {
		id, _ := uuid.Parse(*req.WorkflowID)
		t.WorkflowID = &id
	}
	if req.ProviderConfigID != nil {
		id, _ := uuid.Parse(*req.ProviderConfigID)
		t.ProviderConfigID = &id
	}

	if err := h.repo.CreateTask(r.Context(), t); err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to create: "+err.Error())
		return
	}

	RespondJSON(w, http.StatusCreated, t)
}

func (h *SchedulerHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "taskID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid task ID")
		return
	}

	t, err := h.repo.GetTask(r.Context(), id)
	if err != nil {
		RespondError(w, http.StatusNotFound, "scheduled task not found")
		return
	}
	RespondJSON(w, http.StatusOK, t)
}

func (h *SchedulerHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "taskID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid task ID")
		return
	}

	var req model.UpdateScheduledTaskRequest
	if !DecodeJSON(w, r, &req) {
		return
	}

	t, err := h.repo.UpdateTask(r.Context(), id, req)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to update: "+err.Error())
		return
	}
	RespondJSON(w, http.StatusOK, t)
}

func (h *SchedulerHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "taskID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid task ID")
		return
	}

	if err := h.repo.DeleteTask(r.Context(), id); err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to delete")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *SchedulerHandler) ListRuns(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "taskID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid task ID")
		return
	}

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	perPage, _ := strconv.Atoi(r.URL.Query().Get("per_page"))
	params := model.PaginationParams{Page: page, PerPage: perPage}

	result, err := h.repo.ListRuns(r.Context(), id, params)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to list runs")
		return
	}
	RespondJSON(w, http.StatusOK, result)
}
