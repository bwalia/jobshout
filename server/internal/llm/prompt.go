package llm

import (
	"fmt"
	"strings"
)

// ToolSpec describes a tool in a format suitable for the ReAct system prompt.
type ToolSpec struct {
	Name        string
	Description string
}

// BuildReActSystemPrompt constructs the system prompt that instructs the model
// to operate in a ReAct (Reasoning + Acting) loop.
//
// The LLM must respond with a single JSON object in one of two forms:
//
//  1. Tool call (when more information or action is needed):
//     {"thought":"...","action":"tool_name","action_input":{...},"final_answer":null}
//
//  2. Final answer (when the task is complete):
//     {"thought":"...","action":null,"action_input":null,"final_answer":"..."}
//
// This structured format works with any instruction-following model.
func BuildReActSystemPrompt(agentName, agentRole, agentSystemPrompt string, tools []ToolSpec) string {
	var sb strings.Builder

	// Agent identity
	sb.WriteString(fmt.Sprintf("You are %s, an AI agent with the role: %s.\n\n", agentName, agentRole))

	if agentSystemPrompt != "" {
		sb.WriteString("Additional context about you:\n")
		sb.WriteString(agentSystemPrompt)
		sb.WriteString("\n\n")
	}

	// ReAct instructions
	sb.WriteString("## Instructions\n\n")
	sb.WriteString("You operate in a Reasoning + Acting (ReAct) loop. For every message you receive, ")
	sb.WriteString("you MUST respond with a single JSON object and nothing else — no markdown, no code fences, ")
	sb.WriteString("no explanations outside the JSON.\n\n")

	sb.WriteString("### Response format when you need to use a tool:\n")
	sb.WriteString("```\n")
	sb.WriteString(`{"thought":"<your reasoning>","action":"<tool_name>","action_input":{<tool parameters>},"final_answer":null}`)
	sb.WriteString("\n```\n\n")

	sb.WriteString("### Response format when you have a final answer:\n")
	sb.WriteString("```\n")
	sb.WriteString(`{"thought":"<your reasoning>","action":null,"action_input":null,"final_answer":"<complete answer>"}`)
	sb.WriteString("\n```\n\n")

	sb.WriteString("Rules:\n")
	sb.WriteString("- Always start with a \"thought\" explaining your reasoning.\n")
	sb.WriteString("- Only call one tool per response.\n")
	sb.WriteString("- Set \"final_answer\" to null when calling a tool, and set \"action\"/\"action_input\" to null when providing a final answer.\n")
	sb.WriteString("- When you have enough information, provide a complete, helpful final answer.\n\n")

	// Available tools
	if len(tools) > 0 {
		sb.WriteString("## Available Tools\n\n")
		for _, t := range tools {
			sb.WriteString(fmt.Sprintf("### %s\n%s\n\n", t.Name, t.Description))
		}
	} else {
		sb.WriteString("## Available Tools\n\nNo tools are available. Reason and answer from your knowledge only.\n\n")
	}

	return sb.String()
}

// BuildTaskUserMessage formats the initial task description as a user turn.
func BuildTaskUserMessage(taskPrompt string) string {
	return "Task: " + taskPrompt
}

// BuildToolResultMessage formats a tool result as a user turn fed back into the loop.
func BuildToolResultMessage(toolName, result string) string {
	return fmt.Sprintf("Tool result for %q:\n%s", toolName, result)
}
