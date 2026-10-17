package handler

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"

	"github.com/jobshout/server/internal/blog"
	"github.com/jobshout/server/internal/middleware"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/service"
)

// BlogHandler exposes the article-generator endpoints.
type BlogHandler struct {
	svc      service.BlogService
	validate *validator.Validate
	// insightsSiteURL is the public jobshout.com site; empty hides links.
	insightsSiteURL string
	// opsapiBaseURL is this ring's OpsAPI, the default for a new website.
	opsapiBaseURL string
	// keysSealed is whether a website's key can be stored encrypted here.
	keysSealed bool
}

// NewBlogHandler creates a BlogHandler.
func NewBlogHandler(svc service.BlogService) *BlogHandler {
	return &BlogHandler{svc: svc, validate: validator.New()}
}

// WithInsightsSiteURL sets the public jobshout.com site for this ring, which the
// UI links published articles to.
func (h *BlogHandler) WithInsightsSiteURL(u string) *BlogHandler {
	h.insightsSiteURL = strings.TrimRight(strings.TrimSpace(u), "/")
	return h
}

// WithWebsiteDefaults sets what the "Add a website" form needs: this ring's
// OpsAPI URL (not a secret) and whether keys are stored encrypted.
func (h *BlogHandler) WithWebsiteDefaults(opsapiBaseURL string, keysSealed bool) *BlogHandler {
	h.opsapiBaseURL = strings.TrimRight(strings.TrimSpace(opsapiBaseURL), "/")
	h.keysSealed = keysSealed
	return h
}

// Generate handles POST /api/v1/blogs/generate. The pipeline runs in the
// background, so this returns 202 with the pending run; poll GetRun for
// progress. Generation never touches GitHub — see Publish.
func (h *BlogHandler) Generate(w http.ResponseWriter, r *http.Request) {
	var req model.GenerateBlogRequest
	if !DecodeJSON(w, r, &req) {
		return
	}
	// Normalize folds the legacy topics field into briefs, so the rest of the
	// pipeline reads one shape regardless of which form the caller sent.
	req.Normalize()
	if err := req.Validate(); err != nil {
		RespondError(w, http.StatusBadRequest, err.Error())
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
	if errors.Is(err, service.ErrBlogDestination) {
		RespondError(w, http.StatusBadRequest, err.Error())
		return
	}
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

// PublishInsights handles POST /api/v1/blogs/runs/{runID}/publish-insights —
// files a completed run's articles in the JobShout.com Insights review queue.
func (h *BlogHandler) PublishInsights(w http.ResponseWriter, r *http.Request) {
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

	run, err := h.svc.PublishInsights(r.Context(), orgID, id)
	if err != nil {
		RespondError(w, http.StatusBadRequest, err.Error())
		return
	}
	RespondJSON(w, http.StatusOK, run)
}

// PublishLive handles POST /api/v1/blogs/runs/{runID}/publish-live — makes a
// JobShout.com Content Writer run's CMS drafts public and publishes its
// articles on jobshout.com.
func (h *BlogHandler) PublishLive(w http.ResponseWriter, r *http.Request) {
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
	run, err := h.svc.PublishLive(r.Context(), orgID, id)
	if err != nil {
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

// Cancel handles POST /api/v1/blogs/runs/{runID}/cancel — stops a run that is
// still writing. Returns the failed run; poll is not required.
func (h *BlogHandler) Cancel(w http.ResponseWriter, r *http.Request) {
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

	run, err := h.svc.Cancel(r.Context(), orgID, id)
	if err != nil {
		RespondError(w, http.StatusBadRequest, err.Error())
		return
	}
	RespondJSON(w, http.StatusOK, run)
}

// actions to offer.
func (h *BlogHandler) Config(w http.ResponseWriter, r *http.Request) {
	RespondJSON(w, http.StatusOK, map[string]any{
		"can_publish": h.svc.CanPublish(),
		// The ring's own CMS namespace — where a run with no website files —
		// so the website picker can name its default.
		"cms_namespace": h.svc.CMSNamespace(),
		// Defaults for adding a website: this ring's OpsAPI, and whether its
		// key would be stored encrypted (saving one is refused otherwise).
		"opsapi_base_url": h.opsapiBaseURL,
		"keys_sealed":     h.keysSealed,
		// Whether JobShout.com Insights is reachable, for "Send to Insights".
		"can_publish_insights": h.svc.CanPublishInsights(),
		// Whether the Content Writer's "Publish live" can work end to end.
		"can_publish_live":  h.svc.CanPublishLive(),
		"insights_site_url": h.insightsSiteURL,
		// The provider the writing pipeline is bound to at startup. The model
		// picker filters on it: the pipeline sends a bare model name to this one
		// provider, so offering a model from another provider would save a
		// choice that can only fail when the run starts.
		"provider": h.svc.Provider(),
		// What each role would use right now, so the picker can show the
		// inherited value instead of an empty box that explains nothing.
		"effective_models": h.svc.EffectiveModels(),
		// Which model the benchmark favoured for each role, and why.
		"recommended_models": blog.RecommendedModels(),
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

	// ?agent_id= lists one writer agent's runs — the original or a clone.
	if raw := strings.TrimSpace(r.URL.Query().Get("agent_id")); raw != "" {
		agentID, err := uuid.Parse(raw)
		if err != nil {
			RespondError(w, http.StatusBadRequest, "invalid agent_id")
			return
		}
		result, err := h.svc.ListByAgentID(r.Context(), orgID, agentID, params)
		if err != nil {
			RespondError(w, http.StatusBadRequest, err.Error())
			return
		}
		RespondJSON(w, http.StatusOK, result)
		return
	}

	// ?writer= lists one writer's runs; without it, the Article Writer's, so
	// its tab is unchanged by other writers sharing the tables.
	writer := strings.TrimSpace(r.URL.Query().Get("writer"))
	if writer == "" {
		writer = model.BuiltinArticleWriter
	}
	if _, ok := model.BlogWriters[writer]; !ok {
		RespondError(w, http.StatusBadRequest, "unknown writer")
		return
	}
	result, err := h.svc.ListByWriter(r.Context(), orgID, writer, params)
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

// CheckWebsite handles GET /api/v1/blogs/destinations/check?namespace=&base_url=
// — the add-website form's first step. base_url defaults to this ring's
// OpsAPI. The answer never includes a key.
func (h *BlogHandler) CheckWebsite(w http.ResponseWriter, r *http.Request) {
	orgID, err := uuid.Parse(middleware.GetOrgID(r.Context()))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid org_id in token")
		return
	}
	base := r.URL.Query().Get("base_url")
	if strings.TrimSpace(base) == "" {
		base = h.opsapiBaseURL
	}
	res, err := h.svc.CheckWebsite(r.Context(), orgID, r.URL.Query().Get("namespace"), base)
	if errors.Is(err, service.ErrInvalidIntegration) {
		RespondError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to check the website")
		return
	}
	RespondJSON(w, http.StatusOK, res)
}

// Destinations handles GET /api/v1/blogs/destinations — the websites (opsapi
// integrations) a writer agent can be pointed at. Credentials are never
// returned.
func (h *BlogHandler) Destinations(w http.ResponseWriter, r *http.Request) {
	orgID, err := uuid.Parse(middleware.GetOrgID(r.Context()))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid org_id in token")
		return
	}
	list, err := h.svc.ListDestinations(r.Context(), orgID)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to list destinations")
		return
	}
	RespondJSON(w, http.StatusOK, list)
}
