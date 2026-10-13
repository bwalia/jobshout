package handler

import (
	"context"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/jobshout/server/internal/middleware"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/repository"
	"github.com/jobshout/server/internal/video"
)

// VideoRuns is what the Video Producer tab reads and does. Satisfied by
// *video.Service.
type VideoRuns interface {
	List(ctx context.Context, orgID uuid.UUID, limit int) ([]model.VideoRun, error)
	Get(ctx context.Context, orgID, id uuid.UUID) (*model.VideoRun, error)
	Cancel(ctx context.Context, orgID, id uuid.UUID) (*model.VideoRun, error)
}

// VideoHandler serves the Video Producer's tab: every run and where it has got
// to. Videos start through POST /tasks/launch like any other agent. Every
// route is scoped to the caller's org.
type VideoHandler struct {
	runs VideoRuns
}

// NewVideoHandler constructs the handler.
func NewVideoHandler(runs VideoRuns) *VideoHandler {
	return &VideoHandler{runs: runs}
}

// videoRunsLimit is how much history the tab shows.
const videoRunsLimit = 50

// ListRuns GET /api/v1/videos/runs
func (h *VideoHandler) ListRuns(w http.ResponseWriter, r *http.Request) {
	orgID, ok := videoOrg(w, r)
	if !ok {
		return
	}
	runs, err := h.runs.List(r.Context(), orgID, videoRunsLimit)
	if err != nil {
		videoError(w, err)
		return
	}
	RespondJSON(w, http.StatusOK, map[string]any{"data": runs})
}

// GetRun GET /api/v1/videos/runs/{runID}
func (h *VideoHandler) GetRun(w http.ResponseWriter, r *http.Request) {
	orgID, id, ok := videoRun(w, r)
	if !ok {
		return
	}
	run, err := h.runs.Get(r.Context(), orgID, id)
	if err != nil {
		videoError(w, err)
		return
	}
	RespondJSON(w, http.StatusOK, run)
}

// CancelRun POST /api/v1/videos/runs/{runID}/cancel
func (h *VideoHandler) CancelRun(w http.ResponseWriter, r *http.Request) {
	orgID, id, ok := videoRun(w, r)
	if !ok {
		return
	}
	run, err := h.runs.Cancel(r.Context(), orgID, id)
	if err != nil {
		videoError(w, err)
		return
	}
	RespondJSON(w, http.StatusOK, run)
}

func videoOrg(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	orgID, err := uuid.Parse(middleware.GetOrgID(r.Context()))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid org_id in token")
		return uuid.Nil, false
	}
	return orgID, true
}

func videoRun(w http.ResponseWriter, r *http.Request) (uuid.UUID, uuid.UUID, bool) {
	orgID, ok := videoOrg(w, r)
	if !ok {
		return uuid.Nil, uuid.Nil, false
	}
	id, err := uuid.Parse(chi.URLParam(r, "runID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid run ID")
		return uuid.Nil, uuid.Nil, false
	}
	return orgID, id, true
}

func videoError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, repository.ErrVideoRunNotFound):
		RespondError(w, http.StatusNotFound, "video run not found")
	case errors.Is(err, video.ErrNoRuns):
		RespondError(w, http.StatusServiceUnavailable, err.Error())
	default:
		RespondError(w, http.StatusInternalServerError, "video runs: "+err.Error())
	}
}
