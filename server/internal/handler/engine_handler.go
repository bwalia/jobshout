package handler

import (
	"context"
	"net/http"
	"time"

	"go.uber.org/zap"

	"github.com/jobshout/server/internal/langchain"
	"github.com/jobshout/server/internal/langgraph"
	"github.com/jobshout/server/internal/model"
)

// EngineHandler exposes engine health and listing endpoints.
type EngineHandler struct {
	langchainClient *langchain.Client
	langgraphClient *langgraph.Client
	logger          *zap.Logger
}

// NewEngineHandler creates an EngineHandler. Either client may be nil if the
// Python sidecar is not configured.
func NewEngineHandler(lc *langchain.Client, lg *langgraph.Client, logger *zap.Logger) *EngineHandler {
	return &EngineHandler{
		langchainClient: lc,
		langgraphClient: lg,
		logger:          logger,
	}
}

type engineInfo struct {
	Name      string `json:"name"`
	Status    string `json:"status"`
	Available bool   `json:"available"`
}

// List handles GET /engines — returns all registered engine types with status.
func (h *EngineHandler) List(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	engines := []engineInfo{
		{Name: model.EngineGoNative, Status: "healthy", Available: true},
	}

	if h.langchainClient != nil {
		healthy := h.langchainClient.Healthy(ctx)
		status := "healthy"
		if !healthy {
			status = "unreachable"
		}
		engines = append(engines, engineInfo{
			Name:      model.EngineLangChain,
			Status:    status,
			Available: healthy,
		})
	} else {
		engines = append(engines, engineInfo{
			Name:      model.EngineLangChain,
			Status:    "not_configured",
			Available: false,
		})
	}

	if h.langgraphClient != nil {
		healthy := h.langgraphClient.Healthy(ctx)
		status := "healthy"
		if !healthy {
			status = "unreachable"
		}
		engines = append(engines, engineInfo{
			Name:      model.EngineLangGraph,
			Status:    status,
			Available: healthy,
		})
	} else {
		engines = append(engines, engineInfo{
			Name:      model.EngineLangGraph,
			Status:    "not_configured",
			Available: false,
		})
	}

	RespondJSON(w, http.StatusOK, engines)
}

// Health handles GET /engines/health — combined health of all engines.
func (h *EngineHandler) Health(w http.ResponseWriter, r *http.Request) {
	h.List(w, r)
}
