package llm

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ChatIntent is the structured output of BuildChatIntentPrompt and
// BuildConversationalPrompt.
type ChatIntent struct {
	Intent     string         `json:"intent"`
	Agent      *string        `json:"agent,omitempty"`
	Task       string         `json:"task,omitempty"`
	Workflow   *string        `json:"workflow,omitempty"`
	Params     map[string]any `json:"params,omitempty"`
	Confidence float64        `json:"confidence"`
}

// ChatTaskPlan is the structured output of BuildChatToTaskPrompt.
type ChatTaskPlan struct {
	TaskName    string         `json:"task_name"`
	Description string         `json:"description"`
	Inputs      map[string]any `json:"inputs,omitempty"`
	Priority    string         `json:"priority"`
}

// AgentSelection is the structured output of BuildAgentSelectorPrompt.
type AgentSelection struct {
	Agent  string `json:"agent"`
	Reason string `json:"reason"`
}

// PlanStep is a single step in the multi-step planner output.
type PlanStep struct {
	Step   int    `json:"step"`
	Action string `json:"action"`
	Agent  string `json:"agent,omitempty"`
}

// PlanSteps wraps the steps array produced by BuildMultiStepPlannerPrompt.
type PlanSteps struct {
	Steps []PlanStep `json:"steps"`
}

// ClarificationResult is the structured output of BuildClarificationPrompt.
type ClarificationResult struct {
	Intent   string `json:"intent"`
	Question string `json:"question"`
}

// StatusQueryResult is the structured output of BuildStatusQueryPrompt.
// TaskID is a pointer so the model can explicitly signal "no match" via null.
type StatusQueryResult struct {
	Intent string  `json:"intent"`
	TaskID *string `json:"task_id"`
}

// PolicyCheckResult is the structured output of BuildPolicyCheckPrompt.
type PolicyCheckResult struct {
	Allowed bool   `json:"allowed"`
	Reason  string `json:"reason,omitempty"`
}

// MemoryExtraction is the structured output of BuildMemoryExtractionPrompt.
type MemoryExtraction struct {
	Memory []string `json:"memory"`
}

// RetryPlan is the structured output of BuildRetryPlanPrompt.
type RetryPlan struct {
	Retry       bool           `json:"retry"`
	Adjustments map[string]any `json:"adjustments,omitempty"`
}

// ── parsers ──────────────────────────────────────────────────────────────────

// ParseChatIntent decodes the JSON produced by BuildChatIntentPrompt.
func ParseChatIntent(raw string) (*ChatIntent, error) {
	var out ChatIntent
	if err := decodeJSON(raw, &out); err != nil {
		return nil, err
	}
	if out.Intent == "" {
		return nil, fmt.Errorf("chat_intent: empty intent")
	}
	if out.Params == nil {
		out.Params = map[string]any{}
	}
	if !validChatIntent(out.Intent) {
		return nil, fmt.Errorf("chat_intent: unknown intent %q", out.Intent)
	}
	if out.Confidence < 0 {
		out.Confidence = 0
	}
	if out.Confidence > 1 {
		out.Confidence = 1
	}
	return &out, nil
}

// ParseChatTaskPlan decodes the JSON produced by BuildChatToTaskPrompt.
func ParseChatTaskPlan(raw string) (*ChatTaskPlan, error) {
	var out ChatTaskPlan
	if err := decodeJSON(raw, &out); err != nil {
		return nil, err
	}
	if out.TaskName == "" {
		return nil, fmt.Errorf("chat_task_plan: empty task_name")
	}
	switch out.Priority {
	case "low", "medium", "high":
	case "":
		out.Priority = "medium"
	default:
		return nil, fmt.Errorf("chat_task_plan: invalid priority %q", out.Priority)
	}
	if out.Inputs == nil {
		out.Inputs = map[string]any{}
	}
	return &out, nil
}

// ParseAgentSelection decodes the JSON produced by BuildAgentSelectorPrompt.
func ParseAgentSelection(raw string) (*AgentSelection, error) {
	var out AgentSelection
	if err := decodeJSON(raw, &out); err != nil {
		return nil, err
	}
	if out.Agent == "" {
		return nil, fmt.Errorf("agent_selection: empty agent")
	}
	return &out, nil
}

// ParsePlanSteps decodes the JSON produced by BuildMultiStepPlannerPrompt.
func ParsePlanSteps(raw string) (*PlanSteps, error) {
	var out PlanSteps
	if err := decodeJSON(raw, &out); err != nil {
		return nil, err
	}
	if len(out.Steps) == 0 {
		return nil, fmt.Errorf("plan_steps: no steps returned")
	}
	return &out, nil
}

// ParseClarification decodes the JSON produced by BuildClarificationPrompt.
func ParseClarification(raw string) (*ClarificationResult, error) {
	var out ClarificationResult
	if err := decodeJSON(raw, &out); err != nil {
		return nil, err
	}
	if out.Question == "" {
		return nil, fmt.Errorf("clarification: empty question")
	}
	out.Intent = ChatIntentClarify
	return &out, nil
}

// ParseStatusQuery decodes the JSON produced by BuildStatusQueryPrompt.
func ParseStatusQuery(raw string) (*StatusQueryResult, error) {
	var out StatusQueryResult
	if err := decodeJSON(raw, &out); err != nil {
		return nil, err
	}
	out.Intent = ChatIntentGetStatus
	return &out, nil
}

// ParsePolicyCheck decodes the JSON produced by BuildPolicyCheckPrompt.
func ParsePolicyCheck(raw string) (*PolicyCheckResult, error) {
	var out PolicyCheckResult
	if err := decodeJSON(raw, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ParseMemoryExtraction decodes the JSON produced by BuildMemoryExtractionPrompt.
func ParseMemoryExtraction(raw string) (*MemoryExtraction, error) {
	var out MemoryExtraction
	if err := decodeJSON(raw, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ParseRetryPlan decodes the JSON produced by BuildRetryPlanPrompt.
func ParseRetryPlan(raw string) (*RetryPlan, error) {
	var out RetryPlan
	if err := decodeJSON(raw, &out); err != nil {
		return nil, err
	}
	if out.Adjustments == nil {
		out.Adjustments = map[string]any{}
	}
	return &out, nil
}

// ── helpers ──────────────────────────────────────────────────────────────────

// ExtractJSONObject pulls the outermost {...} block out of a model response
// and strips code fences. Exposed so callers can reuse the same cleanup when
// layering custom schemas on top of the builders here.
func ExtractJSONObject(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	s = strings.TrimSpace(s)

	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start == -1 || end == -1 || end < start {
		return s
	}
	return s[start : end+1]
}

func decodeJSON(raw string, v any) error {
	cleaned := ExtractJSONObject(raw)
	if cleaned == "" {
		return fmt.Errorf("llm: empty response")
	}
	if err := json.Unmarshal([]byte(cleaned), v); err != nil {
		return fmt.Errorf("llm: decode json: %w (raw=%q)", err, truncate(raw, 200))
	}
	return nil
}

func validChatIntent(intent string) bool {
	for _, c := range ChatCapabilities {
		if c == intent {
			return true
		}
	}
	return false
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
