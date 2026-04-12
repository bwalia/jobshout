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

// PluginHandler exposes plugin CRUD and execution endpoints.
type PluginHandler struct {
	svc      service.PluginService
	validate *validator.Validate
}

func NewPluginHandler(svc service.PluginService) *PluginHandler {
	return &PluginHandler{svc: svc, validate: validator.New()}
}

// Create handles POST /plugins
func (h *PluginHandler) Create(w http.ResponseWriter, r *http.Request) {
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

	var req model.CreatePluginRequest
	if !DecodeJSON(w, r, &req) {
		return
	}
	if err := h.validate.Struct(req); err != nil {
		RespondError(w, http.StatusBadRequest, "validation failed: "+err.Error())
		return
	}

	plugin, err := h.svc.Create(r.Context(), orgID, userID, req)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	RespondJSON(w, http.StatusCreated, plugin)
}

// List handles GET /plugins
func (h *PluginHandler) List(w http.ResponseWriter, r *http.Request) {
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
		RespondError(w, http.StatusInternalServerError, "failed to list plugins")
		return
	}
	RespondJSON(w, http.StatusOK, result)
}

// GetByID handles GET /plugins/{pluginID}
func (h *PluginHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	pluginID, err := uuid.Parse(chi.URLParam(r, "pluginID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid plugin ID")
		return
	}

	plugin, err := h.svc.GetByID(r.Context(), pluginID)
	if err != nil || plugin == nil {
		RespondError(w, http.StatusNotFound, "plugin not found")
		return
	}
	RespondJSON(w, http.StatusOK, plugin)
}

// Update handles PUT /plugins/{pluginID}
func (h *PluginHandler) Update(w http.ResponseWriter, r *http.Request) {
	pluginID, err := uuid.Parse(chi.URLParam(r, "pluginID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid plugin ID")
		return
	}

	var req model.UpdatePluginRequest
	if !DecodeJSON(w, r, &req) {
		return
	}

	plugin, err := h.svc.Update(r.Context(), pluginID, req)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	RespondJSON(w, http.StatusOK, plugin)
}

// Delete handles DELETE /plugins/{pluginID}
func (h *PluginHandler) Delete(w http.ResponseWriter, r *http.Request) {
	pluginID, err := uuid.Parse(chi.URLParam(r, "pluginID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid plugin ID")
		return
	}

	if err := h.svc.Delete(r.Context(), pluginID); err != nil {
		RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Execute handles POST /plugins/{pluginID}/execute
func (h *PluginHandler) Execute(w http.ResponseWriter, r *http.Request) {
	pluginID, err := uuid.Parse(chi.URLParam(r, "pluginID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid plugin ID")
		return
	}

	orgID, err := uuid.Parse(middleware.GetOrgID(r.Context()))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid org_id in token")
		return
	}

	var req model.ExecutePluginRequest
	if !DecodeJSON(w, r, &req) {
		return
	}

	result, err := h.svc.Execute(r.Context(), pluginID, orgID, req)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "plugin execution failed: "+err.Error())
		return
	}
	RespondJSON(w, http.StatusOK, result)
}

// ListExecutions handles GET /plugins/{pluginID}/executions
func (h *PluginHandler) ListExecutions(w http.ResponseWriter, r *http.Request) {
	pluginID, err := uuid.Parse(chi.URLParam(r, "pluginID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid plugin ID")
		return
	}

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	perPage, _ := strconv.Atoi(r.URL.Query().Get("per_page"))
	params := model.PaginationParams{Page: page, PerPage: perPage}

	result, err := h.svc.ListExecutions(r.Context(), pluginID, params)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to list plugin executions")
		return
	}
	RespondJSON(w, http.StatusOK, result)
}
