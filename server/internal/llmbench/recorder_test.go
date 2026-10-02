package llmbench

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/llm"
	"github.com/jobshout/server/internal/llmtrace"
)

type memSink struct {
	mu   sync.Mutex
	recs []Record
	err  error
}

func (s *memSink) InsertLLMCalls(_ context.Context, recs []Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.recs = append(s.recs, recs...)
	return s.err
}

func (s *memSink) all() []Record {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Record(nil), s.recs...)
}

type fakeClient struct {
	provider string
	resp     *llm.GenerateResponse
	err      error
	delay    time.Duration
}

func (f *fakeClient) ProviderName() string { return f.provider }
func (f *fakeClient) Generate(ctx context.Context, _ llm.GenerateRequest) (*llm.GenerateResponse, error) {
	if f.delay > 0 {
		time.Sleep(f.delay)
	}
	return f.resp, f.err
}

type listerClient struct{ fakeClient }

func (l *listerClient) ListModels(context.Context) ([]llm.ModelInfo, error) { return nil, nil }

// record runs calls through a fresh Recorder and returns what it wrote.
func record(t *testing.T, c llm.Client, calls func(llm.Client)) []Record {
	t.Helper()
	sink := &memSink{}
	r := New(sink, nil, Options{FlushInterval: time.Hour})
	calls(r.Wrap(c))
	if err := r.Close(context.Background()); err != nil {
		t.Fatalf("Close: %v", err)
	}
	return sink.all()
}

func runCtx() (context.Context, uuid.UUID, uuid.UUID, uuid.UUID) {
	org, agent, run := uuid.New(), uuid.New(), uuid.New()
	ctx := llmtrace.WithTrace(context.Background(), llmtrace.TraceInfo{
		TraceName: "go-course-run", SessionID: run.String(),
		AgentID: agent.String(), OrgID: org.String(),
	})
	return ctx, org, agent, run
}

func TestRecordsSuccessfulCall(t *testing.T) {
	ctx, org, agent, run := runCtx()
	ctx = llm.WithStage(ctx, "outline")
	c := &fakeClient{provider: "gemini", delay: 15 * time.Millisecond,
		resp: &llm.GenerateResponse{Content: "secret reply text", Model: "gemini-flash-latest", InputTokens: 120, OutputTokens: 45}}

	recs := record(t, c, func(w llm.Client) {
		if _, err := w.Generate(ctx, llm.GenerateRequest{Model: "", Messages: []llm.Message{{Role: llm.RoleUser, Content: "secret prompt text"}}}); err != nil {
			t.Fatalf("Generate: %v", err)
		}
	})
	if len(recs) != 1 {
		t.Fatalf("records = %d, want 1", len(recs))
	}
	r := recs[0]
	if r.Provider != "gemini" || r.Model != "gemini-flash-latest" {
		t.Errorf("provider/model = %s/%s", r.Provider, r.Model)
	}
	if r.OrgID == nil || *r.OrgID != org || r.AgentID == nil || *r.AgentID != agent || r.RunID != run.String() {
		t.Errorf("identity = %+v", r)
	}
	if r.RunKind != "course" || r.Stage != "outline" || r.Attempt != 1 {
		t.Errorf("kind/stage/attempt = %s/%s/%d", r.RunKind, r.Stage, r.Attempt)
	}
	if r.TokensIn == nil || *r.TokensIn != 120 || r.TokensOut == nil || *r.TokensOut != 45 {
		t.Errorf("tokens = %v/%v", r.TokensIn, r.TokensOut)
	}
	if r.Status != StatusSuccess || r.Error != "" || r.Retries != 0 {
		t.Errorf("status = %s err=%q retries=%d", r.Status, r.Error, r.Retries)
	}
	if r.LatencyMs < 15 || !r.CompletedAt.After(r.StartedAt) {
		t.Errorf("latency = %dms start=%v end=%v", r.LatencyMs, r.StartedAt, r.CompletedAt)
	}
	// Nothing from the prompt or reply is kept.
	if strings.Contains(r.Error+r.Model+r.RequestedModel+r.Stage, "secret") {
		t.Error("record carries payload text")
	}
}

func TestUnreportedTokensStayNil(t *testing.T) {
	c := &fakeClient{provider: "ollama", resp: &llm.GenerateResponse{Content: "x", Model: "qwen3"}}
	recs := record(t, c, func(w llm.Client) { _, _ = w.Generate(context.Background(), llm.GenerateRequest{}) })
	if recs[0].TokensIn != nil || recs[0].TokensOut != nil {
		t.Errorf("tokens = %v/%v, want nil (unknown)", recs[0].TokensIn, recs[0].TokensOut)
	}
}

func TestFailedCallIsRecordedAndReturned(t *testing.T) {
	boom := errors.New("gemini: overloaded (status 503) key=" + strings.Join([]string{"abc", "def"}, ""))
	c := &fakeClient{provider: "gemini", err: boom}
	recs := record(t, c, func(w llm.Client) {
		if _, err := w.Generate(context.Background(), llm.GenerateRequest{Model: "gemini-flash-latest"}); !errors.Is(err, boom) {
			t.Errorf("err = %v, want the provider's error unchanged", err)
		}
	})
	r := recs[0]
	if r.Status != StatusFailed || r.Model != "gemini-flash-latest" || r.TokensIn != nil {
		t.Errorf("record = %+v", r)
	}
	if strings.Contains(r.Error, "abcdef") || !strings.Contains(r.Error, "[REDACTED]") {
		t.Errorf("error not redacted: %q", r.Error)
	}
}

func TestCancelledCall(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c := &fakeClient{provider: "gemini", err: context.Canceled}
	recs := record(t, c, func(w llm.Client) { _, _ = w.Generate(ctx, llm.GenerateRequest{}) })
	if recs[0].Status != StatusCancelled {
		t.Errorf("status = %s, want cancelled", recs[0].Status)
	}
}

func TestEachCallIsItsOwnRow(t *testing.T) {
	ctx, _, _, _ := runCtx()
	c := &fakeClient{provider: "gemini", resp: &llm.GenerateResponse{Model: "m", InputTokens: 1, OutputTokens: 1}}
	recs := record(t, c, func(w llm.Client) {
		gen := func(ctx context.Context, _ string) (string, error) {
			_, err := w.Generate(ctx, llm.GenerateRequest{})
			return "not json", err
		}
		var v struct{}
		_ = llm.GenerateJSON(ctx, "review", "p", &v, gen, nil)
	})
	if len(recs) != 2 {
		t.Fatalf("records = %d, want 2 (first try + JSON retry)", len(recs))
	}
	if recs[0].Attempt != 1 || recs[1].Attempt != 2 || recs[1].Stage != "review" {
		t.Errorf("attempts = %d,%d stage=%s", recs[0].Attempt, recs[1].Attempt, recs[1].Stage)
	}
}

func TestNestedEngineKeepsRunKind(t *testing.T) {
	ctx, _, _, run := runCtx()
	ctx = llmtrace.WithTraceName(ctx, "go-research-run")
	c := &fakeClient{provider: "gemini", resp: &llm.GenerateResponse{}}
	recs := record(t, c, func(w llm.Client) { _, _ = w.Generate(ctx, llm.GenerateRequest{}) })
	if recs[0].RunKind != "course" || recs[0].RunID != run.String() {
		t.Errorf("kind/run = %s/%s, want course/%s", recs[0].RunKind, recs[0].RunID, run)
	}
}

func TestSinkFailureNeverFailsTheCall(t *testing.T) {
	sink := &memSink{err: errors.New("db down")}
	r := New(sink, nil, Options{FlushInterval: time.Hour})
	w := r.Wrap(&fakeClient{provider: "gemini", resp: &llm.GenerateResponse{Content: "ok"}})
	resp, err := w.Generate(context.Background(), llm.GenerateRequest{})
	if err != nil || resp.Content != "ok" {
		t.Errorf("resp=%v err=%v", resp, err)
	}
	_ = r.Close(context.Background())
}

func TestFullQueueDropsInsteadOfBlocking(t *testing.T) {
	block := make(chan struct{})
	sink := &blockingSink{release: block}
	r := New(sink, nil, Options{QueueSize: 1, BatchSize: 1})
	w := r.Wrap(&fakeClient{provider: "gemini", resp: &llm.GenerateResponse{}})
	done := make(chan struct{})
	go func() {
		for range 50 {
			_, _ = w.Generate(context.Background(), llm.GenerateRequest{})
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Generate blocked on a full benchmark queue")
	}
	close(block)
	_ = r.Close(context.Background())
}

type blockingSink struct{ release chan struct{} }

func (b *blockingSink) InsertLLMCalls(context.Context, []Record) error {
	<-b.release
	return nil
}

func TestWrapPresentsInnerInterfaces(t *testing.T) {
	r := New(&memSink{}, nil, Options{})
	defer r.Close(context.Background())
	if _, ok := r.Wrap(&fakeClient{}).(llm.ModelLister); ok {
		t.Error("plain client must not gain ModelLister")
	}
	if _, ok := r.Wrap(&listerClient{}).(llm.ModelLister); !ok {
		t.Error("lister client lost ModelLister")
	}
	var nilRec *Recorder
	c := &fakeClient{}
	if nilRec.Wrap(c) != llm.Client(c) {
		t.Error("nil recorder must return the client unchanged")
	}
}

func TestRedact(t *testing.T) {
	secret := strings.Repeat("Zq9", 12)
	cases := []string{
		"GET https://x/v1?key=" + secret + "&alt=json",
		"Authorization: Bearer " + secret,
		"x-goog-api-key: " + secret,
		"sk-" + secret,
	}
	for _, in := range cases {
		if out := redact(in); strings.Contains(out, secret) || !strings.Contains(out, "[REDACTED]") {
			t.Errorf("redact(%q) = %q", in, out)
		}
	}
}

func intp(n int) *int { return &n }

func TestRecordsReportedModelUsageAndRequestID(t *testing.T) {
	c := &fakeClient{provider: "gemini", resp: &llm.GenerateResponse{
		Content: "x", Model: "gemini-flash-latest", ProviderModel: "gemini-2.5-flash-preview-09-2025",
		RequestID: "resp-1",
		// A reported 0 must stay 0, not become NULL.
		Usage: llm.Usage{InputTokens: intp(30), OutputTokens: intp(0), TotalTokens: intp(30), ReasoningTokens: intp(0)},
	}}
	recs := record(t, c, func(w llm.Client) { _, _ = w.Generate(context.Background(), llm.GenerateRequest{}) })
	r := recs[0]
	if r.Model != "gemini-2.5-flash-preview-09-2025" || !r.ModelReported || r.RequestedModel != "gemini-flash-latest" {
		t.Errorf("model=%q reported=%v requested=%q", r.Model, r.ModelReported, r.RequestedModel)
	}
	if r.ProviderRequestID != "resp-1" {
		t.Errorf("request id = %q", r.ProviderRequestID)
	}
	if !eq(r.TokensIn, 30) || !eq(r.TokensOut, 0) || !eq(r.TokensTotal, 30) || !eq(r.TokensReasoning, 0) {
		t.Errorf("tokens = %v/%v/%v/%v", r.TokensIn, r.TokensOut, r.TokensTotal, r.TokensReasoning)
	}
}

func eq(p *int, want int) bool { return p != nil && *p == want }

func TestUnreportedModelFallsBackToSentAndTotalStaysNil(t *testing.T) {
	c := &fakeClient{provider: "claude", resp: &llm.GenerateResponse{
		Content: "x", Model: "claude-x", Usage: llm.Usage{InputTokens: intp(5), OutputTokens: intp(2)},
	}}
	recs := record(t, c, func(w llm.Client) { _, _ = w.Generate(context.Background(), llm.GenerateRequest{Model: "claude-x"}) })
	r := recs[0]
	if r.Model != "claude-x" || r.ModelReported || r.RequestedModel != "claude-x" {
		t.Errorf("model=%q reported=%v requested=%q", r.Model, r.ModelReported, r.RequestedModel)
	}
	if r.TokensTotal != nil || r.TokensReasoning != nil || r.ProviderRequestID != "" {
		t.Errorf("total=%v reasoning=%v id=%q, want unreported", r.TokensTotal, r.TokensReasoning, r.ProviderRequestID)
	}
	// The fake does not time HTTP, so the API duration is unknown.
	if r.APIDurationMs != nil {
		t.Errorf("api duration = %v, want nil", *r.APIDurationMs)
	}
}

type namedClient struct{ fakeClient }

func (n *namedClient) ModelName() string { return "default-model" }

func TestFailedCallRecordsClientDefaultAsRequested(t *testing.T) {
	c := &namedClient{fakeClient{provider: "gemini", err: errors.New("boom")}}
	recs := record(t, c, func(w llm.Client) { _, _ = w.Generate(context.Background(), llm.GenerateRequest{}) })
	r := recs[0]
	if r.RequestedModel != "default-model" || r.Model != "default-model" || r.ModelReported {
		t.Errorf("model=%q requested=%q reported=%v", r.Model, r.RequestedModel, r.ModelReported)
	}
	if r.TokensIn != nil || r.TokensOut != nil || r.Status != StatusFailed {
		t.Errorf("tokens=%v/%v status=%s", r.TokensIn, r.TokensOut, r.Status)
	}
}

func TestRecordsTaskRunAndExecutionLinks(t *testing.T) {
	org, agent, task, taskRun, exec := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	// The task run service links the work; the executor then labels its run.
	ctx := llmtrace.WithTaskRun(context.Background(), task.String(), taskRun.String())
	ctx = llmtrace.WithTrace(ctx, llmtrace.TraceInfo{
		TraceName: "go-executor-run", SessionID: exec.String(), ExecutionID: exec.String(),
		AgentID: agent.String(), OrgID: org.String(),
	})
	c := &fakeClient{provider: "ollama", resp: &llm.GenerateResponse{Content: "x", Model: "m"}}
	recs := record(t, c, func(w llm.Client) { _, _ = w.Generate(ctx, llm.GenerateRequest{}) })
	r := recs[0]
	if r.TaskID == nil || *r.TaskID != task || r.TaskRunID == nil || *r.TaskRunID != taskRun ||
		r.ExecutionID == nil || *r.ExecutionID != exec || r.RunKind != "executor" || r.RunID != exec.String() {
		t.Errorf("links = task %v task_run %v exec %v kind %s run %s", r.TaskID, r.TaskRunID, r.ExecutionID, r.RunKind, r.RunID)
	}
}

func TestFailedOllamaCallThroughRealClient(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, `{"error":"model 'qwen3:8b' not found"}`)
	}))
	defer srv.Close()
	ctx, _, _, _ := runCtx()
	recs := record(t, llm.NewOllamaClient(srv.URL, "qwen3:8b"), func(w llm.Client) {
		_, _ = w.Generate(ctx, llm.GenerateRequest{})
	})
	r := recs[0]
	if r.Status != StatusFailed || r.Provider != "ollama" || r.RequestedModel != "qwen3:8b" || r.Model != "qwen3:8b" || r.ModelReported {
		t.Errorf("row = %+v", r)
	}
	if !strings.Contains(r.Error, "500") || r.TokensIn != nil || r.TokensOut != nil || r.TokensTotal != nil {
		t.Errorf("error=%q tokens=%v/%v/%v", r.Error, r.TokensIn, r.TokensOut, r.TokensTotal)
	}
	if r.APIAttempts != 1 || r.Attempts[0].HTTPStatus != 500 || r.APIDurationMs == nil {
		t.Errorf("attempts=%d %+v api=%v", r.APIAttempts, r.Attempts, r.APIDurationMs)
	}
}

func TestFailedGeminiCallKeepsReportedUsage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"candidates":[{"content":{"parts":[{"text":"hmm","thought":true}]},"finishReason":"MAX_TOKENS"}],
		  "usageMetadata":{"promptTokenCount":40,"thoughtsTokenCount":100,"totalTokenCount":140},
		  "modelVersion":"gemini-2.5-flash","responseId":"r-think"}`)
	}))
	defer srv.Close()
	c := llm.NewGeminiClient(srv.URL, strings.Join([]string{"test", "key"}, "-"), "gemini-flash-latest", time.Minute)
	recs := record(t, c, func(w llm.Client) { _, _ = w.Generate(context.Background(), llm.GenerateRequest{}) })
	r := recs[0]
	if r.Status != StatusFailed || r.Model != "gemini-2.5-flash" || !r.ModelReported || r.RequestedModel != "gemini-flash-latest" {
		t.Errorf("status=%s model=%q reported=%v requested=%q", r.Status, r.Model, r.ModelReported, r.RequestedModel)
	}
	if r.ProviderRequestID != "r-think" || !eq(r.TokensIn, 40) || !eq(r.TokensOut, 100) ||
		!eq(r.TokensTotal, 140) || !eq(r.TokensReasoning, 100) {
		t.Errorf("id=%q tokens=%v/%v/%v/%v", r.ProviderRequestID, r.TokensIn, r.TokensOut, r.TokensTotal, r.TokensReasoning)
	}
}

func TestAttemptErrorsAreRedacted(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = io.WriteString(w, "upstream said "+strings.Join([]string{"key", "abcdef123"}, "="))
	}))
	defer srv.Close()
	recs := record(t, llm.NewOllamaClient(srv.URL, "m"), func(w llm.Client) {
		_, _ = w.Generate(context.Background(), llm.GenerateRequest{})
	})
	r := recs[0]
	if strings.Contains(r.Error, "abcdef123") || strings.Contains(r.Attempts[0].Error, "abcdef123") {
		t.Errorf("secret leaked: error=%q attempt=%q", r.Error, r.Attempts[0].Error)
	}
}
