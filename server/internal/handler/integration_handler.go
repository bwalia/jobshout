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

type IntegrationHandler struct {
	svc      service.IntegrationService
	validate *validator.Validate
}

func NewIntegrationHandler(svc service.IntegrationService) *IntegrationHandler {
	return &IntegrationHandler{svc: svc, validate: validator.New()}
}

// Create handles POST /integrations
func (h *IntegrationHandler) Create(w http.ResponseWriter, r *http.Request) {
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

	var req model.CreateIntegrationRequest
	if !DecodeJSON(w, r, &req) {
		return
	}
	if err := h.validate.Struct(req); err != nil {
		RespondError(w, http.StatusBadRequest, "validation failed: "+err.Error())
		return
	}

	i, err := h.svc.Create(r.Context(), orgID, userID, req)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	RespondJSON(w, http.StatusCreated, i)
}

// List handles GET /integrations
func (h *IntegrationHandler) List(w http.ResponseWriter, r *http.Request) {
	orgID, err := uuid.Parse(middleware.GetOrgID(r.Context()))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid org_id in token")
		return
	}

	items, err := h.svc.List(r.Context(), orgID)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if items == nil {
		items = []model.Integration{}
	}
	RespondJSON(w, http.StatusOK, items)
}

// Get handles GET /integrations/{integrationID}
func (h *IntegrationHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "integrationID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid integration id")
		return
	}

	i, err := h.svc.Get(r.Context(), id)
	if err != nil {
		RespondError(w, http.StatusNotFound, err.Error())
		return
	}
	RespondJSON(w, http.StatusOK, i)
}

// Update handles PUT /integrations/{integrationID}
func (h *IntegrationHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "integrationID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid integration id")
		return
	}

	var req model.UpdateIntegrationRequest
	if !DecodeJSON(w, r, &req) {
		return
	}

	i, err := h.svc.Update(r.Context(), id, req)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	RespondJSON(w, http.StatusOK, i)
}

// Delete handles DELETE /integrations/{integrationID}
func (h *IntegrationHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "integrationID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid integration id")
		return
	}

	if err := h.svc.Delete(r.Context(), id); err != nil {
		RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// LinkTask handles POST /integrations/{integrationID}/tasks/{taskID}/link
func (h *IntegrationHandler) LinkTask(w http.ResponseWriter, r *http.Request) {
	integrationID, err := uuid.Parse(chi.URLParam(r, "integrationID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid integration id")
		return
	}
	taskID, err := uuid.Parse(chi.URLParam(r, "taskID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid task id")
		return
	}

	var req model.LinkTaskRequest
	_ = DecodeJSON(w, r, &req) // optional body
	direction := req.Direction
	if direction == "" {
		direction = "bidirectional"
	}

	link, err := h.svc.LinkTask(r.Context(), integrationID, taskID, direction)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	RespondJSON(w, http.StatusCreated, link)
}

// UnlinkTask handles DELETE /integrations/{integrationID}/tasks/{taskID}/link
func (h *IntegrationHandler) UnlinkTask(w http.ResponseWriter, r *http.Request) {
	integrationID, err := uuid.Parse(chi.URLParam(r, "integrationID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid integration id")
		return
	}
	taskID, err := uuid.Parse(chi.URLParam(r, "taskID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid task id")
		return
	}

	if err := h.svc.UnlinkTask(r.Context(), integrationID, taskID); err != nil {
		RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ListLinks handles GET /integrations/{integrationID}/links
func (h *IntegrationHandler) ListLinks(w http.ResponseWriter, r *http.Request) {
	integrationID, err := uuid.Parse(chi.URLParam(r, "integrationID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid integration id")
		return
	}

	links, err := h.svc.ListLinks(r.Context(), integrationID)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if links == nil {
		links = []model.IntegrationTaskLink{}
	}
	RespondJSON(w, http.StatusOK, links)
}

// SyncLink handles POST /integrations/{integrationID}/links/{linkID}/sync
func (h *IntegrationHandler) SyncLink(w http.ResponseWriter, r *http.Request) {
	linkID, err := uuid.Parse(chi.URLParam(r, "linkID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid link id")
		return
	}

	direction := r.URL.Query().Get("direction")
	if direction == "" {
		direction = "push"
	}

	if err := h.svc.SyncLink(r.Context(), linkID, direction); err != nil {
		RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	RespondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// ListSyncLogs handles GET /integrations/{integrationID}/sync-logs
func (h *IntegrationHandler) ListSyncLogs(w http.ResponseWriter, r *http.Request) {
	integrationID, err := uuid.Parse(chi.URLParam(r, "integrationID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid integration id")
		return
	}

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	perPage, _ := strconv.Atoi(r.URL.Query().Get("per_page"))
	params := model.PaginationParams{Page: page, PerPage: perPage}

	logs, err := h.svc.ListSyncLogs(r.Context(), integrationID, params)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	RespondJSON(w, http.StatusOK, logs)
}
