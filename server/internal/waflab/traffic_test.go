package waflab

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jobshout/server/internal/wslproxymcp"
)

func TestTrafficReadsAndWrites(t *testing.T) {
	var gotBody map[string]any
	var gotPlatform string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/servers/host:known.example.org":
			_, _ = w.Write([]byte(`{"data":{"id":"host:known.example.org","rules":["r1"]}}`))
		case "/api/servers/host:missing.example.org":
			_, _ = w.Write([]byte(`{"data":{}}`))
		case "/api/rules/missing":
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":{"code":"NOT_FOUND","message":"Rule not found","status":404}}`))
		case "/api/traffic/backends/weights":
			gotPlatform = r.Header.Get("x-platform")
			_ = json.NewDecoder(r.Body).Decode(&gotBody)
			_, _ = w.Write([]byte(`{"data":{"rule_id":"r1","effective_immediately":true}}`))
		case "/api/traffic/backends/promote":
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"status":404,"message":"Backend with label 'x' not found"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := NewClient(Config{Enabled: true, BaseURL: srv.URL, APIToken: "tok", Profile: "prod"}, nil)
	ctx := context.Background()

	s, err := c.GetServer(ctx, "host:known.example.org", "")
	if err != nil || s["id"] != "host:known.example.org" {
		t.Fatalf("GetServer = %v, %v", s, err)
	}
	if s, err := c.GetServer(ctx, "host:missing.example.org", ""); s != nil || err != nil {
		t.Fatalf("missing server = %v, %v", s, err)
	}
	if r, err := c.GetRule(ctx, "missing", ""); r != nil || err != nil {
		t.Fatalf("missing rule = %v, %v", r, err)
	}
	out, err := c.UpdateTrafficWeights(ctx, "r1", "", []wslproxymcp.BackendWeight{{Label: "v1", Weight: 70}})
	if err != nil || out["rule_id"] != "r1" {
		t.Fatalf("UpdateTrafficWeights = %v, %v", out, err)
	}
	if gotBody["rule_id"] != "r1" || gotBody["profile_id"] != "prod" || gotPlatform == "" {
		t.Fatalf("body=%v platform=%q", gotBody, gotPlatform)
	}
	if _, err := c.PromoteTrafficBackend(ctx, "r1", "x", ""); err == nil {
		t.Fatal("promote of unknown label should fail")
	}
}
