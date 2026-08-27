package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// RuntimeInfo is the build/deploy stamp shown in the web sidebar.
type RuntimeInfo struct {
	Version    string
	Env        string
	DeployedAt time.Time
}

// HealthResponse represents the structure returned by the health endpoint.
type HealthResponse struct {
	Status     string `json:"status"`
	Version    string `json:"version"`
	Env        string `json:"env,omitempty"`
	DeployedAt string `json:"deployed_at,omitempty"`
	DB         string `json:"db"`
}

// Health returns an HTTP handler that checks application health including database connectivity.
func Health(pool *pgxpool.Pool, info RuntimeInfo) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()

		dbStatus := "ok"
		if pool != nil {
			if err := pool.Ping(ctx); err != nil {
				dbStatus = "error"
			}
		}

		status := "ok"
		statusCode := http.StatusOK
		if dbStatus != "ok" {
			status = "degraded"
			statusCode = http.StatusServiceUnavailable
		}

		resp := HealthResponse{
			Status:  status,
			Version: info.Version,
			Env:     info.Env,
			DB:      dbStatus,
		}
		if !info.DeployedAt.IsZero() {
			resp.DeployedAt = info.DeployedAt.UTC().Format(time.RFC3339)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		json.NewEncoder(w).Encode(resp)
	}
}
