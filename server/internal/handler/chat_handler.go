package handler

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"

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
