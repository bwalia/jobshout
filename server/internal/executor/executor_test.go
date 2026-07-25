package executor

import (
	"context"
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
