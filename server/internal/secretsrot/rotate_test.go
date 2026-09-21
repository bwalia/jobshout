package secretsrot

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDetectProvider(t *testing.T) {
	if DetectProvider("https://vault.workstation.co.uk", nil) != "wslvault" {
		t.Fatal("expected wslvault")
	}
	if DetectProvider("https://vault.example.com", map[string]any{"initialized": true}) != "hashicorp" {
		t.Fatal("expected hashicorp")
	}
}

func TestParseKeys(t *testing.T) {
	got := ParseKeys("password, api_key; token")
	if len(got) != 3 {
		t.Fatalf("got %#v", got)
	}
}

func TestExecute_PlanDryKV2(t *testing.T) {
	var lastPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lastPath = r.URL.Path
		switch {
		case strings.HasSuffix(r.URL.Path, "/v1/sys/health"):
			_ = json.NewEncoder(w).Encode(map[string]any{"initialized": true})
		case strings.Contains(r.URL.Path, "/metadata/"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{"current_version": 3, "versions": map[string]any{"3": map[string]any{}}},
			})
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()

	cfg := Config{Addr: srv.URL, Token: "t", Timeout: 0}
	out, err := Execute(context.Background(), cfg, RunOptions{
		Mode: "plan", Engine: "kv2", Mount: "secret", Path: "prod/db", GraceSeconds: 60, Provider: "auto",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if out.Result.CurrentVersion != 3 {
		t.Fatalf("version %d path=%s", out.Result.CurrentVersion, lastPath)
	}
	if len(out.Result.PlanSteps) < 3 {
		t.Fatalf("plan steps: %#v", out.Result.PlanSteps)
	}
}

func TestExecute_RotateDryRun(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/v1/sys/health"):
			_ = json.NewEncoder(w).Encode(map[string]any{"initialized": true})
		case strings.Contains(r.URL.Path, "/data/"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"data":     map[string]any{"password": "old"},
					"metadata": map[string]any{"version": 2},
				},
			})
		default:
			w.WriteHeader(200)
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	defer srv.Close()

	cfg := Config{Addr: srv.URL, Token: "t"}
	out, err := Execute(context.Background(), cfg, RunOptions{
		Mode: "rotate", Engine: "kv2", Mount: "secret", Path: "app/creds",
		Keys: []string{"password"}, DryRun: true, GraceSeconds: 0, RetireOld: true,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if out.Result.PreviousVersion != 2 {
		t.Fatalf("prev %d", out.Result.PreviousVersion)
	}
	if out.Result.CurrentVersion != 3 {
		t.Fatalf("next %d", out.Result.CurrentVersion)
	}
	if !out.Result.ZeroDowntime {
		t.Fatal("expected zero downtime strategy")
	}
}

func TestKV2WriteAndSoftDelete(t *testing.T) {
	var wrote bool
	var deleted []int
	version := 1
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/data/"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"data":     map[string]any{"password": "old", "user": "app"},
					"metadata": map[string]any{"version": version},
				},
			})
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/data/"):
			wrote = true
			version = 2
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{"version": 2},
			})
		case strings.Contains(r.URL.Path, "/delete/"):
			var body struct {
				Versions []int `json:"versions"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			deleted = body.Versions
			w.WriteHeader(204)
		case strings.HasSuffix(r.URL.Path, "/v1/sys/health"):
			w.WriteHeader(200)
			_, _ = w.Write([]byte(`{}`))
		default:
			w.WriteHeader(200)
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	defer srv.Close()

	cfg := Config{Addr: srv.URL, Token: "t"}
	out, err := Execute(context.Background(), cfg, RunOptions{
		Mode: "rotate", Engine: "kv2", Mount: "secret", Path: "x",
		Keys: []string{"password"}, GraceSeconds: 0, RetireOld: true, DryRun: false,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !wrote {
		t.Fatal("expected write")
	}
	if len(deleted) != 1 || deleted[0] != 1 {
		t.Fatalf("deleted %#v", deleted)
	}
	if !out.Result.OldVersionRetired {
		t.Fatal("expected retire")
	}
	if out.Result.CurrentVersion != 2 {
		t.Fatalf("current %d", out.Result.CurrentVersion)
	}
}
