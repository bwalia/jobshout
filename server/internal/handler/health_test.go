package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHealthIncludesDeployStamp(t *testing.T) {
	t.Parallel()
	deployed := time.Date(2026, 8, 27, 6, 56, 26, 0, time.UTC)
	h := Health(nil, RuntimeInfo{
		Version:    "v1.0.8",
		Env:        "int",
		DeployedAt: deployed,
	})

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var got HealthResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Version != "v1.0.8" {
		t.Errorf("version = %q, want v1.0.8", got.Version)
	}
	if got.Env != "int" {
		t.Errorf("env = %q, want int", got.Env)
	}
	if got.DeployedAt != "2026-08-27T06:56:26Z" {
		t.Errorf("deployed_at = %q, want 2026-08-27T06:56:26Z", got.DeployedAt)
	}
	if got.Status != "ok" {
		t.Errorf("status = %q, want ok", got.Status)
	}
}
