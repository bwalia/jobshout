package handler

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/repository"
)

// WebhookHandler handles inbound webhooks from external systems (Jira, GitHub).
type WebhookHandler struct {
	integRepo repository.IntegrationRepository
	linkRepo  repository.TaskLinkRepository
	logger    *zap.Logger
}

func NewWebhookHandler(
	integRepo repository.IntegrationRepository,
	linkRepo repository.TaskLinkRepository,
	logger *zap.Logger,
) *WebhookHandler {
	return &WebhookHandler{
		integRepo: integRepo,
		linkRepo:  linkRepo,
		logger:    logger,
	}
}

// Jira handles POST /webhooks/jira/{integrationID}
func (h *WebhookHandler) Jira(w http.ResponseWriter, r *http.Request) {
	integrationID, err := uuid.Parse(chi.URLParam(r, "integrationID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid integration id")
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		RespondError(w, http.StatusBadRequest, "failed to read body")
		return
	}
	defer r.Body.Close()

	// Parse the Jira webhook payload
	var payload struct {
		WebhookEvent string `json:"webhookEvent"`
		Issue        struct {
			Key    string `json:"key"`
			Fields struct {
				Summary string `json:"summary"`
				Status  struct {
					Name string `json:"name"`
				} `json:"status"`
			} `json:"fields"`
		} `json:"issue"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		RespondError(w, http.StatusBadRequest, "invalid payload")
		return
	}

	h.logger.Info("jira webhook received",
		zap.String("integration_id", integrationID.String()),
		zap.String("event", payload.WebhookEvent),
		zap.String("issue_key", payload.Issue.Key),
	)

	// Look up the task link
	link, err := h.linkRepo.FindByExternalID(r.Context(), integrationID, payload.Issue.Key)
	if err != nil || link == nil {
		// No linked task — acknowledge but ignore
		w.WriteHeader(http.StatusOK)
		return
	}

	// TODO: update local task status based on Jira status change
	h.logger.Info("jira webhook matched task link",
		zap.String("task_id", link.TaskID.String()),
		zap.String("external_id", link.ExternalID),
	)

	w.WriteHeader(http.StatusOK)
}

// GitHub handles POST /webhooks/github/{integrationID}
func (h *WebhookHandler) GitHub(w http.ResponseWriter, r *http.Request) {
	integrationID, err := uuid.Parse(chi.URLParam(r, "integrationID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid integration id")
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		RespondError(w, http.StatusBadRequest, "failed to read body")
		return
	}
	defer r.Body.Close()

	eventType := r.Header.Get("X-GitHub-Event")

	var payload struct {
		Action string `json:"action"`
		Issue  struct {
			Number int    `json:"number"`
			Title  string `json:"title"`
			State  string `json:"state"`
		} `json:"issue"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		RespondError(w, http.StatusBadRequest, "invalid payload")
		return
	}

	h.logger.Info("github webhook received",
		zap.String("integration_id", integrationID.String()),
		zap.String("event", eventType),
		zap.String("action", payload.Action),
		zap.Int("issue_number", payload.Issue.Number),
	)

	// Look up the task link
	externalID := ""
	if payload.Issue.Number > 0 {
		externalID = json.Number(json.Number(payload.Issue.Number).String()).String()
	}
	link, err := h.linkRepo.FindByExternalID(r.Context(), integrationID, externalID)
	if err != nil || link == nil {
		w.WriteHeader(http.StatusOK)
		return
	}

	// TODO: update local task status based on GitHub issue state change
	h.logger.Info("github webhook matched task link",
		zap.String("task_id", link.TaskID.String()),
		zap.String("external_id", link.ExternalID),
	)

	w.WriteHeader(http.StatusOK)
}
