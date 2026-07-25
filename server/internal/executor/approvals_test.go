package executor

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/llm"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/tools"
)

// reactScriptClient is a plain (non-tool-capable) client that returns a scripted
// sequence of ReAct JSON responses, one per turn, recording the requests it saw.
type reactScriptClient struct {
	turns    []string
	call     int
	requests []llm.GenerateRequest
}

func (c *reactScriptClient) ProviderName() string { return "fake" }

func (c *reactScriptClient) Generate(_ context.Context, req llm.GenerateRequest) (*llm.GenerateResponse, error) {
	c.requests = append(c.requests, req)
	idx := c.call
	if idx >= len(c.turns) {
		idx = len(c.turns) - 1
	}
	content := c.turns[idx]
	c.call++
	return &llm.GenerateResponse{Content: content, InputTokens: 2, OutputTokens: 3}, nil
}

// recordingTool records every input it is executed with and returns a canned
// output, so tests can assert whether (and with what) it ran.
type recordingTool struct {
	name   string
	output string
	inputs []map[string]any
}

func (t *recordingTool) Name() string        { return t.name }
func (t *recordingTool) Description() string { return "records inputs" }
func (t *recordingTool) Execute(_ context.Context, input map[string]any) (string, error) {
	t.inputs = append(t.inputs, input)
	return t.output, nil
}

// fakeGate is a scriptable ApprovalGate. gated is the set of tool names that
// require approval; every CreatePending call is captured for assertions.
type fakeGate struct {
	gated     map[string]bool
	created   []gateCall
	nextID    uuid.UUID
	createErr error
}

type gateCall struct {
	execID, agentID, orgID uuid.UUID
	toolName               string
	toolInput              map[string]any
	resumeState            []byte
}

func (g *fakeGate) RequiresApproval(_ uuid.UUID, toolName string) bool {
	return g.gated[toolName]
}

func (g *fakeGate) CreatePending(_ context.Context, execID, agentID, orgID uuid.UUID, toolName string, toolInput map[string]any, resumeState []byte) (uuid.UUID, error) {
	if g.createErr != nil {
		return uuid.Nil, g.createErr
	}
	g.created = append(g.created, gateCall{execID, agentID, orgID, toolName, toolInput, resumeState})
	if g.nextID == uuid.Nil {
		g.nextID = uuid.New()
	}
	return g.nextID, nil
}

func gatedAgent() *model.Agent {
	return &model.Agent{
		ID:            uuid.New(),
		OrgID:         uuid.New(),
		Name:          "Gated",
		Role:          "assistant",
		ModelProvider: strPtr("fake"),
		ModelName:     strPtr("fake-model"),
	}
}

// actionTurn builds a ReAct JSON turn requesting a tool call.
func actionTurn(tool string, input map[string]any) string {
	b, _ := json.Marshal(map[string]any{
		"thought":      "need a tool",
		"action":       tool,
		"action_input": input,
	})
	return string(b)
}

// finalTurn builds a ReAct JSON turn with a final answer.
func finalTurn(answer string) string {
	b, _ := json.Marshal(map[string]any{
		"thought":      "done",
		"final_answer": answer,
	})
	return string(b)
}

// TestExecutor_GateNotConfigured_RunsNormally verifies the default-off contract:
// with no gate, a gated-looking tool still runs and the run completes.
func TestExecutor_GateNotConfigured_RunsNormally(t *testing.T) {
	client := &reactScriptClient{turns: []string{
		actionTurn("danger", map[string]any{"x": 1}),
		finalTurn("all done"),
	}}
	router := llm.NewTestRouter("fake", map[string]llm.Client{"fake": client})
	registry := tools.NewRegistry()
	tool := &recordingTool{name: "danger", output: "ran"}
	registry.Register(tool)

	exec := New(router, registry, zap.NewNop()) // no gate
	res := exec.Run(context.Background(), uuid.New(), gatedAgent(), "go", []string{"danger"})

	if res.Err != nil {
		t.Fatalf("unexpected error: %v", res.Err)
	}
	if res.Status != "" {
		t.Fatalf("expected empty status, got %q", res.Status)
	}
	if res.FinalAnswer != "all done" {
		t.Fatalf("final answer: got %q", res.FinalAnswer)
	}
	if len(tool.inputs) != 1 {
		t.Fatalf("tool should have executed once, got %d", len(tool.inputs))
	}
}

// TestExecutor_PausesOnGatedTool covers path (a): a run that hits a gated tool
// pauses with Status=awaiting_approval, does NOT execute the tool, and persists a
// resume_state that round-trips to the paused conversation.
func TestExecutor_PausesOnGatedTool(t *testing.T) {
	client := &reactScriptClient{turns: []string{
		actionTurn("danger", map[string]any{"target": "prod"}),
		finalTurn("should not reach here"),
	}}
	router := llm.NewTestRouter("fake", map[string]llm.Client{"fake": client})
	registry := tools.NewRegistry()
	tool := &recordingTool{name: "danger", output: "ran"}
	registry.Register(tool)

	gate := &fakeGate{gated: map[string]bool{"danger": true}}
	exec := New(router, registry, zap.NewNop()).WithApprovalGate(gate)

	agent := gatedAgent()
	execID := uuid.New()
	res := exec.Run(context.Background(), execID, agent, "go", []string{"danger"})

	if res.Err != nil {
		t.Fatalf("unexpected error: %v", res.Err)
	}
	if res.Status != StatusAwaitingApproval {
		t.Fatalf("expected status %q, got %q", StatusAwaitingApproval, res.Status)
	}
	if res.ApprovalID == uuid.Nil {
		t.Fatal("expected a non-nil approval id")
	}
	if len(tool.inputs) != 0 {
		t.Fatalf("gated tool must NOT execute before approval, ran %d times", len(tool.inputs))
	}
	if len(gate.created) != 1 {
		t.Fatalf("expected 1 pending approval, got %d", len(gate.created))
	}
	gc := gate.created[0]
	if gc.execID != execID || gc.agentID != agent.ID || gc.orgID != agent.OrgID {
		t.Fatalf("pending approval carries wrong ids: %+v", gc)
	}
	if gc.toolName != "danger" || gc.toolInput["target"] != "prod" {
		t.Fatalf("pending approval carries wrong tool/input: %+v", gc)
	}

	// The resume state must decode and carry the pending tool plus the assistant
	// turn that requested it.
	var rs resumableState
	if err := json.Unmarshal(gc.resumeState, &rs); err != nil {
		t.Fatalf("resume state does not decode: %v", err)
	}
	if rs.PendingTool != "danger" || rs.PendingInput["target"] != "prod" {
		t.Fatalf("resume state missing pending tool: %+v", rs)
	}
	if rs.Iteration != 1 {
		t.Fatalf("resume state iteration: got %d want 1", rs.Iteration)
	}
	if len(rs.AgentTools) != 1 || rs.AgentTools[0] != "danger" {
		t.Fatalf("resume state agent tools: %+v", rs.AgentTools)
	}
	// Only two model turns must have happened before the pause (request + would-be
	// second turn is never reached).
	if client.call != 1 {
		t.Fatalf("expected exactly 1 model turn before pause, got %d", client.call)
	}
}

// TestExecutor_ResumeOnApprove covers path (b): Resume on an approved decision
// executes the pending tool and drives the loop to completion.
func TestExecutor_ResumeOnApprove(t *testing.T) {
	// Turn 1 requested the tool (already consumed at pause time). On resume the
	// executor runs the tool then calls Generate again; that next turn (index 1)
	// is the final answer.
	client := &reactScriptClient{turns: []string{
		actionTurn("danger", map[string]any{"target": "prod"}),
		finalTurn("completed after approval"),
	}}
	router := llm.NewTestRouter("fake", map[string]llm.Client{"fake": client})
	registry := tools.NewRegistry()
	tool := &recordingTool{name: "danger", output: "tool-output-42"}
	registry.Register(tool)

	gate := &fakeGate{gated: map[string]bool{"danger": true}}
	exec := New(router, registry, zap.NewNop()).WithApprovalGate(gate)

	agent := gatedAgent()
	execID := uuid.New()
	paused := exec.Run(context.Background(), execID, agent, "go", []string{"danger"})
	if paused.Status != StatusAwaitingApproval {
		t.Fatalf("setup: expected pause, got status %q err %v", paused.Status, paused.Err)
	}

	approval := &model.Approval{
		ID:          paused.ApprovalID,
		OrgID:       agent.OrgID,
		ExecutionID: execID,
		AgentID:     agent.ID,
		ToolName:    "danger",
		Status:      model.ApprovalStatusApproved,
		ResumeState: gate.created[0].resumeState,
	}

	res := exec.Resume(context.Background(), approval)
	if res.Err != nil {
		t.Fatalf("resume returned error: %v", res.Err)
	}
	if res.Status != "" {
		t.Fatalf("resumed run should complete (empty status), got %q", res.Status)
	}
	if res.FinalAnswer != "completed after approval" {
		t.Fatalf("final answer: got %q", res.FinalAnswer)
	}
	if len(tool.inputs) != 1 || tool.inputs[0]["target"] != "prod" {
		t.Fatalf("approved tool must execute once with original input, got %#v", tool.inputs)
	}
	// The tool call must be metered on the resumed result.
	var found bool
	for _, tc := range res.ToolCalls {
		if tc.ToolName == "danger" && tc.Output == "tool-output-42" {
			found = true
		}
	}
	if !found {
		t.Fatalf("resumed result missing the executed tool call: %#v", res.ToolCalls)
	}
	// The follow-up turn must have seen the tool result in the conversation.
	last := client.requests[len(client.requests)-1]
	var sawResult bool
	for _, m := range last.Messages {
		if strings.Contains(m.Content, "tool-output-42") {
			sawResult = true
		}
	}
	if !sawResult {
		t.Fatal("resumed conversation did not feed the tool output back to the model")
	}
}

// TestExecutor_ResumeOnReject covers path (c): Resume on a rejected decision does
// NOT execute the tool and feeds the rejection back so the loop continues.
func TestExecutor_ResumeOnReject(t *testing.T) {
	client := &reactScriptClient{turns: []string{
		actionTurn("danger", map[string]any{"target": "prod"}),
		finalTurn("acknowledged rejection"),
	}}
	router := llm.NewTestRouter("fake", map[string]llm.Client{"fake": client})
	registry := tools.NewRegistry()
	tool := &recordingTool{name: "danger", output: "should-not-run"}
	registry.Register(tool)

	gate := &fakeGate{gated: map[string]bool{"danger": true}}
	exec := New(router, registry, zap.NewNop()).WithApprovalGate(gate)

	agent := gatedAgent()
	execID := uuid.New()
	paused := exec.Run(context.Background(), execID, agent, "go", []string{"danger"})
	if paused.Status != StatusAwaitingApproval {
		t.Fatalf("setup: expected pause, got status %q", paused.Status)
	}

	reason := "too risky for prod"
	approval := &model.Approval{
		ID:          paused.ApprovalID,
		OrgID:       agent.OrgID,
		ExecutionID: execID,
		AgentID:     agent.ID,
		ToolName:    "danger",
		Status:      model.ApprovalStatusRejected,
		Reason:      &reason,
		DeciderName: "Alice",
		ResumeState: gate.created[0].resumeState,
	}

	res := exec.Resume(context.Background(), approval)
	if res.Err != nil {
		t.Fatalf("resume returned error: %v", res.Err)
	}
	if res.FinalAnswer != "acknowledged rejection" {
		t.Fatalf("final answer: got %q", res.FinalAnswer)
	}
	if len(tool.inputs) != 0 {
		t.Fatalf("rejected tool must NOT execute, ran %d times", len(tool.inputs))
	}
	// The rejection message (with the decider name and reason) must have been fed
	// back to the model on the continuation turn.
	last := client.requests[len(client.requests)-1]
	var sawRejection bool
	for _, m := range last.Messages {
		if strings.Contains(m.Content, "rejected by Alice") && strings.Contains(m.Content, reason) {
			sawRejection = true
		}
	}
	if !sawRejection {
		t.Fatalf("resumed conversation did not feed the rejection back: %#v", last.Messages)
	}
}

// TestExecutor_NativePathGatedToolFailsClearly verifies the documented native-path
// fallback: a gated tool on the native tool-calling path fails with a clear error
// rather than silently bypassing the gate.
func TestExecutor_NativePathGatedToolFailsClearly(t *testing.T) {
	client := &fakeToolClient{} // tool-capable => native path, scripts an "echo" call
	router := llm.NewTestRouter("fake", map[string]llm.Client{"fake": client})
	registry := tools.NewRegistry()
	tool := &echoTool{}
	registry.Register(tool)

	gate := &fakeGate{gated: map[string]bool{"echo": true}}
	exec := New(router, registry, zap.NewNop()).WithApprovalGate(gate)

	res := exec.Run(context.Background(), uuid.New(), gatedAgent(), "do it", []string{"echo"})
	if res.Err == nil {
		t.Fatal("expected an error for a gated tool on the native path")
	}
	if !strings.Contains(res.Err.Error(), "native tool-calling path") {
		t.Fatalf("error should explain the native-path limitation, got: %v", res.Err)
	}
	if tool.lastInput != nil {
		t.Fatal("gated tool must not execute on the native path either")
	}
}
