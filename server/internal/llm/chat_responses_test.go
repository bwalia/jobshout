package llm

import (
	"strings"
	"testing"
)

func TestExtractJSONObjectStripsCodeFences(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"plain", `{"a":1}`, `{"a":1}`},
		{"fenced", "```json\n{\"a\":1}\n```", `{"a":1}`},
		{"bare-fence", "```\n{\"a\":1}\n```", `{"a":1}`},
		{"prose-suffix", "{\"a\":1}\nthanks!", `{"a":1}`},
		{"prose-prefix", "Sure — {\"a\":1}", `{"a":1}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ExtractJSONObject(c.in); got != c.want {
				t.Errorf("ExtractJSONObject(%q) = %q; want %q", c.in, got, c.want)
			}
		})
	}
}

func TestParseChatIntentHappyPath(t *testing.T) {
	raw := `{"intent":"run_task","agent":"devops-agent","task":"debug","workflow":null,"params":{"platform":"kubernetes"},"confidence":0.9}`
	out, err := ParseChatIntent(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Intent != ChatIntentRunTask {
		t.Errorf("intent = %q; want %q", out.Intent, ChatIntentRunTask)
	}
	if out.Agent == nil || *out.Agent != "devops-agent" {
		t.Errorf("agent = %v; want devops-agent", out.Agent)
	}
	if out.Params["platform"] != "kubernetes" {
		t.Errorf("params = %v; want platform=kubernetes", out.Params)
	}
	if out.Confidence != 0.9 {
		t.Errorf("confidence = %v; want 0.9", out.Confidence)
	}
}

func TestParseChatIntentClampsConfidence(t *testing.T) {
	out, err := ParseChatIntent(`{"intent":"clarify","confidence":2.5}`)
	if err != nil {
		t.Fatal(err)
	}
	if out.Confidence != 1.0 {
		t.Errorf("confidence clamp failed: %v", out.Confidence)
	}
}

func TestParseChatIntentRejectsUnknown(t *testing.T) {
	_, err := ParseChatIntent(`{"intent":"delete_prod_db"}`)
	if err == nil {
		t.Error("expected error for unknown intent")
	}
}

func TestParseChatIntentHandlesMalformedJSON(t *testing.T) {
	_, err := ParseChatIntent(`{"intent":`)
	if err == nil {
		t.Error("expected error for malformed JSON")
	}
}

func TestParseChatIntentToleratesProseWrapping(t *testing.T) {
	raw := "Sure — here's my answer:\n```json\n{\"intent\":\"help\",\"confidence\":1.0}\n```\n"
	out, err := ParseChatIntent(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Intent != ChatIntentHelp {
		t.Errorf("intent = %q; want help", out.Intent)
	}
}

func TestParseChatTaskPlanDefaultsPriority(t *testing.T) {
	raw := `{"task_name":"fix-bug","description":"d","inputs":{}}`
	out, err := ParseChatTaskPlan(raw)
	if err != nil {
		t.Fatal(err)
	}
	if out.Priority != "medium" {
		t.Errorf("priority default = %q; want medium", out.Priority)
	}
}

func TestParseChatTaskPlanRejectsInvalidPriority(t *testing.T) {
	_, err := ParseChatTaskPlan(`{"task_name":"x","priority":"yesterday"}`)
	if err == nil {
		t.Error("expected error for invalid priority")
	}
}

func TestParseAgentSelection(t *testing.T) {
	out, err := ParseAgentSelection(`{"agent":"devops-agent","reason":"highest success"}`)
	if err != nil {
		t.Fatal(err)
	}
	if out.Agent != "devops-agent" {
		t.Errorf("agent = %q", out.Agent)
	}
}

func TestParseAgentSelectionRejectsEmpty(t *testing.T) {
	_, err := ParseAgentSelection(`{"agent":"","reason":""}`)
	if err == nil {
		t.Error("expected error for empty agent")
	}
}

func TestParsePlanStepsRequiresAtLeastOne(t *testing.T) {
	if _, err := ParsePlanSteps(`{"steps":[]}`); err == nil {
		t.Error("expected error for empty steps")
	}
	if _, err := ParsePlanSteps(`{"steps":[{"step":1,"action":"a","agent":"x"}]}`); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestParseClarification(t *testing.T) {
	out, err := ParseClarification(`{"intent":"clarify","question":"which agent?"}`)
	if err != nil {
		t.Fatal(err)
	}
	if out.Question != "which agent?" {
		t.Errorf("question = %q", out.Question)
	}
}

func TestParseStatusQueryWithNull(t *testing.T) {
	out, err := ParseStatusQuery(`{"intent":"get_status","task_id":null}`)
	if err != nil {
		t.Fatal(err)
	}
	if out.TaskID != nil {
		t.Errorf("task_id should be nil; got %v", out.TaskID)
	}
	if out.Intent != ChatIntentGetStatus {
		t.Errorf("intent override failed: %q", out.Intent)
	}
}

func TestParsePolicyCheck(t *testing.T) {
	out, err := ParsePolicyCheck(`{"allowed":false,"reason":"destructive op"}`)
	if err != nil {
		t.Fatal(err)
	}
	if out.Allowed || !strings.Contains(out.Reason, "destructive") {
		t.Errorf("policy parse wrong: %+v", out)
	}
}

func TestParseMemoryExtraction(t *testing.T) {
	out, err := ParseMemoryExtraction(`{"memory":["fact one","fact two"]}`)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Memory) != 2 {
		t.Errorf("got %d facts; want 2", len(out.Memory))
	}
}

func TestParseRetryPlanDefaultsAdjustments(t *testing.T) {
	out, err := ParseRetryPlan(`{"retry":true}`)
	if err != nil {
		t.Fatal(err)
	}
	if !out.Retry {
		t.Error("retry should be true")
	}
	if out.Adjustments == nil {
		t.Error("adjustments should default to empty map")
	}
}
