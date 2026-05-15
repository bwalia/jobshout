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

// SprintHandler exposes sprint CRUD + agent/job association endpoints.
type SprintHandler struct {
	svc      service.SprintService
	validate *validator.Validate
}

func NewSprintHandler(svc service.SprintService) *SprintHandler {
	return &SprintHandler{svc: svc, validate: validator.New()}
}

func (h *SprintHandler) Create(w http.ResponseWriter, r *http.Request) {
	orgID, err := uuid.Parse(middleware.GetOrgID(r.Context()))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid org_id")
		return
	}
	var userID *uuid.UUID
	if u, err := uuid.Parse(middleware.GetUserID(r.Context())); err == nil {
		userID = &u
	}

	var req model.CreateSprintRequest
	if !DecodeJSON(w, r, &req) {
		return
	}
	if err := h.validate.Struct(req); err != nil {
		RespondError(w, http.StatusBadRequest, "validation: "+err.Error())
		return
	}

	sp, err := h.svc.Create(r.Context(), orgID, userID, req)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	RespondJSON(w, http.StatusCreated, sp)
}

func (h *SprintHandler) List(w http.ResponseWriter, r *http.Request) {
	orgID, err := uuid.Parse(middleware.GetOrgID(r.Context()))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid org_id")
		return
	}
	out, err := h.svc.List(r.Context(), orgID)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	RespondJSON(w, http.StatusOK, out)
}

func (h *SprintHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "sprintID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid sprint id")
		return
	}
	// Detail view: sprint + jobs + agents + stats. The list view returns the
	// thin Sprint rows so the index page stays fast.
	detail, err := h.svc.GetDetail(r.Context(), id)
	if err != nil {
		RespondError(w, http.StatusNotFound, "sprint not found")
		return
	}
	RespondJSON(w, http.StatusOK, detail)
}

func (h *SprintHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "sprintID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid sprint id")
		return
	}
	var req model.UpdateSprintRequest
	if !DecodeJSON(w, r, &req) {
		return
	}
	if err := h.validate.Struct(req); err != nil {
		RespondError(w, http.StatusBadRequest, "validation: "+err.Error())
		return
	}
	sp, err := h.svc.Update(r.Context(), id, req)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	RespondJSON(w, http.StatusOK, sp)
}

func (h *SprintHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "sprintID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid sprint id")
		return
	}
	if err := h.svc.Delete(r.Context(), id); err != nil {
		RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *SprintHandler) AddJob(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "sprintID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid sprint id")
		return
	}
	var req model.AddSprintJobRequest
	if !DecodeJSON(w, r, &req) {
		return
	}
	if err := h.validate.Struct(req); err != nil {
		RespondError(w, http.StatusBadRequest, "validation: "+err.Error())
		return
	}
	if err := h.svc.AddJob(r.Context(), id, req); err != nil {
		RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *SprintHandler) RemoveJob(w http.ResponseWriter, r *http.Request) {
	sprintID, err := uuid.Parse(chi.URLParam(r, "sprintID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid sprint id")
		return
	}
	jobID, err := uuid.Parse(chi.URLParam(r, "jobID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid job id")
		return
	}
	if err := h.svc.RemoveJob(r.Context(), sprintID, jobID); err != nil {
		RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *SprintHandler) AddAgent(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "sprintID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid sprint id")
		return
	}
	var req model.AddSprintAgentRequest
	if !DecodeJSON(w, r, &req) {
		return
	}
	if err := h.validate.Struct(req); err != nil {
		RespondError(w, http.StatusBadRequest, "validation: "+err.Error())
		return
	}
	if err := h.svc.AddAgent(r.Context(), id, req); err != nil {
		RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *SprintHandler) RemoveAgent(w http.ResponseWriter, r *http.Request) {
	sprintID, err := uuid.Parse(chi.URLParam(r, "sprintID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid sprint id")
		return
	}
	agentID, err := uuid.Parse(chi.URLParam(r, "agentID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid agent id")
		return
	}
	roleLabel := r.URL.Query().Get("role_label")
	if err := h.svc.RemoveAgent(r.Context(), sprintID, agentID, roleLabel); err != nil {
		RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
