// Package bridge provides enhanced Go ↔ Python bridge types with streaming support.
package bridge

// StreamEvent is a single Server-Sent Event emitted during execution.
type StreamEvent struct {
	Type string `json:"type"` // "thought" | "tool_call" | "tool_result" | "node_start" | "node_end" | "final_answer" | "error"
	Data any    `json:"data"`
}

// ThoughtEvent carries a reasoning step.
type ThoughtEvent struct {
	Iteration int    `json:"iteration"`
	Thought   string `json:"thought"`
}

// ToolCallEvent carries a tool invocation.
type ToolCallEvent struct {
	ToolName string         `json:"tool_name"`
	Input    map[string]any `json:"input"`
}

// ToolResultEvent carries the result of a tool call.
type ToolResultEvent struct {
	ToolName   string `json:"tool_name"`
	Output     string `json:"output"`
	Error      string `json:"error,omitempty"`
	DurationMs int    `json:"duration_ms"`
}

// NodeEvent carries a LangGraph node transition.
type NodeEvent struct {
	NodeName   string         `json:"node_name"`
	StepNumber int            `json:"step_number"`
	State      map[string]any `json:"state,omitempty"`
}

// FinalAnswerEvent carries the completed execution result.
type FinalAnswerEvent struct {
	Answer      string `json:"answer"`
	TotalTokens int    `json:"total_tokens"`
	Iterations  int    `json:"iterations"`
}

// ErrorEvent carries an execution error.
type ErrorEvent struct {
	Message string `json:"message"`
}
