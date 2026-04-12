package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"

	"github.com/jobshout/server/internal/middleware"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/service"
)

type NotificationHandler struct {
	svc      service.NotificationService
	validate *validator.Validate
}

func NewNotificationHandler(svc service.NotificationService) *NotificationHandler {
	return &NotificationHandler{svc: svc, validate: validator.New()}
}

// Create handles POST /notifications
func (h *NotificationHandler) Create(w http.ResponseWriter, r *http.Request) {
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

	var req model.CreateNotificationConfigRequest
	if !DecodeJSON(w, r, &req) {
		return
	}
	if err := h.validate.Struct(req); err != nil {
		RespondError(w, http.StatusBadRequest, "validation failed: "+err.Error())
		return
	}

	cfg, err := h.svc.Create(r.Context(), orgID, userID, req)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	RespondJSON(w, http.StatusCreated, cfg)
}

// List handles GET /notifications
func (h *NotificationHandler) List(w http.ResponseWriter, r *http.Request) {
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
		items = []model.NotificationConfig{}
	}
	RespondJSON(w, http.StatusOK, items)
}

// Get handles GET /notifications/{configID}
func (h *NotificationHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "configID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid config id")
		return
	}

	cfg, err := h.svc.Get(r.Context(), id)
	if err != nil {
		RespondError(w, http.StatusNotFound, err.Error())
		return
	}
	RespondJSON(w, http.StatusOK, cfg)
}

// Update handles PUT /notifications/{configID}
func (h *NotificationHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "configID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid config id")
		return
	}

	var req model.UpdateNotificationConfigRequest
	if !DecodeJSON(w, r, &req) {
		return
	}

	cfg, err := h.svc.Update(r.Context(), id, req)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	RespondJSON(w, http.StatusOK, cfg)
}

// Delete handles DELETE /notifications/{configID}
func (h *NotificationHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "configID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid config id")
		return
	}

	if err := h.svc.Delete(r.Context(), id); err != nil {
		RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Test handles POST /notifications/{configID}/test
func (h *NotificationHandler) Test(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "configID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid config id")
		return
	}

	if err := h.svc.TestConfig(r.Context(), id); err != nil {
		RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	RespondJSON(w, http.StatusOK, map[string]string{"status": "ok", "message": "test notification sent"})
}
