package research

import (
	"context"
	"strings"
	"testing"
)

func TestEval_OpenWebUsableBrief(t *testing.T) {
	backend := defaultBackend()
	model := &scriptedLLM{responses: []scriptedResponse{
		{trigger: "planning research", content: `{"queries": ["kubernetes cost optimisation"]}`},
		{trigger: "extracting citable facts", content: `{"findings": [
			{"claim": "Gateway API reached GA in Kubernetes 1.31.",
			 "quote": "Kubernetes 1.31 promoted the Gateway API to general availability after an"}
		]}`},
		{trigger: "fact-checking citations", content: `{"verdicts": [{"index": 0, "supported": true}]}`},
		{trigger: "Summarise the current state", content: "Gateway API is generally available."},
	}}
	agent := newTestAgent(t, backend, model)

	brief, err := agent.Research(context.Background(), Request{
		Topic:   "Kubernetes cost optimisation",
		Context: "Focus on control-plane APIs that affect cluster cost.",
	}, nil)
	if err != nil {
		t.Fatalf("Research: %v", err)
	}
	if !brief.IsUsable() {
		t.Fatalf("brief not usable: findings=%d sources=%d warnings=%v",
			len(brief.Findings), len(brief.Sources), brief.Warnings)
	}
	if len(brief.Findings) == 0 || brief.Findings[0].Quote == "" {
		t.Fatal("expected a quoted finding")
	}
	if !strings.Contains(testDocText, brief.Findings[0].Quote) {
		t.Errorf("quote not in fetched text: %q", brief.Findings[0].Quote)
	}
}

func TestEval_PinnedURLsSkipSearch(t *testing.T) {
	backend := defaultBackend()
	model := &scriptedLLM{responses: []scriptedResponse{
		{trigger: "You extract facts from one knowledge page", content: `{"findings": [{"claim": "Gateway API reached GA in Kubernetes 1.31.", "quote": "Kubernetes 1.31 promoted the Gateway API to general availability after an extended beta period."}]}`},
		{trigger: "fact-checking citations", content: `{"verdicts": [{"index": 0, "supported": true}]}`},
		{trigger: "Summarise the current state", content: "Gateway API is GA."},
	}}
	agent := newTestAgent(t, backend, model)

	brief, err := agent.Research(context.Background(), pinnedMailRequest(
		"Gateway API status",
		"Latest k8s?",
		"What is the current Gateway API status?",
		[]string{"https://kubernetes.io/blog/gateway-ga"},
	), nil)
	if err != nil {
		t.Fatal(err)
	}
	if backend.searchCalls() != 0 {
		t.Fatalf("search called %d times", backend.searchCalls())
	}
	fetched := backend.fetchURLs()
	if len(fetched) != 1 || fetched[0] != "https://kubernetes.io/blog/gateway-ga" {
		t.Fatalf("fetched %v", fetched)
	}
	if !brief.IsUsable() {
		t.Fatalf("pinned brief not usable: %+v", brief)
	}
}

func TestEval_UnreadableURLWarns(t *testing.T) {
	backend := &fixedBackend{
		docs: map[string]*Document{},
		fetchErrs: map[string]error{
			"https://missing.example/page": errUnreadable,
		},
	}
	agent := newTestAgent(t, backend, &scriptedLLM{})

	brief, err := agent.Research(context.Background(), Request{
		Topic: "missing page",
		URLs:  []string{"https://missing.example/page"},
	}, nil)
	if err != nil {
		t.Fatalf("unreadable URL must not panic/error the agent: %v", err)
	}
	if brief == nil {
		t.Fatal("expected a brief")
	}
	if brief.IsUsable() {
		t.Fatal("unreadable-only request should not be usable")
	}
	if len(brief.Warnings) == 0 {
		t.Fatal("expected warnings about the unreadable URL")
	}
}

func TestEval_MailShapedContextYieldsBrief(t *testing.T) {
	backend := defaultBackend()
	model := &scriptedLLM{responses: []scriptedResponse{
		{trigger: "You extract facts from one knowledge page", content: `{"findings": [{"claim": "Gateway API reached GA in Kubernetes 1.31.", "quote": "Kubernetes 1.31 promoted the Gateway API to general availability after an extended beta period."}]}`},
		{trigger: "fact-checking citations", content: `{"verdicts": [{"index": 0, "supported": true}]}`},
		{trigger: "Summarise the current state", content: "Gateway API is GA."},
	}}
	agent := newTestAgent(t, backend, model)

	brief, err := agent.Research(context.Background(), pinnedMailRequest(
		"",
		"Price of the Gateway add-on?",
		"A client asked about Gateway API status in this email body.",
		[]string{"https://kubernetes.io/blog/gateway-ga"},
	), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !brief.IsUsable() {
		t.Fatalf("mail-shaped brief not usable: warnings=%v", brief.Warnings)
	}
}

var errUnreadable = errString("fetch failed")

type errString string

func (e errString) Error() string { return string(e) }
