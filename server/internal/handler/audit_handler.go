package handler

import (
	"net/http"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/middleware"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/repository"
)

// AuditHandler exposes audit log endpoints.
type AuditHandler struct {
	repo repository.AuditRepository
}

// NewAuditHandler creates an AuditHandler.
func NewAuditHandler(repo repository.AuditRepository) *AuditHandler {
	return &AuditHandler{repo: repo}
}

// ListActions handles GET /audit/actions
func (h *AuditHandler) ListActions(w http.ResponseWriter, r *http.Request) {
	orgID, err := uuid.Parse(middleware.GetOrgID(r.Context()))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid org_id in token")
		return
	}

	params := model.AuditQueryParams{
		Action:   r.URL.Query().Get("action"),
		Resource: r.URL.Query().Get("resource"),
		Limit:    parseLimit(r, 100),
	}

	logs, err := h.repo.ListActions(r.Context(), orgID, params)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to list audit actions")
		return
	}
	if logs == nil {
		logs = []model.AuditLog{}
	}
	RespondJSON(w, http.StatusOK, logs)
}

// ListLogins handles GET /audit/logins
func (h *AuditHandler) ListLogins(w http.ResponseWriter, r *http.Request) {
	orgID, err := uuid.Parse(middleware.GetOrgID(r.Context()))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid org_id in token")
		return
	}

	limit := parseLimit(r, 100)
	logs, err := h.repo.ListLogins(r.Context(), orgID, limit)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to list login audit")
		return
	}
	if logs == nil {
		logs = []model.LoginAuditLog{}
	}
	RespondJSON(w, http.StatusOK, logs)
}
