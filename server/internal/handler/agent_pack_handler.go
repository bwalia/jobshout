package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/jobshout/server/internal/agentpack"
	"github.com/jobshout/server/internal/middleware"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/repository"
	"github.com/jobshout/server/internal/service"
)

type AgentPackHandler struct {
	svc  service.AgentPackService
	rbac middleware.RBACService
}

func NewAgentPackHandler(svc service.AgentPackService, rbac middleware.RBACService) *AgentPackHandler {
	return &AgentPackHandler{svc: svc, rbac: rbac}
}

func (h *AgentPackHandler) Export(w http.ResponseWriter, r *http.Request) {
	orgID, err := uuid.Parse(middleware.GetOrgID(r.Context()))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid org_id in token")
		return
	}
	userID, err := uuid.Parse(middleware.GetUserID(r.Context()))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid user_id in token")
		return
	}
	agentID, err := uuid.Parse(chi.URLParam(r, "agentID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid agent ID")
		return
	}
	pkg, filename, err := h.svc.Export(r.Context(), orgID, userID, agentID)
	if err != nil {
		h.packError(w, err)
		return
	}
	body, err := json.MarshalIndent(pkg, "", "  ")
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to encode package")
		return
	}
	safeName := agentpack.HeaderSafe(filename)
	w.Header().Set("Content-Type", agentpack.ContentType)
	w.Header().Set("Content-Disposition", `attachment; filename="`+safeName+`"`)
	w.Header().Set("X-Agent-Pack-Warnings", agentpack.HeaderSafe(strings.Join(pkg.Warnings, "; ")))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

type previewRequest struct {
	Package *agentpack.Package `json:"package"`
}

func (h *AgentPackHandler) Preview(w http.ResponseWriter, r *http.Request) {
	orgID, err := uuid.Parse(middleware.GetOrgID(r.Context()))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid org_id in token")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, agentpack.MaxJSONBytes+4096)
	var req previewRequest
	if !DecodeJSON(w, r, &req) {
		return
	}
	if req.Package == nil {
		RespondError(w, http.StatusBadRequest, "package is required")
		return
	}
	out, err := h.svc.Preview(r.Context(), orgID, req.Package)
	if err != nil {
		h.packError(w, err)
		return
	}
	RespondJSON(w, http.StatusOK, out)
}

func (h *AgentPackHandler) Import(w http.ResponseWriter, r *http.Request) {
	orgID, err := uuid.Parse(middleware.GetOrgID(r.Context()))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid org_id in token")
		return
	}
	userID, err := uuid.Parse(middleware.GetUserID(r.Context()))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid user_id in token")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, agentpack.MaxJSONBytes+4096)
	var req service.ImportAgentRequest
	if !DecodeJSON(w, r, &req) {
		return
	}
	if req.Package == nil && req.PreviewID == "" {
		RespondError(w, http.StatusBadRequest, "preview_id or package is required")
		return
	}
	rep, err := h.svc.ResolvePreview(r.Context(), orgID, req)
	if err != nil {
		h.packError(w, err)
		return
	}
	if err := h.requireImportPerm(r, rep.Mode); err != nil {
		RespondError(w, http.StatusForbidden, err.Error())
		return
	}
	if rep.HasError() {
		RespondError(w, http.StatusBadRequest, firstIssue(rep))
		return
	}
	result, err := h.svc.Import(r.Context(), orgID, userID, req)
	if err != nil {
		h.packError(w, err)
		return
	}
	RespondJSON(w, http.StatusCreated, result)
}

func firstIssue(rep agentpack.Report) string {
	for _, i := range rep.Issues {
		if i.IsError() {
			return i.Message
		}
	}
	return "package cannot be imported"
}

func (h *AgentPackHandler) requireImportPerm(r *http.Request, mode agentpack.Mode) error {
	if h.rbac == nil {
		return nil
	}
	userID, err := uuid.Parse(middleware.GetUserID(r.Context()))
	if err != nil {
		return err
	}
	orgID, err := uuid.Parse(middleware.GetOrgID(r.Context()))
	if err != nil {
		return err
	}
	need := model.PermAgentsCreate
	if mode == agentpack.ModeOverlay {
		need = model.PermAgentsUpdate
	}
	ok, err := h.rbac.UserHasPermission(r.Context(), userID, orgID, need)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("insufficient permissions: " + need)
	}
	return nil
}

func (h *AgentPackHandler) packError(w http.ResponseWriter, err error) {
	if errors.Is(err, repository.ErrAgentPackNotFound) {
		RespondError(w, http.StatusNotFound, "agent not found")
		return
	}
	if errors.Is(err, repository.ErrAgentPackForbidden) {
		RespondError(w, http.StatusForbidden, "agent belongs to another organisation")
		return
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "request body too large"),
		strings.Contains(msg, "exceeds"),
		strings.Contains(msg, "too many"):
		RespondError(w, http.StatusRequestEntityTooLarge, msg)
	case strings.Contains(msg, "specialist") || strings.Contains(msg, "seeded"):
		RespondError(w, http.StatusUnprocessableEntity, msg)
	case strings.Contains(msg, "preview expired"),
		strings.Contains(msg, "package"),
		strings.Contains(msg, "schema_version"),
		strings.Contains(msg, "kind"),
		strings.Contains(msg, "name is required"),
		strings.Contains(msg, "already exists"),
		strings.Contains(msg, "cannot be imported"):
		RespondError(w, http.StatusBadRequest, msg)
	default:
		RespondError(w, http.StatusInternalServerError, "failed to process agent package")
	}
}
