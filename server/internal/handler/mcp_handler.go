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

type MCPHandler struct {
	svc      service.MCPService
	validate *validator.Validate
}

func NewMCPHandler(svc service.MCPService) *MCPHandler {
	return &MCPHandler{svc: svc, validate: validator.New()}
}

// Create handles POST /mcp-servers
func (h *MCPHandler) Create(w http.ResponseWriter, r *http.Request) {
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

	var req model.CreateMCPServerRequest
	if !DecodeJSON(w, r, &req) {
		return
	}
	if err := h.validate.Struct(req); err != nil {
		RespondError(w, http.StatusBadRequest, "validation failed: "+err.Error())
		return
	}

	m, err := h.svc.Create(r.Context(), orgID, userID, req)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	RespondJSON(w, http.StatusCreated, m)
}

// List handles GET /mcp-servers
func (h *MCPHandler) List(w http.ResponseWriter, r *http.Request) {
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
		items = []model.MCPServer{}
	}
	RespondJSON(w, http.StatusOK, items)
}

// Get handles GET /mcp-servers/{mcpID}
func (h *MCPHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "mcpID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid mcp server id")
		return
	}

	m, err := h.svc.Get(r.Context(), id)
	if err != nil {
		RespondError(w, http.StatusNotFound, err.Error())
		return
	}
	RespondJSON(w, http.StatusOK, m)
}

// Update handles PUT /mcp-servers/{mcpID}
func (h *MCPHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "mcpID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid mcp server id")
		return
	}

	var req model.UpdateMCPServerRequest
	if !DecodeJSON(w, r, &req) {
		return
	}

	m, err := h.svc.Update(r.Context(), id, req)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	RespondJSON(w, http.StatusOK, m)
}

// Delete handles DELETE /mcp-servers/{mcpID}
func (h *MCPHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "mcpID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid mcp server id")
		return
	}

	if err := h.svc.Delete(r.Context(), id); err != nil {
		RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ListTools handles GET /mcp-servers/{mcpID}/tools — connects to the server and
// returns the tools it advertises.
func (h *MCPHandler) ListTools(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "mcpID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid mcp server id")
		return
	}

	tools, err := h.svc.ListTools(r.Context(), id)
	if err != nil {
		RespondError(w, http.StatusBadGateway, err.Error())
		return
	}
	RespondJSON(w, http.StatusOK, tools)
}
