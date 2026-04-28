package llm

import (
	"strings"
	"testing"
)

func TestBuildChatSystemPromptContainsCapabilities(t *testing.T) {
	got := BuildChatSystemPrompt()
	for _, cap := range ChatCapabilities {
		if !strings.Contains(got, cap) {
			t.Errorf("system prompt missing capability %q", cap)
		}
	}
	if !strings.Contains(got, "JobShout AI") {
		t.Error("system prompt missing product identity")
	}
}

func TestBuildChatIntentPromptEmbedsInputs(t *testing.T) {
	agents := []AgentSummary{{Name: "devops-agent", Role: "ops", Description: "handles infra issues"}}
	workflows := []WorkflowSummary{{Name: "incident-triage", Description: "triage incidents"}}

	got := BuildChatIntentPrompt("debug my payment service", agents, workflows)

	for _, needle := range []string{
		"debug my payment service",
		"devops-agent",
		"incident-triage",
		"run_task",
		"clarify",
	} {
		if !strings.Contains(got, needle) {
			t.Errorf("intent prompt missing %q; got:\n%s", needle, got)
		}
	}
}

func TestBuildChatIntentPromptHandlesEmptyLists(t *testing.T) {
	got := BuildChatIntentPrompt("hello", nil, nil)
	if !strings.Contains(got, "(none)") {
		t.Errorf("expected (none) placeholder, got:\n%s", got)
	}
}

func TestBuildChatToTaskPromptIncludesAgent(t *testing.T) {
	got := BuildChatToTaskPrompt("fix the payment latency", "devops-agent")
	if !strings.Contains(got, "devops-agent") {
		t.Error("task prompt missing agent name")
	}
	if !strings.Contains(got, "task_name") {
		t.Error("task prompt missing schema")
	}
	if !strings.Contains(got, "low|medium|high") {
		t.Error("task prompt missing priority enum")
	}
}

func TestBuildAgentSelectorPromptRanksMetrics(t *testing.T) {
	agents := []AgentWithMetrics{
		{AgentSummary: AgentSummary{Name: "fast", Role: "r"}, SuccessRate: 0.9, LatencyMs: 100, CostUSD: 0.001},
		{AgentSummary: AgentSummary{Name: "slow", Role: "r"}, SuccessRate: 0.5, LatencyMs: 500, CostUSD: 0.01},
	}
	got := BuildAgentSelectorPrompt("do a thing", agents)
	if !strings.Contains(got, "success=0.90") {
		t.Errorf("expected success rate in prompt, got:\n%s", got)
	}
	if !strings.Contains(got, "fast") || !strings.Contains(got, "slow") {
		t.Error("expected both agent names in prompt")
	}
}

func TestBuildMultiStepPlannerPromptHasAgents(t *testing.T) {
	got := BuildMultiStepPlannerPrompt("ship feature X", []AgentSummary{{Name: "planner"}, {Name: "exec"}})
	for _, needle := range []string{"ship feature X", "planner", "exec", `"steps"`} {
		if !strings.Contains(got, needle) {
			t.Errorf("planner prompt missing %q", needle)
		}
	}
}

func TestBuildClarificationPrompt(t *testing.T) {
	got := BuildClarificationPrompt("do it")
	if !strings.Contains(got, "do it") {
		t.Error("clarification prompt missing user message")
	}
	if !strings.Contains(got, "\"clarify\"") {
		t.Error("clarification prompt missing intent marker")
	}
}

func TestBuildStatusQueryPromptListsTasks(t *testing.T) {
	got := BuildStatusQueryPrompt("status of payment service", []TaskSummary{
		{ID: "t1", Title: "payment service investigation"},
		{ID: "t2", Title: "unrelated task"},
	})
	for _, needle := range []string{"t1", "t2", "payment service investigation", "get_status"} {
		if !strings.Contains(got, needle) {
			t.Errorf("status prompt missing %q", needle)
		}
	}
}

func TestBuildConversationalPromptIncludesHistory(t *testing.T) {
	history := []Message{
		{Role: RoleUser, Content: "run agent ops-bot"},
		{Role: RoleAssistant, Content: "started execution 1234"},
	}
	got := BuildConversationalPrompt(history, "retry it", nil, nil)
	for _, needle := range []string{"retry it", "ops-bot", "started execution 1234", "[user]", "[assistant]"} {
		if !strings.Contains(got, needle) {
			t.Errorf("conversational prompt missing %q", needle)
		}
	}
}

func TestBuildPolicyCheckPromptListsPolicies(t *testing.T) {
	got := BuildPolicyCheckPrompt("delete prod db", []string{"no destructive prod operations"})
	if !strings.Contains(got, "no destructive prod operations") {
		t.Error("policy prompt missing policy text")
	}
	if !strings.Contains(got, "allowed") {
		t.Error("policy prompt missing schema field")
	}
}

func TestBuildResponseFormatterPromptExcludesJSONMarker(t *testing.T) {
	got := BuildResponseFormatterPrompt("Final answer: deployed")
	if strings.Contains(got, "JSON object") {
		t.Error("response formatter should not ask for JSON")
	}
	if !strings.Contains(got, "Final answer: deployed") {
		t.Error("response formatter missing input")
	}
}

func TestBuildMemoryExtractionPromptSchema(t *testing.T) {
	got := BuildMemoryExtractionPrompt("learned that MSK cluster X has 3 brokers")
	if !strings.Contains(got, `"memory"`) {
		t.Error("memory prompt missing schema field")
	}
}

func TestBuildRetryPlanPromptSchema(t *testing.T) {
	got := BuildRetryPlanPrompt("rate limited by upstream")
	if !strings.Contains(got, `"retry"`) || !strings.Contains(got, `"adjustments"`) {
		t.Errorf("retry prompt missing schema fields: %s", got)
	}
}
