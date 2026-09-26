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

type ReviewHandler struct {
	svc      service.ReviewService
	validate *validator.Validate
}

func NewReviewHandler(svc service.ReviewService) *ReviewHandler {
	return &ReviewHandler{svc: svc, validate: validator.New()}
}

func (h *ReviewHandler) ListRepos(w http.ResponseWriter, r *http.Request) {
	RespondJSON(w, http.StatusOK, map[string]any{
		"enabled": h.svc.Enabled(),
		"allowed": h.svc.AllowedRepos(),
	})
}

func (h *ReviewHandler) CreateRun(w http.ResponseWriter, r *http.Request) {
	var req model.CreateReviewRunRequest
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
		if errors.Is(err, service.ErrReviewNotConfigured) {
			RespondError(w, http.StatusServiceUnavailable, err.Error())
			return
		}
		if errors.Is(err, service.ErrReviewRepoNotAllowed) {
			RespondError(w, http.StatusForbidden, err.Error())
			return
		}
		RespondError(w, http.StatusBadRequest, err.Error())
		return
	}
	RespondJSON(w, http.StatusAccepted, run)
}

func (h *ReviewHandler) GetRun(w http.ResponseWriter, r *http.Request) {
	runID, err := uuid.Parse(chi.URLParam(r, "runID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid run ID")
		return
	}
	orgID, err := uuid.Parse(middleware.GetOrgID(r.Context()))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid org_id in token")
		return
	}
	run, err := h.svc.GetRun(r.Context(), runID, orgID)
	if err != nil {
		RespondError(w, http.StatusNotFound, "run not found")
		return
	}
	RespondJSON(w, http.StatusOK, run)
}

func (h *ReviewHandler) ListRuns(w http.ResponseWriter, r *http.Request) {
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
