package handler

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/jobshout/server/internal/mail"
	"github.com/jobshout/server/internal/middleware"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/service"
)

type MailHandler struct {
	svc             service.MailService
	frontendBaseURL string
}

func NewMailHandler(svc service.MailService, frontendBaseURL string) *MailHandler {
	return &MailHandler{svc: svc, frontendBaseURL: strings.TrimRight(frontendBaseURL, "/")}
}

// mailUI is the Task Manager Mail Agent view. Extra query pairs are appended
// with & so they do not wipe ?agent=mail (OAuth connected=1 / error=).
func (h *MailHandler) mailUI(key, value string) string {
	base := h.frontendBaseURL + "/panel/task-manager?agent=mail"
	if key == "" {
		return base
	}
	return base + "&" + url.QueryEscape(key) + "=" + url.QueryEscape(value)
}

func (h *MailHandler) orgID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	orgID, err := uuid.Parse(middleware.GetOrgID(r.Context()))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid org_id in token")
		return uuid.Nil, false
	}
	return orgID, true
}

func (h *MailHandler) userID(r *http.Request) uuid.UUID {
	id, err := uuid.Parse(middleware.GetUserID(r.Context()))
	if err != nil {
		return uuid.Nil
	}
	return id
}

func (h *MailHandler) writeErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, service.ErrMailNotConfigured):
		RespondError(w, http.StatusServiceUnavailable, err.Error())
	case errors.Is(err, service.ErrMailNotConnected):
		RespondError(w, http.StatusConflict, err.Error())
	case errors.Is(err, service.ErrMailNotFound):
		RespondError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, service.ErrMailCannotSend), errors.Is(err, service.ErrMailDraftNotEditable):
		RespondError(w, http.StatusForbidden, err.Error())
	case errors.Is(err, mail.ErrInvalidKnowledgeURL), errors.Is(err, mail.ErrKnowledgeNotesTooLong):
		RespondError(w, http.StatusBadRequest, err.Error())
	default:
		RespondError(w, http.StatusInternalServerError, mail.Redact(err.Error()))
	}
}

// GetConnection GET /api/v1/mail/connection
func (h *MailHandler) GetConnection(w http.ResponseWriter, r *http.Request) {
	orgID, ok := h.orgID(w, r)
	if !ok {
		return
	}
	st, err := h.svc.ConnectionStatus(r.Context(), orgID)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	RespondJSON(w, http.StatusOK, st)
}

// StartOAuth POST /api/v1/mail/connection/oauth/start
func (h *MailHandler) StartOAuth(w http.ResponseWriter, r *http.Request) {
	orgID, ok := h.orgID(w, r)
	if !ok {
		return
	}
	uid := h.userID(r)
	if uid == uuid.Nil {
		RespondError(w, http.StatusBadRequest, "invalid user_id in token")
		return
	}
	out, err := h.svc.StartOAuth(r.Context(), orgID, uid)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	RespondJSON(w, http.StatusOK, out)
}

// OAuthCallback GET /api/v1/mail/connection/oauth/callback
// Public: Google redirects the browser here with code+state.
func (h *MailHandler) OAuthCallback(w http.ResponseWriter, r *http.Request) {
	dest := h.mailUI
	if errParam := r.URL.Query().Get("error"); errParam != "" {
		http.Redirect(w, r, dest("error", mail.Redact(errParam)), http.StatusFound)
		return
	}
	state := r.URL.Query().Get("state")
	code := r.URL.Query().Get("code")
	if state == "" || code == "" {
		http.Redirect(w, r, dest("error", "missing_code"), http.StatusFound)
		return
	}
	if err := h.svc.CompleteOAuth(r.Context(), state, code); err != nil {
		http.Redirect(w, r, dest("error", mail.Redact(err.Error())), http.StatusFound)
		return
	}
	http.Redirect(w, r, dest("connected", "1"), http.StatusFound)
}

// Disconnect DELETE /api/v1/mail/connection
func (h *MailHandler) Disconnect(w http.ResponseWriter, r *http.Request) {
	orgID, ok := h.orgID(w, r)
	if !ok {
		return
	}
	if err := h.svc.Disconnect(r.Context(), orgID); err != nil {
		h.writeErr(w, err)
		return
	}
	RespondJSON(w, http.StatusOK, map[string]bool{"disconnected": true})
}

// PatchConnection PATCH /api/v1/mail/connection
func (h *MailHandler) PatchConnection(w http.ResponseWriter, r *http.Request) {
	orgID, ok := h.orgID(w, r)
	if !ok {
		return
	}
	var req model.UpdateMailConnectionRequest
	if !DecodeJSON(w, r, &req) {
		return
	}
	st, err := h.svc.UpdateConnection(r.Context(), orgID, req)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	RespondJSON(w, http.StatusOK, st)
}

// Sync POST /api/v1/mail/sync
func (h *MailHandler) Sync(w http.ResponseWriter, r *http.Request) {
	orgID, ok := h.orgID(w, r)
	if !ok {
		return
	}
	out, err := h.svc.SyncInbox(r.Context(), orgID)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	RespondJSON(w, http.StatusOK, out)
}

// ListThreads GET /api/v1/mail/threads
func (h *MailHandler) ListThreads(w http.ResponseWriter, r *http.Request) {
	orgID, ok := h.orgID(w, r)
	if !ok {
		return
	}
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	perPage, _ := strconv.Atoi(r.URL.Query().Get("per_page"))
	out, err := h.svc.ListThreads(r.Context(), orgID, model.PaginationParams{Page: page, PerPage: perPage})
	if err != nil {
		h.writeErr(w, err)
		return
	}
	RespondJSON(w, http.StatusOK, out)
}

// GetThread GET /api/v1/mail/threads/{id}
func (h *MailHandler) GetThread(w http.ResponseWriter, r *http.Request) {
	orgID, ok := h.orgID(w, r)
	if !ok {
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid thread id")
		return
	}
	out, err := h.svc.GetThread(r.Context(), orgID, id)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	RespondJSON(w, http.StatusOK, out)
}

// ListDrafts GET /api/v1/mail/drafts
func (h *MailHandler) ListDrafts(w http.ResponseWriter, r *http.Request) {
	orgID, ok := h.orgID(w, r)
	if !ok {
		return
	}
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	perPage, _ := strconv.Atoi(r.URL.Query().Get("per_page"))
	out, err := h.svc.ListPendingDrafts(r.Context(), orgID, model.PaginationParams{Page: page, PerPage: perPage})
	if err != nil {
		h.writeErr(w, err)
		return
	}
	RespondJSON(w, http.StatusOK, out)
}

// PatchDraft PATCH /api/v1/mail/drafts/{id}
func (h *MailHandler) PatchDraft(w http.ResponseWriter, r *http.Request) {
	orgID, ok := h.orgID(w, r)
	if !ok {
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid draft id")
		return
	}
	var req model.UpdateMailDraftRequest
	if !DecodeJSON(w, r, &req) {
		return
	}
	d, err := h.svc.UpdateDraft(r.Context(), orgID, id, req)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	RespondJSON(w, http.StatusOK, d)
}

// ApproveDraft POST /api/v1/mail/drafts/{id}/approve
func (h *MailHandler) ApproveDraft(w http.ResponseWriter, r *http.Request) {
	orgID, ok := h.orgID(w, r)
	if !ok {
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid draft id")
		return
	}
	uid := h.userID(r)
	if uid == uuid.Nil {
		RespondError(w, http.StatusBadRequest, "invalid user_id in token")
		return
	}
	d, err := h.svc.ApproveSend(r.Context(), orgID, id, uid)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	RespondJSON(w, http.StatusOK, d)
}

// RejectDraft POST /api/v1/mail/drafts/{id}/reject
func (h *MailHandler) RejectDraft(w http.ResponseWriter, r *http.Request) {
	orgID, ok := h.orgID(w, r)
	if !ok {
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid draft id")
		return
	}
	uid := h.userID(r)
	if uid == uuid.Nil {
		RespondError(w, http.StatusBadRequest, "invalid user_id in token")
		return
	}
	d, err := h.svc.Reject(r.Context(), orgID, id, uid)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	RespondJSON(w, http.StatusOK, d)
}
