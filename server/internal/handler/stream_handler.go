package handler

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/bridge"
	"github.com/jobshout/server/internal/middleware"
)

// StreamHandler exposes SSE streaming endpoints for LangChain/LangGraph execution.
type StreamHandler struct {
	bridgeClient *bridge.Client
	validate     *validator.Validate
	logger       *zap.Logger
}

// NewStreamHandler creates a StreamHandler.
func NewStreamHandler(bridgeClient *bridge.Client, logger *zap.Logger) *StreamHandler {
	return &StreamHandler{
		bridgeClient: bridgeClient,
		validate:     validator.New(),
		logger:       logger,
	}
}

type streamExecuteRequest struct {
	Prompt       string         `json:"prompt" validate:"required,min=1"`
	AgentID      string         `json:"agent_id" validate:"required,uuid"`
	Engine       string         `json:"engine" validate:"required,oneof=langchain langgraph"`
	SystemPrompt string         `json:"system_prompt,omitempty"`
	Model        string         `json:"model,omitempty"`
	Provider     string         `json:"provider,omitempty"`
	Tools        []string       `json:"tools"`
	Config       map[string]any `json:"config"`
}

// StreamExecute handles POST /stream/execute — SSE streaming execution.
func (h *StreamHandler) StreamExecute(w http.ResponseWriter, r *http.Request) {
	if h.bridgeClient == nil {
		RespondError(w, http.StatusServiceUnavailable, "streaming not available: Python sidecar not configured")
		return
	}

	var req streamExecuteRequest
	if !DecodeJSON(w, r, &req) {
		return
	}
	if err := h.validate.Struct(req); err != nil {
		RespondError(w, http.StatusBadRequest, "validation failed: "+err.Error())
		return
	}

	orgID := middleware.GetOrgID(r.Context())
	execID := uuid.New()

	h.logger.Info("starting streaming execution",
		zap.String("execution_id", execID.String()),
		zap.String("engine", req.Engine),
		zap.String("org_id", orgID),
	)

	streamReq := bridge.StreamRequest{
		ExecutionID:  execID.String(),
		AgentID:      req.AgentID,
		Prompt:       req.Prompt,
		SystemPrompt: req.SystemPrompt,
		Model:        req.Model,
		Provider:     req.Provider,
		Tools:        req.Tools,
		Config:       req.Config,
		Engine:       req.Engine,
	}

	events, err := h.bridgeClient.StreamExecution(r.Context(), streamReq)
	if err != nil {
		RespondError(w, http.StatusBadGateway, "streaming failed: "+err.Error())
		return
	}

	// Set SSE headers.
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Execution-ID", execID.String())
	w.WriteHeader(http.StatusOK)

	flusher, ok := w.(http.Flusher)
	if !ok {
		RespondError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	for event := range events {
		data, err := json.Marshal(event)
		if err != nil {
			continue
		}
		fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()
	}

	fmt.Fprintf(w, "data: [DONE]\n\n")
	flusher.Flush()
}

// StreamWorkflowStep handles GET /workflows/{workflowID}/stream/{stepName} — SSE for a specific step.
func (h *StreamHandler) StreamWorkflowStep(w http.ResponseWriter, r *http.Request) {
	if h.bridgeClient == nil {
		RespondError(w, http.StatusServiceUnavailable, "streaming not available")
		return
	}

	_ = chi.URLParam(r, "workflowID")
	stepName := chi.URLParam(r, "stepName")

	// For now, return a simple SSE stream with the step name.
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	flusher, ok := w.(http.Flusher)
	if !ok {
		return
	}

	event := bridge.StreamEvent{
		Type: "node_start",
		Data: bridge.NodeEvent{NodeName: stepName, StepNumber: 1},
	}
	data, _ := json.Marshal(event)
	fmt.Fprintf(w, "data: %s\n\n", data)
	flusher.Flush()

	fmt.Fprintf(w, "data: [DONE]\n\n")
	flusher.Flush()
}
