// Package executor implements the ReAct (Reasoning + Acting) execution loop
// for AI agents. Each Executor run is associated with a single AgentExecution
// record and drives the model through iterative tool use until a final answer
// is produced or the iteration cap is reached.
package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/llm"
	"github.com/jobshout/server/internal/llmtrace"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/modelselect"
	"github.com/jobshout/server/internal/tools"
)

// MaxIterations is the hard cap on ReAct loop iterations to prevent infinite loops.
const MaxIterations = 15

// StatusAwaitingApproval is the Result.Status value set when a run pauses on a
// gated tool call, waiting for a human decision. An empty Status is the default
// and denotes a normal (completed or failed) run.
const StatusAwaitingApproval = "awaiting_approval"

// reactResponse is the JSON structure every LLM turn must return.
type reactResponse struct {
	Thought     string         `json:"thought"`
	Action      *string        `json:"action"`
	ActionInput map[string]any `json:"action_input"`
	FinalAnswer *string        `json:"final_answer"`
}

// ToolCallRecord is a single tool invocation captured during execution.
type ToolCallRecord struct {
	ToolName   string
	Input      map[string]any
	Output     string
	Err        error
	DurationMs int
}

// Result is the outcome of a completed Executor run.
type Result struct {
	FinalAnswer   string
	Iterations    int
	TotalTokens   int
	InputTokens   int
	OutputTokens  int
	LatencyMs     int
	ModelProvider string
	ModelName     string
	ToolCalls     []ToolCallRecord
	Err           error

	// Status is "" for a normal (completed or failed) run, or
	// StatusAwaitingApproval when the run paused on a gated tool call.
	Status string
	// ApprovalID is the pending approval created when Status ==
	// StatusAwaitingApproval; it is the zero UUID otherwise.
	ApprovalID uuid.UUID
}

// knowledgeTopK is the number of knowledge chunks the executor retrieves and
// injects as context before a run's loop starts.
const knowledgeTopK = 5

// SkillProvider yields the skills currently enabled for an agent. It is
// satisfied by repository.SkillRepository; the executor depends on this narrow
// interface so it never imports the repository package directly.
type SkillProvider interface {
	ListForAgent(ctx context.Context, agentID uuid.UUID) ([]model.Skill, error)
}

// Executor runs the ReAct loop for a single agent against a given task prompt.
type Executor struct {
	llmRouter *llm.Router
	registry  *tools.Registry
	skills    SkillProvider
	knowledge tools.KnowledgeSearcher
	embedder  tools.Embedder
	gate      ApprovalGate
	selector  *modelselect.Selector
	logger    *zap.Logger
}

// New creates an Executor.
func New(router *llm.Router, registry *tools.Registry, logger *zap.Logger) *Executor {
	return &Executor{
		llmRouter: router,
		registry:  registry,
		logger:    logger,
	}
}

// WithSkills attaches a SkillProvider so enabled skills are folded into each
// run (prompt patches for kind=prompt, extra tools for kind=tool). Returns the
// receiver for fluent wiring. When no provider is set, runs proceed unchanged.
func (e *Executor) WithSkills(p SkillProvider) *Executor {
	e.skills = p
	return e
}

// WithKnowledge attaches a chunk searcher and embedder so each run is augmented
// with the agent's most relevant stored knowledge before the loop starts.
// Returns the receiver for fluent wiring. Retrieval is best-effort: it is
// skipped entirely unless BOTH a searcher and an embedder are configured, and a
// retrieval failure never breaks a run.
func (e *Executor) WithKnowledge(searcher tools.KnowledgeSearcher, embedder tools.Embedder) *Executor {
	e.knowledge = searcher
	e.embedder = embedder
	return e
}

// WithApprovalGate attaches a human-in-the-loop approval gate. When a gated tool
// is about to run, the ReAct loop pauses: it serialises its resumable state,
// records a pending approval via the gate, and returns a Result with
// Status == StatusAwaitingApproval. Returns the receiver for fluent wiring. When
// no gate is set (the default), runs proceed unchanged and no tool is ever gated.
func (e *Executor) WithApprovalGate(g ApprovalGate) *Executor {
	e.gate = g
	return e
}

// WithAutoSelect attaches a per-task model selector, enabling agents whose
// ModelProvider is "auto" to have a provider and model chosen for them at call
// time. Returns the receiver for fluent wiring. When no selector is set, an
// agent asking for "auto" quietly falls back to the configured default provider
// rather than failing, so this is safe to leave unwired.
func (e *Executor) WithAutoSelect(s *modelselect.Selector) *Executor {
	e.selector = s
	return e
}

// Run executes the ReAct loop for agent against taskPrompt.
// agentTools is the subset of tool names the agent is permitted to use.
// The execution ID is used only for structured logging correlation.
func (e *Executor) Run(
	ctx context.Context,
	execID uuid.UUID,
	agent *model.Agent,
	taskPrompt string,
	agentTools []string,
) Result {
	log := e.logger.With(
		zap.String("execution_id", execID.String()),
		zap.String("agent_id", agent.ID.String()),
		zap.String("agent_name", agent.Name),
	)

	// Org-scope the context so org-aware tools (the integration tools) resolve
	// this tenant's own credentials when the agent calls them, and agent-scope it
	// so agent-aware tools (knowledge_search) resolve this agent's own knowledge.
	ctx = tools.WithOrg(ctx, agent.OrgID)
	ctx = tools.WithAgent(ctx, agent.ID)
	// Label every LLM call of this run for Langfuse: the execution groups the
	// calls, the agent feeds the by-agent widget.
	ctx = llmtrace.WithTrace(ctx, llmtrace.TraceInfo{
		TraceName: "go-executor-run",
		SessionID: execID.String(),
		AgentID:   agent.ID.String(),
		OrgID:     agent.OrgID.String(),
	})

	// The LLM client is resolved further down, once the tool set is known:
	// auto-selection needs to know whether this run requires native
	// tool-calling before it can rule models in or out.

	// Base system-prompt text from the agent definition.
	systemPromptText := ""
	if agent.SystemPrompt != nil {
		systemPromptText = *agent.SystemPrompt
	}

	// Fold in any enabled skills: kind=prompt skills append to the system
	// prompt, kind=tool skills widen the permitted tool set. Skill failures are
	// non-fatal — a run must never break because the registry hiccuped.
	if e.skills != nil {
		skills, serr := e.skills.ListForAgent(ctx, agent.ID)
		if serr != nil {
			log.Warn("failed to load agent skills; continuing without them", zap.Error(serr))
		} else if len(skills) > 0 {
			var promptPatch string
			agentTools, promptPatch = applySkills(agentTools, skills)
			if promptPatch != "" {
				systemPromptText = strings.TrimSpace(systemPromptText + "\n\n" + promptPatch)
			}
			log.Info("applied agent skills",
				zap.Int("skills", len(skills)),
				zap.Int("effective_tools", len(agentTools)),
			)
		}
	}

	// Build the tool subset this agent may use (after skills widened it).
	toolRegistry := e.registry.Subset(agentTools)
	toolSpecs := make([]llm.ToolSpec, 0)
	for _, t := range toolRegistry.All() {
		toolSpecs = append(toolSpecs, llm.ToolSpec{Name: t.Name(), Description: t.Description()})
	}

	// Build the system prompt once.
	systemPrompt := llm.BuildReActSystemPrompt(agent.Name, agent.Role, systemPromptText, toolSpecs)

	// Resolve provider + model. For an agent set to "auto" this picks a model
	// sized to the task; for every other agent it resolves exactly as before.
	choice, err := resolveModel(e.llmRouter, e.selector, log, agent, modelselect.TaskSignals{
		Kind:         modelselect.KindStep,
		PromptTokens: estimateTokens(systemPrompt, taskPrompt),
		NeedsTools:   len(toolSpecs) > 0,
	})
	if err != nil {
		return Result{Err: err}
	}
	client := choice.Client
	resolvedProvider := choice.Provider
	modelName := choice.Model

	// Best-effort retrieval-augmented context: fetch the agent's most relevant
	// knowledge chunks for this task and inject them as a leading context
	// message. Gated on a configured searcher+embedder and never fatal — a
	// retrieval failure must never break the run. Shared by both loop paths.
	knowledgeMsg := e.retrieveKnowledge(ctx, log, agent.ID, taskPrompt)

	// Native tool-calling path: when the provider advertises tool support AND
	// every tool the agent may use exposes a JSON schema, drive the loop with
	// real function definitions instead of the ReAct JSON-in-prompt fallback.
	// Otherwise fall through to the existing ReAct loop below, unchanged.
	if toolDefs, ok := buildToolDefs(toolRegistry); ok && len(toolDefs) > 0 && clientSupportsTools(client) {
		log.Info("using native tool-calling path", zap.Int("tools", len(toolDefs)))
		nativeSystem := llm.BuildNativeSystemPrompt(agent.Name, agent.Role, systemPromptText)
		return e.runNative(ctx, log, agent.ID, client, modelName, resolvedProvider,
			nativeSystem, taskPrompt, knowledgeMsg, toolDefs, toolRegistry)
	}

	// Seed the conversation and drive the ReAct loop from a fresh state.
	st := &reactLoopState{
		client:       client,
		modelName:    modelName,
		provider:     resolvedProvider,
		toolRegistry: toolRegistry,
		agentID:      agent.ID,
		orgID:        agent.OrgID,
		execID:       execID,
		agentTools:   agentTools,
		messages:     seedMessages(systemPrompt, knowledgeMsg, taskPrompt),
		iteration:    0,
		runStart:     time.Now(),
		log:          log,
	}
	return e.reactLoop(ctx, st)
}

// reactLoop drives the ReAct iteration loop over the supplied state, starting at
// st.iteration+1. It is shared by a fresh Run and by Resume (which pre-populates
// the state from a persisted approval), so both take exactly the same code path.
// When a gated tool is about to execute it pauses via pauseForApproval, returning
// a Result with Status == StatusAwaitingApproval.
func (e *Executor) reactLoop(ctx context.Context, st *reactLoopState) Result {
	for iteration := st.iteration + 1; iteration <= MaxIterations; iteration++ {
		st.log.Info("ReAct iteration", zap.Int("iteration", iteration))

		llmResp, err := st.client.Generate(ctx, llm.GenerateRequest{
			Messages:    st.messages,
			Model:       st.modelName,
			MaxTokens:   4096,
			Temperature: 0.2,
		})
		if err != nil {
			return buildResult("", iteration, st.totalTokens, st.inputTokens, st.outputTokens,
				st.runStart, st.provider, st.modelName, st.toolCalls,
				fmt.Errorf("executor: LLM generate (iteration %d): %w", iteration, err))
		}

		st.inputTokens += llmResp.InputTokens
		st.outputTokens += llmResp.OutputTokens
		st.totalTokens += llmResp.InputTokens + llmResp.OutputTokens

		// Append the assistant turn to maintain conversation history.
		st.messages = append(st.messages, llm.Message{
			Role:    llm.RoleAssistant,
			Content: llmResp.Content,
		})

		// Parse the structured JSON response.
		parsed, parseErr := parseReActResponse(llmResp.Content)
		if parseErr != nil {
			st.log.Warn("failed to parse ReAct JSON; treating as final answer",
				zap.String("raw", llmResp.Content),
				zap.Error(parseErr),
			)
			// Graceful degradation: treat raw content as the final answer.
			return buildResult(llmResp.Content, iteration, st.totalTokens, st.inputTokens, st.outputTokens,
				st.runStart, st.provider, st.modelName, st.toolCalls, nil)
		}

		st.log.Debug("parsed ReAct response",
			zap.String("thought", parsed.Thought),
			zap.Boolp("has_action", boolPtr(parsed.Action != nil)),
			zap.Boolp("has_final_answer", boolPtr(parsed.FinalAnswer != nil)),
		)

		// Case 1: Agent has produced a final answer.
		if parsed.FinalAnswer != nil && *parsed.FinalAnswer != "" {
			return buildResult(*parsed.FinalAnswer, iteration, st.totalTokens, st.inputTokens, st.outputTokens,
				st.runStart, st.provider, st.modelName, st.toolCalls, nil)
		}

		// Case 2: Agent wants to call a tool.
		if parsed.Action == nil || *parsed.Action == "" {
			// No action and no final answer — shouldn't happen but recover gracefully.
			return buildResult(parsed.Thought, iteration, st.totalTokens, st.inputTokens, st.outputTokens,
				st.runStart, st.provider, st.modelName, st.toolCalls, nil)
		}

		toolName := *parsed.Action
		toolInput := parsed.ActionInput
		if toolInput == nil {
			toolInput = map[string]any{}
		}

		tool, ok := st.toolRegistry.Get(toolName)
		if !ok {
			toolResult := fmt.Sprintf("Error: tool %q is not available to this agent.", toolName)
			st.log.Warn("agent requested unavailable tool", zap.String("tool", toolName))

			st.messages = append(st.messages, llm.Message{
				Role:    llm.RoleUser,
				Content: llm.BuildToolResultMessage(toolName, toolResult),
			})

			st.toolCalls = append(st.toolCalls, ToolCallRecord{
				ToolName: toolName,
				Input:    toolInput,
				Err:      fmt.Errorf("tool not available"),
			})
			continue
		}

		// Approval gate: when configured and this tool is gated for the agent,
		// pause the run here — before the tool runs — and record a pending
		// approval. The assistant turn requesting the tool is already in the
		// conversation, so Resume can execute the tool and continue cleanly.
		if e.gate != nil && e.gate.RequiresApproval(st.agentID, toolName) {
			st.iteration = iteration
			return e.pauseForApproval(ctx, st, toolName, toolInput)
		}

		// Execute the tool with a 60-second timeout.
		toolCtx, toolCancel := context.WithTimeout(ctx, 60*time.Second)
		start := time.Now()
		toolOutput, toolErr := tool.Execute(toolCtx, toolInput)
		toolCancel()
		durationMs := int(time.Since(start).Milliseconds())

		record := ToolCallRecord{
			ToolName:   toolName,
			Input:      toolInput,
			Output:     toolOutput,
			Err:        toolErr,
			DurationMs: durationMs,
		}
		st.toolCalls = append(st.toolCalls, record)

		var toolResultMsg string
		if toolErr != nil {
			toolResultMsg = fmt.Sprintf("Error executing tool: %v", toolErr)
			st.log.Warn("tool execution error",
				zap.String("tool", toolName),
				zap.Error(toolErr),
			)
		} else {
			toolResultMsg = toolOutput
			st.log.Info("tool executed",
				zap.String("tool", toolName),
				zap.Int("duration_ms", durationMs),
			)
		}

		st.messages = append(st.messages, llm.Message{
			Role:    llm.RoleUser,
			Content: llm.BuildToolResultMessage(toolName, toolResultMsg),
		})
	}

	// Exceeded max iterations without a final answer.
	return buildResult("", MaxIterations, st.totalTokens, st.inputTokens, st.outputTokens,
		st.runStart, st.provider, st.modelName, st.toolCalls,
		fmt.Errorf("executor: exceeded maximum iterations (%d) without a final answer", MaxIterations))
}

// parseReActResponse extracts the JSON object from the LLM content string.
// It is tolerant of code fences and leading/trailing whitespace.
func parseReActResponse(content string) (*reactResponse, error) {
	// Strip optional markdown code fences.
	s := strings.TrimSpace(content)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	s = strings.TrimSpace(s)

	// Find the JSON object boundaries.
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start == -1 || end == -1 || end < start {
		return nil, fmt.Errorf("no JSON object found in response")
	}
	s = s[start : end+1]

	var resp reactResponse
	if err := json.Unmarshal([]byte(s), &resp); err != nil {
		return nil, fmt.Errorf("unmarshal ReAct JSON: %w", err)
	}
	return &resp, nil
}

func boolPtr(b bool) *bool { return &b }

// applySkills folds enabled skills into the agent's tool allow-list and returns
// any system-prompt text to append. It is deterministic and side-effect free so
// it can be unit-tested without an LLM or database.
//
//   - kind=tool   → config_json.tool (string) and/or config_json.tools ([]string)
//     are added to the tool allow-list (deduped, order preserved).
//   - kind=prompt → config_json.prompt (string), falling back to the skill
//     description, is collected under an "Enabled Skills" heading.
//   - kind=bundle → not expanded yet (no execution semantics); skipped.
func applySkills(agentTools []string, skills []model.Skill) ([]string, string) {
	seen := make(map[string]bool, len(agentTools))
	merged := make([]string, 0, len(agentTools))
	for _, t := range agentTools {
		if !seen[t] {
			seen[t] = true
			merged = append(merged, t)
		}
	}

	var patches []string
	for _, s := range skills {
		switch s.Kind {
		case "tool":
			for _, name := range skillToolNames(s) {
				if name != "" && !seen[name] {
					seen[name] = true
					merged = append(merged, name)
				}
			}
		case "prompt":
			if frag := skillPromptFragment(s); frag != "" {
				patches = append(patches, "- "+frag)
			}
		}
	}

	patch := ""
	if len(patches) > 0 {
		patch = "## Enabled Skills\n\n" + strings.Join(patches, "\n")
	}
	return merged, patch
}

// skillToolNames extracts the tool name(s) a kind=tool skill grants, supporting
// both a single "tool" string and a "tools" array in config_json.
func skillToolNames(s model.Skill) []string {
	var out []string
	if v, ok := s.ConfigJSON["tool"].(string); ok && v != "" {
		out = append(out, v)
	}
	if raw, ok := s.ConfigJSON["tools"].([]any); ok {
		for _, item := range raw {
			if name, ok := item.(string); ok && name != "" {
				out = append(out, name)
			}
		}
	}
	return out
}

// skillPromptFragment returns the prompt text a kind=prompt skill contributes,
// preferring config_json.prompt and falling back to the skill description.
func skillPromptFragment(s model.Skill) string {
	if v, ok := s.ConfigJSON["prompt"].(string); ok && strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	if s.Description != nil {
		return strings.TrimSpace(*s.Description)
	}
	return ""
}

// buildResult constructs a Result with the common metering fields pre-filled.
func buildResult(
	finalAnswer string,
	iterations, totalTokens, inputTokens, outputTokens int,
	startTime time.Time,
	provider, model string,
	toolCalls []ToolCallRecord,
	err error,
) Result {
	return Result{
		FinalAnswer:   finalAnswer,
		Iterations:    iterations,
		TotalTokens:   totalTokens,
		InputTokens:   inputTokens,
		OutputTokens:  outputTokens,
		LatencyMs:     int(time.Since(startTime).Milliseconds()),
		ModelProvider: provider,
		ModelName:     model,
		ToolCalls:     toolCalls,
		Err:           err,
	}
}

// seedMessages builds the initial conversation for a run: the system prompt,
// an optional leading knowledge-context message (omitted when empty), then the
// task. Both loop paths seed identically so retrieval-augmentation applies
// uniformly. The knowledge block is a user-role message rather than a second
// system message: some providers (e.g. Claude) collapse to a single system
// prompt, so a second system message would clobber the agent's own prompt.
func seedMessages(systemPrompt, knowledgeMsg, taskPrompt string) []llm.Message {
	msgs := make([]llm.Message, 0, 3)
	msgs = append(msgs, llm.Message{Role: llm.RoleSystem, Content: systemPrompt})
	if knowledgeMsg != "" {
		msgs = append(msgs, llm.Message{Role: llm.RoleUser, Content: knowledgeMsg})
	}
	msgs = append(msgs, llm.Message{Role: llm.RoleUser, Content: llm.BuildTaskUserMessage(taskPrompt)})
	return msgs
}

// retrieveKnowledge returns a "Relevant knowledge:\n..." context block built
// from the agent's top-K knowledge chunks most similar to taskPrompt, or the
// empty string when retrieval is not configured or yields nothing. It is
// best-effort: any error (embedding or search) is logged and swallowed so a run
// is never broken by the knowledge base.
func (e *Executor) retrieveKnowledge(ctx context.Context, log *zap.Logger, agentID uuid.UUID, taskPrompt string) string {
	if e.knowledge == nil || e.embedder == nil {
		return ""
	}

	vectors, err := e.embedder.Embed(ctx, []string{taskPrompt})
	if err != nil {
		log.Warn("knowledge retrieval: embed failed; continuing without it", zap.Error(err))
		return ""
	}
	if len(vectors) == 0 {
		return ""
	}

	chunks, err := e.knowledge.SearchByAgent(ctx, agentID, vectors[0], knowledgeTopK)
	if err != nil {
		log.Warn("knowledge retrieval: search failed; continuing without it", zap.Error(err))
		return ""
	}
	if len(chunks) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("Relevant knowledge:\n")
	for _, c := range chunks {
		content := strings.TrimSpace(c.Content)
		if content == "" {
			continue
		}
		sb.WriteString("- ")
		sb.WriteString(content)
		sb.WriteString("\n")
	}
	log.Info("injected retrieved knowledge into run", zap.Int("chunks", len(chunks)))
	return strings.TrimRight(sb.String(), "\n")
}

// clientSupportsTools reports whether the resolved LLM client advertises native
// tool-calling. Clients opt in by implementing llm.ToolCapableClient; those that
// don't (or that return false) drive the ReAct loop instead.
func clientSupportsTools(client llm.Client) bool {
	tc, ok := client.(llm.ToolCapableClient)
	return ok && tc.SupportsTools()
}

// buildToolDefs converts every tool in the registry into an llm.ToolDef, but
// only if ALL of them expose a JSON schema (implement tools.SchemaProvider).
// The second return is false the moment any tool lacks a schema, signalling the
// caller to fall back to the ReAct loop.
func buildToolDefs(reg *tools.Registry) ([]llm.ToolDef, bool) {
	all := reg.All()
	defs := make([]llm.ToolDef, 0, len(all))
	for _, t := range all {
		sp, ok := t.(tools.SchemaProvider)
		if !ok {
			return nil, false
		}
		defs = append(defs, llm.ToolDef{
			Name:        t.Name(),
			Description: t.Description(),
			Parameters:  sp.Parameters(),
		})
	}
	return defs, true
}

// runNative drives the agent loop using the provider's native tool-calling
// mechanism. On each turn it sends the tool definitions; when the model returns
// tool calls it executes them via the registry and appends the results in the
// provider-agnostic Message shape (each client translates that to its own wire
// format), repeating up to MaxIterations. Metering and ToolCallRecord/Result
// handling mirror the ReAct path exactly.
func (e *Executor) runNative(
	ctx context.Context,
	log *zap.Logger,
	agentID uuid.UUID,
	client llm.Client,
	modelName, resolvedProvider, systemPrompt, taskPrompt, knowledgeMsg string,
	toolDefs []llm.ToolDef,
	toolRegistry *tools.Registry,
) Result {
	messages := seedMessages(systemPrompt, knowledgeMsg, taskPrompt)

	runStart := time.Now()

	var (
		toolCalls    []ToolCallRecord
		totalTokens  int
		inputTokens  int
		outputTokens int
	)

	for iteration := 1; iteration <= MaxIterations; iteration++ {
		log.Info("native tool-calling iteration", zap.Int("iteration", iteration))

		llmResp, err := client.Generate(ctx, llm.GenerateRequest{
			Messages:    messages,
			Model:       modelName,
			MaxTokens:   4096,
			Temperature: 0.2,
			ToolDefs:    toolDefs,
		})
		if err != nil {
			return buildResult("", iteration, totalTokens, inputTokens, outputTokens,
				runStart, resolvedProvider, modelName, toolCalls,
				fmt.Errorf("executor: LLM generate (iteration %d): %w", iteration, err))
		}

		inputTokens += llmResp.InputTokens
		outputTokens += llmResp.OutputTokens
		totalTokens += llmResp.InputTokens + llmResp.OutputTokens

		// No tool calls => the message content is the final answer.
		if len(llmResp.ToolCalls) == 0 {
			return buildResult(llmResp.Content, iteration, totalTokens, inputTokens, outputTokens,
				runStart, resolvedProvider, modelName, toolCalls, nil)
		}

		// Echo the assistant turn (optional text + the tool_use blocks) so the
		// follow-up request preserves conversation shape for the provider.
		messages = append(messages, llm.Message{
			Role:      llm.RoleAssistant,
			Content:   llmResp.Content,
			ToolCalls: llmResp.ToolCalls,
		})

		// Execute each requested tool and append its result as a RoleTool message.
		for _, tc := range llmResp.ToolCalls {
			input := tc.Arguments
			if input == nil {
				input = map[string]any{}
			}

			tool, ok := toolRegistry.Get(tc.Name)
			if !ok {
				resultMsg := fmt.Sprintf("Error: tool %q is not available to this agent.", tc.Name)
				log.Warn("agent requested unavailable tool", zap.String("tool", tc.Name))
				messages = append(messages, llm.Message{
					Role:       llm.RoleTool,
					ToolCallID: tc.ID,
					Content:    resultMsg,
				})
				toolCalls = append(toolCalls, ToolCallRecord{
					ToolName: tc.Name,
					Input:    input,
					Err:      fmt.Errorf("tool not available"),
				})
				continue
			}

			// Approval gate on the native path is not yet supported: pausing
			// mid-turn would require serialising provider-native tool_call state.
			// Fail fast with a clear message rather than silently bypassing the
			// gate. The ReAct path supports pause/resume fully. (Follow-up.)
			if e.gate != nil && e.gate.RequiresApproval(agentID, tc.Name) {
				return buildResult("", iteration, totalTokens, inputTokens, outputTokens,
					runStart, resolvedProvider, modelName, toolCalls,
					fmt.Errorf("executor: tool %q requires human approval, but pause/resume is not yet supported on the native tool-calling path", tc.Name))
			}

			// Execute the tool with a 60-second timeout.
			toolCtx, toolCancel := context.WithTimeout(ctx, 60*time.Second)
			start := time.Now()
			toolOutput, toolErr := tool.Execute(toolCtx, input)
			toolCancel()
			durationMs := int(time.Since(start).Milliseconds())

			toolCalls = append(toolCalls, ToolCallRecord{
				ToolName:   tc.Name,
				Input:      input,
				Output:     toolOutput,
				Err:        toolErr,
				DurationMs: durationMs,
			})

			resultMsg := toolOutput
			if toolErr != nil {
				resultMsg = fmt.Sprintf("Error executing tool: %v", toolErr)
				log.Warn("tool execution error", zap.String("tool", tc.Name), zap.Error(toolErr))
			} else {
				log.Info("tool executed", zap.String("tool", tc.Name), zap.Int("duration_ms", durationMs))
			}

			messages = append(messages, llm.Message{
				Role:       llm.RoleTool,
				ToolCallID: tc.ID,
				Content:    resultMsg,
			})
		}
	}

	// Exceeded max iterations without a final answer.
	return buildResult("", MaxIterations, totalTokens, inputTokens, outputTokens,
		runStart, resolvedProvider, modelName, toolCalls,
		fmt.Errorf("executor: exceeded maximum iterations (%d) without a final answer", MaxIterations))
}
