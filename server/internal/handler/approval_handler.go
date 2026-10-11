package handler

import (
	"errors"
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
	if out == nil {
		out = []model.Approval{}
	}
	RespondJSON(w, http.StatusOK, out)
}

// Get returns a single approval by id, scoped to the caller's org.
func (h *ApprovalHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "approvalID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid approval id")
		return
	}
	out, ok := h.load(w, r, id)
	if !ok {
		return
	}
	RespondJSON(w, http.StatusOK, out)
}

// load fetches an approval and hides one from another org as not found.
func (h *ApprovalHandler) load(w http.ResponseWriter, r *http.Request, id uuid.UUID) (*model.Approval, bool) {
	out, err := h.svc.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, service.ErrApprovalNotFound) {
			RespondError(w, http.StatusNotFound, "approval not found")
		} else {
			RespondError(w, http.StatusInternalServerError, err.Error())
		}
		return nil, false
	}
	if orgID, err := uuid.Parse(middleware.GetOrgID(r.Context())); err == nil && out.OrgID != orgID {
		RespondError(w, http.StatusNotFound, "approval not found")
		return nil, false
	}
	return out, true
}

// Decide records an approve/reject decision and answers 202 Accepted with the
// decided approval at once. The paused execution resumes in the background;
// poll GET /task-runs/{id} (or the execution) for its outcome. Deciding an
// approval that is already decided answers 409 Conflict.
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
	if _, ok := h.load(w, r, id); !ok {
		return
	}

	out, err := h.svc.Decide(r.Context(), id, deciderID, req.Decision, req.Reason)
	switch {
	case errors.Is(err, service.ErrApprovalAlreadyDecided):
		RespondError(w, http.StatusConflict, err.Error())
		return
	case errors.Is(err, service.ErrApprovalNotFound):
		RespondError(w, http.StatusNotFound, "approval not found")
		return
	case err != nil:
		RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	RespondJSON(w, http.StatusAccepted, out)
}
