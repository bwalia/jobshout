package handler

import (
	"net/http"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/middleware"
	"github.com/jobshout/server/internal/service"
)

// ChatRouterHandler exposes the 12-stage chat router as a stateless endpoint.
// Unlike the session-based chat handler, this one doesn't persist messages —
// it's intended for ad-hoc UI probes, Slack slash commands, and tests that
// want the intent classification without setting up a chat_session.
type ChatRouterHandler struct {
	svc service.ChatRouterService
}

// NewChatRouterHandler creates the handler.
func NewChatRouterHandler(svc service.ChatRouterService) *ChatRouterHandler {
	return &ChatRouterHandler{svc: svc}
}

// RouteChatRequest is the request body for POST /api/v1/chat/route.
type RouteChatRequest struct {
	Message string `json:"message" validate:"required,min=1"`
}

// Route classifies a single message and, depending on the intent, executes the
// matching agent or workflow. The response is the full ChatRouteResult so
// callers can render tailored UIs (e.g. surface the clarify question inline).
func (h *ChatRouterHandler) Route(w http.ResponseWriter, r *http.Request) {
	orgID, err := uuid.Parse(middleware.GetOrgID(r.Context()))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid org ID")
		return
	}
	userID, err := uuid.Parse(middleware.GetUserID(r.Context()))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid user ID")
		return
	}

	var req RouteChatRequest
	if !DecodeJSON(w, r, &req) {
		return
	}
	if req.Message == "" {
		RespondError(w, http.StatusBadRequest, "message is required")
		return
	}

	// sessionID is not persisted here — we pass a fresh UUID so downstream
	// components that key by session still get a stable value per request.
	res, err := h.svc.Route(r.Context(), orgID, userID, uuid.New(), req.Message, nil)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	RespondJSON(w, http.StatusOK, res)
}
