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

type AgentHandler struct {
	svc      service.AgentService
	validate *validator.Validate
}

func NewAgentHandler(svc service.AgentService) *AgentHandler {
	return &AgentHandler{svc: svc, validate: validator.New()}
}

func (h *AgentHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req model.CreateAgentRequest
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

	agent, err := h.svc.Create(r.Context(), orgID, userID, req)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to create agent")
		return
	}
	RespondJSON(w, http.StatusCreated, agent)
}

func (h *AgentHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "agentID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid agent ID")
		return
	}

	agent, err := h.svc.GetByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, service.ErrAgentNotFound) {
			RespondError(w, http.StatusNotFound, err.Error())
			return
		}
		RespondError(w, http.StatusInternalServerError, "failed to get agent")
		return
	}
	RespondJSON(w, http.StatusOK, agent)
}

func (h *AgentHandler) List(w http.ResponseWriter, r *http.Request) {
	orgID, err := uuid.Parse(middleware.GetOrgID(r.Context()))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid org_id in token")
		return
	}

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	perPage, _ := strconv.Atoi(r.URL.Query().Get("per_page"))
	params := model.PaginationParams{Page: page, PerPage: perPage}

	result, err := h.svc.List(r.Context(), orgID, params)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to list agents")
		return
	}
	RespondJSON(w, http.StatusOK, result)
}

func (h *AgentHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "agentID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid agent ID")
		return
	}

	var req model.UpdateAgentRequest
	if !DecodeJSON(w, r, &req) {
		return
	}

	agent, err := h.svc.Update(r.Context(), id, req)
	if err != nil {
		if errors.Is(err, service.ErrAgentNotFound) {
			RespondError(w, http.StatusNotFound, err.Error())
			return
		}
		RespondError(w, http.StatusInternalServerError, "failed to update agent")
		return
	}
	RespondJSON(w, http.StatusOK, agent)
}

func (h *AgentHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "agentID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid agent ID")
		return
	}

	if err := h.svc.Delete(r.Context(), id); err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to delete agent")
		return
	}
	RespondJSON(w, http.StatusNoContent, nil)
}

func (h *AgentHandler) UpdateStatus(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "agentID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid agent ID")
		return
	}

	var req model.UpdateAgentStatusRequest
	if !DecodeJSON(w, r, &req) {
		return
	}
	if err := h.validate.Struct(req); err != nil {
		RespondError(w, http.StatusBadRequest, "validation failed: "+err.Error())
		return
	}

	if err := h.svc.UpdateStatus(r.Context(), id, req.Status); err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to update agent status")
		return
	}
	RespondJSON(w, http.StatusOK, map[string]string{"status": req.Status})
}
