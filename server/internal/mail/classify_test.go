package mail

import (
	"strings"
	"testing"
)

func fixtureSupportQuestion() InboxMessage {
	return InboxMessage{
		GmailThreadID: "t1",
		FromEmail:     "alex@customer.com",
		FromName:      "Alex",
		ToEmail:       "hello@org.com",
		Subject:       "What's the latest Kubernetes version you support?",
		Body:          "Hi — can you tell us the current version and link the changelog? Thanks.",
	}
}

func fixtureNewsletter() InboxMessage {
	return InboxMessage{
		GmailThreadID: "t2",
		FromEmail:     "news@vendor.example",
		Subject:       "This week's digest",
		Body:          "View in browser. Click unsubscribe if you no longer want this newsletter.",
	}
}

func TestBuildClassifyPromptIncludesSubjectAndBody(t *testing.T) {
	p := BuildClassifyPrompt(fixtureSupportQuestion())
	for _, want := range []string{"Kubernetes", "changelog", "alex@customer.com", "needs_research"} {
		if !strings.Contains(p, want) {
			t.Errorf("prompt missing %q", want)
		}
	}
}

func TestParseClassifyResultNeedsResearch(t *testing.T) {
	reply := `{
	  "intent": "question",
	  "needs_research": true,
	  "urgency": "normal",
	  "suggested_action": "reply",
	  "reason": "They asked for a current version.",
	  "triage_label": "support"
	}`
	got, err := ParseClassifyResult(reply)
	if err != nil {
		t.Fatal(err)
	}
	if !got.NeedsResearch {
		t.Fatal("expected needs_research")
	}
	if got.SuggestedAction != "reply" {
		t.Errorf("action %q", got.SuggestedAction)
	}
}

func TestParseClassifyResultIgnoresProseFence(t *testing.T) {
	reply := "Sure.\n```json\n{\"intent\":\"fyi\",\"needs_research\":false,\"urgency\":\"low\",\"suggested_action\":\"ignore\",\"reason\":\"newsletter\",\"triage_label\":\"newsletter\"}\n```"
	got, err := ParseClassifyResult(reply)
	if err != nil {
		t.Fatal(err)
	}
	if got.SuggestedAction != "ignore" {
		t.Errorf("action %q", got.SuggestedAction)
	}
}

func TestHeuristicClassifyNewsletterIgnored(t *testing.T) {
	got := HeuristicClassify(fixtureNewsletter())
	if got.SuggestedAction != "ignore" {
		t.Errorf("action %q, want ignore", got.SuggestedAction)
	}
	if got.NeedsResearch {
		t.Error("newsletter should not need research")
	}
}

func TestHeuristicClassifyQuestionNeedsResearch(t *testing.T) {
	got := HeuristicClassify(fixtureSupportQuestion())
	if !got.NeedsResearch {
		t.Error("expected needs_research for a current-version question")
	}
	if got.SuggestedAction != "reply" {
		t.Errorf("action %q", got.SuggestedAction)
	}
}
