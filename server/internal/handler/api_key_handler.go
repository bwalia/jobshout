package handler

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"

	"github.com/jobshout/server/internal/apikey"
	"github.com/jobshout/server/internal/middleware"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/repository"
)

// APIKeyHandler manages the org's API keys (org admins only; see main.go).
//
//	GET    /api/v1/api-keys          list (prefix + metadata, never the key)
//	POST   /api/v1/api-keys          create — the response carries the key ONCE
//	DELETE /api/v1/api-keys/{keyID}  revoke
type APIKeyHandler struct {
	svc      *apikey.Service
	validate *validator.Validate
}

func NewAPIKeyHandler(svc *apikey.Service) *APIKeyHandler {
	return &APIKeyHandler{svc: svc, validate: validator.New()}
}

func (h *APIKeyHandler) List(w http.ResponseWriter, r *http.Request) {
	orgID, err := uuid.Parse(middleware.GetOrgID(r.Context()))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid org_id")
		return
	}
	keys, err := h.svc.List(r.Context(), orgID)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to list api keys")
		return
	}
	RespondJSON(w, http.StatusOK, keys)
}

func (h *APIKeyHandler) Create(w http.ResponseWriter, r *http.Request) {
	orgID, err := uuid.Parse(middleware.GetOrgID(r.Context()))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid org_id")
		return
	}
	userID, err := uuid.Parse(middleware.GetUserID(r.Context()))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid user id")
		return
	}
	var req model.CreateAPIKeyRequest
	if !DecodeJSON(w, r, &req) {
		return
	}
	if err := h.validate.Struct(req); err != nil {
		RespondError(w, http.StatusBadRequest, "validation: "+err.Error())
		return
	}
	created, err := h.svc.Create(r.Context(), orgID, userID, req)
	if err != nil {
		RespondError(w, http.StatusBadRequest, err.Error())
		return
	}
	RespondJSON(w, http.StatusCreated, created)
}

func (h *APIKeyHandler) Revoke(w http.ResponseWriter, r *http.Request) {
	orgID, err := uuid.Parse(middleware.GetOrgID(r.Context()))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid org_id")
		return
	}
	keyID, err := uuid.Parse(chi.URLParam(r, "keyID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid key id")
		return
	}
	if err := h.svc.Revoke(r.Context(), orgID, keyID); err != nil {
		if errors.Is(err, repository.ErrAPIKeyNotFound) {
			RespondError(w, http.StatusNotFound, "api key not found")
			return
		}
		RespondError(w, http.StatusInternalServerError, "failed to revoke api key")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
