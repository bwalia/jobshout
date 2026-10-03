package handler

import (
	"errors"
	"net/http"
	"net/url"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/jobshout/server/internal/linkedin"
	"github.com/jobshout/server/internal/middleware"
	"github.com/jobshout/server/internal/repository"
	"github.com/jobshout/server/internal/service"
)

// LinkedInHandler exposes the LinkedIn Poster's tab: the LinkedIn connection
// and the drafted posts. Drafting starts through POST /tasks/launch like any
// other agent. Every route is scoped to the caller's org.
type LinkedInHandler struct {
	svc             service.LinkedInService
	frontendBaseURL string
}

// NewLinkedInHandler constructs the handler.
func NewLinkedInHandler(svc service.LinkedInService, frontendBaseURL string) *LinkedInHandler {
	return &LinkedInHandler{svc: svc, frontendBaseURL: frontendBaseURL}
}

// tabURL is the agent's Task Manager tab, with the OAuth outcome appended.
func (h *LinkedInHandler) tabURL(key, value string) string {
	return h.frontendBaseURL + "/panel/task-manager?agent=linkedin&" + url.QueryEscape(key) + "=" + url.QueryEscape(value)
}

func (h *LinkedInHandler) ids(w http.ResponseWriter, r *http.Request) (orgID, userID uuid.UUID, ok bool) {
	orgID, err := uuid.Parse(middleware.GetOrgID(r.Context()))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid org_id in token")
		return uuid.Nil, uuid.Nil, false
	}
	userID, _ = uuid.Parse(middleware.GetUserID(r.Context()))
	return orgID, userID, true
}

func (h *LinkedInHandler) postID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "postID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid post ID")
		return uuid.Nil, false
	}
	return id, true
}

func (h *LinkedInHandler) writeErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, repository.ErrLinkedInNotFound):
		RespondError(w, http.StatusNotFound, "not found")
	case errors.Is(err, repository.ErrLinkedInConflict):
		RespondError(w, http.StatusConflict, "this post can no longer be changed (it is being drafted, posted, or already posted)")
	case errors.Is(err, service.ErrLinkedInNotConfigured):
		RespondError(w, http.StatusServiceUnavailable, err.Error())
	case errors.Is(err, service.ErrLinkedInNotConnected), errors.Is(err, linkedin.ErrTokenExpired):
		RespondError(w, http.StatusPreconditionFailed, err.Error())
	default:
		RespondError(w, http.StatusBadGateway, err.Error())
	}
}

// Status GET /api/v1/linkedin/connection
func (h *LinkedInHandler) Status(w http.ResponseWriter, r *http.Request) {
	orgID, _, ok := h.ids(w, r)
	if !ok {
		return
	}
	st, err := h.svc.Status(r.Context(), orgID)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	RespondJSON(w, http.StatusOK, st)
}

// StartConnect POST /api/v1/linkedin/connection/oauth/start
func (h *LinkedInHandler) StartConnect(w http.ResponseWriter, r *http.Request) {
	orgID, userID, ok := h.ids(w, r)
	if !ok {
		return
	}
	u, err := h.svc.StartConnect(r.Context(), orgID, userID)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	RespondJSON(w, http.StatusOK, map[string]string{"url": u})
}

// OAuthCallback GET /api/v1/linkedin/oauth/callback — LinkedIn redirects the
// browser here with ?code=&state=, so it carries no JWT.
func (h *LinkedInHandler) OAuthCallback(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if e := q.Get("error"); e != "" {
		msg := q.Get("error_description")
		if msg == "" {
			msg = e
		}
		http.Redirect(w, r, h.tabURL("linkedin_error", msg), http.StatusFound)
		return
	}
	state, code := q.Get("state"), q.Get("code")
	if state == "" || code == "" {
		http.Redirect(w, r, h.tabURL("linkedin_error", "missing_code"), http.StatusFound)
		return
	}
	if err := h.svc.CompleteConnect(r.Context(), state, code); err != nil {
		http.Redirect(w, r, h.tabURL("linkedin_error", err.Error()), http.StatusFound)
		return
	}
	http.Redirect(w, r, h.tabURL("linkedin_connected", "1"), http.StatusFound)
}

// Disconnect DELETE /api/v1/linkedin/connection
func (h *LinkedInHandler) Disconnect(w http.ResponseWriter, r *http.Request) {
	orgID, _, ok := h.ids(w, r)
	if !ok {
		return
	}
	if err := h.svc.Disconnect(r.Context(), orgID); err != nil {
		h.writeErr(w, err)
		return
	}
	RespondJSON(w, http.StatusOK, map[string]bool{"disconnected": true})
}

// ListPosts GET /api/v1/linkedin/posts
func (h *LinkedInHandler) ListPosts(w http.ResponseWriter, r *http.Request) {
	orgID, _, ok := h.ids(w, r)
	if !ok {
		return
	}
	posts, err := h.svc.ListPosts(r.Context(), orgID)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	RespondJSON(w, http.StatusOK, map[string]any{"data": posts})
}

// UpdatePost PATCH /api/v1/linkedin/posts/{postID}
func (h *LinkedInHandler) UpdatePost(w http.ResponseWriter, r *http.Request) {
	orgID, _, ok := h.ids(w, r)
	if !ok {
		return
	}
	id, ok := h.postID(w, r)
	if !ok {
		return
	}
	var body struct {
		Commentary string `json:"commentary"`
	}
	if !DecodeJSON(w, r, &body) {
		return
	}
	p, err := h.svc.UpdatePost(r.Context(), orgID, id, body.Commentary)
	if err != nil {
		if errors.Is(err, repository.ErrLinkedInNotFound) || errors.Is(err, repository.ErrLinkedInConflict) {
			h.writeErr(w, err)
			return
		}
		RespondError(w, http.StatusBadRequest, err.Error())
		return
	}
	RespondJSON(w, http.StatusOK, p)
}

// Redraft POST /api/v1/linkedin/posts/{postID}/redraft
func (h *LinkedInHandler) Redraft(w http.ResponseWriter, r *http.Request) {
	orgID, _, ok := h.ids(w, r)
	if !ok {
		return
	}
	id, ok := h.postID(w, r)
	if !ok {
		return
	}
	p, err := h.svc.Redraft(r.Context(), orgID, id)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	RespondJSON(w, http.StatusAccepted, p)
}

// Publish POST /api/v1/linkedin/posts/{postID}/publish
func (h *LinkedInHandler) Publish(w http.ResponseWriter, r *http.Request) {
	orgID, userID, ok := h.ids(w, r)
	if !ok {
		return
	}
	id, ok := h.postID(w, r)
	if !ok {
		return
	}
	p, err := h.svc.Publish(r.Context(), orgID, userID, id)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	RespondJSON(w, http.StatusOK, p)
}
