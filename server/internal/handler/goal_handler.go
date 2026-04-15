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

// GoalHandler handles autonomous agent goal endpoints.
type GoalHandler struct {
	svc      service.GoalService
	validate *validator.Validate
}

// NewGoalHandler creates a new GoalHandler.
func NewGoalHandler(svc service.GoalService) *GoalHandler {
	return &GoalHandler{svc: svc, validate: validator.New()}
}

// CreateGoal starts a new autonomous goal for an agent.
func (h *GoalHandler) CreateGoal(w http.ResponseWriter, r *http.Request) {
	orgID, _ := uuid.Parse(middleware.GetOrgID(r.Context()))

	agentID, err := uuid.Parse(chi.URLParam(r, "agentID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid agent ID")
		return
	}

	var req model.CreateGoalRequest
	if !DecodeJSON(w, r, &req) {
		return
	}
	if err := h.validate.Struct(req); err != nil {
		RespondError(w, http.StatusBadRequest, err.Error())
		return
	}

	goal, err := h.svc.CreateGoal(r.Context(), orgID, agentID, req)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	RespondJSON(w, http.StatusAccepted, goal)
}

// GetGoal returns a single goal by ID.
func (h *GoalHandler) GetGoal(w http.ResponseWriter, r *http.Request) {
	goalID, err := uuid.Parse(chi.URLParam(r, "goalID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid goal ID")
		return
	}

	goal, err := h.svc.GetGoal(r.Context(), goalID)
	if err != nil {
		RespondError(w, http.StatusNotFound, "goal not found")
		return
	}
	RespondJSON(w, http.StatusOK, goal)
}

// ListGoals lists goals for an agent.
func (h *GoalHandler) ListGoals(w http.ResponseWriter, r *http.Request) {
	agentID, err := uuid.Parse(chi.URLParam(r, "agentID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid agent ID")
		return
	}

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	perPage, _ := strconv.Atoi(r.URL.Query().Get("per_page"))
	params := model.PaginationParams{Page: page, PerPage: perPage}

	result, err := h.svc.ListGoals(r.Context(), agentID, params)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	RespondJSON(w, http.StatusOK, result)
}
