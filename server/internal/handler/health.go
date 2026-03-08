package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// HealthResponse represents the structure returned by the health endpoint.
type HealthResponse struct {
	Status  string `json:"status"`
	Version string `json:"version"`
	DB      string `json:"db"`
}

// Health returns an HTTP handler that checks application health including database connectivity.
func Health(pool *pgxpool.Pool, version string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()

		dbStatus := "ok"
		if err := pool.Ping(ctx); err != nil {
			dbStatus = "error"
		}

		status := "ok"
		statusCode := http.StatusOK
		if dbStatus != "ok" {
			status = "degraded"
			statusCode = http.StatusServiceUnavailable
		}

		resp := HealthResponse{
			Status:  status,
			Version: version,
			DB:      dbStatus,
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		json.NewEncoder(w).Encode(resp)
	}
}
