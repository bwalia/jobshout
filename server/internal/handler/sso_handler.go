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

// SSOHandler exposes SSO/OIDC configuration and login endpoints.
type SSOHandler struct {
	svc      service.SSOService
	jwtSvc   service.JWTService
	validate *validator.Validate
}

// NewSSOHandler creates an SSOHandler.
func NewSSOHandler(svc service.SSOService, jwtSvc service.JWTService) *SSOHandler {
	return &SSOHandler{svc: svc, jwtSvc: jwtSvc, validate: validator.New()}
}

// ListConfigs handles GET /sso/configs
func (h *SSOHandler) ListConfigs(w http.ResponseWriter, r *http.Request) {
	orgID, err := uuid.Parse(middleware.GetOrgID(r.Context()))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid org_id in token")
		return
	}

	configs, err := h.svc.ListConfigs(r.Context(), orgID)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to list SSO configs")
		return
	}
	if configs == nil {
		configs = []model.SSOConfig{}
	}
	RespondJSON(w, http.StatusOK, configs)
}

// CreateConfig handles POST /sso/configs
func (h *SSOHandler) CreateConfig(w http.ResponseWriter, r *http.Request) {
	orgID, err := uuid.Parse(middleware.GetOrgID(r.Context()))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid org_id in token")
		return
	}

	var req model.CreateSSOConfigRequest
	if !DecodeJSON(w, r, &req) {
		return
	}
	if err := h.validate.Struct(req); err != nil {
		RespondError(w, http.StatusBadRequest, "validation failed: "+err.Error())
		return
	}

	cfg, err := h.svc.CreateConfig(r.Context(), orgID, req)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to create SSO config: "+err.Error())
		return
	}
	RespondJSON(w, http.StatusCreated, cfg)
}

// DeleteConfig handles DELETE /sso/configs/{configID}
func (h *SSOHandler) DeleteConfig(w http.ResponseWriter, r *http.Request) {
	configID, err := uuid.Parse(chi.URLParam(r, "configID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid config ID")
		return
	}

	if err := h.svc.DeleteConfig(r.Context(), configID); err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to delete SSO config")
		return
	}
	RespondJSON(w, http.StatusNoContent, nil)
}

// Authorize handles GET /sso/authorize?provider=...&state=...
func (h *SSOHandler) Authorize(w http.ResponseWriter, r *http.Request) {
	orgID, err := uuid.Parse(middleware.GetOrgID(r.Context()))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid org_id in token")
		return
	}

	provider := r.URL.Query().Get("provider")
	state := r.URL.Query().Get("state")
	if provider == "" || state == "" {
		RespondError(w, http.StatusBadRequest, "provider and state query params required")
		return
	}

	authURL, err := h.svc.GetAuthorizationURL(r.Context(), orgID, provider, state)
	if err != nil {
		RespondError(w, http.StatusBadRequest, err.Error())
		return
	}
	RespondJSON(w, http.StatusOK, map[string]string{"authorization_url": authURL})
}

// Callback handles POST /sso/callback
func (h *SSOHandler) Callback(w http.ResponseWriter, r *http.Request) {
	orgID, err := uuid.Parse(middleware.GetOrgID(r.Context()))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid org_id in token")
		return
	}

	var req model.SSOCallbackRequest
	if !DecodeJSON(w, r, &req) {
		return
	}
	if err := h.validate.Struct(req); err != nil {
		RespondError(w, http.StatusBadRequest, "validation failed: "+err.Error())
		return
	}

	// Extract provider from state or query param.
	provider := r.URL.Query().Get("provider")
	if provider == "" {
		RespondError(w, http.StatusBadRequest, "provider query param required")
		return
	}

	user, err := h.svc.ExchangeCode(r.Context(), orgID, provider, req.Code)
	if err != nil {
		RespondError(w, http.StatusUnauthorized, err.Error())
		return
	}

	// Generate JWT tokens for the authenticated user.
	accessToken, err := h.jwtSvc.GenerateAccessToken(user.ID, user.Email, user.OrgID, user.Role)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to generate token")
		return
	}

	RespondJSON(w, http.StatusOK, map[string]any{
		"user":         user,
		"access_token": accessToken,
	})
}

// ListLoginAudit handles GET /sso/login-audit
func (h *SSOHandler) ListLoginAudit(w http.ResponseWriter, r *http.Request) {
	orgID, err := uuid.Parse(middleware.GetOrgID(r.Context()))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid org_id in token")
		return
	}

	limit := parseLimit(r, 100)
	logs, err := h.svc.ListLoginAudit(r.Context(), orgID, limit)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to list login audit")
		return
	}
	if logs == nil {
		logs = []model.LoginAuditLog{}
	}
	RespondJSON(w, http.StatusOK, logs)
}
