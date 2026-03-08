package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"

	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/repository"
)

type OrganizationHandler struct {
	repo     repository.OrganizationRepository
	validate *validator.Validate
}

func NewOrganizationHandler(repo repository.OrganizationRepository) *OrganizationHandler {
	return &OrganizationHandler{repo: repo, validate: validator.New()}
}

func (h *OrganizationHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "orgID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid org ID")
		return
	}

	org, err := h.repo.FindByID(r.Context(), id)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to get organization")
		return
	}
	if org == nil {
		RespondError(w, http.StatusNotFound, "organization not found")
		return
	}
	RespondJSON(w, http.StatusOK, org)
}

func (h *OrganizationHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "orgID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid org ID")
		return
	}

	org, err := h.repo.FindByID(r.Context(), id)
	if err != nil || org == nil {
		RespondError(w, http.StatusNotFound, "organization not found")
		return
	}

	var update struct {
		Name string `json:"name"`
	}
	if !DecodeJSON(w, r, &update) {
		return
	}

	if update.Name != "" {
		org.Name = update.Name
	}

	if err := h.repo.Update(r.Context(), org); err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to update organization")
		return
	}
	RespondJSON(w, http.StatusOK, org)
}

func (h *OrganizationHandler) UpdateChart(w http.ResponseWriter, r *http.Request) {
	orgID, err := uuid.Parse(chi.URLParam(r, "orgID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid org ID")
		return
	}

	var req model.UpdateOrgChartRequest
	if !DecodeJSON(w, r, &req) {
		return
	}
	if err := h.validate.Struct(req); err != nil {
		RespondError(w, http.StatusBadRequest, "validation failed: "+err.Error())
		return
	}

	if err := h.repo.UpdateChart(r.Context(), orgID, req.Agents); err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to update org chart")
		return
	}
	RespondJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}
