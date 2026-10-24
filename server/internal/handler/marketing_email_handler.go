package handler

import (
	"context"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/jobshout/server/internal/integration/adapters/opsapi"
	"github.com/jobshout/server/internal/marketingemail"
	"github.com/jobshout/server/internal/middleware"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/repository"
)

// MarketingEmailRuns is what the Marketing Email Writer tab reads and does.
// Satisfied by *marketingemail.Service.
type MarketingEmailRuns interface {
	List(ctx context.Context, orgID uuid.UUID, limit int) ([]model.MarketingEmailRun, error)
	Get(ctx context.Context, orgID, id uuid.UUID) (*model.MarketingEmailRun, error)
	ReGenerate(ctx context.Context, orgID, id uuid.UUID) (*model.MarketingEmailRun, error)
	Unpublish(ctx context.Context, orgID, id uuid.UUID) (*model.MarketingEmailRun, error)
}

// MarketingEmailHandler serves the Marketing Email Writer's tab: every run, its
// rendered HTML and browser-version URL, re-generate (upsert) and unpublish
// (archive). Emails start through POST /tasks/launch like any other agent.
// Every route is scoped to the caller's org.
type MarketingEmailHandler struct {
	runs MarketingEmailRuns
}

// NewMarketingEmailHandler constructs the handler.
func NewMarketingEmailHandler(runs MarketingEmailRuns) *MarketingEmailHandler {
	return &MarketingEmailHandler{runs: runs}
}

// marketingEmailRunsLimit is how much history the tab shows.
const marketingEmailRunsLimit = 50

// ListRuns GET /api/v1/marketing-emails/runs
func (h *MarketingEmailHandler) ListRuns(w http.ResponseWriter, r *http.Request) {
	orgID, ok := marketingEmailOrg(w, r)
	if !ok {
		return
	}
	runs, err := h.runs.List(r.Context(), orgID, marketingEmailRunsLimit)
	if err != nil {
		marketingEmailError(w, err)
		return
	}
	// The list omits each email's full HTML; Get returns it.
	for i := range runs {
		runs[i].ContentHTML = ""
	}
	RespondJSON(w, http.StatusOK, map[string]any{"data": runs})
}

// GetRun GET /api/v1/marketing-emails/runs/{runID}
func (h *MarketingEmailHandler) GetRun(w http.ResponseWriter, r *http.Request) {
	orgID, id, ok := marketingEmailRun(w, r)
	if !ok {
		return
	}
	run, err := h.runs.Get(r.Context(), orgID, id)
	if err != nil {
		marketingEmailError(w, err)
		return
	}
	RespondJSON(w, http.StatusOK, run)
}

// Regenerate POST /api/v1/marketing-emails/runs/{runID}/regenerate — writes the
// email again and updates the same CMS page.
func (h *MarketingEmailHandler) Regenerate(w http.ResponseWriter, r *http.Request) {
	orgID, id, ok := marketingEmailRun(w, r)
	if !ok {
		return
	}
	run, err := h.runs.ReGenerate(r.Context(), orgID, id)
	if err != nil {
		marketingEmailError(w, err)
		return
	}
	RespondJSON(w, http.StatusAccepted, run)
}

// Unpublish POST /api/v1/marketing-emails/runs/{runID}/unpublish — archives the
// CMS page.
func (h *MarketingEmailHandler) Unpublish(w http.ResponseWriter, r *http.Request) {
	orgID, id, ok := marketingEmailRun(w, r)
	if !ok {
		return
	}
	run, err := h.runs.Unpublish(r.Context(), orgID, id)
	if err != nil {
		marketingEmailError(w, err)
		return
	}
	RespondJSON(w, http.StatusOK, run)
}

func marketingEmailOrg(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	orgID, err := uuid.Parse(middleware.GetOrgID(r.Context()))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid org_id in token")
		return uuid.Nil, false
	}
	return orgID, true
}

func marketingEmailRun(w http.ResponseWriter, r *http.Request) (uuid.UUID, uuid.UUID, bool) {
	orgID, ok := marketingEmailOrg(w, r)
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

func marketingEmailError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, repository.ErrMarketingEmailRunNotFound):
		RespondError(w, http.StatusNotFound, "marketing email run not found")
	case errors.Is(err, marketingemail.ErrNoRuns):
		RespondError(w, http.StatusServiceUnavailable, err.Error())
	case errors.Is(err, marketingemail.ErrBusy), errors.Is(err, marketingemail.ErrNotPublished):
		RespondError(w, http.StatusConflict, err.Error())
	case errors.Is(err, marketingemail.ErrDestination):
		RespondError(w, http.StatusBadRequest, err.Error())
	case opsapi.IsUpdateForbidden(err):
		RespondError(w, http.StatusBadGateway, err.Error())
	default:
		RespondError(w, http.StatusInternalServerError, "marketing emails: "+err.Error())
	}
}
