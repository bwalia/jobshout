package handler

import (
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/middleware"
	"github.com/jobshout/server/internal/service"
)

// LeaderboardHandler exposes agent leaderboard and anomaly detection endpoints.
type LeaderboardHandler struct {
	svc service.LeaderboardService
}

// NewLeaderboardHandler creates a LeaderboardHandler.
func NewLeaderboardHandler(svc service.LeaderboardService) *LeaderboardHandler {
	return &LeaderboardHandler{svc: svc}
}

// Leaderboard handles GET /leaderboard
func (h *LeaderboardHandler) Leaderboard(w http.ResponseWriter, r *http.Request) {
	orgID, err := uuid.Parse(middleware.GetOrgID(r.Context()))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid org_id in token")
		return
	}

	limit := 10
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, parseErr := strconv.Atoi(v); parseErr == nil && n > 0 {
			limit = n
		}
	}

	from, to := parseLeaderboardTimeRange(r)

	entries, err := h.svc.Leaderboard(r.Context(), orgID, limit, from, to)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to get leaderboard")
		return
	}
	if entries == nil {
		entries = []service.LeaderboardEntry{}
	}
	RespondJSON(w, http.StatusOK, entries)
}

// Anomalies handles GET /leaderboard/anomalies
func (h *LeaderboardHandler) Anomalies(w http.ResponseWriter, r *http.Request) {
	orgID, err := uuid.Parse(middleware.GetOrgID(r.Context()))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid org_id in token")
		return
	}

	from, to := parseLeaderboardTimeRange(r)

	alerts, err := h.svc.DetectAnomalies(r.Context(), orgID, from, to)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to detect anomalies")
		return
	}
	if alerts == nil {
		alerts = []service.AnomalyAlert{}
	}
	RespondJSON(w, http.StatusOK, alerts)
}

func parseLeaderboardTimeRange(r *http.Request) (time.Time, time.Time) {
	to := time.Now()
	from := to.AddDate(0, 0, -30)

	if v := r.URL.Query().Get("from"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			from = t
		}
	}
	if v := r.URL.Query().Get("to"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			to = t
		}
	}
	return from, to
}
