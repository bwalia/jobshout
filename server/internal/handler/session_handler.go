package handler

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"

	"github.com/jobshout/server/internal/middleware"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/repository"
)

type SessionHandler struct {
	repo     repository.SessionRepository
	validate *validator.Validate
}

func NewSessionHandler(repo repository.SessionRepository) *SessionHandler {
	return &SessionHandler{repo: repo, validate: validator.New()}
}

func (h *SessionHandler) List(w http.ResponseWriter, r *http.Request) {
	orgID, err := uuid.Parse(middleware.GetOrgID(r.Context()))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid org_id")
		return
	}

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	perPage, _ := strconv.Atoi(r.URL.Query().Get("per_page"))
	params := model.PaginationParams{Page: page, PerPage: perPage}

	result, err := h.repo.List(r.Context(), orgID, params)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to list sessions")
		return
	}
	RespondJSON(w, http.StatusOK, result)
}

func (h *SessionHandler) Create(w http.ResponseWriter, r *http.Request) {
	orgID, err := uuid.Parse(middleware.GetOrgID(r.Context()))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid org_id")
		return
	}
	userID, _ := uuid.Parse(middleware.GetUserID(r.Context()))

	var req model.CreateSessionRequest
	if !DecodeJSON(w, r, &req) {
		return
	}
	if err := h.validate.Struct(req); err != nil {
		RespondError(w, http.StatusBadRequest, "validation: "+err.Error())
		return
	}

	s := &model.Session{
		ID:              uuid.New(),
		OrgID:           orgID,
		Name:            req.Name,
		Description:     req.Description,
		ModelName:       req.ModelName,
		Status:          "active",
		ContextMessages: []model.SessionMsg{},
		Tags:            req.Tags,
		CreatedBy:       &userID,
	}
	if s.Tags == nil {
		s.Tags = []string{}
	}

	if req.ProviderConfigID != nil {
		id, _ := uuid.Parse(*req.ProviderConfigID)
		s.ProviderConfigID = &id
	}

	if err := h.repo.Create(r.Context(), s); err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to create session: "+err.Error())
		return
	}

	RespondJSON(w, http.StatusCreated, s)
}

func (h *SessionHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "sessionID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid session ID")
		return
	}

	s, err := h.repo.GetByID(r.Context(), id)
	if err != nil {
		RespondError(w, http.StatusNotFound, "session not found")
		return
	}
	RespondJSON(w, http.StatusOK, s)
}

func (h *SessionHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "sessionID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid session ID")
		return
	}

	var req model.UpdateSessionRequest
	if !DecodeJSON(w, r, &req) {
		return
	}

	s, err := h.repo.Update(r.Context(), id, req)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to update session")
		return
	}
	RespondJSON(w, http.StatusOK, s)
}

func (h *SessionHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "sessionID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid session ID")
		return
	}

	if err := h.repo.Delete(r.Context(), id); err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to delete session")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// CopyContext copies context from a source session into the target session.
func (h *SessionHandler) CopyContext(w http.ResponseWriter, r *http.Request) {
	targetID, err := uuid.Parse(chi.URLParam(r, "sessionID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid session ID")
		return
	}

	var req model.CopyContextRequest
	if !DecodeJSON(w, r, &req) {
		return
	}
	if err := h.validate.Struct(req); err != nil {
		RespondError(w, http.StatusBadRequest, "validation: "+err.Error())
		return
	}

	sourceID, _ := uuid.Parse(req.SourceSessionID)
	source, err := h.repo.GetByID(r.Context(), sourceID)
	if err != nil {
		RespondError(w, http.StatusNotFound, "source session not found")
		return
	}

	// Filter messages
	msgs := source.ContextMessages
	if !req.IncludeSystem {
		filtered := make([]model.SessionMsg, 0, len(msgs))
		for _, m := range msgs {
			if m.Role != "system" {
				filtered = append(filtered, m)
			}
		}
		msgs = filtered
	}

	if req.MaxMessages != nil && *req.MaxMessages > 0 && len(msgs) > *req.MaxMessages {
		msgs = msgs[len(msgs)-*req.MaxMessages:]
	}

	// Append to target
	var tokensDelta int
	for _, m := range msgs {
		tokensDelta += m.TokenCount
	}

	if err := h.repo.AppendMessages(r.Context(), targetID, msgs, tokensDelta); err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to copy context: "+err.Error())
		return
	}

	// Return updated session
	updated, _ := h.repo.GetByID(r.Context(), targetID)
	RespondJSON(w, http.StatusOK, updated)
}

// CreateSnapshot saves a snapshot of the current session context.
func (h *SessionHandler) CreateSnapshot(w http.ResponseWriter, r *http.Request) {
	sessionID, err := uuid.Parse(chi.URLParam(r, "sessionID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid session ID")
		return
	}

	var req model.CreateSnapshotRequest
	if !DecodeJSON(w, r, &req) {
		return
	}
	if err := h.validate.Struct(req); err != nil {
		RespondError(w, http.StatusBadRequest, "validation: "+err.Error())
		return
	}

	session, err := h.repo.GetByID(r.Context(), sessionID)
	if err != nil {
		RespondError(w, http.StatusNotFound, "session not found")
		return
	}

	snap := &model.SessionSnapshot{
		ID:              uuid.New(),
		SessionID:       sessionID,
		Name:            req.Name,
		Description:     req.Description,
		ContextMessages: session.ContextMessages,
		ModelName:       session.ModelName,
		TotalTokens:     session.TotalTokens,
		MessageCount:    session.MessageCount,
	}

	if err := h.repo.CreateSnapshot(r.Context(), snap); err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to create snapshot")
		return
	}
	RespondJSON(w, http.StatusCreated, snap)
}

// ListSnapshots returns all snapshots for a session.
func (h *SessionHandler) ListSnapshots(w http.ResponseWriter, r *http.Request) {
	sessionID, err := uuid.Parse(chi.URLParam(r, "sessionID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid session ID")
		return
	}

	snaps, err := h.repo.ListSnapshots(r.Context(), sessionID)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to list snapshots")
		return
	}
	if snaps == nil {
		snaps = []model.SessionSnapshot{}
	}
	RespondJSON(w, http.StatusOK, snaps)
}

// RestoreSnapshot loads a snapshot's context back into the session.
func (h *SessionHandler) RestoreSnapshot(w http.ResponseWriter, r *http.Request) {
	sessionID, err := uuid.Parse(chi.URLParam(r, "sessionID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid session ID")
		return
	}

	snapID, err := uuid.Parse(chi.URLParam(r, "snapshotID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid snapshot ID")
		return
	}

	snap, err := h.repo.GetSnapshot(r.Context(), snapID)
	if err != nil {
		RespondError(w, http.StatusNotFound, "snapshot not found")
		return
	}

	// Verify snapshot belongs to this session
	if snap.SessionID != sessionID {
		RespondError(w, http.StatusBadRequest, "snapshot does not belong to this session")
		return
	}

	// Replace session context with snapshot
	if err := h.repo.AppendMessages(r.Context(), sessionID, snap.ContextMessages, snap.TotalTokens); err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to restore snapshot")
		return
	}

	updated, _ := h.repo.GetByID(r.Context(), sessionID)
	RespondJSON(w, http.StatusOK, updated)
}
