// Package selector implements cost-aware intelligent agent selection.
// It picks the best agent for a task based on historical performance, latency, and cost.
package selector

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/metrics"
)

// Weights for the scoring formula.
const (
	WeightSuccessRate = 0.5
	WeightLatency     = 0.2
	WeightCost        = 0.3
)

// Candidate represents a scored agent candidate.
type Candidate struct {
	AgentID      uuid.UUID `json:"agent_id"`
	AgentName    string    `json:"agent_name"`
	SuccessRate  float64   `json:"success_rate"`
	AvgLatencyMs int       `json:"avg_latency_ms"`
	AvgCostUSD   float64   `json:"avg_cost_usd"`
	TotalRuns    int       `json:"total_runs"`
	Score        float64   `json:"score"`
}

// SelectionResult is the outcome of agent selection.
type SelectionResult struct {
	Selected   *Candidate  `json:"selected"`
	Fallback   *Candidate  `json:"fallback,omitempty"`
	Candidates []Candidate `json:"candidates"`
	Reason     string      `json:"reason"`
}

// Selector picks the best agent for a task based on historical performance.
type Selector struct {
	pool   *pgxpool.Pool
	logger *zap.Logger
}

// New creates a Selector.
func New(pool *pgxpool.Pool, logger *zap.Logger) *Selector {
	return &Selector{pool: pool, logger: logger}
}

// Select returns the best agent for the given task type from the candidate list.
// agentIDs is the set of eligible agents (e.g. agents assigned to a project).
// If agentIDs is empty, all agents in the org are considered.
func (s *Selector) Select(ctx context.Context, orgID uuid.UUID, taskType string, agentIDs []uuid.UUID) (*SelectionResult, error) {
	candidates, err := s.loadCandidates(ctx, orgID, taskType, agentIDs)
	if err != nil {
		return nil, fmt.Errorf("selector: load candidates: %w", err)
	}

	if len(candidates) == 0 {
		return &SelectionResult{Reason: "no candidates found"}, nil
	}

	// Score each candidate.
	maxLatency := 1.0
	maxCost := 1.0
	for _, c := range candidates {
		if float64(c.AvgLatencyMs) > maxLatency {
			maxLatency = float64(c.AvgLatencyMs)
		}
		if c.AvgCostUSD > maxCost {
			maxCost = c.AvgCostUSD
		}
	}

	for i := range candidates {
		c := &candidates[i]
		normLatency := float64(c.AvgLatencyMs) / maxLatency
		normCost := c.AvgCostUSD / maxCost
		c.Score = (c.SuccessRate * WeightSuccessRate) -
			(normLatency * WeightLatency) -
			(normCost * WeightCost)
	}

	// Sort by score descending.
	for i := 1; i < len(candidates); i++ {
		for j := i; j > 0 && candidates[j].Score > candidates[j-1].Score; j-- {
			candidates[j], candidates[j-1] = candidates[j-1], candidates[j]
		}
	}

	result := &SelectionResult{
		Selected:   &candidates[0],
		Candidates: candidates,
		Reason:     fmt.Sprintf("selected %s with score %.4f", candidates[0].AgentName, candidates[0].Score),
	}

	// Set fallback (second-best, if different enough).
	if len(candidates) > 1 {
		result.Fallback = &candidates[1]
	}

	// Update Prometheus metrics.
	metrics.AgentScore.WithLabelValues(orgID.String(), result.Selected.AgentID.String(), taskType).
		Set(result.Selected.Score)

	return result, nil
}

// UpdateScores refreshes the agent_scores table from recent execution data.
func (s *Selector) UpdateScores(ctx context.Context, orgID uuid.UUID) error {
	const sql = `
		INSERT INTO agent_scores (org_id, agent_id, task_type, success_rate, avg_latency_ms,
		    avg_cost_usd, total_runs, score, last_updated)
		SELECT
		    org_id, agent_id, 'general',
		    CASE WHEN COUNT(*) > 0 THEN
		        COUNT(*) FILTER (WHERE status = 'completed')::numeric / COUNT(*)
		    ELSE 0 END AS success_rate,
		    COALESCE(AVG(latency_ms), 0)::int AS avg_latency_ms,
		    COALESCE(AVG(cost_usd), 0) AS avg_cost_usd,
		    COUNT(*) AS total_runs,
		    0 AS score,
		    NOW()
		FROM agent_executions
		WHERE org_id = $1 AND created_at >= $2
		GROUP BY org_id, agent_id
		ON CONFLICT (org_id, agent_id, task_type) DO UPDATE SET
		    success_rate   = EXCLUDED.success_rate,
		    avg_latency_ms = EXCLUDED.avg_latency_ms,
		    avg_cost_usd   = EXCLUDED.avg_cost_usd,
		    total_runs     = EXCLUDED.total_runs,
		    last_updated   = NOW()`

	lookback := time.Now().AddDate(0, 0, -30) // last 30 days
	_, err := s.pool.Exec(ctx, sql, orgID, lookback)
	if err != nil {
		return fmt.Errorf("selector: update scores: %w", err)
	}
	return nil
}

func (s *Selector) loadCandidates(ctx context.Context, orgID uuid.UUID, taskType string, agentIDs []uuid.UUID) ([]Candidate, error) {
	// Try pre-computed scores first.
	candidates, err := s.fromScoresTable(ctx, orgID, taskType, agentIDs)
	if err == nil && len(candidates) > 0 {
		return candidates, nil
	}

	// Fallback to live computation from agent_executions.
	return s.fromLiveData(ctx, orgID, agentIDs)
}

func (s *Selector) fromScoresTable(ctx context.Context, orgID uuid.UUID, taskType string, agentIDs []uuid.UUID) ([]Candidate, error) {
	sql := `
		SELECT s.agent_id, COALESCE(a.name, ''), s.success_rate, s.avg_latency_ms,
		       s.avg_cost_usd, s.total_runs
		FROM agent_scores s
		LEFT JOIN agents a ON a.id = s.agent_id
		WHERE s.org_id = $1 AND s.task_type = $2`

	args := []any{orgID, taskType}
	if len(agentIDs) > 0 {
		sql += ` AND s.agent_id = ANY($3)`
		args = append(args, agentIDs)
	}
	sql += ` ORDER BY s.total_runs DESC LIMIT 50`

	rows, err := s.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var candidates []Candidate
	for rows.Next() {
		var c Candidate
		if err := rows.Scan(&c.AgentID, &c.AgentName, &c.SuccessRate,
			&c.AvgLatencyMs, &c.AvgCostUSD, &c.TotalRuns); err != nil {
			return nil, err
		}
		candidates = append(candidates, c)
	}
	return candidates, rows.Err()
}

func (s *Selector) fromLiveData(ctx context.Context, orgID uuid.UUID, agentIDs []uuid.UUID) ([]Candidate, error) {
	sql := `
		SELECT e.agent_id, COALESCE(a.name, ''),
		       CASE WHEN COUNT(*) > 0 THEN
		           COUNT(*) FILTER (WHERE e.status = 'completed')::numeric / COUNT(*)
		       ELSE 0 END,
		       COALESCE(AVG(e.latency_ms), 0)::int,
		       COALESCE(AVG(e.cost_usd), 0),
		       COUNT(*)
		FROM agent_executions e
		LEFT JOIN agents a ON a.id = e.agent_id
		WHERE e.org_id = $1 AND e.created_at >= $2`

	args := []any{orgID, time.Now().AddDate(0, 0, -30)}
	if len(agentIDs) > 0 {
		sql += ` AND e.agent_id = ANY($3)`
		args = append(args, agentIDs)
	}
	sql += ` GROUP BY e.agent_id, a.name ORDER BY COUNT(*) DESC LIMIT 50`

	rows, err := s.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var candidates []Candidate
	for rows.Next() {
		var c Candidate
		var sr float64
		if err := rows.Scan(&c.AgentID, &c.AgentName, &sr,
			&c.AvgLatencyMs, &c.AvgCostUSD, &c.TotalRuns); err != nil {
			return nil, err
		}
		c.SuccessRate = math.Round(sr*10000) / 10000
		candidates = append(candidates, c)
	}
	return candidates, rows.Err()
}

// SelectionRequest is the API payload for agent selection.
type SelectionRequest struct {
	TaskType string      `json:"task_type" validate:"required"`
	AgentIDs []uuid.UUID `json:"agent_ids,omitempty"`
}
