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

// ApprovalHandler exposes the human-in-the-loop approvals API.
type ApprovalHandler struct {
	svc      service.ApprovalService
	validate *validator.Validate
}

func NewApprovalHandler(svc service.ApprovalService) *ApprovalHandler {
	return &ApprovalHandler{svc: svc, validate: validator.New()}
}

// List returns the org's approvals, optionally filtered by ?status=pending.
func (h *ApprovalHandler) List(w http.ResponseWriter, r *http.Request) {
	orgID, err := uuid.Parse(middleware.GetOrgID(r.Context()))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid org_id")
		return
	}
	status := r.URL.Query().Get("status")
	out, err := h.svc.List(r.Context(), orgID, status)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	RespondJSON(w, http.StatusOK, out)
}

// Get returns a single approval by id.
func (h *ApprovalHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "approvalID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid approval id")
		return
	}
	out, err := h.svc.Get(r.Context(), id)
	if err != nil {
		RespondError(w, http.StatusNotFound, "approval not found")
		return
	}
	RespondJSON(w, http.StatusOK, out)
}

// Decide records an approve/reject decision and resumes the paused execution.
func (h *ApprovalHandler) Decide(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "approvalID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid approval id")
		return
	}
	deciderID, err := uuid.Parse(middleware.GetUserID(r.Context()))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid user id")
		return
	}

	var req model.DecideApprovalRequest
	if !DecodeJSON(w, r, &req) {
		return
	}
	if err := h.validate.Struct(req); err != nil {
		RespondError(w, http.StatusBadRequest, "validation: "+err.Error())
		return
	}

	out, err := h.svc.Decide(r.Context(), id, deciderID, req.Decision, req.Reason)
	if err != nil {
		RespondError(w, http.StatusBadRequest, err.Error())
		return
	}
	RespondJSON(w, http.StatusOK, out)
}
