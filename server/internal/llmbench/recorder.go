// Package llmbench records one row per text LLM call for the LLM benchmark.
//
// It is a decorator installed with llm.Router.WrapClients (and on chat's
// separately built client), so every call that passes through the common LLM
// layer is measured the same way whichever provider or agent made it. Nothing
// here is provider- or agent-specific: the provider and model come from the
// response, the run and step from labels already on ctx.
//
// What is stored is metadata only — timing, token counts, retries, status and
// a short error. Prompts, replies and credentials never are.
//
// Recording must never fail or slow a call: rows go to a bounded queue drained
// by a background writer, and a full queue or a database error drops the row
// with a warning.
package llmbench

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/llm"
	"github.com/jobshout/server/internal/llmtrace"
)

// Call statuses.
const (
	StatusSuccess   = "success"
	StatusError     = "error"
	StatusCancelled = "cancelled"
)

// maxErrorLen caps the stored error text.
const maxErrorLen = 1024

// maxModelLen matches usage_records.model.
const maxModelLen = 100

// Record is one LLM call.
type Record struct {
	OrgID          *uuid.UUID
	AgentID        *uuid.UUID
	TaskID         *uuid.UUID
	RunKind        string
	RunID          string
	Stage          string
	Attempt        int
	Provider       string
	Model          string
	RequestedModel string
	// TokensIn and TokensOut are nil when the provider did not report them.
	TokensIn    *int
	TokensOut   *int
	LatencyMs   int
	Retries     int
	Status      string
	Error       string
	StartedAt   time.Time
	CompletedAt time.Time
}

// Sink persists a batch of records.
type Sink interface {
	InsertLLMCalls(ctx context.Context, recs []Record) error
}

// Recorder wraps clients and writes their calls to a Sink in the background.
type Recorder struct {
	sink      Sink
	logger    *zap.Logger
	queue     chan Record
	batchSize int
	interval  time.Duration
	now       func() time.Time

	done      chan struct{}
	closeOnce sync.Once
	mu        sync.RWMutex
	closed    bool
}

// Options tunes a Recorder; zero values take the defaults.
type Options struct {
	QueueSize     int
	BatchSize     int
	FlushInterval time.Duration
}

// New starts a Recorder writing to sink. Call Close on shutdown to flush.
func New(sink Sink, logger *zap.Logger, opts Options) *Recorder {
	if logger == nil {
		logger = zap.NewNop()
	}
	if opts.QueueSize <= 0 {
		opts.QueueSize = 4096
	}
	if opts.BatchSize <= 0 {
		opts.BatchSize = 100
	}
	if opts.FlushInterval <= 0 {
		opts.FlushInterval = 2 * time.Second
	}
	r := &Recorder{
		sink:      sink,
		logger:    logger,
		queue:     make(chan Record, opts.QueueSize),
		batchSize: opts.BatchSize,
		interval:  opts.FlushInterval,
		now:       time.Now,
		done:      make(chan struct{}),
	}
	go r.run()
	return r
}

// Wrap decorates c so each Generate call is recorded. Like llmtrace.Wrap it
// presents exactly the optional interfaces the inner client does: the
// executor asserts llm.ToolCapableClient and model discovery asserts
// llm.ModelLister, whose mere presence switches discovery to live probing.
func (r *Recorder) Wrap(c llm.Client) llm.Client {
	if r == nil {
		return c
	}
	bc := &benchClient{inner: c, rec: r}
	if _, ok := c.(llm.ModelLister); ok {
		return &benchListerClient{bc}
	}
	return bc
}

type benchClient struct {
	inner llm.Client
	rec   *Recorder
}

type benchListerClient struct{ *benchClient }

func (c *benchListerClient) ListModels(ctx context.Context) ([]llm.ModelInfo, error) {
	return c.inner.(llm.ModelLister).ListModels(ctx)
}

func (c *benchClient) ProviderName() string { return c.inner.ProviderName() }

func (c *benchClient) SupportsTools() bool {
	tc, ok := c.inner.(llm.ToolCapableClient)
	return ok && tc.SupportsTools()
}

func (c *benchClient) Generate(ctx context.Context, req llm.GenerateRequest) (*llm.GenerateResponse, error) {
	callCtx, stats := llm.WithCallStats(ctx)
	start := c.rec.now()
	resp, err := c.inner.Generate(callCtx, req)
	end := c.rec.now()
	c.rec.enqueue(buildRecord(ctx, c.inner.ProviderName(), req, resp, err, stats.Retries(), start, end))
	return resp, err
}

// buildRecord turns one finished call into a Record. ctx is the caller's, so
// a cancellation that arrived during the call is visible here.
func buildRecord(
	ctx context.Context, provider string, req llm.GenerateRequest,
	resp *llm.GenerateResponse, err error, retries int, start, end time.Time,
) Record {
	info, _ := llmtrace.FromContext(ctx)
	rec := Record{
		OrgID:          parseID(info.OrgID),
		AgentID:        parseID(info.AgentID),
		TaskID:         parseID(info.TaskID),
		RunKind:        runKind(info),
		RunID:          info.SessionID,
		Stage:          llm.StageFrom(ctx),
		Attempt:        llm.AttemptFrom(ctx),
		Provider:       provider,
		Model:          truncate(req.Model, maxModelLen),
		RequestedModel: truncate(req.Model, 200),
		LatencyMs:      int(end.Sub(start).Milliseconds()),
		Retries:        retries,
		Status:         StatusSuccess,
		StartedAt:      start.UTC(),
		CompletedAt:    end.UTC(),
	}
	if resp != nil {
		if resp.Model != "" {
			rec.Model = truncate(resp.Model, maxModelLen)
		}
		// Providers leave a count at 0 when they did not report it (Ollama
		// without its final chunk); a real prompt or reply is never 0 tokens.
		rec.TokensIn = positive(resp.InputTokens)
		rec.TokensOut = positive(resp.OutputTokens)
	}
	if err != nil {
		rec.Status = StatusError
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || ctx.Err() != nil {
			rec.Status = StatusCancelled
		}
		rec.Error = truncate(redact(err.Error()), maxErrorLen)
	}
	return rec
}

// runKind is the engine of the run the call belongs to: "go-course-run" →
// "course". Calls outside any run fall back to the engine label alone.
func runKind(info llmtrace.TraceInfo) string {
	k := info.RunKind
	if k == "" {
		k = info.TraceName
	}
	k = strings.TrimPrefix(k, "go-")
	k = strings.TrimSuffix(k, "-run")
	if k == "" {
		k = "llm"
	}
	return k
}

func (r *Recorder) enqueue(rec Record) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.closed {
		return
	}
	select {
	case r.queue <- rec:
	default:
		r.logger.Warn("llm benchmark: queue full, dropping record",
			zap.String("provider", rec.Provider), zap.String("model", rec.Model))
	}
}

func (r *Recorder) run() {
	defer close(r.done)
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	batch := make([]Record, 0, r.batchSize)
	flush := func() {
		if len(batch) == 0 {
			return
		}
		// Not tied to any request: the call that produced a row may already
		// have been cancelled, and its timing is still worth keeping.
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		if err := r.sink.InsertLLMCalls(ctx, batch); err != nil {
			r.logger.Warn("llm benchmark: dropping records after write failure",
				zap.Int("records", len(batch)), zap.Error(err))
		}
		cancel()
		batch = batch[:0]
	}
	for {
		select {
		case rec, ok := <-r.queue:
			if !ok {
				flush()
				return
			}
			batch = append(batch, rec)
			if len(batch) >= r.batchSize {
				flush()
			}
		case <-ticker.C:
			flush()
		}
	}
}

// Close stops accepting records and waits (up to ctx) for queued ones to be
// written.
func (r *Recorder) Close(ctx context.Context) error {
	r.closeOnce.Do(func() {
		r.mu.Lock()
		r.closed = true
		close(r.queue)
		r.mu.Unlock()
	})
	select {
	case <-r.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func parseID(s string) *uuid.UUID {
	if s == "" {
		return nil
	}
	id, err := uuid.Parse(s)
	if err != nil || id == uuid.Nil {
		return nil
	}
	return &id
}

func positive(n int) *int {
	if n <= 0 {
		return nil
	}
	return &n
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	// Cut on a rune boundary so the stored text stays valid UTF-8.
	for n > 0 && !utf8Start(s[n]) {
		n--
	}
	return s[:n]
}

func utf8Start(b byte) bool { return b&0xC0 != 0x80 }

// Credentials are sent in headers, not URLs, by every provider here; this is
// a second line of defence in case an error message ever echoes one.
var secretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(\b(?:key|api_key|apikey|access_token|token)=)[^&\s"']+`),
	regexp.MustCompile(`(?i)(bearer\s+)[A-Za-z0-9._~+/=-]+`),
	regexp.MustCompile(`(?i)((?:x-goog-api-key|x-api-key|authorization)["']?\s*[:=]\s*["']?)[^\s"',}]+`),
	regexp.MustCompile(`\bsk-[A-Za-z0-9_-]{10,}`),
	regexp.MustCompile(`\bAIza[0-9A-Za-z_-]{20,}`),
}

func redact(s string) string {
	for _, re := range secretPatterns {
		if re.NumSubexp() > 0 {
			s = re.ReplaceAllString(s, "${1}[REDACTED]")
		} else {
			s = re.ReplaceAllString(s, "[REDACTED]")
		}
	}
	return s
}
