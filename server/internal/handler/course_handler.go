package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/jobshout/server/internal/middleware"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/service"
)

// CourseHandler exposes Course Generator HTTP routes. Every route is scoped
// to the caller's organisation from the token; a run in another org is a 404.
type CourseHandler struct {
	svc service.CourseService
}

// NewCourseHandler constructs the handler.
func NewCourseHandler(svc service.CourseService) *CourseHandler {
	return &CourseHandler{svc: svc}
}

// CreateRun POST /api/v1/courses/runs
func (h *CourseHandler) CreateRun(w http.ResponseWriter, r *http.Request) {
	var req model.CreateCourseRunRequest
	if !DecodeJSON(w, r, &req) {
		return
	}
	if req.AgentID == uuid.Nil {
		RespondError(w, http.StatusBadRequest, "agent_id is required")
		return
	}
	orgID, ok := courseOrg(w, r)
	if !ok {
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

// ListRuns GET /api/v1/courses/runs
func (h *CourseHandler) ListRuns(w http.ResponseWriter, r *http.Request) {
	orgID, ok := courseOrg(w, r)
	if !ok {
		return
	}
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	perPage, _ := strconv.Atoi(r.URL.Query().Get("per_page"))
	result, err := h.svc.ListRuns(r.Context(), orgID, model.PaginationParams{Page: page, PerPage: perPage})
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to list course runs")
		return
	}
	RespondJSON(w, http.StatusOK, result)
}

// GetRun GET /api/v1/courses/runs/{runID}
func (h *CourseHandler) GetRun(w http.ResponseWriter, r *http.Request) {
	runID, orgID, ok := courseRunOrg(w, r)
	if !ok {
		return
	}
	run, err := h.svc.GetRun(r.Context(), runID, orgID)
	if err != nil {
		courseError(w, err)
		return
	}
	RespondJSON(w, http.StatusOK, run)
}

// ListChapters GET /api/v1/courses/runs/{runID}/chapters
func (h *CourseHandler) ListChapters(w http.ResponseWriter, r *http.Request) {
	runID, orgID, ok := courseRunOrg(w, r)
	if !ok {
		return
	}
	chapters, err := h.svc.ListChapters(r.Context(), runID, orgID)
	if err != nil {
		courseError(w, err)
		return
	}
	RespondJSON(w, http.StatusOK, map[string]any{"data": chapters})
}

// CancelRun POST /api/v1/courses/runs/{runID}/cancel
func (h *CourseHandler) CancelRun(w http.ResponseWriter, r *http.Request) {
	runID, orgID, ok := courseRunOrg(w, r)
	if !ok {
		return
	}
	run, err := h.svc.CancelRun(r.Context(), runID, orgID)
	if err != nil {
		courseError(w, err)
		return
	}
	RespondJSON(w, http.StatusOK, run)
}

func courseError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, service.ErrCourseRunNotFound):
		RespondError(w, http.StatusNotFound, "course run not found")
	case errors.Is(err, service.ErrCourseNotCancellable):
		RespondError(w, http.StatusConflict, err.Error())
	default:
		RespondError(w, http.StatusInternalServerError, "course request failed")
	}
}

func courseOrg(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	orgID, err := uuid.Parse(middleware.GetOrgID(r.Context()))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid org_id in token")
		return uuid.Nil, false
	}
	return orgID, true
}

func courseRunOrg(w http.ResponseWriter, r *http.Request) (uuid.UUID, uuid.UUID, bool) {
	runID, err := uuid.Parse(chi.URLParam(r, "runID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid run ID")
		return uuid.Nil, uuid.Nil, false
	}
	orgID, ok := courseOrg(w, r)
	return runID, orgID, ok
}
