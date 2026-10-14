package handler

import (
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/jobshout/server/internal/middleware"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/service"
)

type IntegrationHandler struct {
	svc      service.IntegrationService
	validate *validator.Validate
}

func NewIntegrationHandler(svc service.IntegrationService) *IntegrationHandler {
	return &IntegrationHandler{svc: svc, validate: validator.New()}
}

// publicCredentialKeys are identifiers some adapters keep under credentials
// (Jira email and project, GitHub owner and repo). They are not secrets, and
// the settings form needs them to show what an integration points at. Every
// other credential value is masked.
var publicCredentialKeys = map[string]bool{"email": true, "project_key": true, "owner": true, "repo": true}

// integrationView is an integration as the API returns it: secrets masked,
// plus the names of the credentials that are set.
type integrationView struct {
	model.Integration
	CredentialKeys []string `json:"credential_keys"`
}

// maskSecret keeps only the last four characters, enough to tell two keys
// apart without revealing either.
func maskSecret(s string) string {
	if len(s) > 8 {
		return "••••" + s[len(s)-4:]
	}
	return "••••"
}

// maskIntegration builds the API view of i. Clients never get a secret back —
// they set it, the server keeps it.
func maskIntegration(i model.Integration) integrationView {
	masked := make(map[string]any, len(i.Credentials))
	keys := []string{}
	for k, v := range i.Credentials {
		s, isStr := v.(string)
		if v == nil || (isStr && strings.TrimSpace(s) == "") {
			continue
		}
		keys = append(keys, k)
		if isStr && publicCredentialKeys[k] {
			masked[k] = s
		} else {
			masked[k] = maskSecret(s)
		}
	}
	sort.Strings(keys)
	i.Credentials = masked
	return integrationView{Integration: i, CredentialKeys: keys}
}

// respondWriteError maps a create/update failure to a status. A duplicate
// name is the one users hit, so it gets a plain message.
func respondWriteError(w http.ResponseWriter, err error) {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		RespondError(w, http.StatusConflict, "an integration with this name already exists")
		return
	}
	RespondError(w, http.StatusInternalServerError, err.Error())
}

// ownedIntegration resolves {integrationID} to an integration of the caller's
// org. Another org's integration is a 404 — it holds that org's credentials.
func (h *IntegrationHandler) ownedIntegration(w http.ResponseWriter, r *http.Request) (*model.Integration, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "integrationID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid integration id")
		return nil, false
	}
	orgID, err := uuid.Parse(middleware.GetOrgID(r.Context()))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid org_id in token")
		return nil, false
	}
	i, err := h.svc.Get(r.Context(), id)
	if err != nil || i == nil || i.OrgID != orgID {
		RespondError(w, http.StatusNotFound, "integration not found")
		return nil, false
	}
	return i, true
}

// Create handles POST /integrations
func (h *IntegrationHandler) Create(w http.ResponseWriter, r *http.Request) {
	orgID, err := uuid.Parse(middleware.GetOrgID(r.Context()))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid org_id in token")
		return
	}
	userID, err := uuid.Parse(middleware.GetUserID(r.Context()))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid user_id in token")
		return
	}

	var req model.CreateIntegrationRequest
	if !DecodeJSON(w, r, &req) {
		return
	}
	if err := h.validate.Struct(req); err != nil {
		RespondError(w, http.StatusBadRequest, "validation failed: "+err.Error())
		return
	}

	i, err := h.svc.Create(r.Context(), orgID, userID, req)
	if err != nil {
		respondWriteError(w, err)
		return
	}
	RespondJSON(w, http.StatusCreated, maskIntegration(*i))
}

// List handles GET /integrations
func (h *IntegrationHandler) List(w http.ResponseWriter, r *http.Request) {
	orgID, err := uuid.Parse(middleware.GetOrgID(r.Context()))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid org_id in token")
		return
	}

	items, err := h.svc.List(r.Context(), orgID)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]integrationView, 0, len(items))
	for _, i := range items {
		out = append(out, maskIntegration(i))
	}
	RespondJSON(w, http.StatusOK, out)
}

// Get handles GET /integrations/{integrationID}
func (h *IntegrationHandler) Get(w http.ResponseWriter, r *http.Request) {
	i, ok := h.ownedIntegration(w, r)
	if !ok {
		return
	}
	RespondJSON(w, http.StatusOK, maskIntegration(*i))
}

// Update handles PUT /integrations/{integrationID}. Credentials are merged:
// a key sent blank or left out keeps its stored value.
func (h *IntegrationHandler) Update(w http.ResponseWriter, r *http.Request) {
	existing, ok := h.ownedIntegration(w, r)
	if !ok {
		return
	}

	var req model.UpdateIntegrationRequest
	if !DecodeJSON(w, r, &req) {
		return
	}

	i, err := h.svc.Update(r.Context(), existing.ID, req)
	if err != nil {
		respondWriteError(w, err)
		return
	}
	RespondJSON(w, http.StatusOK, maskIntegration(*i))
}

// Delete handles DELETE /integrations/{integrationID}
func (h *IntegrationHandler) Delete(w http.ResponseWriter, r *http.Request) {
	i, ok := h.ownedIntegration(w, r)
	if !ok {
		return
	}

	if err := h.svc.Delete(r.Context(), i.ID); err != nil {
		RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// LinkTask handles POST /integrations/{integrationID}/tasks/{taskID}/link
func (h *IntegrationHandler) LinkTask(w http.ResponseWriter, r *http.Request) {
	integ, ok := h.ownedIntegration(w, r)
	if !ok {
		return
	}
	taskID, err := uuid.Parse(chi.URLParam(r, "taskID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid task id")
		return
	}

	var req model.LinkTaskRequest
	_ = DecodeJSON(w, r, &req) // optional body
	direction := req.Direction
	if direction == "" {
		direction = "bidirectional"
	}

	link, err := h.svc.LinkTask(r.Context(), integ.ID, taskID, direction)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	RespondJSON(w, http.StatusCreated, link)
}

// UnlinkTask handles DELETE /integrations/{integrationID}/tasks/{taskID}/link
func (h *IntegrationHandler) UnlinkTask(w http.ResponseWriter, r *http.Request) {
	integ, ok := h.ownedIntegration(w, r)
	if !ok {
		return
	}
	taskID, err := uuid.Parse(chi.URLParam(r, "taskID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid task id")
		return
	}

	if err := h.svc.UnlinkTask(r.Context(), integ.ID, taskID); err != nil {
		RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ListLinks handles GET /integrations/{integrationID}/links
func (h *IntegrationHandler) ListLinks(w http.ResponseWriter, r *http.Request) {
	integ, ok := h.ownedIntegration(w, r)
	if !ok {
		return
	}

	links, err := h.svc.ListLinks(r.Context(), integ.ID)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if links == nil {
		links = []model.IntegrationTaskLink{}
	}
	RespondJSON(w, http.StatusOK, links)
}

// SyncLink handles POST /integrations/{integrationID}/links/{linkID}/sync
func (h *IntegrationHandler) SyncLink(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.ownedIntegration(w, r); !ok {
		return
	}
	linkID, err := uuid.Parse(chi.URLParam(r, "linkID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid link id")
		return
	}

	direction := r.URL.Query().Get("direction")
	if direction == "" {
		direction = "push"
	}

	if err := h.svc.SyncLink(r.Context(), linkID, direction); err != nil {
		RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	RespondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// ListSyncLogs handles GET /integrations/{integrationID}/sync-logs
func (h *IntegrationHandler) ListSyncLogs(w http.ResponseWriter, r *http.Request) {
	integ, ok := h.ownedIntegration(w, r)
	if !ok {
		return
	}

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	perPage, _ := strconv.Atoi(r.URL.Query().Get("per_page"))
	params := model.PaginationParams{Page: page, PerPage: perPage}

	logs, err := h.svc.ListSyncLogs(r.Context(), integ.ID, params)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	RespondJSON(w, http.StatusOK, logs)
}
