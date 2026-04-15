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

// ── Autonomous Agent Prompts ─────────────────────────────────────────────────

// BuildPlanningPrompt instructs the LLM to decompose a goal into concrete steps.
func BuildPlanningPrompt(agentName, agentRole, goalText string, memories []string) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("You are %s, an AI agent with the role: %s.\n\n", agentName, agentRole))
	sb.WriteString("## Task\n\n")
	sb.WriteString("Break the following goal into a numbered list of concrete execution steps.\n")
	sb.WriteString("Each step should be specific enough that it can be executed by calling a single tool or a small chain of tool calls.\n\n")
	sb.WriteString(fmt.Sprintf("Goal: %s\n\n", goalText))

	if len(memories) > 0 {
		sb.WriteString("## Relevant Memory\n\n")
		for _, m := range memories {
			sb.WriteString("- " + m + "\n")
		}
		sb.WriteString("\n")
	}

	sb.WriteString("## Output Format\n\n")
	sb.WriteString("Respond with ONLY a JSON object — no markdown, no code fences, no explanations.\n")
	sb.WriteString("```\n")
	sb.WriteString(`{"steps":[{"index":0,"description":"...","tool_hint":"optional_tool_name"},{"index":1,"description":"..."}]}`)
	sb.WriteString("\n```\n")

	return sb.String()
}

// BuildReflectionPrompt instructs the LLM to reflect on goal execution.
func BuildReflectionPrompt(agentName, goalText string, observations []string) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("You are %s.\n\n", agentName))
	sb.WriteString("## Reflection Task\n\n")
	sb.WriteString("Review the work done toward the goal below. Summarize:\n")
	sb.WriteString("1. What was accomplished\n")
	sb.WriteString("2. Key findings or results\n")
	sb.WriteString("3. Any issues or improvements for next time\n\n")
	sb.WriteString(fmt.Sprintf("Goal: %s\n\n", goalText))
	sb.WriteString("## Step Observations\n\n")

	for i, obs := range observations {
		sb.WriteString(fmt.Sprintf("Step %d: %s\n", i+1, obs))
	}

	sb.WriteString("\nProvide a concise reflection as plain text (2-4 sentences).\n")

	return sb.String()
}

// BuildIntentPrompt instructs the LLM to classify a user message into a structured action.
func BuildIntentPrompt(userMessage string, availableActions []string) string {
	var sb strings.Builder

	sb.WriteString("You are an intent classifier for an AI agent platform.\n\n")
	sb.WriteString("## Task\n\n")
	sb.WriteString("Classify the user message into one of the supported actions and extract parameters.\n\n")
	sb.WriteString(fmt.Sprintf("User message: %q\n\n", userMessage))

	sb.WriteString("## Supported Actions\n\n")
	for _, a := range availableActions {
		sb.WriteString("- " + a + "\n")
	}

	sb.WriteString("\n## Output Format\n\n")
	sb.WriteString("Respond with ONLY a JSON object:\n")
	sb.WriteString("```\n")
	sb.WriteString(`{"action":"<action_name>","parameters":{"key":"value"},"confidence":0.9}`)
	sb.WriteString("\n```\n\n")

	sb.WriteString("Rules:\n")
	sb.WriteString("- Pick the best matching action.\n")
	sb.WriteString("- If no action matches well, use \"chat\" as the action.\n")
	sb.WriteString("- Extract any relevant parameters (agent name, task title, workflow name, etc.).\n")
	sb.WriteString("- Confidence should be between 0.0 and 1.0.\n")

	return sb.String()
}
