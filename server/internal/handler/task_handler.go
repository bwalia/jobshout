package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"

	mw "github.com/jobshout/server/internal/middleware"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/service"
)

type TaskHandler struct {
	svc      service.TaskService
	validate *validator.Validate
}

func NewTaskHandler(svc service.TaskService) *TaskHandler {
	return &TaskHandler{svc: svc, validate: validator.New()}
}

func (h *TaskHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req model.CreateTaskRequest
	if !DecodeJSON(w, r, &req) {
		return
	}
	if err := h.validate.Struct(req); err != nil {
		RespondError(w, http.StatusBadRequest, "validation failed: "+err.Error())
		return
	}

	userID, _ := uuid.Parse(mw.GetUserID(r.Context()))

	task, err := h.svc.Create(r.Context(), userID, req)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to create task: "+err.Error())
		return
	}
	RespondJSON(w, http.StatusCreated, task)
}

func (h *TaskHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "taskID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid task ID")
		return
	}

	task, err := h.svc.GetByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, service.ErrTaskNotFound) {
			RespondError(w, http.StatusNotFound, err.Error())
			return
		}
		RespondError(w, http.StatusInternalServerError, "failed to get task")
		return
	}
	RespondJSON(w, http.StatusOK, task)
}

func (h *TaskHandler) List(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	perPage, _ := strconv.Atoi(r.URL.Query().Get("per_page"))
	params := model.PaginationParams{Page: page, PerPage: perPage}

	projectIDStr := r.URL.Query().Get("project_id")
	if projectIDStr != "" {
		projectID, err := uuid.Parse(projectIDStr)
		if err != nil {
			RespondError(w, http.StatusBadRequest, "invalid project_id")
			return
		}
		result, err := h.svc.List(r.Context(), projectID, params)
		if err != nil {
			RespondError(w, http.StatusInternalServerError, "failed to list tasks")
			return
		}
		RespondJSON(w, http.StatusOK, result)
		return
	}

	// No project_id: list all tasks for the user's org
	orgID, err := uuid.Parse(mw.GetOrgID(r.Context()))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid org_id in token")
		return
	}
	result, err := h.svc.ListByOrg(r.Context(), orgID, params)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to list tasks")
		return
	}
	RespondJSON(w, http.StatusOK, result)
}

func (h *TaskHandler) ListComments(w http.ResponseWriter, r *http.Request) {
	taskID, err := uuid.Parse(chi.URLParam(r, "taskID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid task ID")
		return
	}

	comments, err := h.svc.ListComments(r.Context(), taskID)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to list comments")
		return
	}
	RespondJSON(w, http.StatusOK, comments)
}

func (h *TaskHandler) AddComment(w http.ResponseWriter, r *http.Request) {
	taskID, err := uuid.Parse(chi.URLParam(r, "taskID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid task ID")
		return
	}

	var req model.AddCommentRequest
	if !DecodeJSON(w, r, &req) {
		return
	}
	if err := h.validate.Struct(req); err != nil {
		RespondError(w, http.StatusBadRequest, "validation failed: "+err.Error())
		return
	}

	userID, _ := uuid.Parse(mw.GetUserID(r.Context()))

	comment, err := h.svc.AddComment(r.Context(), taskID, userID, req.Body)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to add comment")
		return
	}
	RespondJSON(w, http.StatusCreated, comment)
}

func (h *TaskHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "taskID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid task ID")
		return
	}

	var req model.UpdateTaskRequest
	if !DecodeJSON(w, r, &req) {
		return
	}

	task, err := h.svc.Update(r.Context(), id, req)
	if err != nil {
		if errors.Is(err, service.ErrTaskNotFound) {
			RespondError(w, http.StatusNotFound, err.Error())
			return
		}
		RespondError(w, http.StatusInternalServerError, "failed to update task")
		return
	}
	RespondJSON(w, http.StatusOK, task)
}

func (h *TaskHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "taskID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid task ID")
		return
	}
	if err := h.svc.Delete(r.Context(), id); err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to delete task")
		return
	}
	RespondJSON(w, http.StatusNoContent, nil)
}

func (h *TaskHandler) Transition(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "taskID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid task ID")
		return
	}

	var req model.TransitionTaskRequest
	if !DecodeJSON(w, r, &req) {
		return
	}
	if err := h.validate.Struct(req); err != nil {
		RespondError(w, http.StatusBadRequest, "validation failed: "+err.Error())
		return
	}

	if err := h.svc.Transition(r.Context(), id, req.Status); err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to transition task")
		return
	}
	RespondJSON(w, http.StatusOK, map[string]string{"status": req.Status})
}

func (h *TaskHandler) Reorder(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "taskID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid task ID")
		return
	}

	var req model.ReorderTaskRequest
	if !DecodeJSON(w, r, &req) {
		return
	}
	if err := h.validate.Struct(req); err != nil {
		RespondError(w, http.StatusBadRequest, "validation failed: "+err.Error())
		return
	}

	if err := h.svc.Reorder(r.Context(), id, req.Status, req.Position); err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to reorder task")
		return
	}
	RespondJSON(w, http.StatusOK, map[string]string{"status": "reordered"})
}
