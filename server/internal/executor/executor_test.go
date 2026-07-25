package executor

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/llm"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/tools"
)

// fakeToolClient is a tool-capable llm.Client that scripts one tool call
// followed by a final answer, and records the requests it received.
type fakeToolClient struct {
	calls    int
	requests []llm.GenerateRequest
}

func (f *fakeToolClient) ProviderName() string { return "fake" }
func (f *fakeToolClient) SupportsTools() bool  { return true }

func (f *fakeToolClient) Generate(_ context.Context, req llm.GenerateRequest) (*llm.GenerateResponse, error) {
	f.requests = append(f.requests, req)
	f.calls++
	if f.calls == 1 {
		// First turn: request the echo tool.
		return &llm.GenerateResponse{
			ToolCalls: []llm.ToolCall{{
				ID:        "call_1",
				Name:      "echo",
				Arguments: map[string]any{"text": "hello"},
			}},
			InputTokens:  10,
			OutputTokens: 5,
		}, nil
	}
	// Second turn: final answer.
	return &llm.GenerateResponse{
		Content:      "final answer",
		InputTokens:  7,
		OutputTokens: 3,
	}, nil
}

// echoTool is a minimal schema-advertising tool that records its input.
type echoTool struct{ lastInput map[string]any }

func (t *echoTool) Name() string        { return "echo" }
func (t *echoTool) Description() string { return "echo the text back" }
func (t *echoTool) Parameters() tools.ParameterSchema {
	return tools.ObjectSchema(map[string]any{
		"text": map[string]any{"type": "string"},
	}, "text")
}
func (t *echoTool) Execute(_ context.Context, input map[string]any) (string, error) {
	t.lastInput = input
	return "echoed: " + input["text"].(string), nil
}

func TestExecutor_NativeToolCallingPath(t *testing.T) {
	client := &fakeToolClient{}
	router := llm.NewTestRouter("fake", map[string]llm.Client{"fake": client})

	registry := tools.NewRegistry()
	tool := &echoTool{}
	registry.Register(tool)

	exec := New(router, registry, zap.NewNop())

	agent := &model.Agent{
		ID:            uuid.New(),
		Name:          "Tester",
		Role:          "assistant",
		ModelProvider: strPtr("fake"),
		ModelName:     strPtr("fake-model"),
	}

	res := exec.Run(context.Background(), uuid.New(), agent, "do the thing", []string{"echo"})

	if res.Err != nil {
		t.Fatalf("unexpected error: %v", res.Err)
	}
	if res.FinalAnswer != "final answer" {
		t.Fatalf("final answer: got %q", res.FinalAnswer)
	}
	if client.calls != 2 {
		t.Fatalf("expected 2 model turns, got %d", client.calls)
	}

	// The first request must carry native tool definitions (not a ReAct prompt).
	if len(client.requests[0].ToolDefs) != 1 || client.requests[0].ToolDefs[0].Name != "echo" {
		t.Fatalf("first request missing ToolDefs: %#v", client.requests[0].ToolDefs)
	}

	// The tool must have executed with the model-supplied arguments.
	if tool.lastInput["text"] != "hello" {
		t.Fatalf("tool not executed with model args: %#v", tool.lastInput)
	}

	// The tool call must be metered in the result, and tokens accumulated.
	if len(res.ToolCalls) != 1 || res.ToolCalls[0].ToolName != "echo" {
		t.Fatalf("tool call not recorded: %#v", res.ToolCalls)
	}
	if res.ToolCalls[0].Output != "echoed: hello" {
		t.Fatalf("tool output not recorded: %q", res.ToolCalls[0].Output)
	}
	if res.TotalTokens != 25 || res.InputTokens != 17 || res.OutputTokens != 8 {
		t.Fatalf("token metering wrong: total=%d in=%d out=%d", res.TotalTokens, res.InputTokens, res.OutputTokens)
	}

	// The follow-up request must echo the assistant tool call + the tool result.
	second := client.requests[1]
	var sawAssistantCall, sawToolResult bool
	for _, m := range second.Messages {
		if m.Role == llm.RoleAssistant && len(m.ToolCalls) == 1 && m.ToolCalls[0].ID == "call_1" {
			sawAssistantCall = true
		}
		if m.Role == llm.RoleTool && m.ToolCallID == "call_1" {
			sawToolResult = true
		}
	}
	if !sawAssistantCall || !sawToolResult {
		t.Fatalf("follow-up conversation missing tool turns: %#v", second.Messages)
	}
}

// TestExecutor_FallsBackToReActWithoutSchema verifies that when a tool lacks a
// schema the native path is skipped: buildToolDefs returns ok=false.
func TestExecutor_FallsBackToReActWithoutSchema(t *testing.T) {
	registry := tools.NewRegistry()
	registry.Register(&noSchemaTool{})
	if _, ok := buildToolDefs(registry); ok {
		t.Fatalf("expected buildToolDefs to report ok=false when a tool lacks a schema")
	}
}

type noSchemaTool struct{}

func (t *noSchemaTool) Name() string        { return "noschema" }
func (t *noSchemaTool) Description() string { return "no schema" }
func (t *noSchemaTool) Execute(_ context.Context, _ map[string]any) (string, error) {
	return "", nil
}

func TestClientSupportsTools(t *testing.T) {
	if !clientSupportsTools(&fakeToolClient{}) {
		t.Fatal("fakeToolClient should be tool-capable")
	}
	if clientSupportsTools(&plainClient{}) {
		t.Fatal("plainClient should not be tool-capable")
	}
}

// plainClient implements llm.Client but not ToolCapableClient.
type plainClient struct{}

func (plainClient) ProviderName() string { return "plain" }
func (plainClient) Generate(_ context.Context, _ llm.GenerateRequest) (*llm.GenerateResponse, error) {
	return &llm.GenerateResponse{}, nil
}

// ── Knowledge auto-injection ────────────────────────────────────────────────

// fakeEmbedder returns a fixed vector per text, or an error, and records the
// texts it was asked to embed.
type fakeEmbedder struct {
	vec  []float32
	err  error
	seen []string
}

func (f *fakeEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	f.seen = append(f.seen, texts...)
	if f.err != nil {
		return nil, f.err
	}
	out := make([][]float32, len(texts))
	for i := range texts {
		out[i] = f.vec
	}
	return out, nil
}

// fakeSearcher returns canned chunks (or an error) and records its arguments.
type fakeSearcher struct {
	chunks   []model.KnowledgeChunk
	err      error
	gotAgent uuid.UUID
	gotK     int
	calls    int
}

func (f *fakeSearcher) SearchByAgent(_ context.Context, agentID uuid.UUID, _ []float32, k int) ([]model.KnowledgeChunk, error) {
	f.calls++
	f.gotAgent = agentID
	f.gotK = k
	return f.chunks, f.err
}

// firstUserKnowledge finds the leading knowledge context message in a request's
// message list, if present.
func firstUserKnowledge(msgs []llm.Message) (string, bool) {
	for _, m := range msgs {
		if m.Role == llm.RoleUser && strings.HasPrefix(m.Content, "Relevant knowledge:") {
			return m.Content, true
		}
	}
	return "", false
}

func knowledgeAgent() *model.Agent {
	return &model.Agent{
		ID:            uuid.New(),
		Name:          "Tester",
		Role:          "assistant",
		ModelProvider: strPtr("fake"),
		ModelName:     strPtr("fake-model"),
	}
}

// simpleClient is a tool-capable client that always returns a final answer on
// the first turn (no tool calls), so a run is a single turn.
type simpleClient struct {
	requests []llm.GenerateRequest
}

func (c *simpleClient) ProviderName() string { return "fake" }
func (c *simpleClient) SupportsTools() bool  { return true }
func (c *simpleClient) Generate(_ context.Context, req llm.GenerateRequest) (*llm.GenerateResponse, error) {
	c.requests = append(c.requests, req)
	return &llm.GenerateResponse{Content: "final answer", InputTokens: 1, OutputTokens: 1}, nil
}

func TestExecutor_InjectsKnowledge_NativePath(t *testing.T) {
	client := &simpleClient{}
	router := llm.NewTestRouter("fake", map[string]llm.Client{"fake": client})

	registry := tools.NewRegistry()
	registry.Register(&echoTool{}) // schema-advertising => native path taken

	searcher := &fakeSearcher{chunks: []model.KnowledgeChunk{
		{Content: "Refunds are processed within 5 business days."},
		{Content: "Sale items are final and cannot be returned."},
	}}
	embedder := &fakeEmbedder{vec: []float32{0.1, 0.2}}

	exec := New(router, registry, zap.NewNop()).WithKnowledge(searcher, embedder)
	agent := knowledgeAgent()

	res := exec.Run(context.Background(), uuid.New(), agent, "explain the refund policy", []string{"echo"})
	if res.Err != nil {
		t.Fatalf("unexpected error: %v", res.Err)
	}

	if searcher.calls != 1 {
		t.Fatalf("expected 1 search, got %d", searcher.calls)
	}
	if searcher.gotAgent != agent.ID {
		t.Fatalf("searcher got agent %v, want %v", searcher.gotAgent, agent.ID)
	}
	if searcher.gotK != knowledgeTopK {
		t.Fatalf("searcher got k=%d, want %d", searcher.gotK, knowledgeTopK)
	}
	// The task prompt (not a tool query) must have been embedded.
	if len(embedder.seen) != 1 || embedder.seen[0] != "explain the refund policy" {
		t.Fatalf("embedder should embed the task prompt, saw %#v", embedder.seen)
	}

	if len(client.requests) == 0 {
		t.Fatal("client received no requests")
	}
	msg, ok := firstUserKnowledge(client.requests[0].Messages)
	if !ok {
		t.Fatalf("no knowledge message injected: %#v", client.requests[0].Messages)
	}
	if !strings.Contains(msg, "Refunds are processed within 5 business days.") ||
		!strings.Contains(msg, "Sale items are final") {
		t.Fatalf("knowledge message missing chunk content: %q", msg)
	}
	// It must precede the task message.
	var kIdx, tIdx = -1, -1
	for i, m := range client.requests[0].Messages {
		if m.Role == llm.RoleUser && strings.HasPrefix(m.Content, "Relevant knowledge:") {
			kIdx = i
		}
		if m.Role == llm.RoleUser && strings.HasPrefix(m.Content, "Task:") {
			tIdx = i
		}
	}
	if kIdx == -1 || tIdx == -1 || kIdx >= tIdx {
		t.Fatalf("knowledge message must precede task message (k=%d t=%d)", kIdx, tIdx)
	}
}

func TestExecutor_NoKnowledgeConfigured_NoInjection(t *testing.T) {
	client := &simpleClient{}
	router := llm.NewTestRouter("fake", map[string]llm.Client{"fake": client})
	registry := tools.NewRegistry()
	registry.Register(&echoTool{})

	// No WithKnowledge call: retrieval must be skipped entirely.
	exec := New(router, registry, zap.NewNop())
	res := exec.Run(context.Background(), uuid.New(), knowledgeAgent(), "do it", []string{"echo"})
	if res.Err != nil {
		t.Fatalf("unexpected error: %v", res.Err)
	}
	if _, ok := firstUserKnowledge(client.requests[0].Messages); ok {
		t.Fatal("no knowledge should be injected when none is configured")
	}
}

func TestExecutor_KnowledgeRetrievalIsBestEffort(t *testing.T) {
	client := &simpleClient{}
	router := llm.NewTestRouter("fake", map[string]llm.Client{"fake": client})
	registry := tools.NewRegistry()
	registry.Register(&echoTool{})

	// Searcher fails: the run must still succeed, with no knowledge injected.
	searcher := &fakeSearcher{err: errBoom}
	embedder := &fakeEmbedder{vec: []float32{0.1}}
	exec := New(router, registry, zap.NewNop()).WithKnowledge(searcher, embedder)

	res := exec.Run(context.Background(), uuid.New(), knowledgeAgent(), "do it", []string{"echo"})
	if res.Err != nil {
		t.Fatalf("retrieval failure must not break the run: %v", res.Err)
	}
	if res.FinalAnswer != "final answer" {
		t.Fatalf("final answer: got %q", res.FinalAnswer)
	}
	if _, ok := firstUserKnowledge(client.requests[0].Messages); ok {
		t.Fatal("no knowledge should be injected when retrieval fails")
	}
}

func TestExecutor_InjectsKnowledge_ReActPath(t *testing.T) {
	// A ReAct client (not tool-capable) that returns a final answer immediately.
	client := &reactFinalClient{}
	router := llm.NewTestRouter("fake", map[string]llm.Client{"fake": client})
	registry := tools.NewRegistry() // no tools => ReAct path

	searcher := &fakeSearcher{chunks: []model.KnowledgeChunk{{Content: "The API rate limit is 100 req/min."}}}
	embedder := &fakeEmbedder{vec: []float32{0.5}}
	exec := New(router, registry, zap.NewNop()).WithKnowledge(searcher, embedder)

	res := exec.Run(context.Background(), uuid.New(), knowledgeAgent(), "what is the rate limit?", nil)
	if res.Err != nil {
		t.Fatalf("unexpected error: %v", res.Err)
	}
	if len(client.requests) == 0 {
		t.Fatal("client received no requests")
	}
	msg, ok := firstUserKnowledge(client.requests[0].Messages)
	if !ok {
		t.Fatalf("no knowledge message injected on ReAct path: %#v", client.requests[0].Messages)
	}
	if !strings.Contains(msg, "The API rate limit is 100 req/min.") {
		t.Fatalf("knowledge message missing content: %q", msg)
	}
}

var errBoom = fmt.Errorf("boom")

// reactFinalClient is a plain (non-tool-capable) client returning ReAct JSON
// with a final answer on the first turn.
type reactFinalClient struct {
	requests []llm.GenerateRequest
}

func (c *reactFinalClient) ProviderName() string { return "fake" }
func (c *reactFinalClient) Generate(_ context.Context, req llm.GenerateRequest) (*llm.GenerateResponse, error) {
	c.requests = append(c.requests, req)
	return &llm.GenerateResponse{
		Content:      `{"thought":"done","action":null,"action_input":null,"final_answer":"ok"}`,
		InputTokens:  1,
		OutputTokens: 1,
	}, nil
}
