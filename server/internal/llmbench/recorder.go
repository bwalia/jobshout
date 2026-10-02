// Package llmbench records one row per text LLM call for the LLM benchmark.
//
// It is a decorator installed with llm.Router.WrapClients (and on chat's
// separately built client), so every call that passes through the common LLM
// layer is measured the same way whichever provider or agent made it. Nothing
// here is provider- or agent-specific: the provider is the concrete client
// that ran, the model, usage and request ID come from the provider's reply,
// and the run, task and step from labels already on ctx.
//
// Nothing is guessed. A token count, total or request ID the provider did not
// report is stored as NULL, and the model the provider reported is kept apart
// from the one requested.
//
// Granularity: one row is one logical Generate call — what the agent asked
// for. Every HTTP request actually sent to the provider for it (the first try
// and each transport retry) is kept on that row as an attempt: api_attempts
// counts them and metadata.api_attempts lists each with its start, duration,
// HTTP status, request ID and error. GenerateJSON's corrective retry and
// chat's fallback model are separate Generate calls, so they are rows of their
// own (attempt = 2 / a different model).
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
	StatusFailed    = "failed"
	StatusCancelled = "cancelled"
)

// maxErrorLen caps the stored error text.
const maxErrorLen = 1024

// maxModelLen matches usage_records.model and requested_model.
const maxModelLen = 200

// maxRequestIDLen matches usage_records.provider_request_id.
const maxRequestIDLen = 200

// Record is one LLM call.
type Record struct {
	OrgID       *uuid.UUID
	AgentID     *uuid.UUID
	TaskID      *uuid.UUID
	TaskRunID   *uuid.UUID
	ExecutionID *uuid.UUID
	RunKind     string
	RunID       string
	Stage       string
	Attempt     int
	// Provider is the concrete client that made the call.
	Provider string
	// Model is the model the provider's reply named when it named one
	// (ModelReported), else the model that was sent.
	Model         string
	ModelReported bool
	// RequestedModel is the model sent to the provider: the caller's, or the
	// client's default when the caller named none.
	RequestedModel string
	// Token counts are nil when the provider did not report them. TokensTotal
	// is the provider's own total, never a sum computed here.
	TokensIn        *int
	TokensOut       *int
	TokensTotal     *int
	TokensReasoning *int
	// ProviderRequestID is the provider's ID for the call; "" when none.
	ProviderRequestID string
	// LatencyMs is the whole Generate call, retries and backoff included.
	// APIDurationMs is only the time spent in HTTP exchanges with the
	// provider; nil when the client does not measure it.
	LatencyMs     int
	APIDurationMs *int
	// APIAttempts is how many HTTP requests were actually sent; Attempts
	// lists them (errors redacted). Both are empty when the client does not
	// report its HTTP exchanges.
	APIAttempts int
	Attempts    []llm.APIAttempt
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
	c.rec.enqueue(buildRecord(ctx, c.inner.ProviderName(), c.requestedModel(req), resp, err, stats, start, end))
	return resp, err
}

// requestedModel is the model the call asks for: the request's, else the
// client's default when it can say (llm.ModelNamed). A successful reply
// overrides it with the model the client actually sent.
func (c *benchClient) requestedModel(req llm.GenerateRequest) string {
	if m := strings.TrimSpace(req.Model); m != "" {
		return m
	}
	if mn, ok := c.inner.(llm.ModelNamed); ok {
		return mn.ModelName()
	}
	return ""
}

// buildRecord turns one finished call into a Record. ctx is the caller's, so
// a cancellation that arrived during the call is visible here.
func buildRecord(
	ctx context.Context, provider, requested string,
	resp *llm.GenerateResponse, err error, stats *llm.CallStats, start, end time.Time,
) Record {
	info, _ := llmtrace.FromContext(ctx)
	rec := Record{
		OrgID:       parseID(info.OrgID),
		AgentID:     parseID(info.AgentID),
		TaskID:      parseID(info.TaskID),
		TaskRunID:   parseID(info.TaskRunID),
		ExecutionID: parseID(info.ExecutionID),
		RunKind:     runKind(info),
		RunID:       info.SessionID,
		Stage:       llm.StageFrom(ctx),
		Attempt:     llm.AttemptFrom(ctx),
		Provider:    provider,
		LatencyMs:   int(end.Sub(start).Milliseconds()),
		Status:      StatusSuccess,
		StartedAt:   start.UTC(),
		CompletedAt: end.UTC(),
	}
	var reply *llm.ReplyInfo
	if stats != nil {
		rec.Retries = stats.Retries()
		if d, ok := stats.APIDuration(); ok {
			ms := int(d.Milliseconds())
			rec.APIDurationMs = &ms
		}
		rec.ProviderRequestID = stats.RequestID()
		rec.Attempts = stats.Attempts()
		rec.APIAttempts = len(rec.Attempts)
		for i := range rec.Attempts {
			rec.Attempts[i].Error = truncate(redact(rec.Attempts[i].Error), maxErrorLen)
		}
		reply = stats.Reply()
	}
	// A failed call whose reply was decoded before it was rejected (blocked,
	// thinking-only, an error body) still records what that reply reported.
	if resp == nil && reply != nil {
		resp = &llm.GenerateResponse{ProviderModel: reply.Model, RequestID: reply.RequestID, Usage: reply.Usage}
	}
	if resp != nil {
		if resp.Model != "" {
			requested = resp.Model
		}
		if resp.ProviderModel != "" {
			rec.Model = truncate(resp.ProviderModel, maxModelLen)
			rec.ModelReported = true
		}
		if resp.RequestID != "" {
			rec.ProviderRequestID = resp.RequestID
		}
		if resp.Usage.Reported() {
			rec.TokensIn = resp.Usage.InputTokens
			rec.TokensOut = resp.Usage.OutputTokens
			rec.TokensTotal = resp.Usage.TotalTokens
			rec.TokensReasoning = resp.Usage.ReasoningTokens
		} else {
			// A client outside package llm that fills only the legacy int
			// fields: a 0 there means "not reported" (a real prompt or reply
			// is never 0 tokens), so it is stored as unknown.
			rec.TokensIn = positive(resp.InputTokens)
			rec.TokensOut = positive(resp.OutputTokens)
		}
	}
	rec.RequestedModel = truncate(requested, maxModelLen)
	if rec.Model == "" {
		rec.Model = rec.RequestedModel
	}
	rec.ProviderRequestID = truncate(rec.ProviderRequestID, maxRequestIDLen)
	if err != nil {
		rec.Status = StatusFailed
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
