package executor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/chatstream"
	"github.com/jobshout/server/internal/llm"
	"github.com/jobshout/server/internal/llmtrace"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/tools"
)

// ApprovalGate is the narrow human-in-the-loop hook the executor consults before
// running a tool. It is satisfied by a service; the executor depends only on this
// interface so it never imports the service or repository packages. When no gate
// is configured, every run behaves exactly as before (default-off).
type ApprovalGate interface {
	// RequiresApproval reports whether the given tool needs a human decision
	// before this agent may execute it. It must be cheap and best-effort:
	// returning false on any internal error keeps the gate additive and safe.
	RequiresApproval(agentID uuid.UUID, toolName string) bool

	// CreatePending records a paused execution awaiting a human decision and
	// returns the new approval's ID. resumeState is the opaque serialised
	// executor state that Resume later reconstructs.
	CreatePending(
		ctx context.Context,
		execID, agentID, orgID uuid.UUID,
		toolName string,
		toolInput map[string]any,
		resumeState []byte,
	) (uuid.UUID, error)
}

// Loop modes recorded in a resume state, so Resume re-enters the loop that paused.
const (
	loopModeReAct  = "react"
	loopModeNative = "native"
)

// reactLoopState is the mutable state threaded through both agent loops (ReAct
// and native tool-calling). It is the single source of truth for both a fresh
// Run and a resumed run, and it is what gets serialised (via resumableState)
// when a run pauses on a gated tool.
type reactLoopState struct {
	client       llm.Client
	modelName    string
	provider     string
	toolRegistry *tools.Registry
	// toolDefs is set only on the native tool-calling path; nil means ReAct.
	toolDefs     []llm.ToolDef
	agentID      uuid.UUID
	orgID        uuid.UUID
	execID       uuid.UUID
	agentTools   []string
	messages     []llm.Message
	iteration    int // last completed iteration; the loop starts at iteration+1
	toolCalls    []ToolCallRecord
	inputTokens  int
	outputTokens int
	totalTokens  int
	runStart     time.Time
	log          *zap.Logger
}

func (st *reactLoopState) mode() string {
	if st.toolDefs != nil {
		return loopModeNative
	}
	return loopModeReAct
}

// resumableState is the JSON-serialised snapshot persisted with a pending
// approval. Messages are stored as persistedMessage so the native path's
// ToolCalls/ToolCallID (which llm.Message hides from JSON) survive the round
// trip; a state written before those fields existed still decodes, as ReAct.
type resumableState struct {
	Mode         string              `json:"mode,omitempty"` // "" or "react" = ReAct, "native"
	Provider     string              `json:"provider"`
	ModelName    string              `json:"model_name"`
	AgentTools   []string            `json:"agent_tools"`
	Messages     []persistedMessage  `json:"messages"`
	Iteration    int                 `json:"iteration"`
	InputTokens  int                 `json:"input_tokens"`
	OutputTokens int                 `json:"output_tokens"`
	TotalTokens  int                 `json:"total_tokens"`
	ToolCalls    []persistedToolCall `json:"tool_calls"`
	PendingTool  string              `json:"pending_tool"`
	PendingInput map[string]any      `json:"pending_input"`
	// ElapsedMs is the run's compute time before the pause, so the resumed
	// run's latency covers the whole run but not the time a human took.
	ElapsedMs int `json:"elapsed_ms,omitempty"`

	// Native path only: the provider's id for the paused tool call (its result
	// must answer that id), and the calls of the same model turn that come
	// after it and have not run yet.
	PendingToolCallID  string                `json:"pending_tool_call_id,omitempty"`
	RemainingToolCalls []persistedNativeCall `json:"remaining_tool_calls,omitempty"`
}

// persistedMessage is the JSON-safe form of llm.Message.
type persistedMessage struct {
	Role       string                `json:"role"`
	Content    string                `json:"content"`
	ToolCalls  []persistedNativeCall `json:"tool_calls,omitempty"`
	ToolCallID string                `json:"tool_call_id,omitempty"`
}

// persistedNativeCall is the JSON-safe form of llm.ToolCall.
type persistedNativeCall struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

func toPersistedCalls(in []llm.ToolCall) []persistedNativeCall {
	if len(in) == 0 {
		return nil
	}
	out := make([]persistedNativeCall, 0, len(in))
	for _, c := range in {
		out = append(out, persistedNativeCall{ID: c.ID, Name: c.Name, Arguments: c.Arguments})
	}
	return out
}

func fromPersistedCalls(in []persistedNativeCall) []llm.ToolCall {
	if len(in) == 0 {
		return nil
	}
	out := make([]llm.ToolCall, 0, len(in))
	for _, c := range in {
		out = append(out, llm.ToolCall{ID: c.ID, Name: c.Name, Arguments: c.Arguments})
	}
	return out
}

func toPersistedMessages(in []llm.Message) []persistedMessage {
	out := make([]persistedMessage, 0, len(in))
	for _, m := range in {
		out = append(out, persistedMessage{
			Role:       m.Role,
			Content:    m.Content,
			ToolCalls:  toPersistedCalls(m.ToolCalls),
			ToolCallID: m.ToolCallID,
		})
	}
	return out
}

func fromPersistedMessages(in []persistedMessage) []llm.Message {
	out := make([]llm.Message, 0, len(in))
	for _, m := range in {
		out = append(out, llm.Message{
			Role:       m.Role,
			Content:    m.Content,
			ToolCalls:  fromPersistedCalls(m.ToolCalls),
			ToolCallID: m.ToolCallID,
		})
	}
	return out
}

// persistedToolCall is the JSON-safe form of ToolCallRecord (whose Err is an
// error and cannot round-trip through JSON directly).
type persistedToolCall struct {
	ToolName   string         `json:"tool_name"`
	Input      map[string]any `json:"input"`
	Output     string         `json:"output"`
	Error      string         `json:"error,omitempty"`
	DurationMs int            `json:"duration_ms"`
}

func toPersistedToolCalls(in []ToolCallRecord) []persistedToolCall {
	out := make([]persistedToolCall, 0, len(in))
	for _, r := range in {
		p := persistedToolCall{
			ToolName:   r.ToolName,
			Input:      r.Input,
			Output:     r.Output,
			DurationMs: r.DurationMs,
		}
		if r.Err != nil {
			p.Error = r.Err.Error()
		}
		out = append(out, p)
	}
	return out
}

func fromPersistedToolCalls(in []persistedToolCall) []ToolCallRecord {
	out := make([]ToolCallRecord, 0, len(in))
	for _, p := range in {
		r := ToolCallRecord{
			ToolName:   p.ToolName,
			Input:      p.Input,
			Output:     p.Output,
			DurationMs: p.DurationMs,
		}
		if p.Error != "" {
			r.Err = errors.New(p.Error)
		}
		out = append(out, r)
	}
	return out
}

// pendingCall is the gated tool call a run paused on. id is the provider's tool
// call id on the native path and empty on the ReAct path.
type pendingCall struct {
	id    string
	name  string
	input map[string]any
}

// pauseForApproval serialises the current loop state, records a pending approval
// via the gate, and returns a Result with Status == StatusAwaitingApproval. The
// pending tool has NOT executed; Resume will execute it (on approve) or feed the
// rejection back (on reject). st.iteration must already be set to the iteration
// that requested the tool. remaining are the native tool calls of the same model
// turn that come after the pending one (always nil on the ReAct path).
func (e *Executor) pauseForApproval(ctx context.Context, st *reactLoopState, pending pendingCall, remaining []llm.ToolCall) Result {
	rs := resumableState{
		Mode:               st.mode(),
		Provider:           st.provider,
		ModelName:          st.modelName,
		AgentTools:         st.agentTools,
		Messages:           toPersistedMessages(st.messages),
		Iteration:          st.iteration,
		InputTokens:        st.inputTokens,
		OutputTokens:       st.outputTokens,
		TotalTokens:        st.totalTokens,
		ToolCalls:          toPersistedToolCalls(st.toolCalls),
		PendingTool:        pending.name,
		PendingInput:       pending.input,
		ElapsedMs:          int(time.Since(st.runStart).Milliseconds()),
		PendingToolCallID:  pending.id,
		RemainingToolCalls: toPersistedCalls(remaining),
	}
	raw, err := json.Marshal(rs)
	if err != nil {
		return st.result("", st.iteration, fmt.Errorf("executor: marshal resume state: %w", err))
	}

	approvalID, err := e.gate.CreatePending(ctx, st.execID, st.agentID, st.orgID, pending.name, pending.input, raw)
	if err != nil {
		return st.result("", st.iteration, fmt.Errorf("executor: create pending approval: %w", err))
	}

	st.log.Info("execution paused awaiting approval",
		zap.String("tool", pending.name),
		zap.String("mode", rs.Mode),
		zap.String("approval_id", approvalID.String()),
	)

	res := st.result("", st.iteration, nil)
	res.Status = StatusAwaitingApproval
	res.ApprovalID = approvalID
	return res
}

// result builds a Result from the loop state's metering.
func (st *reactLoopState) result(finalAnswer string, iterations int, err error) Result {
	return buildResult(finalAnswer, iterations, st.totalTokens, st.inputTokens, st.outputTokens,
		st.runStart, st.provider, st.modelName, st.toolCalls, err)
}

// Resume reconstructs a paused run from an approval's resume state and continues
// it to completion (or the next gate), on whichever loop paused — ReAct or native
// tool-calling. On an approved decision it executes the pending tool and feeds
// its result back; on a rejected decision it feeds "Action rejected by <human>:
// <reason>" back instead. Metering and ToolCallRecord semantics are preserved
// across the pause boundary: tokens and tool calls are cumulative, latency is
// the run's compute time excluding the human wait, and PriorToolCalls tells the
// persister which tool calls were already recorded at the pause.
func (e *Executor) Resume(ctx context.Context, approval *model.Approval) Result {
	var rs resumableState
	if err := json.Unmarshal(approval.ResumeState, &rs); err != nil {
		return Result{Err: fmt.Errorf("executor: resume: unmarshal resume state: %w", err)}
	}
	res := e.resume(ctx, approval, rs)
	res.PriorToolCalls = len(rs.ToolCalls)
	if res.PriorToolCalls > len(res.ToolCalls) {
		res.PriorToolCalls = len(res.ToolCalls)
	}
	return res
}

func (e *Executor) resume(ctx context.Context, approval *model.Approval, rs resumableState) Result {
	log := e.logger.With(
		zap.String("execution_id", approval.ExecutionID.String()),
		zap.String("agent_id", approval.AgentID.String()),
		zap.String("approval_id", approval.ID.String()),
	)

	// Restore org/agent scoping so org- and agent-aware tools resolve correctly.
	ctx = tools.WithOrg(ctx, approval.OrgID)
	ctx = tools.WithAgent(ctx, approval.AgentID)
	// Re-label the run's LLM calls for Langfuse too — Resume crosses the pause
	// boundary with a fresh ctx, and the resumed calls belong to the same
	// execution session as the ones before the gate.
	ctx = llmtrace.WithTrace(ctx, llmtrace.TraceInfo{
		TraceName:   "go-executor-run",
		SessionID:   approval.ExecutionID.String(),
		ExecutionID: approval.ExecutionID.String(),
		AgentID:     approval.AgentID.String(),
		OrgID:       approval.OrgID.String(),
	})

	client, err := e.llmRouter.ForWithFallback(rs.Provider)
	if err != nil {
		return Result{Err: fmt.Errorf("executor: resume: resolve LLM client: %w", err)}
	}

	st := &reactLoopState{
		client:       client,
		modelName:    rs.ModelName,
		provider:     client.ProviderName(),
		toolRegistry: e.registry.Subset(rs.AgentTools),
		agentID:      approval.AgentID,
		orgID:        approval.OrgID,
		execID:       approval.ExecutionID,
		agentTools:   rs.AgentTools,
		messages:     fromPersistedMessages(rs.Messages),
		iteration:    rs.Iteration,
		toolCalls:    fromPersistedToolCalls(rs.ToolCalls),
		inputTokens:  rs.InputTokens,
		outputTokens: rs.OutputTokens,
		totalTokens:  rs.TotalTokens,
		runStart:     time.Now().Add(-time.Duration(rs.ElapsedMs) * time.Millisecond),
		log:          log,
	}
	pending := pendingCall{id: rs.PendingToolCallID, name: rs.PendingTool, input: rs.PendingInput}
	if pending.input == nil {
		pending.input = map[string]any{}
	}

	if rs.Mode == loopModeNative {
		defs, ok := buildToolDefs(st.toolRegistry)
		if !ok || len(defs) == 0 || !clientSupportsTools(client) {
			return st.result("", st.iteration,
				fmt.Errorf("executor: resume: run paused on the native tool-calling path, but provider %q no longer supports it", rs.Provider))
		}
		st.toolDefs = defs

		var content string
		if approval.Status == model.ApprovalStatusApproved {
			content = e.runApprovedTool(ctx, st, pending)
		} else {
			content = rejectionMessage(approval)
			st.log.Info("resume: action rejected, feeding rejection back", zap.String("tool", pending.name))
		}
		st.messages = append(st.messages, llm.Message{Role: llm.RoleTool, ToolCallID: pending.id, Content: content})

		// Finish the rest of the paused turn before asking the model again: the
		// provider needs a result for every tool call it made in that turn.
		if res, paused := e.runNativeToolCalls(ctx, st, fromPersistedCalls(rs.RemainingToolCalls)); paused {
			return res
		}
		return e.nativeLoop(ctx, st)
	}

	var content string
	if approval.Status == model.ApprovalStatusApproved {
		content = e.runApprovedTool(ctx, st, pending)
	} else {
		// Rejected (or any non-approved state): feed the rejection back so the
		// agent can react instead of executing the tool.
		content = rejectionMessage(approval)
		st.log.Info("resume: action rejected, feeding rejection back", zap.String("tool", pending.name))
	}
	st.messages = append(st.messages, llm.Message{
		Role:    llm.RoleUser,
		Content: llm.BuildToolResultMessage(pending.name, content),
	})
	return e.reactLoop(ctx, st)
}

// runApprovedTool executes the approved pending tool, records it in the run's
// tool calls, and returns the text to feed back to the model.
func (e *Executor) runApprovedTool(ctx context.Context, st *reactLoopState, pending pendingCall) string {
	tool, ok := st.toolRegistry.Get(pending.name)
	if !ok {
		st.toolCalls = append(st.toolCalls, ToolCallRecord{
			ToolName: pending.name,
			Input:    pending.input,
			Err:      fmt.Errorf("tool not available"),
		})
		return fmt.Sprintf("Error: tool %q is not available to this agent.", pending.name)
	}
	st.log.Info("resume: executing approved tool", zap.String("tool", pending.name))
	return e.executeTool(ctx, st, tool, pending.name, pending.input)
}

// executeTool runs one tool with the standard 60-second timeout, records it in
// the run's tool calls and returns the text to feed back to the model. It is
// shared by both loops and by Resume so metering is identical everywhere.
func (e *Executor) executeTool(ctx context.Context, st *reactLoopState, tool tools.Tool, name string, input map[string]any) string {
	chatstream.Emit(ctx, chatstream.Event{Type: chatstream.EventTool, Data: map[string]any{"name": name, "state": "start"}})
	toolCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	start := time.Now()
	out, terr := tool.Execute(toolCtx, input)
	cancel()
	durationMs := int(time.Since(start).Milliseconds())
	chatstream.Emit(ctx, chatstream.Event{Type: chatstream.EventTool, Data: map[string]any{"name": name, "state": "end", "duration_ms": durationMs, "ok": terr == nil}})

	st.toolCalls = append(st.toolCalls, ToolCallRecord{
		ToolName:   name,
		Input:      input,
		Output:     out,
		Err:        terr,
		DurationMs: durationMs,
	})

	if terr != nil {
		st.log.Warn("tool execution error", zap.String("tool", name), zap.Error(terr))
		return fmt.Sprintf("Error executing tool: %v", terr)
	}
	st.log.Info("tool executed", zap.String("tool", name), zap.Int("duration_ms", durationMs))
	return out
}

// rejectionMessage is what the agent is told when a human rejects its action.
func rejectionMessage(approval *model.Approval) string {
	who := approval.DeciderName
	if who == "" {
		if approval.DecidedBy != nil {
			who = approval.DecidedBy.String()
		} else {
			who = "a human reviewer"
		}
	}
	reason := ""
	if approval.Reason != nil {
		reason = *approval.Reason
	}
	return fmt.Sprintf("Action rejected by %s: %s", who, reason)
}
