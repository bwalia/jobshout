package handler

import (
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/model"
)

// Externally keyed knowledge: a system of record (OpsAPI) that versions and
// approves knowledge pushes each approved item under its own key and version.
// The item lives in an ordinary knowledge file named
//
//	ext/<key>@v<version>.md
//
// so the agent's normal retrieval (run-start injection and knowledge_search)
// picks it up, and the key and version are visible in the file list. Pushing a
// newer version replaces every older one; an older version is refused (409),
// so out-of-order deliveries cannot roll a rule back.

var externalKeyRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._\-]{0,99}$`)

const externalPrefix = "ext/"

// ExternalKnowledgeFilename is the stored filename for key at version.
func ExternalKnowledgeFilename(key string, version int) string {
	return fmt.Sprintf("%s%s@v%d.md", externalPrefix, key, version)
}

// parseExternalFilename returns the key and version of an externally keyed
// file, or ok=false for any other file.
func parseExternalFilename(name string) (key string, version int, ok bool) {
	if !strings.HasPrefix(name, externalPrefix) || !strings.HasSuffix(name, ".md") {
		return "", 0, false
	}
	rest := strings.TrimSuffix(strings.TrimPrefix(name, externalPrefix), ".md")
	at := strings.LastIndex(rest, "@v")
	if at <= 0 {
		return "", 0, false
	}
	v, err := strconv.Atoi(rest[at+2:])
	if err != nil || v < 1 {
		return "", 0, false
	}
	return rest[:at], v, true
}

type putExternalKnowledgeRequest struct {
	Version int    `json:"version"`
	Title   string `json:"title"`
	Content string `json:"content"`
	// Scope and Source are recorded in the file header (e.g. "company",
	// "opsapi"); informational.
	Scope  string `json:"scope"`
	Source string `json:"source"`
}

// renderExternalKnowledge puts the key and version in the text itself, so a
// retrieved chunk can be cited as knowledge <key> v<version>.
func renderExternalKnowledge(key string, req putExternalKnowledgeRequest) string {
	var b strings.Builder
	fmt.Fprintf(&b, "[knowledge id=%s version=%d", key, req.Version)
	if s := strings.TrimSpace(req.Scope); s != "" {
		fmt.Fprintf(&b, " scope=%s", s)
	}
	if s := strings.TrimSpace(req.Source); s != "" {
		fmt.Fprintf(&b, " source=%s", s)
	}
	b.WriteString("]\n")
	if t := strings.TrimSpace(req.Title); t != "" {
		b.WriteString("# " + t + "\n\n")
	}
	b.WriteString(strings.TrimSpace(req.Content))
	b.WriteString("\n")
	return b.String()
}

func (h *KnowledgeHandler) externalScope(w http.ResponseWriter, r *http.Request) (orgID, agentID, key string, ok bool) {
	orgID, agentID, _, ok = h.scope(w, r, false)
	if !ok {
		return "", "", "", false
	}
	key = chi.URLParam(r, "key")
	if !externalKeyRe.MatchString(key) {
		RespondError(w, http.StatusBadRequest, "invalid key (letters, digits, . _ - ; max 100)")
		return "", "", "", false
	}
	inOrg, err := h.store.AgentInOrg(r.Context(), orgID, agentID)
	if err != nil {
		h.logger.Error("failed to check knowledge agent", zap.Error(err))
		RespondError(w, http.StatusInternalServerError, "failed to check agent")
		return "", "", "", false
	}
	if !inOrg {
		RespondError(w, http.StatusNotFound, "agent not found")
		return "", "", "", false
	}
	return orgID, agentID, key, true
}

func (h *KnowledgeHandler) externalFiles(r *http.Request, orgID, agentID, key string) ([]model.KnowledgeFile, int, error) {
	files, err := h.store.ListByAgent(r.Context(), orgID, agentID)
	if err != nil {
		return nil, 0, err
	}
	var mine []model.KnowledgeFile
	maxV := 0
	for _, f := range files {
		k, v, ok := parseExternalFilename(f.Filename)
		if !ok || k != key {
			continue
		}
		mine = append(mine, f)
		if v > maxV {
			maxV = v
		}
	}
	return mine, maxV, nil
}

// PutExternal upserts one externally keyed knowledge item.
//
//	PUT /agents/{agentID}/knowledge/external/{key}
//	{"version": 3, "title": "...", "content": "...", "scope": "company", "source": "opsapi"}
//
// 201 created (first version or a newer one replacing older versions), 200
// for the same version (content refreshed), 409 when a newer version is
// already stored.
func (h *KnowledgeHandler) PutExternal(w http.ResponseWriter, r *http.Request) {
	orgID, agentID, key, ok := h.externalScope(w, r)
	if !ok {
		return
	}
	var req putExternalKnowledgeRequest
	if !DecodeJSON(w, r, &req) {
		return
	}
	if req.Version < 1 {
		RespondError(w, http.StatusBadRequest, "version must be a positive integer")
		return
	}
	if strings.TrimSpace(req.Content) == "" {
		RespondError(w, http.StatusBadRequest, "content is required")
		return
	}
	existing, maxV, err := h.externalFiles(r, orgID, agentID, key)
	if err != nil {
		h.logger.Error("failed to list knowledge files", zap.Error(err))
		RespondError(w, http.StatusInternalServerError, "failed to read knowledge")
		return
	}
	if maxV > req.Version {
		RespondJSON(w, http.StatusConflict, map[string]any{
			"error": "a newer version is already stored", "key": key, "current_version": maxV,
		})
		return
	}

	content := renderExternalKnowledge(key, req)
	filename := ExternalKnowledgeFilename(key, req.Version)
	var fileID, status string
	for _, f := range existing {
		if f.Filename == filename {
			fileID = f.ID
		}
	}
	if fileID != "" {
		status = "updated"
		if err := h.store.UpdateContent(r.Context(), orgID, agentID, fileID, content); err != nil {
			h.respondStoreErr(w, err, "failed to update knowledge")
			return
		}
	} else {
		status = "created"
		if maxV > 0 {
			status = "replaced"
		}
		fileID, err = h.store.Create(r.Context(), agentID, filename, content)
		if err != nil {
			h.logger.Error("failed to create knowledge file", zap.Error(err))
			RespondError(w, http.StatusInternalServerError, "failed to store knowledge")
			return
		}
	}
	// Retire every other version of this key (their chunks cascade).
	removed := 0
	for _, f := range existing {
		if f.ID == fileID {
			continue
		}
		if err := h.store.Delete(r.Context(), orgID, agentID, f.ID); err != nil {
			h.logger.Warn("failed to retire old knowledge version", zap.Error(err), zap.String("file", f.ID))
			continue
		}
		removed++
	}
	h.ingestFile(r.Context(), orgID, agentID, fileID, content)

	code := http.StatusCreated
	if status == "updated" {
		code = http.StatusOK
	}
	RespondJSON(w, code, map[string]any{
		"key": key, "version": req.Version, "file_id": fileID, "filename": filename,
		"status": status, "removed_versions": removed,
	})
}

// DeleteExternal removes every version of an externally keyed item (a rule
// retired in the system of record). Idempotent: 204 even when none exist.
func (h *KnowledgeHandler) DeleteExternal(w http.ResponseWriter, r *http.Request) {
	orgID, agentID, key, ok := h.externalScope(w, r)
	if !ok {
		return
	}
	existing, _, err := h.externalFiles(r, orgID, agentID, key)
	if err != nil {
		h.logger.Error("failed to list knowledge files", zap.Error(err))
		RespondError(w, http.StatusInternalServerError, "failed to read knowledge")
		return
	}
	for _, f := range existing {
		if err := h.store.Delete(r.Context(), orgID, agentID, f.ID); err != nil {
			h.respondStoreErr(w, err, "failed to delete knowledge")
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}
