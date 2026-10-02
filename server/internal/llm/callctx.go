package llm

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Per-call labels carried on ctx. They are read by decorators installed with
// Router.WrapClients (the LLM benchmark recorder) and never change what a
// call does: an agent marks which step it is in, and the decorator records it.

type stageKey struct{}
type attemptKey struct{}
type callStatsKey struct{}

// WithStage labels every LLM call made under ctx as belonging to step, e.g.
// "research", "outline" or "write". Labelling only — it changes nothing about
// the call.
func WithStage(ctx context.Context, step string) context.Context {
	return context.WithValue(ctx, stageKey{}, step)
}

// StageFrom returns the step label on ctx, or "" when none was set.
func StageFrom(ctx context.Context) string {
	s, _ := ctx.Value(stageKey{}).(string)
	return s
}

// withAttempt marks the calls under ctx as the n-th try at the same request
// (GenerateJSON's corrective retry is attempt 2).
func withAttempt(ctx context.Context, n int) context.Context {
	return context.WithValue(ctx, attemptKey{}, n)
}

// AttemptFrom returns the attempt number on ctx; 1 when none was set.
func AttemptFrom(ctx context.Context) int {
	if n, ok := ctx.Value(attemptKey{}).(int); ok && n > 0 {
		return n
	}
	return 1
}

// CallStats collects what a provider did inside one Generate call that the
// caller cannot see from the outside: every HTTP attempt it made (so retries
// stay visible as individual API calls), the time those took, a request ID
// from response headers, and what a reply reported even when the call then
// failed.
type CallStats struct {
	retries  atomic.Int32
	apiNanos atomic.Int64
	apiTimed atomic.Bool

	mu        sync.Mutex
	requestID string
	attempts  []APIAttempt
	reply     *ReplyInfo
}

// APIAttempt is one HTTP request actually sent to a provider.
type APIAttempt struct {
	StartedAt  time.Time `json:"started_at"`
	DurationMs int       `json:"duration_ms"`
	// HTTPStatus is 0 when no response arrived (connection error, timeout).
	HTTPStatus int    `json:"http_status,omitempty"`
	RequestID  string `json:"request_id,omitempty"`
	// Error is the transport or provider error for this attempt; the
	// recorder redacts it before storing.
	Error string `json:"error,omitempty"`
}

// ReplyInfo is what a decoded provider reply reported. Providers note it as
// soon as a reply is decoded, so a call that fails afterwards (a blocked or
// thinking-only reply) still records the model, request ID and usage it had.
type ReplyInfo struct {
	Model     string
	RequestID string
	Usage     Usage
}

// Retries is how many times the request was re-sent after a transient
// failure; 0 when the first try settled it.
func (s *CallStats) Retries() int { return int(s.retries.Load()) }

// APIDuration is the time spent sending requests to the provider and reading
// its replies, summed over every attempt. It leaves out retry backoff waits
// and request building. ok is false when the provider client does not time
// its HTTP exchanges (a stub, or a client outside this package).
func (s *CallStats) APIDuration() (d time.Duration, ok bool) {
	return time.Duration(s.apiNanos.Load()), s.apiTimed.Load()
}

// RequestID is the provider request ID read from the last response's headers;
// "" when there was none.
func (s *CallStats) RequestID() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.requestID
}

// WithCallStats attaches a fresh CallStats to ctx for one Generate call.
func WithCallStats(ctx context.Context) (context.Context, *CallStats) {
	s := &CallStats{}
	return context.WithValue(ctx, callStatsKey{}, s), s
}

func statsFrom(ctx context.Context) *CallStats {
	s, _ := ctx.Value(callStatsKey{}).(*CallStats)
	return s
}

// noteRetry counts one re-send on the CallStats on ctx, if any.
func noteRetry(ctx context.Context) {
	if s := statsFrom(ctx); s != nil {
		s.retries.Add(1)
	}
}

// Attempts returns every HTTP request sent during the call, in order.
func (s *CallStats) Attempts() []APIAttempt {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]APIAttempt(nil), s.attempts...)
}

// Reply returns what the last decoded reply reported, or nil.
func (s *CallStats) Reply() *ReplyInfo {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.reply == nil {
		return nil
	}
	r := *s.reply
	return &r
}

// apiAttempt times one HTTP exchange with the provider: from sending the
// request to having read the reply. It excludes request building and any
// retry backoff.
type apiAttempt struct {
	stats *CallStats
	start time.Time
	once  sync.Once
	rec   APIAttempt
}

// beginAttempt starts one HTTP attempt. It is a no-op handle when ctx carries
// no CallStats.
func beginAttempt(ctx context.Context) *apiAttempt {
	return &apiAttempt{stats: statsFrom(ctx), start: time.Now()}
}

// response records the status and request ID header of the reply. OpenAI
// sends x-request-id and Anthropic request-id; a failed attempt keeps it too,
// which is what a provider's support asks for.
func (a *apiAttempt) response(resp *http.Response) {
	if a.stats == nil || resp == nil {
		return
	}
	a.rec.HTTPStatus = resp.StatusCode
	id := strings.TrimSpace(firstNonEmpty(resp.Header.Get("x-request-id"), resp.Header.Get("request-id")))
	if id == "" {
		return
	}
	a.rec.RequestID = id
	a.stats.mu.Lock()
	a.stats.requestID = id
	a.stats.mu.Unlock()
}

// end closes the attempt, adding it and its duration to the CallStats. Only
// the first call counts, so it is safe both deferred and called explicitly.
func (a *apiAttempt) end(err error) {
	if a.stats == nil {
		return
	}
	a.once.Do(func() {
		d := time.Since(a.start)
		a.rec.StartedAt = a.start.UTC()
		a.rec.DurationMs = int(d.Milliseconds())
		if err != nil {
			a.rec.Error = err.Error()
		}
		a.stats.apiNanos.Add(int64(d))
		a.stats.apiTimed.Store(true)
		a.stats.mu.Lock()
		a.stats.attempts = append(a.stats.attempts, a.rec)
		a.stats.mu.Unlock()
	})
}

// noteReply records what a decoded reply reported, before the provider
// decides whether the reply is usable.
func noteReply(ctx context.Context, model, requestID string, u Usage) {
	s := statsFrom(ctx)
	if s == nil {
		return
	}
	s.mu.Lock()
	s.reply = &ReplyInfo{Model: model, RequestID: requestID, Usage: u}
	s.mu.Unlock()
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
