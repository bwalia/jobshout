// Package langfuse reports finished agent executions to Langfuse as OpenTelemetry
// spans, so the Go-native engine is observable alongside the Python sidecar.
//
// Why OTLP rather than the batch ingestion API: Langfuse 4.x defaults to
// LANGFUSE_MIGRATION_V4_WRITE_MODE=events_only, under which
// /api/public/ingestion accepts only score and log events — a trace-create
// there comes back 400 "Event type not accepted". /api/public/otel/v1/traces
// is the only write path for spans, and it speaks plain OTLP/HTTP+JSON, so
// this package is a small hand-rolled client rather than the full OTel SDK
// (which would add a dozen modules to emit one span per execution).
//
// Tracing is on only when host and both keys are configured; otherwise every
// method is a no-op and executions behave exactly as before — the same
// contract the sidecar's observability module offers, so a JobShout install
// never *requires* a Langfuse deployment.
package langfuse

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/jobshout/server/internal/model"
)

// queueDepth bounds the in-flight span buffer. Telemetry must never slow an
// execution down, so a full queue drops the span (with a warning) rather than
// blocking the caller.
const queueDepth = 256

// flushInterval is how long a partial batch waits for company before being
// sent. Executions arrive far apart at JobShout's volume, so this is what
// actually governs delivery latency.
const flushInterval = 2 * time.Second

// maxBatch caps spans per request so a burst cannot build an unbounded body.
const maxBatch = 64

// Client reports executions to Langfuse. The zero value is not usable; use New.
// A nil *Client is safe to call — every method no-ops — so callers can hold one
// unconditionally without nil checks at each site.
type Client struct {
	endpoint string
	auth     string
	env      string
	http     *http.Client
	logger   *zap.Logger

	queue chan span
	wg    sync.WaitGroup

	stopOnce sync.Once
	stop     chan struct{}
}

// New builds a Client posting to host's OTLP trace endpoint. It returns nil
// when host, publicKey or secretKey is empty, which callers treat as "tracing
// disabled" — every method on a nil *Client is a no-op.
func New(host, publicKey, secretKey, environment string, logger *zap.Logger) *Client {
	if host == "" || publicKey == "" || secretKey == "" {
		return nil
	}
	if environment == "" {
		environment = "default"
	}
	c := &Client{
		endpoint: strings.TrimRight(host, "/") + "/api/public/otel/v1/traces",
		auth:     "Basic " + base64.StdEncoding.EncodeToString([]byte(publicKey+":"+secretKey)),
		env:      environment,
		http:     &http.Client{Timeout: 10 * time.Second},
		logger:   logger,
		queue:    make(chan span, queueDepth),
		stop:     make(chan struct{}),
	}
	c.wg.Add(1)
	go c.run()
	logger.Info("Langfuse tracing enabled for go-native executions",
		zap.String("endpoint", c.endpoint), zap.String("environment", c.env))
	return c
}

// Enabled reports whether spans will actually be sent.
func (c *Client) Enabled() bool { return c != nil }

// RecordExecution queues one finished execution as a generation span. It never
// blocks and never returns an error: a failure to report telemetry must not
// change the outcome of the run that produced it.
func (c *Client) RecordExecution(exec *model.AgentExecution) {
	if c == nil || exec == nil || !tracedHere(exec.EngineType) {
		return
	}
	select {
	case c.queue <- newSpan(exec, c.env):
	default:
		c.logger.Warn("Langfuse span dropped: queue full",
			zap.String("execution_id", exec.ID.String()))
	}
}

// tracedHere reports whether this process should emit the span for an engine.
//
// LangChain and LangGraph runs execute in the Python sidecar, which already
// reports them through the Langfuse SDK. Emitting again from here would create
// a second, unrelated trace for the same run and double its tokens and cost in
// every dashboard that sums them — so the Go side traces only the engines the
// sidecar never sees. An empty engine type is go-native: that is the default
// the execution repository backfills for older rows.
func tracedHere(engineType string) bool {
	return engineType == "" || engineType == model.EngineGoNative
}

// Close flushes queued spans and stops the background worker. Safe to call
// more than once, and on a nil *Client.
func (c *Client) Close() {
	if c == nil {
		return
	}
	c.stopOnce.Do(func() { close(c.stop) })
	c.wg.Wait()
}

// run batches queued spans and posts them until Close.
func (c *Client) run() {
	defer c.wg.Done()
	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()

	batch := make([]span, 0, maxBatch)
	flush := func() {
		if len(batch) == 0 {
			return
		}
		c.post(batch)
		batch = batch[:0]
	}

	for {
		select {
		case s := <-c.queue:
			batch = append(batch, s)
			if len(batch) >= maxBatch {
				flush()
			}
		case <-ticker.C:
			flush()
		case <-c.stop:
			// Drain whatever is still queued so a shutdown does not lose the
			// tail of a run, then make a final flush.
			for {
				select {
				case s := <-c.queue:
					batch = append(batch, s)
					if len(batch) >= maxBatch {
						flush()
					}
					continue
				default:
				}
				break
			}
			flush()
			return
		}
	}
}

// post sends one batch, logging rather than propagating any failure.
func (c *Client) post(batch []span) {
	body, err := json.Marshal(payloadFor(batch))
	if err != nil {
		c.logger.Error("Langfuse payload encode failed", zap.Error(err))
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		c.logger.Error("Langfuse request build failed", zap.Error(err))
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", c.auth)

	resp, err := c.http.Do(req)
	if err != nil {
		c.logger.Warn("Langfuse export failed", zap.Error(err), zap.Int("spans", len(batch)))
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		c.logger.Warn("Langfuse export rejected",
			zap.Int("status", resp.StatusCode), zap.Int("spans", len(batch)))
	}
}

// span is the subset of an execution needed to build one OTLP span.
type span struct {
	traceID    string
	spanID     string
	name       string
	start      time.Time
	end        time.Time
	attrs      []attribute
	failed     bool
	errMessage string
}

// newSpan projects an execution onto the Langfuse span model.
//
// The execution id becomes both the trace id and the Langfuse session id, so a
// run is one trace and retries of the same execution group together — matching
// how the sidecar attributes its runs. The agent id becomes the Langfuse user
// id, which is what lets the dashboard slice cost by agent.
func newSpan(exec *model.AgentExecution, env string) span {
	provider, modelName := "", ""
	if exec.ModelProvider != nil {
		provider = *exec.ModelProvider
	}
	if exec.ModelName != nil {
		modelName = *exec.ModelName
	}

	start, end := spanWindow(exec)

	// One trace name per engine: an execution row with no engine type is
	// go-native (the repository's backfill default), and must not show up under
	// a second name in the dashboard's by-trace-name breakdown.
	engineType := exec.EngineType
	if engineType == "" {
		engineType = model.EngineGoNative
	}
	name := engineType + "-run"

	attrs := []attribute{
		str("langfuse.observation.type", "generation"),
		str("langfuse.trace.name", name),
		str("langfuse.session.id", exec.ID.String()),
		str("langfuse.user.id", exec.AgentID.String()),
		str("langfuse.environment", env),
		str("langfuse.observation.usage_details", usageJSON(exec)),
		str("langfuse.observation.cost_details", costJSON(exec)),
		integer("gen_ai.usage.input_tokens", int64(exec.InputTokens)),
		integer("gen_ai.usage.output_tokens", int64(exec.OutputTokens)),
	}
	if modelName != "" {
		attrs = append(attrs, str("gen_ai.request.model", modelName))
	}
	if provider != "" {
		attrs = append(attrs, str("gen_ai.system", provider))
	}
	if exec.InputPrompt != "" {
		attrs = append(attrs, str("langfuse.observation.input", exec.InputPrompt))
	}
	if exec.Output != nil && *exec.Output != "" {
		attrs = append(attrs, str("langfuse.observation.output", *exec.Output))
	}
	attrs = append(attrs,
		str("langfuse.metadata.engine_type", exec.EngineType),
		str("langfuse.metadata.org_id", exec.OrgID.String()),
		str("langfuse.metadata.execution_id", exec.ID.String()),
		integer("langfuse.metadata.iterations", int64(exec.Iterations)),
	)

	s := span{
		traceID: traceIDFor(exec),
		spanID:  spanIDFor(exec),
		name:    name,
		start:   start,
		end:     end,
		attrs:   attrs,
		failed:  exec.Status == model.ExecutionStatusFailed,
	}
	if exec.ErrorMessage != nil {
		s.errMessage = *exec.ErrorMessage
	}
	return s
}

// spanWindow picks the span's start and end, falling back to LatencyMs when
// the execution timestamps are absent. A zero-width span is rendered as a
// 1ms one so Langfuse does not show it as instantaneous.
func spanWindow(exec *model.AgentExecution) (time.Time, time.Time) {
	end := time.Now().UTC()
	if exec.CompletedAt != nil {
		end = exec.CompletedAt.UTC()
	}
	var start time.Time
	switch {
	case exec.StartedAt != nil:
		start = exec.StartedAt.UTC()
	case exec.LatencyMs > 0:
		start = end.Add(-time.Duration(exec.LatencyMs) * time.Millisecond)
	default:
		start = end.Add(-time.Millisecond)
	}
	if !start.Before(end) {
		start = end.Add(-time.Millisecond)
	}
	return start, end
}

// usageJSON renders the token counts in Langfuse's usage_details shape.
func usageJSON(exec *model.AgentExecution) string {
	total := exec.TotalTokens
	if total == 0 {
		total = exec.InputTokens + exec.OutputTokens
	}
	b, _ := json.Marshal(map[string]int{
		"input":  exec.InputTokens,
		"output": exec.OutputTokens,
		"total":  total,
	})
	return string(b)
}

// costJSON renders the computed cost in Langfuse's cost_details shape. The Go
// cost engine produces a single total, so only "total" is reported rather than
// inventing an input/output split Langfuse would then chart as fact.
func costJSON(exec *model.AgentExecution) string {
	b, _ := json.Marshal(map[string]float64{"total": exec.CostUSD})
	return string(b)
}

// traceIDFor uses the execution UUID's 16 bytes directly, which is exactly an
// OTLP trace id — so a trace id is stable across restarts and reconstructible
// from an execution row.
func traceIDFor(exec *model.AgentExecution) string {
	return hex.EncodeToString(exec.ID[:])
}

// spanIDFor derives 8 bytes from the execution id. Hashed rather than sliced
// from the UUID so the span id is not a visible prefix of the trace id.
func spanIDFor(exec *model.AgentExecution) string {
	sum := sha256.Sum256(exec.ID[:])
	return hex.EncodeToString(sum[:8])
}

// --- OTLP/HTTP JSON encoding -------------------------------------------------

type attribute struct {
	Key   string         `json:"key"`
	Value attributeValue `json:"value"`
}

type attributeValue struct {
	StringValue *string `json:"stringValue,omitempty"`
	IntValue    *string `json:"intValue,omitempty"`
}

func str(k, v string) attribute {
	return attribute{Key: k, Value: attributeValue{StringValue: &v}}
}

func integer(k string, v int64) attribute {
	s := strconv.FormatInt(v, 10)
	return attribute{Key: k, Value: attributeValue{IntValue: &s}}
}

type otlpPayload struct {
	ResourceSpans []resourceSpans `json:"resourceSpans"`
}

type resourceSpans struct {
	Resource   resource     `json:"resource"`
	ScopeSpans []scopeSpans `json:"scopeSpans"`
}

type resource struct {
	Attributes []attribute `json:"attributes"`
}

type scopeSpans struct {
	Scope scope      `json:"scope"`
	Spans []otlpSpan `json:"spans"`
}

type scope struct {
	Name string `json:"name"`
}

type otlpSpan struct {
	TraceID           string      `json:"traceId"`
	SpanID            string      `json:"spanId"`
	Name              string      `json:"name"`
	Kind              int         `json:"kind"`
	StartTimeUnixNano string      `json:"startTimeUnixNano"`
	EndTimeUnixNano   string      `json:"endTimeUnixNano"`
	Attributes        []attribute `json:"attributes"`
	Status            *otlpStatus `json:"status,omitempty"`
}

type otlpStatus struct {
	Code    int    `json:"code"`
	Message string `json:"message,omitempty"`
}

// payloadFor renders a batch as one OTLP resourceSpans document.
func payloadFor(batch []span) otlpPayload {
	spans := make([]otlpSpan, 0, len(batch))
	for _, s := range batch {
		o := otlpSpan{
			TraceID:           s.traceID,
			SpanID:            s.spanID,
			Name:              s.name,
			Kind:              1, // SPAN_KIND_INTERNAL
			StartTimeUnixNano: strconv.FormatInt(s.start.UnixNano(), 10),
			EndTimeUnixNano:   strconv.FormatInt(s.end.UnixNano(), 10),
			Attributes:        s.attrs,
		}
		if s.failed {
			o.Status = &otlpStatus{Code: 2, Message: s.errMessage} // STATUS_CODE_ERROR
		}
		spans = append(spans, o)
	}
	return otlpPayload{ResourceSpans: []resourceSpans{{
		Resource:   resource{Attributes: []attribute{str("service.name", "jobshout-api")}},
		ScopeSpans: []scopeSpans{{Scope: scope{Name: "jobshout/go-native"}, Spans: spans}},
	}}}
}
