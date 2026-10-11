package handler

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"

	"github.com/jobshout/server/internal/middleware"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/repository"
	"github.com/jobshout/server/internal/webhook"
)

// WebhookEndpointHandler manages an org's outbound webhook endpoints (org
// admins only; see main.go). Secrets are returned only on create and rotate.
//
//	GET    /api/v1/webhook-endpoints
//	POST   /api/v1/webhook-endpoints
//	GET    /api/v1/webhook-endpoints/{endpointID}
//	PUT    /api/v1/webhook-endpoints/{endpointID}
//	DELETE /api/v1/webhook-endpoints/{endpointID}
//	GET    /api/v1/webhook-endpoints/{endpointID}/deliveries?limit=50
//	POST   /api/v1/webhook-endpoints/{endpointID}/test
type WebhookEndpointHandler struct {
	repo     repository.WebhookRepository
	disp     *webhook.Dispatcher
	validate *validator.Validate
}

func NewWebhookEndpointHandler(repo repository.WebhookRepository, disp *webhook.Dispatcher) *WebhookEndpointHandler {
	return &WebhookEndpointHandler{repo: repo, disp: disp, validate: validator.New()}
}

func newWebhookSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "whsec_" + base64.RawURLEncoding.EncodeToString(b), nil
}

func validateWebhookURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("url must be an absolute http(s) URL")
	}
	return nil
}

func validateWebhookEvents(events []string) ([]string, error) {
	out := []string{}
	seen := map[string]bool{}
	for _, ev := range events {
		ok := ev == model.WebhookEventAll
		for _, known := range model.AllWebhookEvents {
			ok = ok || ev == known
		}
		if !ok {
			return nil, fmt.Errorf("unknown event %q", ev)
		}
		if !seen[ev] {
			seen[ev] = true
			out = append(out, ev)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("events: subscribe to at least one event")
	}
	return out, nil
}

func (h *WebhookEndpointHandler) orgID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(middleware.GetOrgID(r.Context()))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid org_id")
		return uuid.Nil, false
	}
	return id, true
}

// owned loads {endpointID} for the caller's org (404 for another org's).
func (h *WebhookEndpointHandler) owned(w http.ResponseWriter, r *http.Request) (*model.WebhookEndpoint, bool) {
	orgID, ok := h.orgID(w, r)
	if !ok {
		return nil, false
	}
	id, err := uuid.Parse(chi.URLParam(r, "endpointID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid endpoint id")
		return nil, false
	}
	ep, err := h.repo.GetEndpoint(r.Context(), orgID, id)
	if err != nil {
		if errors.Is(err, repository.ErrWebhookNotFound) {
			RespondError(w, http.StatusNotFound, "webhook endpoint not found")
		} else {
			RespondError(w, http.StatusInternalServerError, "failed to load webhook endpoint")
		}
		return nil, false
	}
	return ep, true
}

func (h *WebhookEndpointHandler) List(w http.ResponseWriter, r *http.Request) {
	orgID, ok := h.orgID(w, r)
	if !ok {
		return
	}
	eps, err := h.repo.ListEndpoints(r.Context(), orgID)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to list webhook endpoints")
		return
	}
	for i := range eps {
		eps[i].Secret = ""
	}
	RespondJSON(w, http.StatusOK, eps)
}

func (h *WebhookEndpointHandler) Create(w http.ResponseWriter, r *http.Request) {
	orgID, ok := h.orgID(w, r)
	if !ok {
		return
	}
	var req model.CreateWebhookEndpointRequest
	if !DecodeJSON(w, r, &req) {
		return
	}
	if err := h.validate.Struct(req); err != nil {
		RespondError(w, http.StatusBadRequest, "validation: "+err.Error())
		return
	}
	if err := validateWebhookURL(req.URL); err != nil {
		RespondError(w, http.StatusBadRequest, err.Error())
		return
	}
	events, err := validateWebhookEvents(req.Events)
	if err != nil {
		RespondError(w, http.StatusBadRequest, err.Error())
		return
	}
	secret := req.Secret
	if secret == "" {
		if secret, err = newWebhookSecret(); err != nil {
			RespondError(w, http.StatusInternalServerError, "failed to generate secret")
			return
		}
	}
	ep := &model.WebhookEndpoint{OrgID: orgID, URL: req.URL, Secret: secret, Events: events,
		Description: req.Description, Enabled: true}
	if uid, err := uuid.Parse(middleware.GetUserID(r.Context())); err == nil {
		ep.CreatedBy = &uid
	}
	if err := h.repo.CreateEndpoint(r.Context(), ep); err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to create webhook endpoint")
		return
	}
	RespondJSON(w, http.StatusCreated, ep) // the one response with the secret
}

func (h *WebhookEndpointHandler) Get(w http.ResponseWriter, r *http.Request) {
	ep, ok := h.owned(w, r)
	if !ok {
		return
	}
	ep.Secret = ""
	RespondJSON(w, http.StatusOK, ep)
}

func (h *WebhookEndpointHandler) Update(w http.ResponseWriter, r *http.Request) {
	ep, ok := h.owned(w, r)
	if !ok {
		return
	}
	var req model.UpdateWebhookEndpointRequest
	if !DecodeJSON(w, r, &req) {
		return
	}
	if req.URL != nil {
		if err := validateWebhookURL(*req.URL); err != nil {
			RespondError(w, http.StatusBadRequest, err.Error())
			return
		}
		ep.URL = *req.URL
	}
	if req.Events != nil {
		events, err := validateWebhookEvents(req.Events)
		if err != nil {
			RespondError(w, http.StatusBadRequest, err.Error())
			return
		}
		ep.Events = events
	}
	if req.Description != nil {
		ep.Description = *req.Description
	}
	if req.Enabled != nil {
		ep.Enabled = *req.Enabled
	}
	if req.RotateSecret {
		s, err := newWebhookSecret()
		if err != nil {
			RespondError(w, http.StatusInternalServerError, "failed to generate secret")
			return
		}
		ep.Secret = s
	}
	if err := h.repo.UpdateEndpoint(r.Context(), ep); err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to update webhook endpoint")
		return
	}
	if !req.RotateSecret {
		ep.Secret = ""
	}
	RespondJSON(w, http.StatusOK, ep)
}

func (h *WebhookEndpointHandler) Delete(w http.ResponseWriter, r *http.Request) {
	ep, ok := h.owned(w, r)
	if !ok {
		return
	}
	if err := h.repo.DeleteEndpoint(r.Context(), ep.OrgID, ep.ID); err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to delete webhook endpoint")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *WebhookEndpointHandler) Deliveries(w http.ResponseWriter, r *http.Request) {
	ep, ok := h.owned(w, r)
	if !ok {
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	out, err := h.repo.ListDeliveries(r.Context(), ep.OrgID, ep.ID, limit)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to list deliveries")
		return
	}
	RespondJSON(w, http.StatusOK, out)
}

// Test sends a signed "ping" to the endpoint now and returns the delivery
// (status succeeded, or pending with last_error and the next retry).
func (h *WebhookEndpointHandler) Test(w http.ResponseWriter, r *http.Request) {
	ep, ok := h.owned(w, r)
	if !ok {
		return
	}
	del, err := h.disp.Ping(r.Context(), ep)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to send test event")
		return
	}
	RespondJSON(w, http.StatusOK, del)
}
