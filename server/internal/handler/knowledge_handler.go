package handler

import (
	"context"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/middleware"
	"github.com/jobshout/server/internal/repository"
	"github.com/jobshout/server/internal/service"
)

// KnowledgeHandler handles agent knowledge file endpoints. Every endpoint is
// scoped to the caller's org through the route's agent: a file of another org
// (or of another agent) is a 404, never a read or a write.
type KnowledgeHandler struct {
	store  repository.KnowledgeFileStore
	ingest service.KnowledgeIngestService
	logger *zap.Logger
}

// NewKnowledgeHandler constructs a KnowledgeHandler. ingest may be nil, in which
// case knowledge files are stored without vector ingestion.
func NewKnowledgeHandler(pool *pgxpool.Pool, ingest service.KnowledgeIngestService, logger *zap.Logger) *KnowledgeHandler {
	return NewKnowledgeHandlerWithStore(repository.NewKnowledgeFileStore(pool), ingest, logger)
}

// NewKnowledgeHandlerWithStore constructs a KnowledgeHandler over any store.
func NewKnowledgeHandlerWithStore(store repository.KnowledgeFileStore, ingest service.KnowledgeIngestService, logger *zap.Logger) *KnowledgeHandler {
	return &KnowledgeHandler{store: store, ingest: ingest, logger: logger}
}

// scope reads the caller's org and the route's agent (and, when wantFile, the
// file) ids, validating them as UUIDs. It writes the error response itself.
func (h *KnowledgeHandler) scope(w http.ResponseWriter, r *http.Request, wantFile bool) (orgID, agentID, fileID string, ok bool) {
	orgID = middleware.GetOrgID(r.Context())
	if _, err := uuid.Parse(orgID); err != nil {
		RespondError(w, http.StatusBadRequest, "invalid org_id")
		return "", "", "", false
	}
	agentID = chi.URLParam(r, "agentID")
	if _, err := uuid.Parse(agentID); err != nil {
		RespondError(w, http.StatusBadRequest, "invalid agent id")
		return "", "", "", false
	}
	if wantFile {
		fileID = chi.URLParam(r, "fileID")
		if _, err := uuid.Parse(fileID); err != nil {
			RespondError(w, http.StatusBadRequest, "invalid file id")
			return "", "", "", false
		}
	}
	return orgID, agentID, fileID, true
}

// ListByAgent returns all knowledge files for a given agent of the caller's org.
func (h *KnowledgeHandler) ListByAgent(w http.ResponseWriter, r *http.Request) {
	orgID, agentID, _, ok := h.scope(w, r, false)
	if !ok {
		return
	}
	inOrg, err := h.store.AgentInOrg(r.Context(), orgID, agentID)
	if err != nil {
		h.logger.Error("failed to check knowledge agent", zap.Error(err))
		RespondError(w, http.StatusInternalServerError, "failed to list knowledge files")
		return
	}
	if !inOrg {
		RespondError(w, http.StatusNotFound, "agent not found")
		return
	}
	files, err := h.store.ListByAgent(r.Context(), orgID, agentID)
	if err != nil {
		h.logger.Error("failed to list knowledge files", zap.Error(err))
		RespondError(w, http.StatusInternalServerError, "failed to list knowledge files")
		return
	}
	RespondJSON(w, http.StatusOK, files)
}

// GetFile returns a single knowledge file.
func (h *KnowledgeHandler) GetFile(w http.ResponseWriter, r *http.Request) {
	orgID, agentID, fileID, ok := h.scope(w, r, true)
	if !ok {
		return
	}
	f, err := h.store.Get(r.Context(), orgID, agentID, fileID)
	if err != nil {
		h.respondStoreErr(w, err, "failed to get knowledge file")
		return
	}
	RespondJSON(w, http.StatusOK, f)
}

type createKnowledgeFileRequest struct {
	Filename string `json:"filename"`
	Content  string `json:"content"`
}

// CreateFile creates a new knowledge file for an agent.
func (h *KnowledgeHandler) CreateFile(w http.ResponseWriter, r *http.Request) {
	orgID, agentID, _, ok := h.scope(w, r, false)
	if !ok {
		return
	}

	var req createKnowledgeFileRequest
	if !DecodeJSON(w, r, &req) {
		return
	}
	if req.Filename == "" {
		RespondError(w, http.StatusBadRequest, "filename is required")
		return
	}

	inOrg, err := h.store.AgentInOrg(r.Context(), orgID, agentID)
	if err != nil || !inOrg {
		RespondError(w, http.StatusNotFound, "agent not found")
		return
	}

	id, err := h.store.Create(r.Context(), agentID, req.Filename, req.Content)
	if err != nil {
		h.logger.Error("failed to create knowledge file", zap.Error(err))
		RespondError(w, http.StatusInternalServerError, "failed to create knowledge file")
		return
	}

	// Best-effort vector ingestion — never blocks the CRUD response.
	h.ingestFile(r.Context(), orgID, agentID, id, req.Content)

	RespondJSON(w, http.StatusCreated, map[string]string{
		"id":       id,
		"filename": req.Filename,
	})
}

// ingestFile runs best-effort vector ingestion for a knowledge file. Errors are
// logged and swallowed so knowledge-file CRUD is never blocked.
func (h *KnowledgeHandler) ingestFile(ctx context.Context, orgIDStr, agentIDStr, fileIDStr, content string) {
	if h.ingest == nil {
		return
	}
	orgID, err := uuid.Parse(orgIDStr)
	if err != nil {
		h.logger.Warn("knowledge ingest: invalid org id", zap.Error(err))
		return
	}
	agentID, err := uuid.Parse(agentIDStr)
	if err != nil {
		h.logger.Warn("knowledge ingest: invalid agent id", zap.Error(err))
		return
	}
	fileID, err := uuid.Parse(fileIDStr)
	if err != nil {
		h.logger.Warn("knowledge ingest: invalid file id", zap.Error(err))
		return
	}
	if err := h.ingest.IngestFile(ctx, orgID, agentID, fileID, content); err != nil {
		h.logger.Warn("knowledge ingest failed", zap.Error(err))
	}
}

type updateKnowledgeFileRequest struct {
	Content string `json:"content"`
}

// UpdateFile updates the content of an existing knowledge file.
func (h *KnowledgeHandler) UpdateFile(w http.ResponseWriter, r *http.Request) {
	orgID, agentID, fileID, ok := h.scope(w, r, true)
	if !ok {
		return
	}

	var req updateKnowledgeFileRequest
	if !DecodeJSON(w, r, &req) {
		return
	}

	if err := h.store.UpdateContent(r.Context(), orgID, agentID, fileID, req.Content); err != nil {
		h.respondStoreErr(w, err, "failed to update knowledge file")
		return
	}

	// Best-effort re-ingestion — never blocks the CRUD response.
	h.ingestFile(r.Context(), orgID, agentID, fileID, req.Content)

	RespondJSON(w, http.StatusOK, map[string]string{"message": "updated"})
}

// DeleteFile deletes a knowledge file.
func (h *KnowledgeHandler) DeleteFile(w http.ResponseWriter, r *http.Request) {
	orgID, agentID, fileID, ok := h.scope(w, r, true)
	if !ok {
		return
	}
	if err := h.store.Delete(r.Context(), orgID, agentID, fileID); err != nil {
		h.respondStoreErr(w, err, "failed to delete knowledge file")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *KnowledgeHandler) respondStoreErr(w http.ResponseWriter, err error, msg string) {
	if errors.Is(err, repository.ErrKnowledgeNotFound) {
		RespondError(w, http.StatusNotFound, "knowledge file not found")
		return
	}
	h.logger.Error(msg, zap.Error(err))
	RespondError(w, http.StatusInternalServerError, msg)
}
