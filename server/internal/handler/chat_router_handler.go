package handler

import (
	"net/http"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/chatsvc"
	"github.com/jobshout/server/internal/middleware"
)

// ChatRouterHandler exposes the chat agent as a stateless endpoint.
// Unlike the session-based chat handler, this one doesn't persist messages —
// it's intended for ad-hoc UI probes and tests.
type ChatRouterHandler struct {
	svc chatsvc.ChatRouterService
}

func NewChatRouterHandler(svc chatsvc.ChatRouterService) *ChatRouterHandler {
	return &ChatRouterHandler{svc: svc}
}

type RouteChatRequest struct {
	Message string `json:"message" validate:"required,min=1"`
}

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

	res, err := h.svc.Route(r.Context(), orgID, userID, uuid.New(), req.Message, nil)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "couldn't route that message")
		return
	}
	RespondJSON(w, http.StatusOK, res)
}
