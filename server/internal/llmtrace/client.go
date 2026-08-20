package llmtrace

import (
	"context"
	"encoding/json"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/jobshout/server/internal/llm"
)

// Wrap decorates c so every Generate call is exported to Langfuse as one
// trace holding one GENERATION observation. With tracing disabled it returns
// c itself, so the wrapped and unwrapped paths are byte-identical.
//
// Callers discover optional capabilities by type assertion — the executor
// asserts llm.ToolCapableClient, model discovery asserts llm.ModelLister —
// so the wrapper must present exactly the interfaces its inner client does.
// SupportsTools can always be present (asserting-then-calling it delegates
// faithfully either way), but ModelLister's mere presence switches discovery
// from the static list to live probing, so it is only offered when the inner
// client implements it.
func (t *Tracing) Wrap(c llm.Client) llm.Client {
	if !t.Enabled() {
		return c
	}
	tc := &tracedClient{inner: c, tracer: t.tracer, environment: t.environment}
	if _, ok := c.(llm.ModelLister); ok {
		return &tracedListerClient{tc}
	}
	return tc
}

type tracedClient struct {
	inner       llm.Client
	tracer      trace.Tracer
	environment string
}

// tracedListerClient additionally forwards ListModels; chosen by Wrap only
// when the inner client is an llm.ModelLister.
type tracedListerClient struct{ *tracedClient }

func (c *tracedListerClient) ListModels(ctx context.Context) ([]llm.ModelInfo, error) {
	return c.inner.(llm.ModelLister).ListModels(ctx)
}

func (c *tracedClient) ProviderName() string { return c.inner.ProviderName() }

// SupportsTools reports the inner client's capability; false when the inner
// client does not implement llm.ToolCapableClient, which matches what a
// caller's failed type assertion on the unwrapped client would conclude.
func (c *tracedClient) SupportsTools() bool {
	tc, ok := c.inner.(llm.ToolCapableClient)
	return ok && tc.SupportsTools()
}

func (c *tracedClient) Generate(ctx context.Context, req llm.GenerateRequest) (*llm.GenerateResponse, error) {
	info, _ := FromContext(ctx)
	if info.TraceName == "" {
		info.TraceName = "go-llm"
	}

	ctx, span := c.tracer.Start(ctx, "llm.generate", trace.WithSpanKind(trace.SpanKindClient))
	resp, err := c.inner.Generate(ctx, req)
	c.record(span, info, req, resp, err)
	span.End()
	return resp, err
}

// record stamps the span with Langfuse's attribute contract. The keys are the
// ones Langfuse's OTLP ingest maps onto trace and observation fields; the
// dashboard's widgets group observations by providedModelName, userId and
// traceName, so those three must be present on the generation span itself.
func (c *tracedClient) record(span trace.Span, info TraceInfo, req llm.GenerateRequest, resp *llm.GenerateResponse, err error) {
	model := req.Model
	if resp != nil && resp.Model != "" {
		model = resp.Model
	}
	if model == "" {
		model = "unknown"
	}
	provider := c.inner.ProviderName()

	tags := []string{provider, model}
	if info.OrgID != "" {
		tags = append(tags, "org:"+info.OrgID)
	}

	attrs := []attribute.KeyValue{
		attribute.String("langfuse.observation.type", "generation"),
		attribute.String("gen_ai.request.model", model),
		attribute.String("langfuse.trace.name", info.TraceName),
		attribute.StringSlice("langfuse.trace.tags", tags),
	}
	if info.SessionID != "" {
		attrs = append(attrs, attribute.String("session.id", info.SessionID))
	}
	if info.AgentID != "" {
		attrs = append(attrs, attribute.String("user.id", info.AgentID))
	}
	if c.environment != "" {
		attrs = append(attrs, attribute.String("langfuse.environment", c.environment))
	}
	if input, merr := json.Marshal(req.Messages); merr == nil {
		attrs = append(attrs, attribute.String("langfuse.observation.input", string(input)))
	}
	if resp != nil {
		attrs = append(attrs,
			attribute.Int("gen_ai.usage.input_tokens", resp.InputTokens),
			attribute.Int("gen_ai.usage.output_tokens", resp.OutputTokens),
		)
		output := resp.Content
		if len(resp.ToolCalls) > 0 {
			if calls, merr := json.Marshal(resp.ToolCalls); merr == nil {
				output += "\n[tool_calls] " + string(calls)
			}
		}
		attrs = append(attrs, attribute.String("langfuse.observation.output", output))
	}
	span.SetAttributes(attrs...)

	if err != nil {
		span.SetAttributes(
			attribute.String("langfuse.observation.level", "ERROR"),
			attribute.String("langfuse.observation.status_message", err.Error()),
		)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
}
