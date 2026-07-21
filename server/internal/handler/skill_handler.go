package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"

	"github.com/jobshout/server/internal/middleware"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/repository"
)

// SkillHandler exposes the OpenClaw-style skills registry.
//
// The handler talks directly to the repository because the surface today is
// pure CRUD + enable/disable. Once an execution path is added, that logic
// will move into a SkillService and this handler will keep the same routes.
type SkillHandler struct {
	repo     repository.SkillRepository
	validate *validator.Validate
}

func NewSkillHandler(repo repository.SkillRepository) *SkillHandler {
	return &SkillHandler{repo: repo, validate: validator.New()}
}

func (h *SkillHandler) List(w http.ResponseWriter, r *http.Request) {
	orgID, err := uuid.Parse(middleware.GetOrgID(r.Context()))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid org_id")
		return
	}
	out, err := h.repo.List(r.Context(), orgID)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	RespondJSON(w, http.StatusOK, out)
}

func (h *SkillHandler) Create(w http.ResponseWriter, r *http.Request) {
	orgID, err := uuid.Parse(middleware.GetOrgID(r.Context()))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid org_id")
		return
	}
	var userID *uuid.UUID
	if u, err := uuid.Parse(middleware.GetUserID(r.Context())); err == nil {
		userID = &u
	}

	var req model.CreateSkillRequest
	if !DecodeJSON(w, r, &req) {
		return
	}
	if err := h.validate.Struct(req); err != nil {
		RespondError(w, http.StatusBadRequest, "validation: "+err.Error())
		return
	}

	s := &model.Skill{
		ID:          uuid.New(),
		OrgID:       &orgID,
		Slug:        req.Slug,
		Name:        req.Name,
		Description: req.Description,
		Kind:        req.Kind,
		ConfigJSON:  req.ConfigJSON,
		Version:     req.Version,
		CreatedBy:   userID,
	}
	if err := h.repo.Create(r.Context(), s); err != nil {
		RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	RespondJSON(w, http.StatusCreated, s)
}

func (h *SkillHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "skillID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid skill id")
		return
	}
	s, err := h.repo.Get(r.Context(), id)
	if err != nil {
		RespondError(w, http.StatusNotFound, "skill not found")
		return
	}
	RespondJSON(w, http.StatusOK, s)
}

func (h *SkillHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "skillID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid skill id")
		return
	}
	var req model.UpdateSkillRequest
	if !DecodeJSON(w, r, &req) {
		return
	}
	if err := h.validate.Struct(req); err != nil {
		RespondError(w, http.StatusBadRequest, "validation: "+err.Error())
		return
	}

	s, err := h.repo.Get(r.Context(), id)
	if err != nil {
		RespondError(w, http.StatusNotFound, "skill not found")
		return
	}

	// Apply only the fields the caller provided (partial update).
	if req.Name != nil {
		s.Name = *req.Name
	}
	if req.Description != nil {
		s.Description = req.Description
	}
	if req.Kind != nil {
		s.Kind = *req.Kind
	}
	if req.ConfigJSON != nil {
		s.ConfigJSON = req.ConfigJSON
	}
	if req.Version != nil {
		s.Version = *req.Version
	}
	if req.Status != nil {
		s.Status = *req.Status
	}

	if err := h.repo.Update(r.Context(), s); err != nil {
		RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	RespondJSON(w, http.StatusOK, s)
}

func (h *SkillHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "skillID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid skill id")
		return
	}
	if err := h.repo.Delete(r.Context(), id); err != nil {
		RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *SkillHandler) ListForAgent(w http.ResponseWriter, r *http.Request) {
	agentID, err := uuid.Parse(chi.URLParam(r, "agentID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid agent id")
		return
	}
	out, err := h.repo.ListForAgent(r.Context(), agentID)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	RespondJSON(w, http.StatusOK, out)
}

func (h *SkillHandler) EnableForAgent(w http.ResponseWriter, r *http.Request) {
	agentID, err := uuid.Parse(chi.URLParam(r, "agentID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid agent id")
		return
	}
	var req model.EnableSkillRequest
	if !DecodeJSON(w, r, &req) {
		return
	}
	if err := h.validate.Struct(req); err != nil {
		RespondError(w, http.StatusBadRequest, "validation: "+err.Error())
		return
	}
	if err := h.repo.EnableForAgent(r.Context(), agentID, req.SkillID, req.ConfigOverride); err != nil {
		RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *SkillHandler) DisableForAgent(w http.ResponseWriter, r *http.Request) {
	agentID, err := uuid.Parse(chi.URLParam(r, "agentID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid agent id")
		return
	}
	skillID, err := uuid.Parse(chi.URLParam(r, "skillID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid skill id")
		return
	}
	if err := h.repo.DisableForAgent(r.Context(), agentID, skillID); err != nil {
		RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
