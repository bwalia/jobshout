package llmbench

import (
	"context"
	"errors"
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
	if r.Status != StatusError || r.Model != "gemini-flash-latest" || r.TokensIn != nil {
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
