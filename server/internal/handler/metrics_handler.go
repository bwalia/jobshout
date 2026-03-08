package handler

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/middleware"
)

// MetricsHandler handles metrics/analytics endpoints.
type MetricsHandler struct {
	pool   *pgxpool.Pool
	logger *zap.Logger
}

// NewMetricsHandler constructs a MetricsHandler.
func NewMetricsHandler(pool *pgxpool.Pool, logger *zap.Logger) *MetricsHandler {
	return &MetricsHandler{pool: pool, logger: logger}
}

type dashboardSummary struct {
	TotalAgents    int `json:"total_agents"`
	ActiveAgents   int `json:"active_agents"`
	TotalProjects  int `json:"total_projects"`
	TotalTasks     int `json:"total_tasks"`
	TasksCompleted int `json:"tasks_completed"`
	TasksInProgress int `json:"tasks_in_progress"`
}

// Summary returns high-level dashboard metrics for the user's org.
func (h *MetricsHandler) Summary(w http.ResponseWriter, r *http.Request) {
	orgID, _ := r.Context().Value(middleware.ContextKeyOrgID).(string)
	if orgID == "" {
		RespondError(w, http.StatusUnauthorized, "missing organization context")
		return
	}

	var summary dashboardSummary

	err := h.pool.QueryRow(r.Context(),
		`SELECT COUNT(*) FROM agents WHERE org_id = $1`, orgID,
	).Scan(&summary.TotalAgents)
	if err != nil {
		h.logger.Error("failed to count agents", zap.Error(err))
	}

	err = h.pool.QueryRow(r.Context(),
		`SELECT COUNT(*) FROM agents WHERE org_id = $1 AND status = 'active'`, orgID,
	).Scan(&summary.ActiveAgents)
	if err != nil {
		h.logger.Error("failed to count active agents", zap.Error(err))
	}

	err = h.pool.QueryRow(r.Context(),
		`SELECT COUNT(*) FROM projects WHERE org_id = $1`, orgID,
	).Scan(&summary.TotalProjects)
	if err != nil {
		h.logger.Error("failed to count projects", zap.Error(err))
	}

	err = h.pool.QueryRow(r.Context(),
		`SELECT COUNT(*) FROM tasks WHERE project_id IN (SELECT id FROM projects WHERE org_id = $1)`, orgID,
	).Scan(&summary.TotalTasks)
	if err != nil {
		h.logger.Error("failed to count tasks", zap.Error(err))
	}

	err = h.pool.QueryRow(r.Context(),
		`SELECT COUNT(*) FROM tasks WHERE status = 'done' AND project_id IN (SELECT id FROM projects WHERE org_id = $1)`, orgID,
	).Scan(&summary.TasksCompleted)
	if err != nil {
		h.logger.Error("failed to count completed tasks", zap.Error(err))
	}

	err = h.pool.QueryRow(r.Context(),
		`SELECT COUNT(*) FROM tasks WHERE status = 'in_progress' AND project_id IN (SELECT id FROM projects WHERE org_id = $1)`, orgID,
	).Scan(&summary.TasksInProgress)
	if err != nil {
		h.logger.Error("failed to count in-progress tasks", zap.Error(err))
	}

	RespondJSON(w, http.StatusOK, summary)
}

type agentMetricRow struct {
	ID             string    `json:"id"`
	AgentID        string    `json:"agent_id"`
	MetricType     string    `json:"metric_type"`
	Value          float64   `json:"value"`
	RecordedAt     time.Time `json:"recorded_at"`
}

// AgentMetrics returns metrics for a specific agent with optional date range.
func (h *MetricsHandler) AgentMetrics(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "agentID")
	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")

	query := `SELECT id, agent_id, metric_type, value, recorded_at
		FROM agent_metrics WHERE agent_id = $1`
	args := []any{agentID}
	argIdx := 2

	if from != "" {
		query += " AND recorded_at >= $" + itoa(argIdx)
		args = append(args, from)
		argIdx++
	}
	if to != "" {
		query += " AND recorded_at <= $" + itoa(argIdx)
		args = append(args, to)
	}
	query += " ORDER BY recorded_at DESC LIMIT 500"

	rows, err := h.pool.Query(r.Context(), query, args...)
	if err != nil {
		h.logger.Error("failed to query agent metrics", zap.Error(err))
		RespondError(w, http.StatusInternalServerError, "failed to query metrics")
		return
	}
	defer rows.Close()

	metrics := []agentMetricRow{}
	for rows.Next() {
		var m agentMetricRow
		if err := rows.Scan(&m.ID, &m.AgentID, &m.MetricType, &m.Value, &m.RecordedAt); err != nil {
			h.logger.Error("failed to scan agent metric", zap.Error(err))
			continue
		}
		metrics = append(metrics, m)
	}

	RespondJSON(w, http.StatusOK, metrics)
}

type taskCompletionDataPoint struct {
	Date      string `json:"date"`
	Completed int    `json:"completed"`
}

// TaskCompletion returns daily task completion counts for the org.
func (h *MetricsHandler) TaskCompletion(w http.ResponseWriter, r *http.Request) {
	orgID, _ := r.Context().Value(middleware.ContextKeyOrgID).(string)
	if orgID == "" {
		RespondError(w, http.StatusUnauthorized, "missing organization context")
		return
	}

	days := r.URL.Query().Get("days")
	if days == "" {
		days = "30"
	}

	rows, err := h.pool.Query(r.Context(),
		`SELECT DATE(tsh.changed_at) as date, COUNT(*) as completed
			FROM task_status_history tsh
			JOIN tasks t ON t.id = tsh.task_id
			JOIN projects p ON p.id = t.project_id
			WHERE p.org_id = $1
				AND tsh.new_status = 'done'
				AND tsh.changed_at >= NOW() - ($2 || ' days')::interval
			GROUP BY DATE(tsh.changed_at)
			ORDER BY date`, orgID, days)
	if err != nil {
		h.logger.Error("failed to query task completion", zap.Error(err))
		RespondError(w, http.StatusInternalServerError, "failed to query task completion")
		return
	}
	defer rows.Close()

	data := []taskCompletionDataPoint{}
	for rows.Next() {
		var dp taskCompletionDataPoint
		var date time.Time
		if err := rows.Scan(&date, &dp.Completed); err != nil {
			h.logger.Error("failed to scan task completion", zap.Error(err))
			continue
		}
		dp.Date = date.Format("2006-01-02")
		data = append(data, dp)
	}

	RespondJSON(w, http.StatusOK, data)
}

// itoa converts an int to its string representation without importing strconv.
func itoa(i int) string {
	if i < 10 {
		return string(rune('0' + i))
	}
	return itoa(i/10) + string(rune('0'+i%10))
}
