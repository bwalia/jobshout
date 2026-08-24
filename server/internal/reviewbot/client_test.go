package reviewbot

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestStartAndStatus(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/reviews", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"job_id": "abcd1234", "state": "queued", "queue_position": 0, "existing": false,
		})
	})
	mux.HandleFunc("/api/reviews/abcd1234", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"job_id": "abcd1234", "state": "done",
			"result": map[string]any{"decision": "MERGE"},
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	c := NewClient(srv.URL, "secret", DefaultTimeout, nil)
	handle, err := c.Start(t.Context(), StartRequest{Repo: "bwalia/jobshout", PRNumber: 1, DryRun: true, RunRef: "run-1"})
	if err != nil {
		t.Fatal(err)
	}
	if handle.JobID != "abcd1234" {
		t.Fatalf("job id = %s", handle.JobID)
	}
	snap, err := c.Status(t.Context(), handle.JobID)
	if err != nil {
		t.Fatal(err)
	}
	if !snap.Terminal() || snap.State != "done" {
		t.Fatalf("snapshot = %+v", snap)
	}
}

func TestStatusJobNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"Job not found — the server may have restarted."}`))
	}))
	t.Cleanup(srv.Close)

	c := NewClient(srv.URL, "secret", DefaultTimeout, nil)
	_, err := c.Status(t.Context(), "gone")
	if !errors.Is(err, ErrJobNotFound) {
		t.Fatalf("got %v", err)
	}
}

func TestRepoAllowed(t *testing.T) {
	list := []string{"bwalia/jobshout", "acme/widgets"}
	if !RepoAllowed("bwalia/jobshout", list) {
		t.Fatal("expected allowed")
	}
	if RepoAllowed("evil/repo", list) {
		t.Fatal("expected rejected")
	}
	if RepoAllowed("bwalia/jobshout", nil) {
		t.Fatal("empty allowlist must refuse")
	}
}
