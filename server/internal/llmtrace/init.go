package llmtrace

import (
	"context"
	"encoding/base64"
	"strings"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/config"
)

// attrValueLimit caps any single span attribute (notably the prompt and
// completion) at 32 KiB. Real prompts fit comfortably; a runaway one is
// truncated by the SDK rather than ballooning export payloads.
const attrValueLimit = 32 * 1024

// Tracing owns the OTel tracer provider that exports to Langfuse. A disabled
// Tracing is fully usable: Wrap returns the client unchanged and Shutdown is
// a no-op, so callers never branch on whether tracing is on.
type Tracing struct {
	tp          *sdktrace.TracerProvider
	tracer      trace.Tracer
	environment string
	enabled     bool
}

// Init builds the Langfuse exporter from config. Tracing is on only when the
// host and both API keys are set — the same rule as the python-sidecar's
// observability.enabled(). Any construction error degrades to disabled with a
// warning; telemetry never fails startup.
//
// The transport is OTLP over HTTP: the deployed Langfuse (v4) rejects the
// legacy /api/public/ingestion events, so /api/public/otel is the supported
// ingest path for a language without a first-party SDK. The
// x-langfuse-ingestion-version header keeps ingestion on the realtime path.
func Init(cfg *config.Config, logger *zap.Logger) *Tracing {
	if cfg.LangfuseHost == "" || cfg.LangfusePublicKey == "" || cfg.LangfuseSecretKey == "" {
		logger.Info("LLM tracing disabled (Langfuse host or keys not configured)")
		return &Tracing{}
	}

	auth := base64.StdEncoding.EncodeToString(
		[]byte(cfg.LangfusePublicKey + ":" + cfg.LangfuseSecretKey))
	exp, err := otlptracehttp.New(context.Background(),
		// WithEndpointURL takes the path verbatim (unlike WithEndpoint, which
		// appends /v1/traces) and infers plain HTTP from the scheme.
		otlptracehttp.WithEndpointURL(strings.TrimRight(cfg.LangfuseHost, "/")+"/api/public/otel/v1/traces"),
		otlptracehttp.WithHeaders(map[string]string{
			"Authorization":                "Basic " + auth,
			"x-langfuse-ingestion-version": "4",
		}),
	)
	if err != nil {
		logger.Warn("LLM tracing disabled: could not build Langfuse exporter", zap.Error(err))
		return &Tracing{}
	}

	limits := sdktrace.NewSpanLimits()
	limits.AttributeValueLengthLimit = attrValueLimit

	// The provider stays private — otel.SetTracerProvider is deliberately not
	// called, so no other library starts emitting spans to Langfuse.
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithRawSpanLimits(limits),
		sdktrace.WithResource(resource.NewSchemaless(
			attribute.String("service.name", "jobshout-server"),
		)),
	)

	return &Tracing{
		tp:          tp,
		tracer:      tp.Tracer("jobshout/llmtrace"),
		environment: cfg.LangfuseEnvironment,
		enabled:     true,
	}
}

// Enabled reports whether spans are being exported.
func (t *Tracing) Enabled() bool { return t != nil && t.enabled }

// Shutdown flushes queued spans and stops the exporter. Called once during
// graceful shutdown so the tail of the last run is not lost.
func (t *Tracing) Shutdown(ctx context.Context) error {
	if t == nil || !t.enabled {
		return nil
	}
	return t.tp.Shutdown(ctx)
}
