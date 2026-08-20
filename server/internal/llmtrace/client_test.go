package llmtrace

import (
	"context"
	"errors"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/jobshout/server/internal/llm"
)

// fakeClient is the minimal llm.Client.
type fakeClient struct {
	resp *llm.GenerateResponse
	err  error
}

func (f *fakeClient) Generate(_ context.Context, _ llm.GenerateRequest) (*llm.GenerateResponse, error) {
	return f.resp, f.err
}
func (f *fakeClient) ProviderName() string { return "fake" }

// toolClient adds SupportsTools.
type toolClient struct {
	fakeClient
	supports bool
}

func (t *toolClient) SupportsTools() bool { return t.supports }

// listerClient adds ListModels.
type listerClient struct{ fakeClient }

func (l *listerClient) ListModels(_ context.Context) ([]llm.ModelInfo, error) {
	return []llm.ModelInfo{{Name: "listed-model"}}, nil
}

// toolListerClient implements both capabilities.
type toolListerClient struct{ listerClient }

func (t *toolListerClient) SupportsTools() bool { return true }

// newTestTracing returns an enabled Tracing recording into the returned
// SpanRecorder instead of exporting anywhere.
func newTestTracing(env string) (*Tracing, *tracetest.SpanRecorder) {
	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))
	return &Tracing{
		tp:          tp,
		tracer:      tp.Tracer("test"),
		environment: env,
		enabled:     true,
	}, rec
}

func attrMap(span sdktrace.ReadOnlySpan) map[attribute.Key]attribute.Value {
	m := make(map[attribute.Key]attribute.Value)
	for _, kv := range span.Attributes() {
		m[kv.Key] = kv.Value
	}
	return m
}

func TestGenerateRecordsAttributeContract(t *testing.T) {
	tracing, rec := newTestTracing("test-env")
	inner := &fakeClient{resp: &llm.GenerateResponse{
		Content:      "hello",
		Model:        "qwen3-coder",
		InputTokens:  10,
		OutputTokens: 20,
	}}
	client := tracing.Wrap(inner)

	ctx := WithTrace(context.Background(), TraceInfo{
		TraceName: "go-executor-run",
		SessionID: "exec-1",
		AgentID:   "agent-1",
		OrgID:     "org-1",
	})
	resp, err := client.Generate(ctx, llm.GenerateRequest{
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if resp.Content != "hello" {
		t.Fatalf("response altered by wrapper: %+v", resp)
	}

	spans := rec.Ended()
	if len(spans) != 1 {
		t.Fatalf("want 1 span, got %d", len(spans))
	}
	attrs := attrMap(spans[0])

	want := map[attribute.Key]string{
		"langfuse.observation.type":   "generation",
		"gen_ai.request.model":        "qwen3-coder",
		"langfuse.trace.name":         "go-executor-run",
		"session.id":                  "exec-1",
		"user.id":                     "agent-1",
		"langfuse.environment":        "test-env",
		"langfuse.observation.output": "hello",
	}
	for key, val := range want {
		got, ok := attrs[key]
		if !ok {
			t.Errorf("missing attribute %s", key)
			continue
		}
		if got.AsString() != val {
			t.Errorf("%s = %q, want %q", key, got.AsString(), val)
		}
	}
	if got := attrs["gen_ai.usage.input_tokens"].AsInt64(); got != 10 {
		t.Errorf("input_tokens = %d, want 10", got)
	}
	if got := attrs["gen_ai.usage.output_tokens"].AsInt64(); got != 20 {
		t.Errorf("output_tokens = %d, want 20", got)
	}
	tags := attrs["langfuse.trace.tags"].AsStringSlice()
	wantTags := []string{"fake", "qwen3-coder", "org:org-1"}
	if len(tags) != len(wantTags) {
		t.Fatalf("tags = %v, want %v", tags, wantTags)
	}
	for i := range wantTags {
		if tags[i] != wantTags[i] {
			t.Errorf("tags[%d] = %q, want %q", i, tags[i], wantTags[i])
		}
	}
	input := attrs["langfuse.observation.input"].AsString()
	if !strings.Contains(input, `"content":"hi"`) {
		t.Errorf("input attribute does not carry the messages: %s", input)
	}
}

func TestGenerateErrorPath(t *testing.T) {
	tracing, rec := newTestTracing("")
	innerErr := errors.New("model exploded")
	client := tracing.Wrap(&fakeClient{err: innerErr})

	_, err := client.Generate(context.Background(), llm.GenerateRequest{Model: "llama3"})
	if !errors.Is(err, innerErr) {
		t.Fatalf("error not passed through: %v", err)
	}

	spans := rec.Ended()
	if len(spans) != 1 {
		t.Fatalf("want 1 span, got %d", len(spans))
	}
	attrs := attrMap(spans[0])
	if got := attrs["langfuse.observation.level"].AsString(); got != "ERROR" {
		t.Errorf("level = %q, want ERROR", got)
	}
	if got := attrs["langfuse.observation.status_message"].AsString(); got != "model exploded" {
		t.Errorf("status_message = %q", got)
	}
	if spans[0].Status().Code != codes.Error {
		t.Errorf("span status = %v, want Error", spans[0].Status().Code)
	}
}

func TestGenerateFallbackLabeling(t *testing.T) {
	tracing, rec := newTestTracing("")
	client := tracing.Wrap(&fakeClient{resp: &llm.GenerateResponse{Content: "x"}})

	// No TraceInfo on the ctx, no model anywhere.
	if _, err := client.Generate(context.Background(), llm.GenerateRequest{}); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	attrs := attrMap(rec.Ended()[0])
	if got := attrs["langfuse.trace.name"].AsString(); got != "go-llm" {
		t.Errorf("trace name = %q, want go-llm", got)
	}
	if got := attrs["gen_ai.request.model"].AsString(); got != "unknown" {
		t.Errorf("model = %q, want unknown", got)
	}
	for _, key := range []attribute.Key{"session.id", "user.id", "langfuse.environment"} {
		if _, ok := attrs[key]; ok {
			t.Errorf("attribute %s should be absent", key)
		}
	}
}

// TestWrapPreservesCapabilities proves that the wrapper presents exactly the
// optional interfaces its inner client does — otherwise the executor silently
// falls back to ReAct and model discovery to the static list.
func TestWrapPreservesCapabilities(t *testing.T) {
	tracing, _ := newTestTracing("")

	cases := []struct {
		name   string
		client llm.Client
	}{
		{"plain", &fakeClient{}},
		{"tools", &toolClient{supports: true}},
		{"tools-off", &toolClient{supports: false}},
		{"lister", &listerClient{}},
		{"tools+lister", &toolListerClient{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			wrapped := tracing.Wrap(tc.client)

			// SupportsTools semantics: assert-then-call must agree with the
			// unwrapped client (executor.go's clientSupportsTools pattern).
			innerOK := false
			if c, ok := tc.client.(llm.ToolCapableClient); ok {
				innerOK = c.SupportsTools()
			}
			wrappedOK := false
			if c, ok := wrapped.(llm.ToolCapableClient); ok {
				wrappedOK = c.SupportsTools()
			}
			if innerOK != wrappedOK {
				t.Errorf("SupportsTools: inner %v, wrapped %v", innerOK, wrappedOK)
			}

			// ModelLister presence must match exactly.
			_, innerLister := tc.client.(llm.ModelLister)
			_, wrappedLister := wrapped.(llm.ModelLister)
			if innerLister != wrappedLister {
				t.Errorf("ModelLister presence: inner %v, wrapped %v", innerLister, wrappedLister)
			}
			if wrappedLister {
				models, err := wrapped.(llm.ModelLister).ListModels(context.Background())
				if err != nil || len(models) != 1 || models[0].Name != "listed-model" {
					t.Errorf("ListModels not forwarded: %v %v", models, err)
				}
			}
		})
	}
}

func TestDisabledWrapIsIdentity(t *testing.T) {
	inner := &fakeClient{}
	if got := (&Tracing{}).Wrap(inner); got != llm.Client(inner) {
		t.Fatalf("disabled Wrap returned a new client: %T", got)
	}
	var nilTracing *Tracing
	if got := nilTracing.Wrap(inner); got != llm.Client(inner) {
		t.Fatalf("nil Wrap returned a new client: %T", got)
	}
}
