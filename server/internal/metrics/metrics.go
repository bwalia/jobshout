// Package metrics provides Prometheus instrumentation for JobShout.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// ─── Task / Execution Counters ──────────────────────────────────────

	TaskTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "jobshout_task_total",
		Help: "Total number of agent executions",
	}, []string{"org_id", "agent_id", "status", "provider", "model"})

	TaskLatency = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "jobshout_task_latency_seconds",
		Help:    "Agent execution latency in seconds",
		Buckets: []float64{0.1, 0.5, 1, 2, 5, 10, 30, 60, 120},
	}, []string{"org_id", "agent_id", "provider", "model"})

	// ─── Token Usage ────────────────────────────────────────────────────

	TokenUsageTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "jobshout_token_usage_total",
		Help: "Total tokens consumed",
	}, []string{"org_id", "provider", "model", "direction"}) // direction: "input" or "output"

	// ─── Cost ───────────────────────────────────────────────────────────

	CostTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "jobshout_cost_usd_total",
		Help: "Total cost in USD",
	}, []string{"org_id", "provider", "model"})

	// ─── Budget ─────────────────────────────────────────────────────────

	BudgetUsageRatio = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "jobshout_budget_usage_ratio",
		Help: "Current budget utilization ratio (0.0 - 1.0+)",
	}, []string{"org_id", "period"})

	BudgetAlertTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "jobshout_budget_alert_total",
		Help: "Total budget alerts triggered",
	}, []string{"org_id", "alert_type"})

	// ─── Agent Performance ──────────────────────────────────────────────

	AgentSuccessRate = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "jobshout_agent_success_rate",
		Help: "Agent success rate (0.0 - 1.0)",
	}, []string{"org_id", "agent_id"})

	AgentScore = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "jobshout_agent_score",
		Help: "Agent composite score for intelligent routing",
	}, []string{"org_id", "agent_id", "task_type"})

	// ─── Tool Usage ─────────────────────────────────────────────────────

	ToolCallTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "jobshout_tool_call_total",
		Help: "Total tool invocations",
	}, []string{"org_id", "agent_id", "tool_name", "status"}) // status: "success" or "error"

	ToolCallLatency = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "jobshout_tool_call_latency_seconds",
		Help:    "Tool call latency in seconds",
		Buckets: []float64{0.01, 0.05, 0.1, 0.5, 1, 5, 10, 30, 60},
	}, []string{"tool_name"})

	// ─── HTTP ───────────────────────────────────────────────────────────

	HTTPRequestTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "jobshout_http_requests_total",
		Help: "Total HTTP requests",
	}, []string{"method", "path", "status_code"})

	HTTPRequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "jobshout_http_request_duration_seconds",
		Help:    "HTTP request duration in seconds",
		Buckets: prometheus.DefBuckets,
	}, []string{"method", "path"})

	// ─── Auth / SSO ─────────────────────────────────────────────────────

	LoginTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "jobshout_login_total",
		Help: "Total login attempts",
	}, []string{"provider", "status"})

	// ─── Policy ─────────────────────────────────────────────────────────

	PolicyBlockTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "jobshout_policy_block_total",
		Help: "Total executions blocked by policy",
	}, []string{"org_id", "agent_id", "reason"})

	// ─── Active executions ──────────────────────────────────────────────

	ActiveExecutions = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "jobshout_active_executions",
		Help: "Number of currently running executions",
	}, []string{"org_id"})
)

// RecordExecution is a convenience method to record all execution-related metrics.
func RecordExecution(orgID, agentID, provider, modelName, status string, latencyMs int, inputTokens, outputTokens int, costUSD float64) {
	TaskTotal.WithLabelValues(orgID, agentID, status, provider, modelName).Inc()
	TaskLatency.WithLabelValues(orgID, agentID, provider, modelName).Observe(float64(latencyMs) / 1000)
	TokenUsageTotal.WithLabelValues(orgID, provider, modelName, "input").Add(float64(inputTokens))
	TokenUsageTotal.WithLabelValues(orgID, provider, modelName, "output").Add(float64(outputTokens))
	CostTotal.WithLabelValues(orgID, provider, modelName).Add(costUSD)
}
