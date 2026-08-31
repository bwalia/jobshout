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
	repo     repository.LLMProviderRepository
	router   *llm.Router
	validate *validator.Validate
	// autoEnabled mirrors AUTO_MODEL_SELECTION so the model picker only offers
	// "Auto" when the server would actually honour it.
	autoEnabled bool
}

func NewLLMProviderHandler(repo repository.LLMProviderRepository, router *llm.Router, autoEnabled bool) *LLMProviderHandler {
	return &LLMProviderHandler{repo: repo, router: router, validate: validator.New(), autoEnabled: autoEnabled}
}

// ListBuiltin returns the providers registered in the LLM router (env-based).
func (h *LLMProviderHandler) ListBuiltin(w http.ResponseWriter, r *http.Request) {
	RespondJSON(w, http.StatusOK, h.router.RegisteredProviders())
}

// AvailableModelsResponse is the payload behind the per-agent model picker.
type AvailableModelsResponse struct {
	Auto      AutoOption           `json:"auto"`
	Providers []ProviderModelGroup `json:"providers"`
}

// AutoOption describes the "let the platform choose" entry.
type AutoOption struct {
	Available bool   `json:"available"`
	Label     string `json:"label"`
}

// ProviderModelGroup is one provider's models, ready to render as an optgroup.
type ProviderModelGroup struct {
	Provider  string           `json:"provider"`
	IsDefault bool             `json:"is_default"`
	Source    string           `json:"source"`
	Models    []AvailableModel `json:"models"`
	Error     string           `json:"error,omitempty"`
}

// AvailableModel is one selectable model.
type AvailableModel struct {
	Name           string   `json:"name"`
	ContextTokens  int      `json:"context_tokens"`
	ParameterSize  string   `json:"parameter_size,omitempty"`
	Capabilities   []string `json:"capabilities"`
	SupportsTools  bool     `json:"supports_tools"`
	SupportsVision bool     `json:"supports_vision"`
}

// ListModels returns every model each registered provider can actually run, for
// the per-agent model picker.
//
// Embedding-only models are filtered out here rather than in discovery, so the
// llm layer stays faithful to what a provider reported and the embedder code can
// still use it. Providers that are not registered never appear at all; Error is
// reserved for a registered provider whose probe failed.
func (h *LLMProviderHandler) ListModels(w http.ResponseWriter, r *http.Request) {
	discovered := h.router.AvailableModels(r.Context())

	groups := make([]ProviderModelGroup, 0, len(discovered))
	for _, pm := range discovered {
		models := make([]AvailableModel, 0, len(pm.Models))
		for _, m := range pm.Models {
			if m.IsEmbeddingOnly() {
				continue
			}
			models = append(models, AvailableModel{
				Name:           m.Name,
				ContextTokens:  m.ContextTokens,
				ParameterSize:  m.ParameterSize,
				Capabilities:   m.Capabilities,
				SupportsTools:  m.SupportsTools(),
				SupportsVision: m.SupportsVision(),
			})
		}
		groups = append(groups, ProviderModelGroup{
			Provider:  pm.Provider,
			IsDefault: pm.IsDefault,
			Source:    pm.Source,
			Models:    models,
			Error:     pm.Error,
		})
	}

	RespondJSON(w, http.StatusOK, AvailableModelsResponse{
		Auto: AutoOption{
			Available: h.autoEnabled,
			Label:     "Auto — best model per task",
		},
		Providers: groups,
	})
}

// maskProviderKey redacts the stored secret before it leaves the API. Clients
// never need the full key back — they set it, we keep it.
func maskProviderKey(p *model.LLMProviderConfig) {
	if len(p.APIKey) > 8 {
		p.APIKey = p.APIKey[:4] + "****" + p.APIKey[len(p.APIKey)-4:]
	} else if p.APIKey != "" {
		p.APIKey = "****"
	}
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
	// This endpoint used to return every provider's API key verbatim.
	for i := range providers {
		maskProviderKey(&providers[i])
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

	maskProviderKey(p)

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

	maskProviderKey(p)

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
	maskProviderKey(p)
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
