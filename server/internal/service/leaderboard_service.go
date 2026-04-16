package service

import (
	"context"
	"math"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/repository"
)

// LeaderboardEntry is a ranked agent with performance metrics.
type LeaderboardEntry struct {
	Rank         int     `json:"rank"`
	AgentID      uuid.UUID `json:"agent_id"`
	AgentName    string  `json:"agent_name"`
	SuccessRate  float64 `json:"success_rate"`
	AvgLatencyMs int     `json:"avg_latency_ms"`
	TotalCostUSD float64 `json:"total_cost_usd"`
	TotalRuns    int     `json:"total_runs"`
	Score        float64 `json:"score"`
}

// AnomalyAlert flags an agent with abnormal behavior.
type AnomalyAlert struct {
	AgentID    uuid.UUID `json:"agent_id"`
	AgentName  string    `json:"agent_name"`
	Metric     string    `json:"metric"`
	Value      float64   `json:"value"`
	Mean       float64   `json:"mean"`
	StdDev     float64   `json:"std_dev"`
	ZScore     float64   `json:"z_score"`
	Severity   string    `json:"severity"`
}

// LeaderboardService provides agent rankings, analytics, and anomaly detection.
type LeaderboardService interface {
	Leaderboard(ctx context.Context, orgID uuid.UUID, limit int, from, to time.Time) ([]LeaderboardEntry, error)
	DetectAnomalies(ctx context.Context, orgID uuid.UUID, from, to time.Time) ([]AnomalyAlert, error)
}

type leaderboardService struct {
	usageRepo repository.UsageRepository
	logger    *zap.Logger
}

// NewLeaderboardService creates a LeaderboardService.
func NewLeaderboardService(usageRepo repository.UsageRepository, logger *zap.Logger) LeaderboardService {
	return &leaderboardService{usageRepo: usageRepo, logger: logger}
}

func (s *leaderboardService) Leaderboard(ctx context.Context, orgID uuid.UUID, limit int, from, to time.Time) ([]LeaderboardEntry, error) {
	agents, err := s.usageRepo.TopAgentsBySpend(ctx, orgID, 100, from, to)
	if err != nil {
		return nil, err
	}

	// Compute composite score and rank.
	entries := make([]LeaderboardEntry, 0, len(agents))
	for _, a := range agents {
		score := computeScore(a)
		entries = append(entries, LeaderboardEntry{
			AgentID:      a.AgentID,
			AgentName:    a.AgentName,
			SuccessRate:  a.SuccessRate,
			AvgLatencyMs: a.AvgLatencyMs,
			TotalCostUSD: a.TotalCostUSD,
			TotalRuns:    a.TotalRuns,
			Score:        score,
		})
	}

	// Sort by score descending (already partially sorted by spend, but re-rank by composite score).
	sortByScore(entries)

	// Assign ranks and trim.
	if limit <= 0 {
		limit = 10
	}
	if limit > len(entries) {
		limit = len(entries)
	}
	for i := range entries[:limit] {
		entries[i].Rank = i + 1
	}
	return entries[:limit], nil
}

func (s *leaderboardService) DetectAnomalies(ctx context.Context, orgID uuid.UUID, from, to time.Time) ([]AnomalyAlert, error) {
	agents, err := s.usageRepo.TopAgentsBySpend(ctx, orgID, 100, from, to)
	if err != nil {
		return nil, err
	}
	if len(agents) < 3 {
		return nil, nil // need enough data points
	}

	var alerts []AnomalyAlert

	// Check cost anomalies using z-score.
	alerts = append(alerts, detectMetricAnomalies(agents, "cost", func(a model.AgentAnalytics) float64 {
		return a.TotalCostUSD
	})...)

	// Check latency anomalies.
	alerts = append(alerts, detectMetricAnomalies(agents, "latency", func(a model.AgentAnalytics) float64 {
		return float64(a.AvgLatencyMs)
	})...)

	// Check error rate anomalies.
	alerts = append(alerts, detectMetricAnomalies(agents, "error_rate", func(a model.AgentAnalytics) float64 {
		if a.TotalRuns == 0 {
			return 0
		}
		return float64(a.ErrorCount) / float64(a.TotalRuns)
	})...)

	return alerts, nil
}

// computeScore: score = (success_rate * 0.5) - (normalized_latency * 0.2) - (normalized_cost * 0.3)
func computeScore(a model.AgentAnalytics) float64 {
	// Normalize latency: cap at 60s, scale to 0-1.
	normLatency := math.Min(float64(a.AvgLatencyMs)/60000.0, 1.0)
	// Normalize cost: cap at $1, scale to 0-1.
	normCost := math.Min(a.TotalCostUSD/1.0, 1.0)
	if a.TotalRuns > 0 {
		normCost = math.Min(a.TotalCostUSD/float64(a.TotalRuns), 1.0)
	}

	return (a.SuccessRate * 0.5) - (normLatency * 0.2) - (normCost * 0.3)
}

func sortByScore(entries []LeaderboardEntry) {
	for i := 1; i < len(entries); i++ {
		for j := i; j > 0 && entries[j].Score > entries[j-1].Score; j-- {
			entries[j], entries[j-1] = entries[j-1], entries[j]
		}
	}
}

func detectMetricAnomalies(agents []model.AgentAnalytics, metric string, extract func(model.AgentAnalytics) float64) []AnomalyAlert {
	values := make([]float64, len(agents))
	for i, a := range agents {
		values[i] = extract(a)
	}

	mean, stddev := meanStdDev(values)
	if stddev == 0 {
		return nil
	}

	const zThreshold = 2.0
	var alerts []AnomalyAlert
	for i, a := range agents {
		z := (values[i] - mean) / stddev
		if math.Abs(z) >= zThreshold {
			severity := "warning"
			if math.Abs(z) >= 3.0 {
				severity = "critical"
			}
			alerts = append(alerts, AnomalyAlert{
				AgentID:   a.AgentID,
				AgentName: a.AgentName,
				Metric:    metric,
				Value:     values[i],
				Mean:      mean,
				StdDev:    stddev,
				ZScore:    z,
				Severity:  severity,
			})
		}
	}
	return alerts
}

func meanStdDev(values []float64) (float64, float64) {
	n := float64(len(values))
	if n == 0 {
		return 0, 0
	}
	var sum float64
	for _, v := range values {
		sum += v
	}
	mean := sum / n

	var variance float64
	for _, v := range values {
		d := v - mean
		variance += d * d
	}
	variance /= n
	return mean, math.Sqrt(variance)
}
