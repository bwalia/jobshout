package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"

	"github.com/jobshout/server/internal/llm"
	"github.com/jobshout/server/internal/middleware"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/repository"
)

type LLMProviderHandler struct {
	repo      repository.LLMProviderRepository
	router    *llm.Router
	validate  *validator.Validate
}

func NewLLMProviderHandler(repo repository.LLMProviderRepository, router *llm.Router) *LLMProviderHandler {
	return &LLMProviderHandler{repo: repo, router: router, validate: validator.New()}
}

// ListBuiltin returns the providers registered in the LLM router (env-based).
func (h *LLMProviderHandler) ListBuiltin(w http.ResponseWriter, r *http.Request) {
	RespondJSON(w, http.StatusOK, h.router.RegisteredProviders())
}

// List returns all user-managed LLM provider configs for the org.
func (h *LLMProviderHandler) List(w http.ResponseWriter, r *http.Request) {
	orgID, err := uuid.Parse(middleware.GetOrgID(r.Context()))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid org_id")
		return
	}

	providers, err := h.repo.List(r.Context(), orgID)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to list providers")
		return
	}
	if providers == nil {
		providers = []model.LLMProviderConfig{}
	}
	RespondJSON(w, http.StatusOK, providers)
}

// Create adds a new LLM provider config.
func (h *LLMProviderHandler) Create(w http.ResponseWriter, r *http.Request) {
	orgID, err := uuid.Parse(middleware.GetOrgID(r.Context()))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid org_id")
		return
	}
	userID, _ := uuid.Parse(middleware.GetUserID(r.Context()))

	var req model.CreateLLMProviderRequest
	if !DecodeJSON(w, r, &req) {
		return
	}
	if err := h.validate.Struct(req); err != nil {
		RespondError(w, http.StatusBadRequest, "validation: "+err.Error())
		return
	}

	// If this is set as default, clear existing default
	if req.IsDefault {
		_ = h.repo.ClearDefault(r.Context(), orgID)
	}

	p := &model.LLMProviderConfig{
		ID:           uuid.New(),
		OrgID:        orgID,
		Name:         req.Name,
		ProviderType: req.ProviderType,
		BaseURL:      req.BaseURL,
		APIKey:       req.APIKey,
		DefaultModel: req.DefaultModel,
		IsDefault:    req.IsDefault,
		IsActive:     true,
		ConfigJSON:   req.ConfigJSON,
		CreatedBy:    &userID,
	}

	if err := h.repo.Create(r.Context(), p); err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to create provider: "+err.Error())
		return
	}

	// Mask key in response
	if len(p.APIKey) > 8 {
		p.APIKey = p.APIKey[:4] + "****" + p.APIKey[len(p.APIKey)-4:]
	} else if p.APIKey != "" {
		p.APIKey = "****"
	}

	RespondJSON(w, http.StatusCreated, p)
}

// GetByID returns a single provider config.
func (h *LLMProviderHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "providerID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid provider ID")
		return
	}

	p, err := h.repo.GetByID(r.Context(), id)
	if err != nil {
		RespondError(w, http.StatusNotFound, "provider not found")
		return
	}

	// Mask key
	if len(p.APIKey) > 8 {
		p.APIKey = p.APIKey[:4] + "****" + p.APIKey[len(p.APIKey)-4:]
	} else if p.APIKey != "" {
		p.APIKey = "****"
	}

	RespondJSON(w, http.StatusOK, p)
}

// Update modifies a provider config.
func (h *LLMProviderHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "providerID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid provider ID")
		return
	}

	var req model.UpdateLLMProviderRequest
	if !DecodeJSON(w, r, &req) {
		return
	}

	if req.IsDefault != nil && *req.IsDefault {
		orgID, _ := uuid.Parse(middleware.GetOrgID(r.Context()))
		_ = h.repo.ClearDefault(r.Context(), orgID)
	}

	p, err := h.repo.Update(r.Context(), id, req)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to update provider")
		return
	}
	RespondJSON(w, http.StatusOK, p)
}

// Delete removes a provider config.
func (h *LLMProviderHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "providerID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid provider ID")
		return
	}

	if err := h.repo.Delete(r.Context(), id); err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to delete provider")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
