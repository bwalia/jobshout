package facebook

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/jobshout/server/internal/middleware"
)

// Handler exposes the Facebook Page Manager's tab: the connection, Pages,
// posts, engagement and analytics. Drafting starts through POST /tasks/launch
// like any other agent. Every route is scoped to the caller's org.
type Handler struct {
	svc             *Service
	frontendBaseURL string
}

// NewHandler constructs the handler.
func NewHandler(svc *Service, frontendBaseURL string) *Handler {
	return &Handler{svc: svc, frontendBaseURL: frontendBaseURL}
}

// Routes mounts the authenticated routes under /facebook.
func (h *Handler) Routes(r chi.Router) {
	r.Get("/status", h.Status)
	r.Delete("/connection", h.Disconnect)
	r.Post("/connection/oauth/start", h.StartConnect)

	r.Patch("/pages/{pageID}/profile", h.SetProfile)
	r.Post("/pages/{pageID}/automation", h.SetAutomation)
	r.Post("/pages/{pageID}/refresh", h.RefreshPage)
	r.Patch("/pages/{pageID}/info", h.UpdatePageInfo)
	r.Get("/pages/{pageID}/analytics", h.Analytics)

	r.Get("/posts", h.ListPosts)
	r.Patch("/posts/{postID}", h.UpdatePost)
	r.Patch("/posts/{postID}/published", h.EditPublished)
	r.Post("/posts/{postID}/approve", h.Approve)
	r.Post("/posts/{postID}/publish", h.Publish)
	r.Post("/posts/{postID}/unschedule", h.Unschedule)
	r.Post("/posts/{postID}/redraft", h.Redraft)
	r.Post("/posts/{postID}/image", h.GenerateImage)
	r.Delete("/posts/{postID}", h.Delete)
	r.Get("/posts/{postID}/comments", h.Comments)
	r.Post("/posts/{postID}/comments/reply", h.Reply)
}

func respond(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if v != nil {
		_ = json.NewEncoder(w).Encode(v)
	}
}

func fail(w http.ResponseWriter, status int, msg string) {
	respond(w, status, map[string]string{"error": msg})
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		fail(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return false
	}
	return true
}

func (h *Handler) tabURL(key, value string) string {
	return h.frontendBaseURL + "/panel/task-manager?agent=facebook&" + url.QueryEscape(key) + "=" + url.QueryEscape(value)
}

func (h *Handler) ids(w http.ResponseWriter, r *http.Request) (orgID, userID uuid.UUID, ok bool) {
	orgID, err := uuid.Parse(middleware.GetOrgID(r.Context()))
	if err != nil {
		fail(w, http.StatusBadRequest, "invalid org_id in token")
		return uuid.Nil, uuid.Nil, false
	}
	userID, _ = uuid.Parse(middleware.GetUserID(r.Context()))
	return orgID, userID, true
}

func postID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "postID"))
	if err != nil {
		fail(w, http.StatusBadRequest, "invalid post ID")
		return uuid.Nil, false
	}
	return id, true
}

func (h *Handler) writeErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		fail(w, http.StatusNotFound, "not found")
	case errors.Is(err, ErrConflict):
		fail(w, http.StatusConflict, "this post can no longer be changed in that way (it is being drafted or published, or is already published)")
	case errors.Is(err, ErrNotConfigured):
		fail(w, http.StatusServiceUnavailable, err.Error())
	case errors.Is(err, ErrNotConnected), errors.Is(err, ErrTokenExpired), errors.Is(err, ErrPermission):
		fail(w, http.StatusPreconditionFailed, err.Error())
	default:
		fail(w, http.StatusBadGateway, err.Error())
	}
}

// badOr maps validation errors (plain errors from the service) to 400 and
// sentinel errors to their usual status.
func (h *Handler) badOr(w http.ResponseWriter, err error) {
	for _, s := range []error{ErrNotFound, ErrConflict, ErrNotConfigured, ErrNotConnected, ErrTokenExpired, ErrPermission, ErrNoAnswer} {
		if errors.Is(err, s) {
			h.writeErr(w, err)
			return
		}
	}
	fail(w, http.StatusBadRequest, err.Error())
}

// Status GET /api/v1/facebook/status
func (h *Handler) Status(w http.ResponseWriter, r *http.Request) {
	orgID, _, ok := h.ids(w, r)
	if !ok {
		return
	}
	st, err := h.svc.Status(r.Context(), orgID)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	respond(w, http.StatusOK, st)
}

// StartConnect POST /api/v1/facebook/connection/oauth/start
func (h *Handler) StartConnect(w http.ResponseWriter, r *http.Request) {
	orgID, userID, ok := h.ids(w, r)
	if !ok {
		return
	}
	u, err := h.svc.StartConnect(r.Context(), orgID, userID)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	respond(w, http.StatusOK, map[string]string{"url": u})
}

// OAuthCallback GET /api/v1/facebook/oauth/callback — Facebook redirects the
// browser here with ?code=&state=, so it carries no JWT.
func (h *Handler) OAuthCallback(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if e := q.Get("error"); e != "" {
		msg := q.Get("error_description")
		if msg == "" {
			msg = e
		}
		http.Redirect(w, r, h.tabURL("facebook_error", msg), http.StatusFound)
		return
	}
	state, code := q.Get("state"), q.Get("code")
	if state == "" || code == "" {
		http.Redirect(w, r, h.tabURL("facebook_error", "missing_code"), http.StatusFound)
		return
	}
	if err := h.svc.CompleteConnect(r.Context(), state, code); err != nil {
		http.Redirect(w, r, h.tabURL("facebook_error", err.Error()), http.StatusFound)
		return
	}
	http.Redirect(w, r, h.tabURL("facebook_connected", "1"), http.StatusFound)
}

// Disconnect DELETE /api/v1/facebook/connection
func (h *Handler) Disconnect(w http.ResponseWriter, r *http.Request) {
	orgID, _, ok := h.ids(w, r)
	if !ok {
		return
	}
	if err := h.svc.Disconnect(r.Context(), orgID); err != nil {
		h.writeErr(w, err)
		return
	}
	respond(w, http.StatusOK, map[string]bool{"disconnected": true})
}

// SetProfile PATCH /api/v1/facebook/pages/{pageID}/profile
func (h *Handler) SetProfile(w http.ResponseWriter, r *http.Request) {
	orgID, _, ok := h.ids(w, r)
	if !ok {
		return
	}
	var body struct {
		Brief    string `json:"brief"`
		Voice    string `json:"voice"`
		Timezone string `json:"timezone"`
	}
	if !decode(w, r, &body) {
		return
	}
	p, err := h.svc.SetProfile(r.Context(), orgID, chi.URLParam(r, "pageID"), body.Brief, body.Voice, body.Timezone)
	if err != nil {
		h.badOr(w, err)
		return
	}
	respond(w, http.StatusOK, p)
}

// SetAutomation POST /api/v1/facebook/pages/{pageID}/automation {"paused":bool}
func (h *Handler) SetAutomation(w http.ResponseWriter, r *http.Request) {
	orgID, _, ok := h.ids(w, r)
	if !ok {
		return
	}
	var body struct {
		Paused bool `json:"paused"`
	}
	if !decode(w, r, &body) {
		return
	}
	p, err := h.svc.SetPaused(r.Context(), orgID, chi.URLParam(r, "pageID"), body.Paused)
	if err != nil {
		h.badOr(w, err)
		return
	}
	respond(w, http.StatusOK, p)
}

// RefreshPage POST /api/v1/facebook/pages/{pageID}/refresh
func (h *Handler) RefreshPage(w http.ResponseWriter, r *http.Request) {
	orgID, _, ok := h.ids(w, r)
	if !ok {
		return
	}
	p, err := h.svc.RefreshPage(r.Context(), orgID, chi.URLParam(r, "pageID"))
	if err != nil {
		h.writeErr(w, err)
		return
	}
	respond(w, http.StatusOK, p)
}

// UpdatePageInfo PATCH /api/v1/facebook/pages/{pageID}/info
func (h *Handler) UpdatePageInfo(w http.ResponseWriter, r *http.Request) {
	orgID, _, ok := h.ids(w, r)
	if !ok {
		return
	}
	var body struct {
		About       *string `json:"about"`
		Description *string `json:"description"`
		Website     *string `json:"website"`
	}
	if !decode(w, r, &body) {
		return
	}
	p, err := h.svc.UpdatePageInfo(r.Context(), orgID, chi.URLParam(r, "pageID"),
		PageUpdate{About: body.About, Description: body.Description, Website: body.Website})
	if err != nil {
		h.writeErr(w, err)
		return
	}
	respond(w, http.StatusOK, p)
}

// Analytics GET /api/v1/facebook/pages/{pageID}/analytics
func (h *Handler) Analytics(w http.ResponseWriter, r *http.Request) {
	orgID, _, ok := h.ids(w, r)
	if !ok {
		return
	}
	a, err := h.svc.Analytics(r.Context(), orgID, chi.URLParam(r, "pageID"))
	if err != nil {
		h.writeErr(w, err)
		return
	}
	respond(w, http.StatusOK, a)
}

// ListPosts GET /api/v1/facebook/posts?page=
func (h *Handler) ListPosts(w http.ResponseWriter, r *http.Request) {
	orgID, _, ok := h.ids(w, r)
	if !ok {
		return
	}
	posts, err := h.svc.ListPosts(r.Context(), orgID, r.URL.Query().Get("page"))
	if err != nil {
		h.writeErr(w, err)
		return
	}
	if posts == nil {
		posts = []Post{}
	}
	respond(w, http.StatusOK, map[string]any{"data": posts})
}

// UpdatePost PATCH /api/v1/facebook/posts/{postID}
func (h *Handler) UpdatePost(w http.ResponseWriter, r *http.Request) {
	orgID, _, ok := h.ids(w, r)
	if !ok {
		return
	}
	id, ok := postID(w, r)
	if !ok {
		return
	}
	var body struct {
		Message       string     `json:"message"`
		ScheduledFor  *time.Time `json:"scheduled_for"`
		ClearSchedule bool       `json:"clear_schedule"`
	}
	if !decode(w, r, &body) {
		return
	}
	p, err := h.svc.UpdatePost(r.Context(), orgID, id, PostEdit{Message: body.Message, ScheduledFor: body.ScheduledFor, ClearSchedule: body.ClearSchedule})
	if err != nil {
		h.badOr(w, err)
		return
	}
	respond(w, http.StatusOK, p)
}

// EditPublished PATCH /api/v1/facebook/posts/{postID}/published
func (h *Handler) EditPublished(w http.ResponseWriter, r *http.Request) {
	orgID, _, ok := h.ids(w, r)
	if !ok {
		return
	}
	id, ok := postID(w, r)
	if !ok {
		return
	}
	var body struct {
		Message string `json:"message"`
	}
	if !decode(w, r, &body) {
		return
	}
	p, err := h.svc.EditPublished(r.Context(), orgID, id, body.Message)
	if err != nil {
		h.badOr(w, err)
		return
	}
	respond(w, http.StatusOK, p)
}

// Approve POST /api/v1/facebook/posts/{postID}/approve — schedules a draft at
// scheduled_for (or its suggested time); a time in the past publishes now.
func (h *Handler) Approve(w http.ResponseWriter, r *http.Request) {
	orgID, userID, ok := h.ids(w, r)
	if !ok {
		return
	}
	id, ok := postID(w, r)
	if !ok {
		return
	}
	var body struct {
		ScheduledFor *time.Time `json:"scheduled_for"`
	}
	if r.ContentLength != 0 && !decode(w, r, &body) {
		return
	}
	p, err := h.svc.Approve(r.Context(), orgID, userID, id, body.ScheduledFor)
	if err != nil {
		h.badOr(w, err)
		return
	}
	respond(w, http.StatusOK, p)
}

// Publish POST /api/v1/facebook/posts/{postID}/publish
func (h *Handler) Publish(w http.ResponseWriter, r *http.Request) {
	orgID, userID, ok := h.ids(w, r)
	if !ok {
		return
	}
	id, ok := postID(w, r)
	if !ok {
		return
	}
	p, err := h.svc.PublishNow(r.Context(), orgID, userID, id)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	respond(w, http.StatusOK, p)
}

// Unschedule POST /api/v1/facebook/posts/{postID}/unschedule
func (h *Handler) Unschedule(w http.ResponseWriter, r *http.Request) {
	orgID, _, ok := h.ids(w, r)
	if !ok {
		return
	}
	id, ok := postID(w, r)
	if !ok {
		return
	}
	p, err := h.svc.Unschedule(r.Context(), orgID, id)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	respond(w, http.StatusOK, p)
}

// Redraft POST /api/v1/facebook/posts/{postID}/redraft
func (h *Handler) Redraft(w http.ResponseWriter, r *http.Request) {
	orgID, userID, ok := h.ids(w, r)
	if !ok {
		return
	}
	id, ok := postID(w, r)
	if !ok {
		return
	}
	p, err := h.svc.Redraft(r.Context(), orgID, userID, id)
	if err != nil {
		h.badOr(w, err)
		return
	}
	respond(w, http.StatusAccepted, p)
}

// GenerateImage POST /api/v1/facebook/posts/{postID}/image
func (h *Handler) GenerateImage(w http.ResponseWriter, r *http.Request) {
	orgID, userID, ok := h.ids(w, r)
	if !ok {
		return
	}
	id, ok := postID(w, r)
	if !ok {
		return
	}
	p, err := h.svc.GenerateImage(r.Context(), orgID, userID, id)
	if err != nil {
		h.badOr(w, err)
		return
	}
	respond(w, http.StatusOK, p)
}

// Delete DELETE /api/v1/facebook/posts/{postID}
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	orgID, _, ok := h.ids(w, r)
	if !ok {
		return
	}
	id, ok := postID(w, r)
	if !ok {
		return
	}
	if err := h.svc.Delete(r.Context(), orgID, id); err != nil {
		h.writeErr(w, err)
		return
	}
	respond(w, http.StatusOK, map[string]bool{"deleted": true})
}

// Comments GET /api/v1/facebook/posts/{postID}/comments
func (h *Handler) Comments(w http.ResponseWriter, r *http.Request) {
	orgID, _, ok := h.ids(w, r)
	if !ok {
		return
	}
	id, ok := postID(w, r)
	if !ok {
		return
	}
	cs, err := h.svc.Comments(r.Context(), orgID, id)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	if cs == nil {
		cs = []Comment{}
	}
	respond(w, http.StatusOK, map[string]any{"data": cs})
}

// Reply POST /api/v1/facebook/posts/{postID}/comments/reply
func (h *Handler) Reply(w http.ResponseWriter, r *http.Request) {
	orgID, _, ok := h.ids(w, r)
	if !ok {
		return
	}
	id, ok := postID(w, r)
	if !ok {
		return
	}
	var body struct {
		CommentID string `json:"comment_id"`
		Message   string `json:"message"`
	}
	if !decode(w, r, &body) {
		return
	}
	if err := h.svc.Reply(r.Context(), orgID, id, body.CommentID, body.Message); err != nil {
		h.badOr(w, err)
		return
	}
	respond(w, http.StatusOK, map[string]bool{"replied": true})
}
