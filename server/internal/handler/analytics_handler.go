package handler

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/jobshout/server/internal/middleware"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/service"
)

// AnalyticsHandler exposes usage analytics and cost reporting endpoints.
type AnalyticsHandler struct {
	svc service.AnalyticsService
}

// NewAnalyticsHandler creates an AnalyticsHandler.
func NewAnalyticsHandler(svc service.AnalyticsService) *AnalyticsHandler {
	return &AnalyticsHandler{svc: svc}
}

// UsageTimeSeries handles GET /analytics/usage
// Query params: from, to (RFC3339), granularity (hourly|daily), agent_id, provider, model
func (h *AnalyticsHandler) UsageTimeSeries(w http.ResponseWriter, r *http.Request) {
	params, err := parseUsageQuery(r)
	if err != nil {
		RespondError(w, http.StatusBadRequest, err.Error())
		return
	}

	rollups, err := h.svc.UsageTimeSeries(r.Context(), params)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to query usage")
		return
	}
	if rollups == nil {
		rollups = []model.UsageRollup{}
	}
	RespondJSON(w, http.StatusOK, rollups)
}

// OrgUsageSummary handles GET /analytics/usage/summary
// Query params: from, to (RFC3339)
func (h *AnalyticsHandler) OrgUsageSummary(w http.ResponseWriter, r *http.Request) {
	orgID, err := uuid.Parse(middleware.GetOrgID(r.Context()))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid org_id in token")
		return
	}

	from, to := parseTimeRange(r)

	summary, err := h.svc.OrgUsageSummary(r.Context(), orgID, from, to)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to get usage summary")
		return
	}
	RespondJSON(w, http.StatusOK, summary)
}

// AgentAnalytics handles GET /analytics/agents/{agentID}
// Query params: from, to (RFC3339)
func (h *AnalyticsHandler) AgentAnalytics(w http.ResponseWriter, r *http.Request) {
	agentID, err := uuid.Parse(chi.URLParam(r, "agentID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid agent ID")
		return
	}

	from, to := parseTimeRange(r)

	analytics, err := h.svc.AgentAnalytics(r.Context(), agentID, from, to)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to get agent analytics")
		return
	}
	RespondJSON(w, http.StatusOK, analytics)
}

// TopAgents handles GET /analytics/top-agents
// Query params: from, to (RFC3339), limit (int)
func (h *AnalyticsHandler) TopAgents(w http.ResponseWriter, r *http.Request) {
	orgID, err := uuid.Parse(middleware.GetOrgID(r.Context()))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid org_id in token")
		return
	}

	from, to := parseTimeRange(r)
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = 10
	}

	agents, err := h.svc.TopAgentsBySpend(r.Context(), orgID, limit, from, to)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to get top agents")
		return
	}
	if agents == nil {
		agents = []model.AgentAnalytics{}
	}
	RespondJSON(w, http.StatusOK, agents)
}

// ─── Helpers ────────────────────────────────────────────────────────────────

func parseTimeRange(r *http.Request) (time.Time, time.Time) {
	from, _ := time.Parse(time.RFC3339, r.URL.Query().Get("from"))
	to, _ := time.Parse(time.RFC3339, r.URL.Query().Get("to"))
	if from.IsZero() {
		from = time.Now().AddDate(0, -1, 0) // default: last 30 days
	}
	if to.IsZero() {
		to = time.Now()
	}
	return from, to
}

func parseUsageQuery(r *http.Request) (model.UsageQueryParams, error) {
	q := r.URL.Query()
	from, to := parseTimeRange(r)

	params := model.UsageQueryParams{
		Provider:    q.Get("provider"),
		Model:       q.Get("model"),
		From:        from,
		To:          to,
		Granularity: q.Get("granularity"),
	}
	if params.Granularity == "" {
		params.Granularity = "daily"
	}

	if agentStr := q.Get("agent_id"); agentStr != "" {
		agentID, err := uuid.Parse(agentStr)
		if err != nil {
			return params, fmt.Errorf("invalid agent_id: %w", err)
		}
		params.AgentID = &agentID
	}

	return params, nil
}
