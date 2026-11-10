package handler

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"

	"github.com/jobshout/server/internal/middleware"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/service"
)

// SEOHandler exposes SEO Analyst HTTP routes.
type SEOHandler struct {
	svc      service.SEOService
	validate *validator.Validate
}

// NewSEOHandler constructs the handler.
func NewSEOHandler(svc service.SEOService) *SEOHandler {
	return &SEOHandler{svc: svc, validate: validator.New()}
}

// Status GET /api/v1/seo/status
func (h *SEOHandler) Status(w http.ResponseWriter, r *http.Request) {
	RespondJSON(w, http.StatusOK, h.svc.Status(r.Context()))
}

// ListConnections GET /api/v1/seo/connections
func (h *SEOHandler) ListConnections(w http.ResponseWriter, r *http.Request) {
	orgID, err := uuid.Parse(middleware.GetOrgID(r.Context()))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid org_id in token")
		return
	}
	rows, err := h.svc.ListConnections(r.Context(), orgID)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	RespondJSON(w, http.StatusOK, map[string]any{"data": rows})
}

type upsertSEOConnectionRequest struct {
	Name          string `json:"name"`
	Repo          string `json:"repo"`
	DefaultBranch string `json:"default_branch"`
	AccessToken   string `json:"access_token"`
}

// UpsertConnection PUT /api/v1/seo/connections
func (h *SEOHandler) UpsertConnection(w http.ResponseWriter, r *http.Request) {
	var req upsertSEOConnectionRequest
	if !DecodeJSON(w, r, &req) {
		return
	}
	orgID, err := uuid.Parse(middleware.GetOrgID(r.Context()))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid org_id in token")
		return
	}
	var userID *uuid.UUID
	if uid, err := uuid.Parse(middleware.GetUserID(r.Context())); err == nil {
		userID = &uid
	}
	conn, err := h.svc.UpsertConnection(r.Context(), orgID, userID, req.Name, req.Repo, req.DefaultBranch, req.AccessToken)
	if err != nil {
		RespondError(w, http.StatusBadRequest, err.Error())
		return
	}
	RespondJSON(w, http.StatusOK, conn)
}

// DeleteConnection DELETE /api/v1/seo/connections/{name}
func (h *SEOHandler) DeleteConnection(w http.ResponseWriter, r *http.Request) {
	orgID, err := uuid.Parse(middleware.GetOrgID(r.Context()))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid org_id in token")
		return
	}
	name := strings.TrimSpace(chi.URLParam(r, "name"))
	if name == "" {
		RespondError(w, http.StatusBadRequest, "name required")
		return
	}
	if err := h.svc.DeleteConnection(r.Context(), orgID, name); err != nil {
		RespondError(w, http.StatusBadRequest, err.Error())
		return
	}
	RespondJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// CreateRun POST /api/v1/seo/runs
func (h *SEOHandler) CreateRun(w http.ResponseWriter, r *http.Request) {
	var req model.CreateSEORunRequest
	if !DecodeJSON(w, r, &req) {
		return
	}
	// Soften legacy clients that still send only url.
	if strings.TrimSpace(req.URLs) == "" && strings.TrimSpace(req.URL) != "" {
		req.URLs = req.URL
	}
	if strings.TrimSpace(req.Source) == "" {
		if strings.TrimSpace(req.GitHubConnection) != "" {
			req.Source = "github"
		} else {
			req.Source = "urls"
		}
	}
	if strings.TrimSpace(req.Keywords) == "" {
		RespondError(w, http.StatusBadRequest, "keywords are required")
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

// ListRuns GET /api/v1/seo/runs
func (h *SEOHandler) ListRuns(w http.ResponseWriter, r *http.Request) {
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

// GetRun GET /api/v1/seo/runs/{runID}
func (h *SEOHandler) GetRun(w http.ResponseWriter, r *http.Request) {
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

// CancelRun POST /api/v1/seo/runs/{runID}/cancel
func (h *SEOHandler) CancelRun(w http.ResponseWriter, r *http.Request) {
	runID, orgID, ok := h.parseRunOrg(w, r)
	if !ok {
		return
	}
	run, err := h.svc.CancelRun(r.Context(), runID, orgID)
	if err != nil {
		if errors.Is(err, service.ErrSEORunNotFound) {
			RespondError(w, http.StatusNotFound, "run not found")
			return
		}
		if errors.Is(err, service.ErrSEONotCancellable) {
			RespondError(w, http.StatusConflict, err.Error())
			return
		}
		RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	RespondJSON(w, http.StatusOK, run)
}

func (h *SEOHandler) parseRunOrg(w http.ResponseWriter, r *http.Request) (uuid.UUID, uuid.UUID, bool) {
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
