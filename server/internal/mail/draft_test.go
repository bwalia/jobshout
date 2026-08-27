package mail

import (
	"strings"
	"testing"

	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/research"
)

func TestBuildDraftPromptIncludesResearchAndForbidsSent(t *testing.T) {
	brief := &research.Brief{
		Summary: "Kubernetes 1.31 is the current release.",
		Findings: []research.Finding{{
			Claim:     "1.31 is current",
			SourceURL: "https://kubernetes.io/blog/",
			Quote:     "released 1.31",
		}},
	}
	p := BuildDraftPrompt(fixtureSupportQuestion(), ClassifyResult{
		Intent: "question", SuggestedAction: "reply", Reason: "needs facts",
	}, brief)
	for _, want := range []string{
		"Kubernetes 1.31",
		"https://kubernetes.io/blog/",
		"Do not claim the reply has been sent",
		"alex@customer.com",
	} {
		if !strings.Contains(p, want) {
			t.Errorf("prompt missing %q", want)
		}
	}
}

func TestHeuristicDraftNeverClaimsSent(t *testing.T) {
	d := HeuristicDraft(fixtureSupportQuestion(), nil)
	if d.Status != model.MailDraftDraft {
		t.Errorf("status %q", d.Status)
	}
	lower := strings.ToLower(d.Body)
	if strings.Contains(lower, "i have sent") || strings.Contains(lower, "has been sent") {
		t.Errorf("heuristic draft claimed send: %s", d.Body)
	}
	if !strings.HasPrefix(strings.ToLower(d.Subject), "re:") {
		t.Errorf("subject %q", d.Subject)
	}
}

func TestHeuristicDraftFoldsResearchSummary(t *testing.T) {
	d := HeuristicDraft(fixtureSupportQuestion(), &research.Brief{Summary: "Fact from research."})
	if !strings.Contains(d.Body, "Fact from research.") {
		t.Errorf("research summary missing: %s", d.Body)
	}
}
