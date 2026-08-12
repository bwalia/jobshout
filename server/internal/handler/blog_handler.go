package handler

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"

	"github.com/jobshout/server/internal/middleware"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/service"
)

// BlogHandler exposes the article-generator endpoints.
type BlogHandler struct {
	svc      service.BlogService
	validate *validator.Validate
}

// NewBlogHandler creates a BlogHandler.
func NewBlogHandler(svc service.BlogService) *BlogHandler {
	return &BlogHandler{svc: svc, validate: validator.New()}
}

// Generate handles POST /api/v1/blogs/generate. The pipeline runs in the
// background, so this returns 202 with the pending run; poll GetRun for
// progress. Generation never touches GitHub — see Publish.
func (h *BlogHandler) Generate(w http.ResponseWriter, r *http.Request) {
	var req model.GenerateBlogRequest
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
	var triggeredBy *uuid.UUID
	if parsed, err := uuid.Parse(middleware.GetUserID(r.Context())); err == nil {
		triggeredBy = &parsed
	}

	run, err := h.svc.Generate(r.Context(), orgID, triggeredBy, "api", req)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	RespondJSON(w, http.StatusAccepted, run)
}

// Publish handles POST /api/v1/blogs/runs/{runID}/publish — commits a completed
// run's articles to the content repository and opens a pull request.
func (h *BlogHandler) Publish(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "runID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid run ID")
		return
	}
	orgID, err := uuid.Parse(middleware.GetOrgID(r.Context()))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid org_id in token")
		return
	}

	run, err := h.svc.Publish(r.Context(), orgID, id)
	if err != nil {
		// Publishing is refused for reasons the caller can act on (not
		// configured, wrong status, already published), so surface the message.
		RespondError(w, http.StatusBadRequest, err.Error())
		return
	}
	RespondJSON(w, http.StatusOK, run)
}

// Delete handles DELETE /api/v1/blogs/runs/{runID} — forgets a run and the
// articles it produced. Drafts already in the CMS are not touched.
func (h *BlogHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "runID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid run ID")
		return
	}
	orgID, err := uuid.Parse(middleware.GetOrgID(r.Context()))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid org_id in token")
		return
	}

	if err := h.svc.Delete(r.Context(), orgID, id); err != nil {
		// Refused for reasons the caller can act on — wrong org, still running,
		// already gone — so surface the message rather than a bare 500.
		RespondError(w, http.StatusBadRequest, err.Error())
		return
	}
	RespondJSON(w, http.StatusOK, map[string]any{"deleted": true})
}

// Retry handles POST /api/v1/blogs/runs/{runID}/retry — runs a failed run's
// topics again on the same run. Asynchronous like Generate; poll GetRun.
func (h *BlogHandler) Retry(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "runID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid run ID")
		return
	}
	orgID, err := uuid.Parse(middleware.GetOrgID(r.Context()))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid org_id in token")
		return
	}

	run, err := h.svc.Retry(r.Context(), orgID, id)
	if err != nil {
		RespondError(w, http.StatusBadRequest, err.Error())
		return
	}
	RespondJSON(w, http.StatusAccepted, run)
}

// Config handles GET /api/v1/blogs/config — what the UI needs to decide which
// actions to offer.
func (h *BlogHandler) Config(w http.ResponseWriter, r *http.Request) {
	RespondJSON(w, http.StatusOK, map[string]any{
		"can_publish": h.svc.CanPublish(),
	})
}

// GetRun handles GET /api/v1/blogs/runs/{runID}.
func (h *BlogHandler) GetRun(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "runID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid run ID")
		return
	}
	run, err := h.svc.GetByID(r.Context(), id)
	if err != nil {
		RespondError(w, http.StatusNotFound, "blog run not found")
		return
	}
	RespondJSON(w, http.StatusOK, run)
}

// ListRuns handles GET /api/v1/blogs/runs.
func (h *BlogHandler) ListRuns(w http.ResponseWriter, r *http.Request) {
	orgID, err := uuid.Parse(middleware.GetOrgID(r.Context()))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid org_id in token")
		return
	}
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	perPage, _ := strconv.Atoi(r.URL.Query().Get("per_page"))
	params := model.PaginationParams{Page: page, PerPage: perPage}

	result, err := h.svc.ListByOrg(r.Context(), orgID, params)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to list blog runs")
		return
	}
	RespondJSON(w, http.StatusOK, result)
}

// ListArticles handles GET /api/v1/blogs/runs/{runID}/articles — full markdown
// for every article the run produced.
func (h *BlogHandler) ListArticles(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "runID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid run ID")
		return
	}
	articles, err := h.svc.ListArticles(r.Context(), id)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to list articles")
		return
	}
	RespondJSON(w, http.StatusOK, articles)
}

// GetArticle handles GET /api/v1/blogs/articles/{articleID}.
func (h *BlogHandler) GetArticle(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "articleID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid article ID")
		return
	}
	article, err := h.svc.GetArticle(r.Context(), id)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to load article")
		return
	}
	if article == nil {
		RespondError(w, http.StatusNotFound, "article not found")
		return
	}
	RespondJSON(w, http.StatusOK, article)
}
