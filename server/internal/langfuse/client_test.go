package langfuse

import (
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/model"
)

// capture stands in for Langfuse, recording the bodies it is posted.
type capture struct {
	ch chan []byte
}

func newCapture() (*capture, *httptest.Server) {
	c := &capture{ch: make(chan []byte, 8)}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		c.ch <- body
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	return c, srv
}

// next waits for one exported batch, decoded.
func (c *capture) next(t *testing.T) otlpPayload {
	t.Helper()
	select {
	case body := <-c.ch:
		var p otlpPayload
		if err := json.Unmarshal(body, &p); err != nil {
			t.Fatalf("export body is not valid OTLP JSON: %v", err)
		}
		return p
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for an export")
		return otlpPayload{}
	}
}

// attrs flattens a span's attributes for assertion.
func attrs(t *testing.T, s otlpSpan) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, a := range s.Attributes {
		switch {
		case a.Value.StringValue != nil:
			out[a.Key] = *a.Value.StringValue
		case a.Value.IntValue != nil:
			out[a.Key] = *a.Value.IntValue
		}
	}
	return out
}

func execFixture() *model.AgentExecution {
	started := time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)
	completed := started.Add(1500 * time.Millisecond)
	provider, modelName := "ollama", "llama3:latest"
	output := "hello"
	return &model.AgentExecution{
		ID:            uuid.MustParse("11111111-2222-3333-4444-555555555555"),
		AgentID:       uuid.MustParse("66666666-7777-8888-9999-aaaaaaaaaaaa"),
		OrgID:         uuid.MustParse("bbbbbbbb-cccc-dddd-eeee-ffffffffffff"),
		InputPrompt:   "write something",
		Output:        &output,
		Status:        model.ExecutionStatusCompleted,
		InputTokens:   30,
		OutputTokens:  12,
		TotalTokens:   42,
		LatencyMs:     1500,
		CostUSD:       0.00042,
		ModelName:     &modelName,
		ModelProvider: &provider,
		Iterations:    2,
		EngineType:    model.EngineGoNative,
		StartedAt:     &started,
		CompletedAt:   &completed,
	}
}

func TestNewReturnsNilWhenUnconfigured(t *testing.T) {
	logger := zap.NewNop()
	for _, tc := range []struct{ name, host, pk, sk string }{
		{"no host", "", "pk", "sk"},
		{"no public key", "https://lf.example", "", "sk"},
		{"no secret key", "https://lf.example", "pk", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if c := New(tc.host, tc.pk, tc.sk, "int", logger); c != nil {
				t.Fatalf("expected nil client when %s", tc.name)
			}
		})
	}
}

func TestNilClientIsSafe(t *testing.T) {
	var c *Client
	// None of these may panic — main wires a nil tracer whenever Langfuse is
	// unconfigured, and every call site relies on that.
	c.RecordExecution(execFixture())
	c.Close()
	if c.Enabled() {
		t.Fatal("nil client should report disabled")
	}
}

func TestRecordExecutionExportsUsageAndCost(t *testing.T) {
	cap, srv := newCapture()
	defer srv.Close()

	c := New(srv.URL, "pk", "sk", "int", zap.NewNop())
	if c == nil {
		t.Fatal("expected a configured client")
	}
	defer c.Close()

	exec := execFixture()
	c.RecordExecution(exec)

	payload := cap.next(t)
	spans := payload.ResourceSpans[0].ScopeSpans[0].Spans
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	s := spans[0]
	a := attrs(t, s)

	if got, want := a["gen_ai.request.model"], "llama3:latest"; got != want {
		t.Errorf("model = %q, want %q", got, want)
	}
	if got, want := a["gen_ai.system"], "ollama"; got != want {
		t.Errorf("provider = %q, want %q", got, want)
	}
	if got, want := a["langfuse.observation.usage_details"], `{"input":30,"output":12,"total":42}`; got != want {
		t.Errorf("usage_details = %q, want %q", got, want)
	}
	if got, want := a["langfuse.observation.cost_details"], `{"total":0.00042}`; got != want {
		t.Errorf("cost_details = %q, want %q", got, want)
	}
	if got, want := a["langfuse.session.id"], exec.ID.String(); got != want {
		t.Errorf("session id = %q, want the execution id %q", got, want)
	}
	if got, want := a["langfuse.user.id"], exec.AgentID.String(); got != want {
		t.Errorf("user id = %q, want the agent id %q", got, want)
	}
	if got, want := a["langfuse.environment"], "int"; got != want {
		t.Errorf("environment = %q, want %q", got, want)
	}
	if got, want := a["gen_ai.usage.input_tokens"], "30"; got != want {
		t.Errorf("input tokens = %q, want %q", got, want)
	}

	// The trace id must be the execution UUID's bytes, so a trace is findable
	// from an execution row without storing a second identifier.
	if got, want := s.TraceID, hex.EncodeToString(exec.ID[:]); got != want {
		t.Errorf("trace id = %q, want %q", got, want)
	}
	if len(s.SpanID) != 16 {
		t.Errorf("span id = %q, want 16 hex chars", s.SpanID)
	}
	if s.Status != nil {
		t.Errorf("a completed execution should carry no error status, got %+v", s.Status)
	}
}

func TestSidecarEnginesAreNotDoubleTraced(t *testing.T) {
	cap, srv := newCapture()
	defer srv.Close()

	c := New(srv.URL, "pk", "sk", "int", zap.NewNop())
	defer c.Close()

	for _, engine := range []string{model.EngineLangChain, model.EngineLangGraph} {
		exec := execFixture()
		exec.EngineType = engine
		c.RecordExecution(exec)
	}

	// Give the flush loop more than one interval to prove nothing is sent: the
	// sidecar already reported these runs.
	select {
	case body := <-cap.ch:
		t.Fatalf("sidecar-backed engine was exported from Go, which double-counts it: %s", body)
	case <-time.After(2*flushInterval + 500*time.Millisecond):
	}
}

func TestEmptyEngineTypeIsTreatedAsGoNative(t *testing.T) {
	cap, srv := newCapture()
	defer srv.Close()

	c := New(srv.URL, "pk", "sk", "int", zap.NewNop())
	defer c.Close()

	exec := execFixture()
	exec.EngineType = "" // older rows predate the column default
	c.RecordExecution(exec)

	payload := cap.next(t)
	spans := payload.ResourceSpans[0].ScopeSpans[0].Spans
	if len(spans) != 1 {
		t.Fatalf("expected the span to be exported, got %d", len(spans))
	}
	if got, want := spans[0].Name, "go_native-run"; got != want {
		t.Errorf("span name = %q, want %q", got, want)
	}
}

func TestFailedExecutionCarriesErrorStatus(t *testing.T) {
	cap, srv := newCapture()
	defer srv.Close()

	c := New(srv.URL, "pk", "sk", "int", zap.NewNop())
	defer c.Close()

	exec := execFixture()
	exec.Status = model.ExecutionStatusFailed
	msg := "model timed out"
	exec.ErrorMessage = &msg
	c.RecordExecution(exec)

	s := cap.next(t).ResourceSpans[0].ScopeSpans[0].Spans[0]
	if s.Status == nil {
		t.Fatal("a failed execution should carry an error status")
	}
	if s.Status.Code != 2 {
		t.Errorf("status code = %d, want 2 (error)", s.Status.Code)
	}
	if s.Status.Message != msg {
		t.Errorf("status message = %q, want %q", s.Status.Message, msg)
	}
}

func TestCloseFlushesQueuedSpans(t *testing.T) {
	cap, srv := newCapture()
	defer srv.Close()

	c := New(srv.URL, "pk", "sk", "int", zap.NewNop())
	c.RecordExecution(execFixture())
	// Close well inside the flush interval: the drain path, not the ticker, is
	// what must deliver this span.
	c.Close()

	select {
	case <-cap.ch:
	default:
		t.Fatal("Close did not flush the queued span")
	}
}

func TestSpanWindowFallsBackToLatency(t *testing.T) {
	exec := execFixture()
	exec.StartedAt = nil
	start, end := spanWindow(exec)
	if got := end.Sub(start); got != 1500*time.Millisecond {
		t.Errorf("span width = %v, want the recorded 1.5s latency", got)
	}
}

func TestUsageJSONDerivesTotalWhenAbsent(t *testing.T) {
	exec := execFixture()
	exec.TotalTokens = 0
	if got, want := usageJSON(exec), `{"input":30,"output":12,"total":42}`; got != want {
		t.Errorf("usageJSON = %q, want %q", got, want)
	}
}
