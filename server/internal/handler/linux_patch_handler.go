package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"

	"github.com/jobshout/server/internal/middleware"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/service"
)

// LinuxPatchHandler exposes Linux Patch HTTP routes.
type LinuxPatchHandler struct {
	svc      service.LinuxPatchService
	validate *validator.Validate
}

func NewLinuxPatchHandler(svc service.LinuxPatchService) *LinuxPatchHandler {
	return &LinuxPatchHandler{svc: svc, validate: validator.New()}
}

func (h *LinuxPatchHandler) Status(w http.ResponseWriter, r *http.Request) {
	RespondJSON(w, http.StatusOK, h.svc.Status(r.Context()))
}

func (h *LinuxPatchHandler) CreateRun(w http.ResponseWriter, r *http.Request) {
	var req model.CreateLinuxPatchRunRequest
	if !DecodeJSON(w, r, &req) {
		return
	}
	if err := h.validate.Struct(req); err != nil {
		RespondError(w, http.StatusBadRequest, "validation failed: "+err.Error())
		return
	}
	orgID, err := uuid.Parse(middleware.GetOrgID(r.Context()))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid org_id in token")
		return
	}
	var requestedBy *uuid.UUID
	if userID, err := uuid.Parse(middleware.GetUserID(r.Context())); err == nil {
		requestedBy = &userID
	}
	run, err := h.svc.CreateRun(r.Context(), req, orgID, requestedBy)
	if err != nil {
		RespondError(w, http.StatusBadRequest, err.Error())
		return
	}
	RespondJSON(w, http.StatusAccepted, run)
}

func (h *LinuxPatchHandler) ListRuns(w http.ResponseWriter, r *http.Request) {
	orgID, err := uuid.Parse(middleware.GetOrgID(r.Context()))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid org_id in token")
		return
	}
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	perPage, _ := strconv.Atoi(r.URL.Query().Get("per_page"))
	result, err := h.svc.ListRuns(r.Context(), orgID, model.PaginationParams{Page: page, PerPage: perPage})
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to list runs: "+err.Error())
		return
	}
	RespondJSON(w, http.StatusOK, result)
}

func (h *LinuxPatchHandler) GetRun(w http.ResponseWriter, r *http.Request) {
	runID, orgID, ok := h.parseRunOrg(w, r)
	if !ok {
		return
	}
	run, err := h.svc.GetRun(r.Context(), runID, orgID)
	if err != nil {
		RespondError(w, http.StatusNotFound, "run not found")
		return
	}
	RespondJSON(w, http.StatusOK, run)
}

func (h *LinuxPatchHandler) CancelRun(w http.ResponseWriter, r *http.Request) {
	runID, orgID, ok := h.parseRunOrg(w, r)
	if !ok {
		return
	}
	run, err := h.svc.CancelRun(r.Context(), runID, orgID)
	if err != nil {
		if errors.Is(err, service.ErrLinuxPatchRunNotFound) {
			RespondError(w, http.StatusNotFound, "run not found")
			return
		}
		if errors.Is(err, service.ErrLinuxPatchNotCancellable) {
			RespondError(w, http.StatusConflict, err.Error())
			return
		}
		RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	RespondJSON(w, http.StatusOK, run)
}

func (h *LinuxPatchHandler) parseRunOrg(w http.ResponseWriter, r *http.Request) (uuid.UUID, uuid.UUID, bool) {
	runID, err := uuid.Parse(chi.URLParam(r, "runID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid run ID")
		return uuid.Nil, uuid.Nil, false
	}
	orgID, err := uuid.Parse(middleware.GetOrgID(r.Context()))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid org_id in token")
		return uuid.Nil, uuid.Nil, false
	}
	return runID, orgID, true
}
