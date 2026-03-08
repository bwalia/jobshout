package handler

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/middleware"
)

// KnowledgeHandler handles agent knowledge file endpoints.
type KnowledgeHandler struct {
	pool   *pgxpool.Pool
	logger *zap.Logger
}

// NewKnowledgeHandler constructs a KnowledgeHandler.
func NewKnowledgeHandler(pool *pgxpool.Pool, logger *zap.Logger) *KnowledgeHandler {
	return &KnowledgeHandler{pool: pool, logger: logger}
}

type knowledgeFileRow struct {
	ID        string    `json:"id"`
	AgentID   string    `json:"agent_id"`
	Filename  string    `json:"filename"`
	Content   string    `json:"content"`
	SizeBytes int       `json:"size_bytes"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ListByAgent returns all knowledge files for a given agent.
func (h *KnowledgeHandler) ListByAgent(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "agentID")

	rows, err := h.pool.Query(r.Context(),
		`SELECT id, agent_id, filename, content, size_bytes, created_at, updated_at
			FROM knowledge_files WHERE agent_id = $1 ORDER BY filename`, agentID)
	if err != nil {
		h.logger.Error("failed to list knowledge files", zap.Error(err))
		RespondError(w, http.StatusInternalServerError, "failed to list knowledge files")
		return
	}
	defer rows.Close()

	files := []knowledgeFileRow{}
	for rows.Next() {
		var f knowledgeFileRow
		if err := rows.Scan(&f.ID, &f.AgentID, &f.Filename, &f.Content, &f.SizeBytes, &f.CreatedAt, &f.UpdatedAt); err != nil {
			h.logger.Error("failed to scan knowledge file", zap.Error(err))
			continue
		}
		files = append(files, f)
	}

	RespondJSON(w, http.StatusOK, files)
}

// GetFile returns a single knowledge file.
func (h *KnowledgeHandler) GetFile(w http.ResponseWriter, r *http.Request) {
	fileID := chi.URLParam(r, "fileID")

	var f knowledgeFileRow
	err := h.pool.QueryRow(r.Context(),
		`SELECT id, agent_id, filename, content, size_bytes, created_at, updated_at
			FROM knowledge_files WHERE id = $1`, fileID,
	).Scan(&f.ID, &f.AgentID, &f.Filename, &f.Content, &f.SizeBytes, &f.CreatedAt, &f.UpdatedAt)
	if err != nil {
		RespondError(w, http.StatusNotFound, "knowledge file not found")
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
	agentID := chi.URLParam(r, "agentID")
	orgID, _ := r.Context().Value(middleware.ContextKeyOrgID).(string)

	var req createKnowledgeFileRequest
	if !DecodeJSON(w, r, &req) {
		return
	}

	if req.Filename == "" {
		RespondError(w, http.StatusBadRequest, "filename is required")
		return
	}

	// Verify agent belongs to user's org
	var agentOrgID string
	err := h.pool.QueryRow(r.Context(),
		`SELECT org_id FROM agents WHERE id = $1`, agentID).Scan(&agentOrgID)
	if err != nil || agentOrgID != orgID {
		RespondError(w, http.StatusNotFound, "agent not found")
		return
	}

	sizeBytes := len([]byte(req.Content))

	var id string
	err = h.pool.QueryRow(r.Context(),
		`INSERT INTO knowledge_files (agent_id, filename, content, size_bytes)
			VALUES ($1, $2, $3, $4) RETURNING id`,
		agentID, req.Filename, req.Content, sizeBytes,
	).Scan(&id)
	if err != nil {
		h.logger.Error("failed to create knowledge file", zap.Error(err))
		RespondError(w, http.StatusInternalServerError, "failed to create knowledge file")
		return
	}

	RespondJSON(w, http.StatusCreated, map[string]string{
		"id":       id,
		"filename": req.Filename,
	})
}

type updateKnowledgeFileRequest struct {
	Content string `json:"content"`
}

// UpdateFile updates the content of an existing knowledge file.
func (h *KnowledgeHandler) UpdateFile(w http.ResponseWriter, r *http.Request) {
	fileID := chi.URLParam(r, "fileID")

	var req updateKnowledgeFileRequest
	if !DecodeJSON(w, r, &req) {
		return
	}

	sizeBytes := len([]byte(req.Content))

	tag, err := h.pool.Exec(r.Context(),
		`UPDATE knowledge_files SET content = $1, size_bytes = $2, updated_at = NOW()
			WHERE id = $3`,
		req.Content, sizeBytes, fileID,
	)
	if err != nil {
		h.logger.Error("failed to update knowledge file", zap.Error(err))
		RespondError(w, http.StatusInternalServerError, "failed to update knowledge file")
		return
	}
	if tag.RowsAffected() == 0 {
		RespondError(w, http.StatusNotFound, "knowledge file not found")
		return
	}

	RespondJSON(w, http.StatusOK, map[string]string{"message": "updated"})
}

// DeleteFile deletes a knowledge file.
func (h *KnowledgeHandler) DeleteFile(w http.ResponseWriter, r *http.Request) {
	fileID := chi.URLParam(r, "fileID")

	tag, err := h.pool.Exec(r.Context(),
		`DELETE FROM knowledge_files WHERE id = $1`, fileID)
	if err != nil {
		h.logger.Error("failed to delete knowledge file", zap.Error(err))
		RespondError(w, http.StatusInternalServerError, "failed to delete knowledge file")
		return
	}
	if tag.RowsAffected() == 0 {
		RespondError(w, http.StatusNotFound, "knowledge file not found")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
