package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"

	"github.com/jobshout/server/internal/middleware"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/service"
)

// ExecutionHandler exposes agent execution endpoints.
type ExecutionHandler struct {
	svc      service.ExecutionService
	validate *validator.Validate
}

// NewExecutionHandler creates an ExecutionHandler.
func NewExecutionHandler(svc service.ExecutionService) *ExecutionHandler {
	return &ExecutionHandler{svc: svc, validate: validator.New()}
}

// Execute handles POST /agents/{agentID}/execute
// It runs the agent synchronously and returns the completed execution record.
func (h *ExecutionHandler) Execute(w http.ResponseWriter, r *http.Request) {
	agentID, err := uuid.Parse(chi.URLParam(r, "agentID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid agent ID")
		return
	}

	var req model.ExecuteAgentRequest
	if !DecodeJSON(w, r, &req) {
		return
	}
	if err := h.validate.Struct(req); err != nil {
		RespondError(w, http.StatusBadRequest, "validation failed: "+err.Error())
		return
	}

	orgID, err := uuid.Parse(middleware.GetOrgID(r.Context()))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid org_id in token")
		return
	}

	execution, err := h.svc.Execute(r.Context(), orgID, agentID, req)
	if err != nil {
		if errors.Is(err, service.ErrAgentNotFound) {
			RespondError(w, http.StatusNotFound, "agent not found")
			return
		}
		RespondError(w, http.StatusInternalServerError, "execution failed: "+err.Error())
		return
	}

	RespondJSON(w, http.StatusOK, execution)
}

// GetExecution handles GET /executions/{executionID}
func (h *ExecutionHandler) GetExecution(w http.ResponseWriter, r *http.Request) {
	execID, err := uuid.Parse(chi.URLParam(r, "executionID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid execution ID")
		return
	}

	exec, err := h.svc.GetByID(r.Context(), execID)
	if err != nil {
		RespondError(w, http.StatusNotFound, "execution not found")
		return
	}
	RespondJSON(w, http.StatusOK, exec)
}

// ListByAgent handles GET /agents/{agentID}/executions
func (h *ExecutionHandler) ListByAgent(w http.ResponseWriter, r *http.Request) {
	agentID, err := uuid.Parse(chi.URLParam(r, "agentID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid agent ID")
		return
	}

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	perPage, _ := strconv.Atoi(r.URL.Query().Get("per_page"))
	params := model.PaginationParams{Page: page, PerPage: perPage}

	result, err := h.svc.ListByAgent(r.Context(), agentID, params)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to list executions")
		return
	}
	RespondJSON(w, http.StatusOK, result)
}

// ListLangChainTraces handles GET /executions/{executionID}/langchain-traces
func (h *ExecutionHandler) ListLangChainTraces(w http.ResponseWriter, r *http.Request) {
	execID, err := uuid.Parse(chi.URLParam(r, "executionID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid execution ID")
		return
	}

	traces, err := h.svc.ListLangChainTraces(r.Context(), execID)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to list langchain traces")
		return
	}
	if traces == nil {
		traces = []model.LangChainRunTrace{}
	}
	RespondJSON(w, http.StatusOK, traces)
}

// ListLangGraphSnapshots handles GET /executions/{executionID}/langgraph-snapshots
func (h *ExecutionHandler) ListLangGraphSnapshots(w http.ResponseWriter, r *http.Request) {
	execID, err := uuid.Parse(chi.URLParam(r, "executionID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid execution ID")
		return
	}

	snaps, err := h.svc.ListLangGraphSnapshots(r.Context(), execID)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to list langgraph snapshots")
		return
	}
	if snaps == nil {
		snaps = []model.LangGraphStateSnapshot{}
	}
	RespondJSON(w, http.StatusOK, snaps)
}
