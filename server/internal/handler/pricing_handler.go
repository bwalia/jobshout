package handler

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"

	"github.com/jobshout/server/internal/middleware"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/repository"
)

// PricingHandler exposes pricing configuration endpoints.
type PricingHandler struct {
	repo     repository.PricingRepository
	validate *validator.Validate
}

// NewPricingHandler creates a PricingHandler.
func NewPricingHandler(repo repository.PricingRepository) *PricingHandler {
	return &PricingHandler{repo: repo, validate: validator.New()}
}

// ListActive handles GET /pricing
func (h *PricingHandler) ListActive(w http.ResponseWriter, r *http.Request) {
	orgID, err := uuid.Parse(middleware.GetOrgID(r.Context()))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid org_id in token")
		return
	}

	configs, err := h.repo.ListActive(r.Context(), &orgID)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to list pricing configs")
		return
	}
	if configs == nil {
		configs = []model.PricingConfig{}
	}
	RespondJSON(w, http.StatusOK, configs)
}

// Create handles POST /pricing
func (h *PricingHandler) Create(w http.ResponseWriter, r *http.Request) {
	orgID, err := uuid.Parse(middleware.GetOrgID(r.Context()))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid org_id in token")
		return
	}

	var req model.CreatePricingConfigRequest
	if !DecodeJSON(w, r, &req) {
		return
	}
	if err := h.validate.Struct(req); err != nil {
		RespondError(w, http.StatusBadRequest, "validation failed: "+err.Error())
		return
	}

	cfg := &model.PricingConfig{
		OrgID:                &orgID,
		Provider:             req.Provider,
		Model:                req.Model,
		InputPricePerMToken:  req.InputPricePerMToken,
		OutputPricePerMToken: req.OutputPricePerMToken,
		ComputePricePerSec:   req.ComputePricePerSec,
	}
	if req.EffectiveFrom != nil {
		if t, parseErr := time.Parse(time.RFC3339, *req.EffectiveFrom); parseErr == nil {
			cfg.EffectiveFrom = t
		}
	}

	result, err := h.repo.Create(r.Context(), cfg)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to create pricing config: "+err.Error())
		return
	}
	RespondJSON(w, http.StatusCreated, result)
}

// Deactivate handles DELETE /pricing/{configID}
func (h *PricingHandler) Deactivate(w http.ResponseWriter, r *http.Request) {
	configID, err := uuid.Parse(chi.URLParam(r, "configID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid config ID")
		return
	}

	if err := h.repo.Deactivate(r.Context(), configID); err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to deactivate pricing config")
		return
	}
	RespondJSON(w, http.StatusNoContent, nil)
}
