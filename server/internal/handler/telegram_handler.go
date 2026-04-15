package handler

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/integration/adapters/telegram"
	"github.com/jobshout/server/internal/middleware"
	"github.com/jobshout/server/internal/service"
)

// TelegramHandler handles Telegram webhook and account management endpoints.
type TelegramHandler struct {
	svc         service.TelegramService
	secretToken string
	logger      *zap.Logger
}

// NewTelegramHandler creates a new TelegramHandler.
func NewTelegramHandler(svc service.TelegramService, secretToken string, logger *zap.Logger) *TelegramHandler {
	return &TelegramHandler{svc: svc, secretToken: secretToken, logger: logger}
}

// Webhook handles incoming Telegram updates. This endpoint is public but
// verified by the X-Telegram-Bot-Api-Secret-Token header.
func (h *TelegramHandler) Webhook(w http.ResponseWriter, r *http.Request) {
	// Validate the secret token.
	if h.secretToken != "" {
		headerToken := r.Header.Get("X-Telegram-Bot-Api-Secret-Token")
		if headerToken != h.secretToken {
			h.logger.Warn("telegram webhook: invalid secret token")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
	}

	var update telegram.Update
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		h.logger.Warn("telegram webhook: failed to decode update", zap.Error(err))
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Process asynchronously and return 200 immediately (Telegram retries after 5s).
	go func() {
		if err := h.svc.HandleUpdate(r.Context(), &update); err != nil {
			h.logger.Error("telegram webhook: handle update failed", zap.Error(err))
		}
	}()

	w.WriteHeader(http.StatusOK)
}

// GenerateLinkToken creates a one-time token for linking a Telegram account.
func (h *TelegramHandler) GenerateLinkToken(w http.ResponseWriter, r *http.Request) {
	userID, _ := uuid.Parse(middleware.GetUserID(r.Context()))
	orgID, _ := uuid.Parse(middleware.GetOrgID(r.Context()))

	token, err := h.svc.GenerateLinkToken(r.Context(), userID, orgID)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	RespondJSON(w, http.StatusOK, token)
}

// UnlinkUser removes the Telegram link for the current user.
func (h *TelegramHandler) UnlinkUser(w http.ResponseWriter, r *http.Request) {
	userID, _ := uuid.Parse(middleware.GetUserID(r.Context()))

	if err := h.svc.UnlinkUser(r.Context(), userID); err != nil {
		RespondError(w, http.StatusNotFound, err.Error())
		return
	}

	RespondJSON(w, http.StatusOK, map[string]string{"status": "unlinked"})
}

// LinkStatus returns the current Telegram linking status.
func (h *TelegramHandler) LinkStatus(w http.ResponseWriter, r *http.Request) {
	userID, _ := uuid.Parse(middleware.GetUserID(r.Context()))

	status, err := h.svc.GetLinkStatus(r.Context(), userID)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	RespondJSON(w, http.StatusOK, status)
}
