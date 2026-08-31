package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"

	"github.com/jobshout/server/internal/chatagent"
	"github.com/jobshout/server/internal/chatsvc"
	"github.com/jobshout/server/internal/middleware"
	"github.com/jobshout/server/internal/model"
)

// ChatHandler handles chat session and message endpoints.
type ChatHandler struct {
	svc      chatsvc.ChatService
	validate *validator.Validate
}

func NewChatHandler(svc chatsvc.ChatService) *ChatHandler {
	return &ChatHandler{svc: svc, validate: validator.New()}
}

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
		RespondError(w, http.StatusInternalServerError, "couldn't start a chat session")
		return
	}
	RespondJSON(w, http.StatusCreated, session)
}

func (h *ChatHandler) ListSessions(w http.ResponseWriter, r *http.Request) {
	orgID, _ := uuid.Parse(middleware.GetOrgID(r.Context()))
	userID, _ := uuid.Parse(middleware.GetUserID(r.Context()))

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	perPage, _ := strconv.Atoi(r.URL.Query().Get("per_page"))
	params := model.PaginationParams{Page: page, PerPage: perPage}

	result, err := h.svc.ListSessions(r.Context(), orgID, userID, params)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "couldn't list chat sessions")
		return
	}
	RespondJSON(w, http.StatusOK, result)
}

func (h *ChatHandler) GetHistory(w http.ResponseWriter, r *http.Request) {
	orgID, _ := uuid.Parse(middleware.GetOrgID(r.Context()))
	userID, _ := uuid.Parse(middleware.GetUserID(r.Context()))
	sessionID, err := uuid.Parse(chi.URLParam(r, "sessionID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid session ID")
		return
	}

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = 50
	}

	messages, err := h.svc.GetHistory(r.Context(), orgID, userID, sessionID, limit)
	if err != nil {
		RespondError(w, http.StatusNotFound, "session not found")
		return
	}
	if messages == nil {
		messages = []model.ChatMessage{}
	}
	RespondJSON(w, http.StatusOK, messages)
}

func (h *ChatHandler) DeleteSession(w http.ResponseWriter, r *http.Request) {
	orgID, _ := uuid.Parse(middleware.GetOrgID(r.Context()))
	userID, _ := uuid.Parse(middleware.GetUserID(r.Context()))
	sessionID, err := uuid.Parse(chi.URLParam(r, "sessionID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid session ID")
		return
	}
	if err := h.svc.DeleteSession(r.Context(), orgID, userID, sessionID); err != nil {
		RespondError(w, http.StatusNotFound, "session not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

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

	turn, err := h.svc.SendTurn(r.Context(), orgID, userID, sessionID, req.Content, req.DisplayContent, source, req.ConfirmationToken, nil)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "couldn't send that message")
		return
	}

	RespondJSON(w, http.StatusOK, map[string]any{
		"user_message":  turn.UserMessage,
		"agent_message": turn.AgentMessage,
		"response":      turn.Response,
	})
}

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
		RespondError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	writeEv := func(ev chatagent.Event) {
		data, err := json.Marshal(ev)
		if err != nil {
			return
		}
		if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.Type, data); err != nil {
			return
		}
		flusher.Flush()
	}

	_, err = h.svc.SendTurn(r.Context(), orgID, userID, sessionID, req.Content, req.DisplayContent, source, req.ConfirmationToken, writeEv)
	if err != nil {
		writeEv(chatagent.Event{Type: chatagent.EventError, Error: "couldn't send that message"})
	}
}
