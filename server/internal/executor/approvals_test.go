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

// nativeScriptClient is a tool-capable client that returns scripted native
// turns, recording the requests it saw.
type nativeScriptClient struct {
	turns    []llm.GenerateResponse
	call     int
	requests []llm.GenerateRequest
}

func (c *nativeScriptClient) ProviderName() string { return "fake" }
func (c *nativeScriptClient) SupportsTools() bool  { return true }

func (c *nativeScriptClient) Generate(_ context.Context, req llm.GenerateRequest) (*llm.GenerateResponse, error) {
	c.requests = append(c.requests, req)
	idx := c.call
	if idx >= len(c.turns) {
		idx = len(c.turns) - 1
	}
	c.call++
	out := c.turns[idx]
	return &out, nil
}

// schemaTool is a schema-advertising (native-path) tool that records inputs.
type schemaTool struct {
	name   string
	output string
	inputs []map[string]any
}

func (t *schemaTool) Name() string        { return t.name }
func (t *schemaTool) Description() string { return "records inputs" }
func (t *schemaTool) Parameters() tools.ParameterSchema {
	return tools.ObjectSchema(map[string]any{"target": map[string]any{"type": "string"}})
}
func (t *schemaTool) Execute(_ context.Context, input map[string]any) (string, error) {
	t.inputs = append(t.inputs, input)
	return t.output, nil
}

func approvalFor(paused Result, agent *model.Agent, execID uuid.UUID, gc gateCall, status string) *model.Approval {
	return &model.Approval{
		ID:          paused.ApprovalID,
		OrgID:       agent.OrgID,
		ExecutionID: execID,
		AgentID:     agent.ID,
		ToolName:    gc.toolName,
		Status:      status,
		DeciderName: "Alice",
		ResumeState: gc.resumeState,
	}
}

// toolMessageFor returns the RoleTool message answering a tool call id.
func toolMessageFor(msgs []llm.Message, id string) (llm.Message, bool) {
	for _, m := range msgs {
		if m.Role == llm.RoleTool && m.ToolCallID == id {
			return m, true
		}
	}
	return llm.Message{}, false
}

// TestExecutor_NativePath_PauseAndResume covers the native tool-calling path end
// to end: one model turn asks for an ungated tool, then a gated one, then
// another ungated one. The run must execute the first, pause on the gated one
// (running neither it nor the third), and on resume either execute (approve) or
// refuse (reject) the gated call, then finish the rest of the turn and the run.
func TestExecutor_NativePath_PauseAndResume(t *testing.T) {
	cases := []struct {
		name           string
		decision       string
		wantDangerRuns int
		wantGatedReply string
	}{
		{"approve executes the gated tool", model.ApprovalStatusApproved, 1, "danger-output"},
		{"reject feeds the rejection back", model.ApprovalStatusRejected, 0, "Action rejected by Alice"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := &nativeScriptClient{turns: []llm.GenerateResponse{
				{ToolCalls: []llm.ToolCall{
					{ID: "c1", Name: "safe", Arguments: map[string]any{"target": "a"}},
					{ID: "c2", Name: "danger", Arguments: map[string]any{"target": "prod"}},
					{ID: "c3", Name: "safe", Arguments: map[string]any{"target": "b"}},
				}, InputTokens: 10, OutputTokens: 5},
				{Content: "native done", InputTokens: 7, OutputTokens: 3},
			}}
			router := llm.NewTestRouter("fake", map[string]llm.Client{"fake": client})
			registry := tools.NewRegistry()
			safe := &schemaTool{name: "safe", output: "safe-output"}
			danger := &schemaTool{name: "danger", output: "danger-output"}
			registry.Register(safe)
			registry.Register(danger)

			gate := &fakeGate{gated: map[string]bool{"danger": true}}
			exec := New(router, registry, zap.NewNop()).WithApprovalGate(gate)
			agent := gatedAgent()
			execID := uuid.New()

			paused := exec.Run(context.Background(), execID, agent, "go", []string{"safe", "danger"})
			if paused.Err != nil {
				t.Fatalf("native run should pause, not fail: %v", paused.Err)
			}
			if paused.Status != StatusAwaitingApproval || paused.ApprovalID == uuid.Nil {
				t.Fatalf("expected awaiting_approval with an approval id, got %q / %s", paused.Status, paused.ApprovalID)
			}
			if len(danger.inputs) != 0 {
				t.Fatal("gated tool must not run before approval")
			}
			if len(safe.inputs) != 1 || safe.inputs[0]["target"] != "a" {
				t.Fatalf("only the call before the gated one may run before the pause, got %#v", safe.inputs)
			}
			if len(paused.ToolCalls) != 1 || paused.TotalTokens != 15 {
				t.Fatalf("paused metering: calls=%d tokens=%d", len(paused.ToolCalls), paused.TotalTokens)
			}

			var rs resumableState
			if err := json.Unmarshal(gate.created[0].resumeState, &rs); err != nil {
				t.Fatalf("resume state does not decode: %v", err)
			}
			if rs.Mode != loopModeNative || rs.PendingToolCallID != "c2" || len(rs.RemainingToolCalls) != 1 {
				t.Fatalf("native resume state incomplete: mode=%q pending=%q remaining=%d",
					rs.Mode, rs.PendingToolCallID, len(rs.RemainingToolCalls))
			}

			res := exec.Resume(context.Background(), approvalFor(paused, agent, execID, gate.created[0], tc.decision))
			if res.Err != nil || res.Status != "" {
				t.Fatalf("resumed run should complete: status=%q err=%v", res.Status, res.Err)
			}
			if res.FinalAnswer != "native done" {
				t.Fatalf("final answer: %q", res.FinalAnswer)
			}
			if len(danger.inputs) != tc.wantDangerRuns {
				t.Fatalf("gated tool ran %d times, want %d", len(danger.inputs), tc.wantDangerRuns)
			}
			if len(safe.inputs) != 2 || safe.inputs[1]["target"] != "b" {
				t.Fatalf("the rest of the paused turn must run after resume, got %#v", safe.inputs)
			}
			if res.TotalTokens != 25 {
				t.Fatalf("tokens must be cumulative across the pause: got %d want 25", res.TotalTokens)
			}
			if res.PriorToolCalls != 1 {
				t.Fatalf("PriorToolCalls: got %d want 1", res.PriorToolCalls)
			}

			// The follow-up request must answer every tool call of the paused
			// turn, by id, with the assistant turn's tool calls preserved.
			last := client.requests[len(client.requests)-1]
			for _, id := range []string{"c1", "c2", "c3"} {
				if _, ok := toolMessageFor(last.Messages, id); !ok {
					t.Fatalf("no tool result for call %s in the resumed conversation", id)
				}
			}
			gm, _ := toolMessageFor(last.Messages, "c2")
			if !strings.Contains(gm.Content, tc.wantGatedReply) {
				t.Fatalf("gated call result %q should contain %q", gm.Content, tc.wantGatedReply)
			}
			var sawAssistantCalls bool
			for _, m := range last.Messages {
				if m.Role == llm.RoleAssistant && len(m.ToolCalls) == 3 {
					sawAssistantCalls = true
				}
			}
			if !sawAssistantCalls {
				t.Fatal("assistant tool_use turn lost its tool calls across the pause")
			}
			if len(last.ToolDefs) == 0 {
				t.Fatal("resumed native turn must still send tool definitions")
			}
		})
	}
}

// TestExecutor_NativePath_SecondGateInSameTurn pauses twice in one model turn:
// resuming the first approval runs into the second gated call, which must pause
// again (not fail) with its own resume state.
func TestExecutor_NativePath_SecondGateInSameTurn(t *testing.T) {
	client := &nativeScriptClient{turns: []llm.GenerateResponse{
		{ToolCalls: []llm.ToolCall{
			{ID: "c1", Name: "danger", Arguments: map[string]any{"target": "one"}},
			{ID: "c2", Name: "danger", Arguments: map[string]any{"target": "two"}},
		}},
		{Content: "both done"},
	}}
	router := llm.NewTestRouter("fake", map[string]llm.Client{"fake": client})
	registry := tools.NewRegistry()
	danger := &schemaTool{name: "danger", output: "ok"}
	registry.Register(danger)
	gate := &fakeGate{gated: map[string]bool{"danger": true}}
	exec := New(router, registry, zap.NewNop()).WithApprovalGate(gate)
	agent := gatedAgent()
	execID := uuid.New()

	first := exec.Run(context.Background(), execID, agent, "go", []string{"danger"})
	if first.Status != StatusAwaitingApproval {
		t.Fatalf("expected first pause, got %q (%v)", first.Status, first.Err)
	}
	gate.nextID = uuid.New()
	second := exec.Resume(context.Background(), approvalFor(first, agent, execID, gate.created[0], model.ApprovalStatusApproved))
	if second.Status != StatusAwaitingApproval || second.Err != nil {
		t.Fatalf("expected a second pause, got %q (%v)", second.Status, second.Err)
	}
	if len(gate.created) != 2 || gate.created[1].toolInput["target"] != "two" {
		t.Fatalf("second pending approval wrong: %#v", gate.created)
	}
	if second.PriorToolCalls != 0 || len(second.ToolCalls) != 1 {
		t.Fatalf("second pause metering: prior=%d calls=%d", second.PriorToolCalls, len(second.ToolCalls))
	}
	done := exec.Resume(context.Background(), approvalFor(second, agent, execID, gate.created[1], model.ApprovalStatusApproved))
	if done.Err != nil || done.FinalAnswer != "both done" {
		t.Fatalf("final resume: %q (%v)", done.FinalAnswer, done.Err)
	}
	if len(danger.inputs) != 2 || done.PriorToolCalls != 1 || len(done.ToolCalls) != 2 {
		t.Fatalf("runs=%d prior=%d calls=%d", len(danger.inputs), done.PriorToolCalls, len(done.ToolCalls))
	}
}

// TestExecutor_Resume_LatencyExcludesHumanWait: the resumed result's latency is
// the run's compute time before the pause plus after it, not the wall time the
// approval sat waiting for a human.
func TestExecutor_Resume_LatencyExcludesHumanWait(t *testing.T) {
	client := &reactScriptClient{turns: []string{finalTurn("done")}}
	router := llm.NewTestRouter("fake", map[string]llm.Client{"fake": client})
	registry := tools.NewRegistry()
	registry.Register(&recordingTool{name: "danger", output: "ran"})
	exec := New(router, registry, zap.NewNop())

	state, _ := json.Marshal(resumableState{
		Provider:    "fake",
		ModelName:   "fake-model",
		AgentTools:  []string{"danger"},
		Messages:    []persistedMessage{{Role: llm.RoleUser, Content: "go"}},
		Iteration:   1,
		PendingTool: "danger",
		ElapsedMs:   4000,
	})
	res := exec.Resume(context.Background(), &model.Approval{
		ID: uuid.New(), ExecutionID: uuid.New(), AgentID: uuid.New(), OrgID: uuid.New(),
		Status: model.ApprovalStatusApproved, ResumeState: state,
	})
	if res.Err != nil {
		t.Fatalf("resume: %v", res.Err)
	}
	if res.LatencyMs < 4000 || res.LatencyMs > 60000 {
		t.Fatalf("latency should carry the 4s before the pause and little else, got %dms", res.LatencyMs)
	}
}

// TestExecutor_Resume_LegacyReActState: a resume state written before the
// native path could pause (no mode, messages with only role+content) still
// resumes on the ReAct loop.
func TestExecutor_Resume_LegacyReActState(t *testing.T) {
	client := &reactScriptClient{turns: []string{finalTurn("legacy ok")}}
	router := llm.NewTestRouter("fake", map[string]llm.Client{"fake": client})
	registry := tools.NewRegistry()
	tool := &recordingTool{name: "danger", output: "ran"}
	registry.Register(tool)
	exec := New(router, registry, zap.NewNop())

	legacy := []byte(`{"provider":"fake","model_name":"m","agent_tools":["danger"],
		"messages":[{"role":"system","content":"sys"},{"role":"user","content":"go"}],
		"iteration":1,"input_tokens":1,"output_tokens":1,"total_tokens":2,
		"tool_calls":[{"tool_name":"x","input":{},"output":"o","duration_ms":1}],
		"pending_tool":"danger","pending_input":{"k":"v"}}`)
	res := exec.Resume(context.Background(), &model.Approval{
		ID: uuid.New(), ExecutionID: uuid.New(), AgentID: uuid.New(), OrgID: uuid.New(),
		Status: model.ApprovalStatusApproved, ResumeState: legacy,
	})
	if res.Err != nil || res.FinalAnswer != "legacy ok" {
		t.Fatalf("legacy resume: %q (%v)", res.FinalAnswer, res.Err)
	}
	if len(tool.inputs) != 1 || tool.inputs[0]["k"] != "v" {
		t.Fatalf("legacy pending tool should run with its input, got %#v", tool.inputs)
	}
	if res.PriorToolCalls != 1 || len(res.ToolCalls) != 2 {
		t.Fatalf("prior=%d calls=%d", res.PriorToolCalls, len(res.ToolCalls))
	}
}
