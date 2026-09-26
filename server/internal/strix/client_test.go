package strix

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
)

func testClient(t *testing.T, srv *httptest.Server) *Client {
	t.Helper()
	c := NewClient(srv.URL, "", time.Second, zap.NewNop())
	return c
}

func TestClientEnabled(t *testing.T) {
	if NewClient("", "", time.Second, nil).Enabled() {
		t.Error("client with empty base URL should report disabled")
	}
	if !NewClient("https://strix.example", "", time.Second, nil).Enabled() {
		t.Error("client with a base URL should report enabled")
	}
	// Methods on a disabled client fail fast rather than dialling nothing.
	if _, err := NewClient("", "", time.Second, nil).Start(context.Background(), StartRequest{}); err == nil {
		t.Error("Start on a disabled client should error")
	}
}

func TestStartAndPollToCompletion(t *testing.T) {
	var startBody startRequestWire
	var gotAuthHeader string

	polls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuthHeader = r.Header.Get("x-api-key")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/scan":
			_ = json.NewDecoder(r.Body).Decode(&startBody)
			w.WriteHeader(http.StatusAccepted)
			_ = json.NewEncoder(w).Encode(scanAcceptedWire{
				RunID: "remote-1", RunRef: startBody.RunRef, Status: "queued", QueuePosition: 2,
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/scan/remote-1":
			polls++
			if polls < 2 {
				_ = json.NewEncoder(w).Encode(scanStatusWire{
					RunID: "remote-1", Status: "running", Target: "https://juice.local",
				})
				return
			}
			started := "2026-08-21T10:00:00+00:00"
			completed := "2026-08-21T10:05:00+00:00"
			exit := 2
			_ = json.NewEncoder(w).Encode(scanStatusWire{
				RunID: "remote-1", RunRef: "run-ref-1", Status: "completed",
				Target: "https://juice.local", ScanMode: "quick",
				StartedAt: &started, CompletedAt: &completed, DurationMS: 300000, ExitCode: &exit,
				FindingCount: 1,
				Findings: []Finding{{
					ID: "V-1", Title: "SQLi", Severity: "high", CVSSScore: 8.1,
					Category: "injection", ReproductionSteps: "…",
				}},
			})
		default:
			http.Error(w, "unexpected "+r.Method+" "+r.URL.Path, http.StatusNotFound)
		}
	}))
	defer srv.Close()

	// A signing secret so we can assert the token is attached.
	c := NewClient(srv.URL, "shared-secret", time.Second, zap.NewNop())
	ctx := context.Background()

	handle, err := c.Start(ctx, StartRequest{
		Target: "https://juice.local", ScanMode: "quick", MaxBudget: 10,
		RunRef: "run-ref-1", RequestedBy: "user-1",
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if handle.RemoteRunID != "remote-1" || handle.Status != "queued" || handle.QueuePosition != 2 {
		t.Fatalf("unexpected handle: %+v", handle)
	}
	if startBody.RunRef != "run-ref-1" || startBody.RequestedBy != "user-1" || startBody.MaxBudget != 10 {
		t.Errorf("request body not forwarded: %+v", startBody)
	}
	if gotAuthHeader == "" {
		t.Error("expected a signed x-api-key header when a secret is configured")
	}

	// First poll: still running, not terminal.
	res, err := c.Status(ctx, handle.RemoteRunID)
	if err != nil {
		t.Fatalf("Status (first): %v", err)
	}
	if res.Terminal() {
		t.Fatalf("first poll should be non-terminal, got %q", res.Status)
	}

	// Second poll: completed with a finding and parsed timestamps.
	res, err = c.Status(ctx, handle.RemoteRunID)
	if err != nil {
		t.Fatalf("Status (second): %v", err)
	}
	if !res.Terminal() || res.Status != "completed" {
		t.Fatalf("expected terminal completed, got %q", res.Status)
	}
	if len(res.Findings) != 1 || res.Findings[0].ID != "V-1" || res.Findings[0].CVSSScore != 8.1 {
		t.Errorf("finding not decoded: %+v", res.Findings)
	}
	if res.StartedAt == nil || res.CompletedAt == nil {
		t.Fatal("expected timestamps to parse")
	}
	if res.ExitCode == nil || *res.ExitCode != 2 {
		t.Errorf("exit code not decoded: %v", res.ExitCode)
	}
}

func TestStartOutOfScopeIsPermanent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"detail": "target 'http://10.0.0.1' resolves to internal address 10.0.0.1; name that range in STRIX_TARGET_ALLOWLIST to scan it",
		})
	}))
	defer srv.Close()

	_, err := testClient(t, srv).Start(context.Background(), StartRequest{Target: "http://10.0.0.1"})
	if !errors.Is(err, ErrOutOfScope) {
		t.Fatalf("expected ErrOutOfScope, got %v", err)
	}
	if errors.Is(err, ErrUnauthorized) {
		t.Fatal("a scope 403 must not be read as an auth failure")
	}
}

func TestGatewayHTML502IsErrGatewayNotHTMLDump(t *testing.T) {
	html := `<!DOCTYPE html><html lang="en"><head><title>Server Error | WSL Proxy</title></head><body>502</body></html>`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(html))
	}))
	defer srv.Close()

	_, err := testClient(t, srv).Status(context.Background(), "remote-1")
	if !errors.Is(err, ErrGateway) {
		t.Fatalf("expected ErrGateway, got %v", err)
	}
	if strings.Contains(err.Error(), "<!DOCTYPE") {
		t.Errorf("gateway error must not dump HTML into the UI: %v", err)
	}
}

func TestHTML503IsGatewayNotBusy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`<!DOCTYPE html><html><title>Server Error | WSL Proxy</title></html>`))
	}))
	defer srv.Close()

	_, err := testClient(t, srv).Start(context.Background(), StartRequest{Target: "https://ok.local"})
	if !errors.Is(err, ErrGateway) {
		t.Fatalf("HTML 503 should be ErrGateway, got %v", err)
	}
	if errors.Is(err, ErrBusy) {
		t.Fatal("an edge HTML 503 must not be read as queue-full")
	}
}

func TestStartBusyIsTransient(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "60")
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{"detail": "8 scans already queued (limit 8)"})
	}))
	defer srv.Close()

	_, err := testClient(t, srv).Start(context.Background(), StartRequest{Target: "https://ok.local"})
	if !errors.Is(err, ErrBusy) {
		t.Fatalf("expected ErrBusy, got %v", err)
	}
}

func TestForbiddenAuthBodyIsUnauthorized(t *testing.T) {
	// A 403 whose body is about the token, not the target, is an auth failure.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"detail": "token expired — check the clock on the calling host",
		})
	}))
	defer srv.Close()

	_, err := testClient(t, srv).Status(context.Background(), "remote-1")
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("expected ErrUnauthorized for an expired-token 403, got %v", err)
	}
	if errors.Is(err, ErrOutOfScope) {
		t.Fatal("an auth 403 must not be read as out of scope")
	}
}

func TestUnauthorizedStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "missing x-api-key header", http.StatusUnauthorized)
	}))
	defer srv.Close()

	err := testClient(t, srv).Cancel(context.Background(), "remote-1")
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("expected ErrUnauthorized, got %v", err)
	}
}

func TestNetworkFailureIsPlainError(t *testing.T) {
	// A dead service: closed immediately so the dial fails. Must surface as a
	// plain error, distinct from the scope/busy/auth sentinels.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL
	srv.Close()

	c := NewClient(url, "", 200*time.Millisecond, zap.NewNop())
	_, err := c.Status(context.Background(), "remote-1")
	if err == nil {
		t.Fatal("expected an error reaching a closed server")
	}
	if errors.Is(err, ErrOutOfScope) || errors.Is(err, ErrBusy) || errors.Is(err, ErrUnauthorized) || errors.Is(err, ErrGateway) {
		t.Fatalf("network failure misclassified as a typed error: %v", err)
	}
}

func TestTimeoutBoundsOneCall(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		_ = json.NewEncoder(w).Encode(scanStatusWire{RunID: "remote-1", Status: "running"})
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "", 50*time.Millisecond, zap.NewNop())
	if _, err := c.Status(context.Background(), "remote-1"); err == nil {
		t.Fatal("expected the per-call timeout to fire")
	}
}

