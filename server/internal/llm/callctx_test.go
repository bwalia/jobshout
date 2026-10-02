package llm

import (
	"context"
	"io"
	"net/http"
	"sync/atomic"
	"testing"
)

func TestCallStatsCountsTransportRetries(t *testing.T) {
	var hits atomic.Int32
	c, _ := newTestGemini(t, func(w http.ResponseWriter, _ *http.Request) {
		if hits.Add(1) < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = io.WriteString(w, `{"error":{"code":503,"message":"overloaded","status":"UNAVAILABLE"}}`)
			return
		}
		_, _ = io.WriteString(w, geminiOK)
	})
	ctx, stats := WithCallStats(context.Background())
	if _, err := c.Generate(ctx, GenerateRequest{Messages: []Message{{Role: RoleUser, Content: "hi"}}}); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if got := stats.Retries(); got != 2 {
		t.Errorf("retries = %d, want 2", got)
	}
}

func TestCallStatsZeroWhenFirstTrySucceeds(t *testing.T) {
	c, _ := newTestGemini(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, geminiOK)
	})
	ctx, stats := WithCallStats(context.Background())
	if _, err := c.Generate(ctx, GenerateRequest{Messages: []Message{{Role: RoleUser, Content: "hi"}}}); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if stats.Retries() != 0 {
		t.Errorf("retries = %d, want 0", stats.Retries())
	}
}

func TestGenerateJSONLabelsStageAndAttempt(t *testing.T) {
	type seen struct {
		stage   string
		attempt int
	}
	var calls []seen
	gen := func(ctx context.Context, _ string) (string, error) {
		calls = append(calls, seen{StageFrom(ctx), AttemptFrom(ctx)})
		if len(calls) == 1 {
			return "not json", nil
		}
		return `{"a":1}`, nil
	}
	var v struct{ A int }
	if err := GenerateJSON(context.Background(), "outline", "p", &v, gen, nil); err != nil {
		t.Fatalf("GenerateJSON: %v", err)
	}
	want := []seen{{"outline", 1}, {"outline", 2}}
	if len(calls) != 2 || calls[0] != want[0] || calls[1] != want[1] {
		t.Errorf("calls = %+v, want %+v", calls, want)
	}

	// A step already marked by the caller is kept.
	calls = nil
	ctx := WithStage(context.Background(), "research")
	if err := GenerateJSON(ctx, "plan", "p", &v, gen, nil); err != nil {
		t.Fatalf("GenerateJSON: %v", err)
	}
	if calls[0].stage != "research" {
		t.Errorf("stage = %q, want research", calls[0].stage)
	}
}
