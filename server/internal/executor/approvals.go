package executor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

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

// reactLoopState is the mutable state threaded through the ReAct loop. It is the
// single source of truth for both a fresh Run and a resumed run, and it is what
// gets serialised (via resumableState) when a run pauses on a gated tool.
type reactLoopState struct {
	client       llm.Client
	modelName    string
	provider     string
	toolRegistry *tools.Registry
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

// resumableState is the JSON-serialised snapshot persisted with a pending
// approval. Only the ReAct path is captured: its messages carry only Role +
// Content (the native-only ToolCalls/ToolCallID fields are unused here), so a
// direct []llm.Message round-trips faithfully.
type resumableState struct {
	Provider     string              `json:"provider"`
	ModelName    string              `json:"model_name"`
	AgentTools   []string            `json:"agent_tools"`
	Messages     []llm.Message       `json:"messages"`
	Iteration    int                 `json:"iteration"`
	InputTokens  int                 `json:"input_tokens"`
	OutputTokens int                 `json:"output_tokens"`
	TotalTokens  int                 `json:"total_tokens"`
	ToolCalls    []persistedToolCall `json:"tool_calls"`
	PendingTool  string              `json:"pending_tool"`
	PendingInput map[string]any      `json:"pending_input"`
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

// pauseForApproval serialises the current loop state, records a pending approval
// via the gate, and returns a Result with Status == StatusAwaitingApproval. The
// pending tool has NOT executed; Resume will execute it (on approve) or feed the
// rejection back (on reject). st.iteration must already be set to the iteration
// that requested the tool.
func (e *Executor) pauseForApproval(ctx context.Context, st *reactLoopState, toolName string, toolInput map[string]any) Result {
	rs := resumableState{
		Provider:     st.provider,
		ModelName:    st.modelName,
		AgentTools:   st.agentTools,
		Messages:     st.messages,
		Iteration:    st.iteration,
		InputTokens:  st.inputTokens,
		OutputTokens: st.outputTokens,
		TotalTokens:  st.totalTokens,
		ToolCalls:    toPersistedToolCalls(st.toolCalls),
		PendingTool:  toolName,
		PendingInput: toolInput,
	}
	raw, err := json.Marshal(rs)
	if err != nil {
		return buildResult("", st.iteration, st.totalTokens, st.inputTokens, st.outputTokens,
			st.runStart, st.provider, st.modelName, st.toolCalls,
			fmt.Errorf("executor: marshal resume state: %w", err))
	}

	approvalID, err := e.gate.CreatePending(ctx, st.execID, st.agentID, st.orgID, toolName, toolInput, raw)
	if err != nil {
		return buildResult("", st.iteration, st.totalTokens, st.inputTokens, st.outputTokens,
			st.runStart, st.provider, st.modelName, st.toolCalls,
			fmt.Errorf("executor: create pending approval: %w", err))
	}

	st.log.Info("execution paused awaiting approval",
		zap.String("tool", toolName),
		zap.String("approval_id", approvalID.String()),
	)

	res := buildResult("", st.iteration, st.totalTokens, st.inputTokens, st.outputTokens,
		st.runStart, st.provider, st.modelName, st.toolCalls, nil)
	res.Status = StatusAwaitingApproval
	res.ApprovalID = approvalID
	return res
}

// Resume reconstructs a paused ReAct run from an approval's resume state and
// continues it to completion (or the next gate). On an approved decision it
// executes the pending tool and feeds its result back; on a rejected decision it
// feeds "Action rejected by <human>: <reason>" back instead. Metering and
// ToolCallRecord semantics are preserved across the pause boundary.
func (e *Executor) Resume(ctx context.Context, approval *model.Approval) Result {
	log := e.logger.With(
		zap.String("execution_id", approval.ExecutionID.String()),
		zap.String("agent_id", approval.AgentID.String()),
		zap.String("approval_id", approval.ID.String()),
	)

	var rs resumableState
	if err := json.Unmarshal(approval.ResumeState, &rs); err != nil {
		return Result{Err: fmt.Errorf("executor: resume: unmarshal resume state: %w", err)}
	}

	// Restore org/agent scoping so org- and agent-aware tools resolve correctly.
	ctx = tools.WithOrg(ctx, approval.OrgID)
	ctx = tools.WithAgent(ctx, approval.AgentID)
	// Re-label the run's LLM calls for Langfuse too — Resume crosses the pause
	// boundary with a fresh ctx, and the resumed calls belong to the same
	// execution session as the ones before the gate.
	ctx = llmtrace.WithTrace(ctx, llmtrace.TraceInfo{
		TraceName: "go-executor-run",
		SessionID: approval.ExecutionID.String(),
		AgentID:   approval.AgentID.String(),
		OrgID:     approval.OrgID.String(),
	})

	client, err := e.llmRouter.For(rs.Provider)
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
		messages:     rs.Messages,
		iteration:    rs.Iteration,
		toolCalls:    fromPersistedToolCalls(rs.ToolCalls),
		inputTokens:  rs.InputTokens,
		outputTokens: rs.OutputTokens,
		totalTokens:  rs.TotalTokens,
		runStart:     time.Now(),
		log:          log,
	}

	switch approval.Status {
	case model.ApprovalStatusApproved:
		e.executePendingTool(ctx, st, rs.PendingTool, rs.PendingInput)
	default:
		// Rejected (or any non-approved state): feed the rejection back so the
		// agent can react instead of executing the tool.
		e.feedRejection(st, approval, rs.PendingTool)
	}

	return e.reactLoop(ctx, st)
}

// executePendingTool runs the approved tool and appends its result to the
// conversation, mirroring the ReAct loop's own tool-execution block so metering
// stays identical across the pause boundary.
func (e *Executor) executePendingTool(ctx context.Context, st *reactLoopState, toolName string, toolInput map[string]any) {
	if toolInput == nil {
		toolInput = map[string]any{}
	}

	tool, ok := st.toolRegistry.Get(toolName)
	if !ok {
		msg := fmt.Sprintf("Error: tool %q is not available to this agent.", toolName)
		st.messages = append(st.messages, llm.Message{
			Role:    llm.RoleUser,
			Content: llm.BuildToolResultMessage(toolName, msg),
		})
		st.toolCalls = append(st.toolCalls, ToolCallRecord{
			ToolName: toolName,
			Input:    toolInput,
			Err:      fmt.Errorf("tool not available"),
		})
		return
	}

	toolCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	start := time.Now()
	out, terr := tool.Execute(toolCtx, toolInput)
	cancel()
	durationMs := int(time.Since(start).Milliseconds())

	st.toolCalls = append(st.toolCalls, ToolCallRecord{
		ToolName:   toolName,
		Input:      toolInput,
		Output:     out,
		Err:        terr,
		DurationMs: durationMs,
	})

	resultMsg := out
	if terr != nil {
		resultMsg = fmt.Sprintf("Error executing tool: %v", terr)
		st.log.Warn("resume: approved tool execution error", zap.String("tool", toolName), zap.Error(terr))
	} else {
		st.log.Info("resume: executed approved tool", zap.String("tool", toolName), zap.Int("duration_ms", durationMs))
	}

	st.messages = append(st.messages, llm.Message{
		Role:    llm.RoleUser,
		Content: llm.BuildToolResultMessage(toolName, resultMsg),
	})
}

// feedRejection appends a rejection message so the agent continues the loop
// without the tool having executed.
func (e *Executor) feedRejection(st *reactLoopState, approval *model.Approval, toolName string) {
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
	msg := fmt.Sprintf("Action rejected by %s: %s", who, reason)
	st.messages = append(st.messages, llm.Message{
		Role:    llm.RoleUser,
		Content: llm.BuildToolResultMessage(toolName, msg),
	})
	st.log.Info("resume: action rejected, feeding rejection back", zap.String("tool", toolName))
}
