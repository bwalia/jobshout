package handler

import (
	"net/http"

	"go.uber.org/zap"

	"github.com/jobshout/server/internal/middleware"
	ws "github.com/jobshout/server/internal/websocket"
)

// WSHandler upgrades HTTP connections to WebSocket and registers the client
// with the hub for real-time event delivery.
type WSHandler struct {
	hub    *ws.Hub
	logger *zap.Logger
}

// NewWSHandler constructs a WSHandler.
func NewWSHandler(hub *ws.Hub, logger *zap.Logger) *WSHandler {
	return &WSHandler{hub: hub, logger: logger}
}

// Connect upgrades to WebSocket and starts the read/write pumps.
// The user must be authenticated (RequireAuth middleware) so that user_id and
// org_id are present in the request context.
func (h *WSHandler) Connect(w http.ResponseWriter, r *http.Request) {
	userID, _ := r.Context().Value(middleware.ContextKeyUserID).(string)
	orgID, _ := r.Context().Value(middleware.ContextKeyOrgID).(string)

	if userID == "" || orgID == "" {
		RespondError(w, http.StatusUnauthorized, "missing authentication context")
		return
	}

	client, err := ws.NewClient(h.hub, w, r, orgID, userID, h.logger)
	if err != nil {
		h.logger.Error("websocket upgrade failed",
			zap.String("user_id", userID),
			zap.Error(err),
		)
		return
	}

	h.hub.Register(client)
	client.Start()
}
