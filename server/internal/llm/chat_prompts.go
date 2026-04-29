package llm

import (
	"fmt"
	"strings"
)

// ChatCapability identifies the flat set of high-level actions the chat router
// understands. Keep this list in sync with the ChatIntent "intent" field
// produced by BuildChatIntentPrompt and accepted by BuildPolicyCheckPrompt.
const (
	ChatIntentRunTask      = "run_task"
	ChatIntentCreateTask   = "create_task"
	ChatIntentListAgents   = "list_agents"
	ChatIntentListTasks    = "list_tasks"
	ChatIntentRunWorkflow  = "run_workflow"
	ChatIntentGetStatus    = "get_status"
	ChatIntentHelp         = "help"
	ChatIntentClarify      = "clarify"
)

// ChatCapabilities lists the supported intents in a fixed order so downstream
// prompts render deterministically.
var ChatCapabilities = []string{
	ChatIntentRunTask,
	ChatIntentCreateTask,
	ChatIntentListAgents,
	ChatIntentListTasks,
	ChatIntentRunWorkflow,
	ChatIntentGetStatus,
	ChatIntentHelp,
	ChatIntentClarify,
}

// AgentSummary is the minimal shape of an agent the chat-intent prompts need.
type AgentSummary struct {
	Name        string
	Role        string
	Description string
}

// AgentWithMetrics extends AgentSummary with the signals used by the agent
// selector prompt.
type AgentWithMetrics struct {
	AgentSummary
	SuccessRate float64
	LatencyMs   int
	CostUSD     float64
}

// WorkflowSummary is the minimal shape of a workflow the chat-intent prompts need.
type WorkflowSummary struct {
	Name        string
	Description string
}

// TaskSummary is the minimal shape of a task the status prompt needs.
type TaskSummary struct {
	ID    string
	Title string
}

// BuildChatSystemPrompt produces the global guardrails prompt used as the
// {{system_prompt}} placeholder across the chat router pipeline.
func BuildChatSystemPrompt() string {
	return `You are JobShout AI, an enterprise task orchestration assistant.

Your job:
- Understand user intent from chat messages.
- Map requests to agents, tasks, or workflows.
- Extract structured parameters.
- NEVER hallucinate unknown agents or tools.
- If unclear, ask for clarification.

Strict rules:
- Always return valid JSON.
- Do not include explanations outside JSON.
- Only use agents/tools/workflows from the provided list.
- If unsure, set intent to "clarify".

Available capabilities: ` + strings.Join(ChatCapabilities, ", ") + `.`
}

// BuildChatIntentPrompt produces the intent-detection prompt. The caller feeds
// the result to the LLM as a user turn after the system prompt.
func BuildChatIntentPrompt(message string, agents []AgentSummary, workflows []WorkflowSummary) string {
	var sb strings.Builder

	sb.WriteString("Analyze the user message and extract the intent.\n\n")
	sb.WriteString("User message:\n")
	sb.WriteString(quote(message))
	sb.WriteString("\n\n")

	sb.WriteString("Available agents:\n")
	writeAgentList(&sb, agents)

	sb.WriteString("\nAvailable workflows:\n")
	writeWorkflowList(&sb, workflows)

	sb.WriteString("\nRules:\n")
	sb.WriteString("- Prefer an existing agent or workflow when the user intent plausibly matches one.\n")
	sb.WriteString("- Extract parameters as a flat object.\n")
	sb.WriteString("- If the request is ambiguous, set \"intent\" to \"clarify\".\n")
	sb.WriteString("- Do not invent agent or workflow names that are not in the lists above.\n\n")

	sb.WriteString("Respond with ONLY a JSON object matching this schema:\n")
	sb.WriteString("```\n")
	sb.WriteString(`{"intent":"run_task|create_task|list_agents|list_tasks|run_workflow|get_status|help|clarify","agent":"agent_name or null","task":"task description","workflow":"workflow_name or null","params":{},"confidence":0.0}`)
	sb.WriteString("\n```")

	return sb.String()
}

// BuildChatToTaskPrompt converts a natural-language request into an executable
// task payload for a specific agent.
func BuildChatToTaskPrompt(message, selectedAgent string) string {
	var sb strings.Builder

	sb.WriteString("Convert the user request into a JobShout task.\n\n")
	sb.WriteString("User request:\n")
	sb.WriteString(quote(message))
	sb.WriteString("\n\n")
	sb.WriteString("Assigned agent: ")
	if selectedAgent == "" {
		sb.WriteString("(none — pick priority based on request only)")
	} else {
		sb.WriteString(selectedAgent)
	}
	sb.WriteString("\n\nRules:\n")
	sb.WriteString("- Be concise; task_name should be kebab-case.\n")
	sb.WriteString("- Extract concrete parameters into \"inputs\".\n")
	sb.WriteString("- Priority must be one of: low, medium, high.\n")
	sb.WriteString("- No text outside JSON.\n\n")
	sb.WriteString("Respond with ONLY a JSON object:\n")
	sb.WriteString("```\n")
	sb.WriteString(`{"task_name":"...","description":"...","inputs":{},"priority":"low|medium|high"}`)
	sb.WriteString("\n```")

	return sb.String()
}

// BuildAgentSelectorPrompt picks the best agent for a task using simple
// efficiency/quality signals.
func BuildAgentSelectorPrompt(taskDescription string, agents []AgentWithMetrics) string {
	var sb strings.Builder

	sb.WriteString("Select the BEST agent for the task below.\n\n")
	sb.WriteString("Task:\n")
	sb.WriteString(quote(taskDescription))
	sb.WriteString("\n\nCandidate agents (name — role — success_rate, latency_ms, cost_usd):\n")

	if len(agents) == 0 {
		sb.WriteString("(none)\n")
	} else {
		for _, a := range agents {
			sb.WriteString(fmt.Sprintf("- %s — %s — success=%.2f, latency=%dms, cost=$%.4f\n",
				a.Name, a.Role, a.SuccessRate, a.LatencyMs, a.CostUSD))
			if a.Description != "" {
				sb.WriteString("    ")
				sb.WriteString(a.Description)
				sb.WriteString("\n")
			}
		}
	}

	sb.WriteString("\nConsider success rate, then latency, then cost (in that priority order).\n\n")
	sb.WriteString("Respond with ONLY a JSON object:\n")
	sb.WriteString("```\n")
	sb.WriteString(`{"agent":"name","reason":"short explanation"}`)
	sb.WriteString("\n```")

	return sb.String()
}

// BuildMultiStepPlannerPrompt breaks a task into a short ordered plan of steps
// annotated with the agent each step should run on.
func BuildMultiStepPlannerPrompt(task string, agents []AgentSummary) string {
	var sb strings.Builder

	sb.WriteString("Break the task into an ordered list of concrete steps.\n\n")
	sb.WriteString("Task:\n")
	sb.WriteString(quote(task))
	sb.WriteString("\n\nAvailable agents:\n")
	writeAgentList(&sb, agents)

	sb.WriteString("\nRules:\n")
	sb.WriteString("- Aim for 2–6 steps.\n")
	sb.WriteString("- Assign each step to one of the listed agents by name.\n")
	sb.WriteString("- Steps should be specific enough to execute as a single prompt.\n\n")
	sb.WriteString("Respond with ONLY a JSON object:\n")
	sb.WriteString("```\n")
	sb.WriteString(`{"steps":[{"step":1,"action":"...","agent":"..."}]}`)
	sb.WriteString("\n```")

	return sb.String()
}

// BuildClarificationPrompt asks the model to pose one follow-up question to
// disambiguate the user's request.
func BuildClarificationPrompt(message string) string {
	var sb strings.Builder

	sb.WriteString("The user message is unclear. Ask ONE concise follow-up question that will let you route the request correctly.\n\n")
	sb.WriteString("User message:\n")
	sb.WriteString(quote(message))
	sb.WriteString("\n\nRespond with ONLY a JSON object:\n")
	sb.WriteString("```\n")
	sb.WriteString(`{"intent":"clarify","question":"..."}`)
	sb.WriteString("\n```")

	return sb.String()
}

// BuildStatusQueryPrompt resolves which task/execution the user is asking
// about from an open-ended status question.
func BuildStatusQueryPrompt(message string, tasks []TaskSummary) string {
	var sb strings.Builder

	sb.WriteString("The user is asking about the status of a task.\n\n")
	sb.WriteString("User message:\n")
	sb.WriteString(quote(message))
	sb.WriteString("\n\nKnown tasks (id — title):\n")
	if len(tasks) == 0 {
		sb.WriteString("(none)\n")
	} else {
		for _, t := range tasks {
			sb.WriteString(fmt.Sprintf("- %s — %s\n", t.ID, t.Title))
		}
	}

	sb.WriteString("\nRules:\n")
	sb.WriteString("- Pick the single best-matching task id from the list.\n")
	sb.WriteString("- If no listed task matches, set task_id to null.\n\n")
	sb.WriteString("Respond with ONLY a JSON object:\n")
	sb.WriteString("```\n")
	sb.WriteString(`{"intent":"get_status","task_id":"..."}`)
	sb.WriteString("\n```")

	return sb.String()
}

// BuildConversationalPrompt resolves follow-up references ("that task",
// "retry it") against recent chat history and returns the same intent shape as
// BuildChatIntentPrompt.
func BuildConversationalPrompt(history []Message, newMessage string, agents []AgentSummary, workflows []WorkflowSummary) string {
	var sb strings.Builder

	sb.WriteString("Resolve the new message against the previous conversation, then classify intent.\n\n")
	sb.WriteString("Previous turns (oldest first):\n")
	if len(history) == 0 {
		sb.WriteString("(none)\n")
	} else {
		for _, m := range history {
			sb.WriteString(fmt.Sprintf("- [%s] %s\n", m.Role, singleLine(m.Content)))
		}
	}

	sb.WriteString("\nNew user message:\n")
	sb.WriteString(quote(newMessage))
	sb.WriteString("\n\nAvailable agents:\n")
	writeAgentList(&sb, agents)
	sb.WriteString("\nAvailable workflows:\n")
	writeWorkflowList(&sb, workflows)

	sb.WriteString("\nRules:\n")
	sb.WriteString("- Replace references like \"that task\", \"retry it\", \"run again\" with the concrete target from history.\n")
	sb.WriteString("- If the reference cannot be resolved, set intent to \"clarify\".\n")
	sb.WriteString("- Do not invent agents or workflows that are not listed.\n\n")
	sb.WriteString("Respond with ONLY a JSON object:\n")
	sb.WriteString("```\n")
	sb.WriteString(`{"intent":"run_task|create_task|list_agents|list_tasks|run_workflow|get_status|help|clarify","agent":"agent_name or null","task":"task description","workflow":"workflow_name or null","params":{},"confidence":0.0}`)
	sb.WriteString("\n```")

	return sb.String()
}

// BuildPolicyCheckPrompt asks the model to decide whether a request is allowed
// under the provided policy list.
func BuildPolicyCheckPrompt(message string, policies []string) string {
	var sb strings.Builder

	sb.WriteString("Decide whether the request below violates any of the listed policies.\n\n")
	sb.WriteString("Request:\n")
	sb.WriteString(quote(message))
	sb.WriteString("\n\nPolicies:\n")
	if len(policies) == 0 {
		sb.WriteString("(none)\n")
	} else {
		for _, p := range policies {
			sb.WriteString("- ")
			sb.WriteString(p)
			sb.WriteString("\n")
		}
	}

	sb.WriteString("\nRespond with ONLY a JSON object:\n")
	sb.WriteString("```\n")
	sb.WriteString(`{"allowed":true,"reason":"..."}`)
	sb.WriteString("\n```")

	return sb.String()
}

// BuildResponseFormatterPrompt turns a raw agent result into a short,
// chat-friendly string. Unlike the other builders this one intentionally does
// NOT request JSON — the caller uses the returned content directly.
func BuildResponseFormatterPrompt(agentResult string) string {
	var sb strings.Builder

	sb.WriteString("Format the agent result below as a short Slack/Telegram-friendly message.\n\n")
	sb.WriteString("Agent result:\n")
	sb.WriteString(quote(agentResult))
	sb.WriteString("\n\nRules:\n")
	sb.WriteString("- 1–4 short lines.\n")
	sb.WriteString("- Lead with the key result; drop filler.\n")
	sb.WriteString("- Plain text, not JSON.\n")
	sb.WriteString("- No code fences.\n")

	return sb.String()
}

// BuildMemoryExtractionPrompt extracts durable facts from a task result that
// are worth saving to long-term memory.
func BuildMemoryExtractionPrompt(taskResult string) string {
	var sb strings.Builder

	sb.WriteString("Extract useful long-term memory items from the task result below.\n\n")
	sb.WriteString("Task result:\n")
	sb.WriteString(quote(taskResult))
	sb.WriteString("\n\nRules:\n")
	sb.WriteString("- Each fact should be standalone and reusable in a future conversation.\n")
	sb.WriteString("- Skip ephemeral status updates or procedure text.\n\n")
	sb.WriteString("Respond with ONLY a JSON object:\n")
	sb.WriteString("```\n")
	sb.WriteString(`{"memory":["fact1","fact2"]}`)
	sb.WriteString("\n```")

	return sb.String()
}

// BuildRetryPlanPrompt suggests whether a failed task should be retried and
// what parameters to adjust before retrying.
func BuildRetryPlanPrompt(errorMessage string) string {
	var sb strings.Builder

	sb.WriteString("A task failed with the error below. Suggest whether to retry, and what parameters to adjust.\n\n")
	sb.WriteString("Error:\n")
	sb.WriteString(quote(errorMessage))
	sb.WriteString("\n\nRules:\n")
	sb.WriteString("- Set \"retry\" to false for clearly-unrecoverable errors (permission denied, policy violation).\n")
	sb.WriteString("- Put any suggested parameter changes into \"adjustments\" as a flat object.\n\n")
	sb.WriteString("Respond with ONLY a JSON object:\n")
	sb.WriteString("```\n")
	sb.WriteString(`{"retry":true,"adjustments":{}}`)
	sb.WriteString("\n```")

	return sb.String()
}

// ── helpers ──────────────────────────────────────────────────────────────────

func writeAgentList(sb *strings.Builder, agents []AgentSummary) {
	if len(agents) == 0 {
		sb.WriteString("(none)\n")
		return
	}
	for _, a := range agents {
		sb.WriteString("- ")
		sb.WriteString(a.Name)
		if a.Role != "" {
			sb.WriteString(" — ")
			sb.WriteString(a.Role)
		}
		sb.WriteString("\n")
		if a.Description != "" {
			sb.WriteString("    ")
			sb.WriteString(singleLine(a.Description))
			sb.WriteString("\n")
		}
	}
}

func writeWorkflowList(sb *strings.Builder, workflows []WorkflowSummary) {
	if len(workflows) == 0 {
		sb.WriteString("(none)\n")
		return
	}
	for _, w := range workflows {
		sb.WriteString("- ")
		sb.WriteString(w.Name)
		if w.Description != "" {
			sb.WriteString(" — ")
			sb.WriteString(singleLine(w.Description))
		}
		sb.WriteString("\n")
	}
}

func quote(s string) string {
	return "\"" + strings.ReplaceAll(s, "\"", "\\\"") + "\""
}

func singleLine(s string) string {
	s = strings.ReplaceAll(s, "\r\n", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	return strings.TrimSpace(s)
}
