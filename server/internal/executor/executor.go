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
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/tools"
)

// MaxIterations is the hard cap on ReAct loop iterations to prevent infinite loops.
const MaxIterations = 15

// reactResponse is the JSON structure every LLM turn must return.
type reactResponse struct {
	Thought      string          `json:"thought"`
	Action       *string         `json:"action"`
	ActionInput  map[string]any  `json:"action_input"`
	FinalAnswer  *string         `json:"final_answer"`
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
}

// Executor runs the ReAct loop for a single agent against a given task prompt.
type Executor struct {
	llmRouter *llm.Router
	registry  *tools.Registry
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

	// Resolve the LLM client for this agent.
	providerName := ""
	if agent.ModelProvider != nil {
		providerName = *agent.ModelProvider
	}
	client, err := e.llmRouter.For(providerName)
	if err != nil {
		return Result{Err: fmt.Errorf("executor: resolve LLM client: %w", err)}
	}

	resolvedProvider := client.ProviderName()

	// Build the tool subset this agent may use.
	toolRegistry := e.registry.Subset(agentTools)
	toolSpecs := make([]llm.ToolSpec, 0)
	for _, t := range toolRegistry.All() {
		toolSpecs = append(toolSpecs, llm.ToolSpec{Name: t.Name(), Description: t.Description()})
	}

	// Build the system prompt once.
	systemPromptText := ""
	if agent.SystemPrompt != nil {
		systemPromptText = *agent.SystemPrompt
	}
	systemPrompt := llm.BuildReActSystemPrompt(agent.Name, agent.Role, systemPromptText, toolSpecs)

	// Resolve model name.
	modelName := ""
	if agent.ModelName != nil {
		modelName = *agent.ModelName
	}

	// Seed the conversation.
	messages := []llm.Message{
		{Role: llm.RoleSystem, Content: systemPrompt},
		{Role: llm.RoleUser, Content: llm.BuildTaskUserMessage(taskPrompt)},
	}

	runStart := time.Now()

	var (
		toolCalls    []ToolCallRecord
		totalTokens  int
		inputTokens  int
		outputTokens int
	)

	for iteration := 1; iteration <= MaxIterations; iteration++ {
		log.Info("ReAct iteration", zap.Int("iteration", iteration))

		llmResp, err := client.Generate(ctx, llm.GenerateRequest{
			Messages:    messages,
			Model:       modelName,
			MaxTokens:   4096,
			Temperature: 0.2,
		})
		if err != nil {
			return buildResult("", iteration, totalTokens, inputTokens, outputTokens,
				runStart, resolvedProvider, modelName, toolCalls,
				fmt.Errorf("executor: LLM generate (iteration %d): %w", iteration, err))
		}

		inputTokens += llmResp.InputTokens
		outputTokens += llmResp.OutputTokens
		totalTokens += llmResp.InputTokens + llmResp.OutputTokens

		// Append the assistant turn to maintain conversation history.
		messages = append(messages, llm.Message{
			Role:    llm.RoleAssistant,
			Content: llmResp.Content,
		})

		// Parse the structured JSON response.
		parsed, parseErr := parseReActResponse(llmResp.Content)
		if parseErr != nil {
			log.Warn("failed to parse ReAct JSON; treating as final answer",
				zap.String("raw", llmResp.Content),
				zap.Error(parseErr),
			)
			// Graceful degradation: treat raw content as the final answer.
			return buildResult(llmResp.Content, iteration, totalTokens, inputTokens, outputTokens,
				runStart, resolvedProvider, modelName, toolCalls, nil)
		}

		log.Debug("parsed ReAct response",
			zap.String("thought", parsed.Thought),
			zap.Boolp("has_action", boolPtr(parsed.Action != nil)),
			zap.Boolp("has_final_answer", boolPtr(parsed.FinalAnswer != nil)),
		)

		// Case 1: Agent has produced a final answer.
		if parsed.FinalAnswer != nil && *parsed.FinalAnswer != "" {
			return buildResult(*parsed.FinalAnswer, iteration, totalTokens, inputTokens, outputTokens,
				runStart, resolvedProvider, modelName, toolCalls, nil)
		}

		// Case 2: Agent wants to call a tool.
		if parsed.Action == nil || *parsed.Action == "" {
			// No action and no final answer — shouldn't happen but recover gracefully.
			return buildResult(parsed.Thought, iteration, totalTokens, inputTokens, outputTokens,
				runStart, resolvedProvider, modelName, toolCalls, nil)
		}

		toolName := *parsed.Action
		toolInput := parsed.ActionInput
		if toolInput == nil {
			toolInput = map[string]any{}
		}

		tool, ok := toolRegistry.Get(toolName)
		if !ok {
			toolResult := fmt.Sprintf("Error: tool %q is not available to this agent.", toolName)
			log.Warn("agent requested unavailable tool", zap.String("tool", toolName))

			messages = append(messages, llm.Message{
				Role:    llm.RoleUser,
				Content: llm.BuildToolResultMessage(toolName, toolResult),
			})

			toolCalls = append(toolCalls, ToolCallRecord{
				ToolName: toolName,
				Input:    toolInput,
				Err:      fmt.Errorf("tool not available"),
			})
			continue
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
		toolCalls = append(toolCalls, record)

		var toolResultMsg string
		if toolErr != nil {
			toolResultMsg = fmt.Sprintf("Error executing tool: %v", toolErr)
			log.Warn("tool execution error",
				zap.String("tool", toolName),
				zap.Error(toolErr),
			)
		} else {
			toolResultMsg = toolOutput
			log.Info("tool executed",
				zap.String("tool", toolName),
				zap.Int("duration_ms", durationMs),
			)
		}

		messages = append(messages, llm.Message{
			Role:    llm.RoleUser,
			Content: llm.BuildToolResultMessage(toolName, toolResultMsg),
		})
	}

	// Exceeded max iterations without a final answer.
	return buildResult("", MaxIterations, totalTokens, inputTokens, outputTokens,
		runStart, resolvedProvider, modelName, toolCalls,
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
		ModelProvider:  provider,
		ModelName:      model,
		ToolCalls:      toolCalls,
		Err:            err,
	}
}
