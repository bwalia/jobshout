package llm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// gatewayServer answers the first fail requests with status, then a reply.
func gatewayServer(t *testing.T, status, fail int) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if int(calls.Add(1)) <= fail {
			w.WriteHeader(status)
			_, _ = w.Write([]byte("<html><title>Server Error | WSL Proxy</title></html>"))
			return
		}
		w.Header().Set("Content-Type", "application/x-ndjson")
		_, _ = w.Write([]byte(`{"message":{"role":"assistant","content":"draft"},"done":true}` + "\n"))
	}))
	t.Cleanup(srv.Close)
	return srv, &calls
}

func gatewayClient(url string) *OllamaClient {
	c := NewOllamaClient(url, "muse")
	c.retryWait = time.Millisecond
	return c
}

// A proxy 504 while a cold model loads is the Article Writer's draft failure
// on int: the second attempt finds the model resident and must be made.
func TestOllamaGenerate_RetriesOnceAfterGatewayTimeout(t *testing.T) {
	srv, calls := gatewayServer(t, http.StatusGatewayTimeout, 1)
	ctx, stats := WithCallStats(context.Background())

	resp, err := gatewayClient(srv.URL).Generate(ctx, GenerateRequest{
		Messages: []Message{{Role: RoleUser, Content: "write"}},
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if resp.Content != "draft" {
		t.Errorf("content = %q, want draft", resp.Content)
	}
	if got := calls.Load(); got != 2 {
		t.Errorf("requests = %d, want 2", got)
	}
	if got := stats.Retries(); got != 1 {
		t.Errorf("retries = %d, want 1", got)
	}
	if got := len(stats.Attempts()); got != 2 {
		t.Errorf("attempts recorded = %d, want 2", got)
	}
}

// Each attempt can cost the proxy's whole read timeout, so a gateway that
// keeps failing gets exactly one retry and its status is reported.
func TestOllamaGenerate_GivesUpAfterSecondGatewayError(t *testing.T) {
	for _, status := range []int{http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout} {
		srv, calls := gatewayServer(t, status, 99)
		_, err := gatewayClient(srv.URL).Generate(context.Background(), GenerateRequest{
			Messages: []Message{{Role: RoleUser, Content: "write"}},
		})
		if err == nil || !strings.Contains(err.Error(), "unexpected status") {
			t.Fatalf("status %d: err = %v, want unexpected status", status, err)
		}
		if got := calls.Load(); got != ollamaGatewayAttempts {
			t.Errorf("status %d: requests = %d, want %d", status, got, ollamaGatewayAttempts)
		}
	}
}

// Other errors are not gateway trouble and are returned on the first answer.
func TestOllamaGenerate_DoesNotRetryOtherErrors(t *testing.T) {
	srv, calls := gatewayServer(t, http.StatusInternalServerError, 99)
	_, err := gatewayClient(srv.URL).Generate(context.Background(), GenerateRequest{
		Messages: []Message{{Role: RoleUser, Content: "write"}},
	})
	if err == nil {
		t.Fatal("want an error")
	}
	if got := calls.Load(); got != 1 {
		t.Errorf("requests = %d, want 1", got)
	}
}

// A cancelled run must not sit out the retry wait.
func TestOllamaGenerate_GatewayRetryStopsOnCancel(t *testing.T) {
	srv, calls := gatewayServer(t, http.StatusGatewayTimeout, 99)
	c := NewOllamaClient(srv.URL, "muse")
	c.retryWait = time.Hour
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := c.Generate(ctx, GenerateRequest{Messages: []Message{{Role: RoleUser, Content: "write"}}})
	if err != context.DeadlineExceeded {
		t.Fatalf("err = %v, want context.DeadlineExceeded", err)
	}
	if got := calls.Load(); got != 1 {
		t.Errorf("requests = %d, want 1", got)
	}
}
