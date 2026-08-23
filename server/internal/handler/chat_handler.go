package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"

	"github.com/jobshout/server/internal/chatstream"
	"github.com/jobshout/server/internal/middleware"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/service"
)

// ChatHandler handles chat session and message endpoints.
type ChatHandler struct {
	svc      service.ChatService
	validate *validator.Validate
}

// NewChatHandler creates a new ChatHandler.
func NewChatHandler(svc service.ChatService) *ChatHandler {
	return &ChatHandler{svc: svc, validate: validator.New()}
}

// StartSession creates a new chat session.
func (h *ChatHandler) StartSession(w http.ResponseWriter, r *http.Request) {
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

	var req model.StartChatSessionRequest
	if !DecodeJSON(w, r, &req) {
		return
	}

	session, err := h.svc.StartSession(r.Context(), orgID, userID, req)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	RespondJSON(w, http.StatusCreated, session)
}

// ListSessions lists chat sessions for the current user.
func (h *ChatHandler) ListSessions(w http.ResponseWriter, r *http.Request) {
	orgID, _ := uuid.Parse(middleware.GetOrgID(r.Context()))
	userID, _ := uuid.Parse(middleware.GetUserID(r.Context()))

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	perPage, _ := strconv.Atoi(r.URL.Query().Get("per_page"))
	params := model.PaginationParams{Page: page, PerPage: perPage}

	result, err := h.svc.ListSessions(r.Context(), orgID, userID, params)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	RespondJSON(w, http.StatusOK, result)
}

// GetHistory returns messages for a chat session.
func (h *ChatHandler) GetHistory(w http.ResponseWriter, r *http.Request) {
	sessionID, err := uuid.Parse(chi.URLParam(r, "sessionID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid session ID")
		return
	}

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = 50
	}

	messages, err := h.svc.GetHistory(r.Context(), sessionID, limit)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if messages == nil {
		messages = []model.ChatMessage{}
	}
	RespondJSON(w, http.StatusOK, messages)
}

// SendMessage sends a message in a chat session and returns the response.
func (h *ChatHandler) SendMessage(w http.ResponseWriter, r *http.Request) {
	orgID, _ := uuid.Parse(middleware.GetOrgID(r.Context()))
	userID, _ := uuid.Parse(middleware.GetUserID(r.Context()))

	sessionID, err := uuid.Parse(chi.URLParam(r, "sessionID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid session ID")
		return
	}

	var req model.SendChatMessageRequest
	if !DecodeJSON(w, r, &req) {
		return
	}
	if err := h.validate.Struct(req); err != nil {
		RespondError(w, http.StatusBadRequest, err.Error())
		return
	}

	source := req.Source
	if source == "" {
		source = model.ChatSourceWeb
	}

	userMsg, agentMsg, err := h.svc.SendMessage(r.Context(), orgID, userID, sessionID, req.Content, source)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	RespondJSON(w, http.StatusOK, map[string]any{
		"user_message":  userMsg,
		"agent_message": agentMsg,
	})
}

// StreamMessage POST /api/v1/chat/sessions/{sessionID}/messages/stream
//
// Server-Sent Events variant of SendMessage: it runs the exact same routing +
// execution path, but streams safe progress frames (status + tool activity)
// while the turn runs, then a final "message" frame and "done". Progress comes
// from a per-request emitter placed on the context, which the executor writes to
// as it works — no layer between here and the executor changed its signature.
func (h *ChatHandler) StreamMessage(w http.ResponseWriter, r *http.Request) {
	orgID, _ := uuid.Parse(middleware.GetOrgID(r.Context()))
	userID, _ := uuid.Parse(middleware.GetUserID(r.Context()))

	sessionID, err := uuid.Parse(chi.URLParam(r, "sessionID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid session ID")
		return
	}

	var req model.SendChatMessageRequest
	if !DecodeJSON(w, r, &req) {
		return
	}
	if err := h.validate.Struct(req); err != nil {
		RespondError(w, http.StatusBadRequest, err.Error())
		return
	}
	source := req.Source
	if source == "" {
		source = model.ChatSourceWeb
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		RespondError(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // defeat proxy buffering (nginx)
	w.WriteHeader(http.StatusOK)

	// Buffered so a burst of tool events never blocks the executor; progress
	// events drop if the buffer is full (the reader is normally faster).
	ch := make(chan chatstream.Event, 256)
	emit := func(ev chatstream.Event) {
		select {
		case ch <- ev:
		default:
		}
	}
	ctx := chatstream.WithEmitter(r.Context(), emit)

	go func() {
		defer close(ch)
		chatstream.Status(ctx, "planning", nil)
		_, agentMsg, serr := h.svc.SendMessage(ctx, orgID, userID, sessionID, req.Content, source)
		if serr != nil {
			// Blocking send guarded by disconnect: the terminal frames must not
			// be dropped under a full buffer.
			sendFinal(ctx, ch, chatstream.Event{Type: chatstream.EventError, Data: map[string]any{"message": serr.Error()}})
			return
		}
		sendFinal(ctx, ch, chatstream.Event{Type: "message", Data: map[string]any{
			"id":       agentMsg.ID.String(),
			"role":     agentMsg.Role,
			"content":  agentMsg.Content,
			"metadata": agentMsg.Metadata,
		}})
		sendFinal(ctx, ch, chatstream.Event{Type: chatstream.EventStatus, Data: map[string]any{"state": "completed"}})
	}()

	for ev := range ch {
		data, mErr := json.Marshal(ev)
		if mErr != nil {
			continue
		}
		_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.Type, data)
		flusher.Flush()
	}
	_, _ = fmt.Fprint(w, "event: done\ndata: {}\n\n")
	flusher.Flush()
}

// sendFinal delivers a terminal frame with a blocking send, abandoning it only
// if the client has disconnected — so the final message/error is never dropped.
func sendFinal(ctx context.Context, ch chan<- chatstream.Event, ev chatstream.Event) {
	select {
	case ch <- ev:
	case <-ctx.Done():
	}
}
