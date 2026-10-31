package handler

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/jobshout/server/internal/metalead"
	"github.com/jobshout/server/internal/middleware"
)

// MetaLeadHandler serves Meta Lead Analyst HTTP surfaces.
type MetaLeadHandler struct {
	svc *metalead.Service
}

// NewMetaLeadHandler builds the handler.
func NewMetaLeadHandler(svc *metalead.Service) *MetaLeadHandler {
	return &MetaLeadHandler{svc: svc}
}

func (h *MetaLeadHandler) ids(w http.ResponseWriter, r *http.Request) (orgID, userID uuid.UUID, ok bool) {
	orgID, err := uuid.Parse(middleware.GetOrgID(r.Context()))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid org_id in token")
		return uuid.Nil, uuid.Nil, false
	}
	userID, _ = uuid.Parse(middleware.GetUserID(r.Context()))
	return orgID, userID, true
}

// WebhookVerify handles Meta's GET subscription challenge.
func (h *MetaLeadHandler) WebhookVerify(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	challenge, ok := h.svc.VerifyWebhookChallenge(
		q.Get("hub.mode"),
		q.Get("hub.verify_token"),
		q.Get("hub.challenge"),
	)
	if !ok {
		RespondError(w, http.StatusForbidden, "forbidden")
		return
	}
	w.Header().Set("Content-Type", "text/plain")
	_, _ = w.Write([]byte(challenge))
}

// WebhookReceive handles POST leadgen notifications.
func (h *MetaLeadHandler) WebhookReceive(w http.ResponseWriter, r *http.Request) {
	raw, err := metalead.ReadBody(r, 1<<20)
	if err != nil {
		RespondError(w, http.StatusBadRequest, "bad body")
		return
	}
	sig := r.Header.Get("X-Hub-Signature-256")
	if err := h.svc.HandleWebhook(r.Context(), raw, sig); err != nil {
		RespondError(w, http.StatusUnauthorized, err.Error())
		return
	}
	RespondJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// Status returns configuration readiness.
func (h *MetaLeadHandler) Status(w http.ResponseWriter, r *http.Request) {
	RespondJSON(w, http.StatusOK, h.svc.Status())
}

// ListConnections lists named Page connections (no tokens).
func (h *MetaLeadHandler) ListConnections(w http.ResponseWriter, r *http.Request) {
	orgID, _, ok := h.ids(w, r)
	if !ok {
		return
	}
	list, err := h.svc.ListConnections(r.Context(), orgID)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	RespondJSON(w, http.StatusOK, map[string]any{"data": list})
}

type upsertConnectionBody struct {
	Name        string `json:"name"`
	PageID      string `json:"page_id"`
	PageName    string `json:"page_name"`
	AccessToken string `json:"access_token"`
}

// UpsertConnection stores a sealed Page token under a name.
func (h *MetaLeadHandler) UpsertConnection(w http.ResponseWriter, r *http.Request) {
	orgID, userID, ok := h.ids(w, r)
	if !ok {
		return
	}
	var body upsertConnectionBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		RespondError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	conn, err := h.svc.UpsertConnection(r.Context(), orgID, userID, body.Name, body.PageID, body.PageName, body.AccessToken)
	if err != nil {
		RespondError(w, http.StatusBadRequest, err.Error())
		return
	}
	RespondJSON(w, http.StatusOK, conn)
}

// ListRuns lists recent analysis runs.
func (h *MetaLeadHandler) ListRuns(w http.ResponseWriter, r *http.Request) {
	orgID, _, ok := h.ids(w, r)
	if !ok {
		return
	}
	list, err := h.svc.ListRuns(r.Context(), orgID, 30)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	RespondJSON(w, http.StatusOK, map[string]any{"data": list})
}

// GetRun returns one run.
func (h *MetaLeadHandler) GetRun(w http.ResponseWriter, r *http.Request) {
	orgID, _, ok := h.ids(w, r)
	if !ok {
		return
	}
	runID, err := uuid.Parse(chi.URLParam(r, "runID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid run id")
		return
	}
	run, err := h.svc.GetRun(r.Context(), orgID, runID)
	if err != nil {
		RespondError(w, http.StatusNotFound, err.Error())
		return
	}
	RespondJSON(w, http.StatusOK, run)
}

// ListRunLeads returns leads for a run.
func (h *MetaLeadHandler) ListRunLeads(w http.ResponseWriter, r *http.Request) {
	orgID, _, ok := h.ids(w, r)
	if !ok {
		return
	}
	runID, err := uuid.Parse(chi.URLParam(r, "runID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid run id")
		return
	}
	list, err := h.svc.ListLeadsByRun(r.Context(), orgID, runID)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	RespondJSON(w, http.StatusOK, map[string]any{"data": list})
}

// ListLeads returns recent leads for the org.
func (h *MetaLeadHandler) ListLeads(w http.ResponseWriter, r *http.Request) {
	orgID, _, ok := h.ids(w, r)
	if !ok {
		return
	}
	list, err := h.svc.ListRecentLeads(r.Context(), orgID, 50)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	RespondJSON(w, http.StatusOK, map[string]any{"data": list})
}
