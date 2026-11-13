package proposalgen_test

import (
	"context"
	"strings"
	"testing"

	"github.com/jobshout/server/internal/proposalgen"
)

// Eval-style checks: draft stays on-brief and never claims to have bid.
func TestEvalDraftStaysOnBrief(t *testing.T) {
	draft, err := proposalgen.Heuristic{}.Draft(context.Background(), proposalgen.DraftRequest{
		ProjectTitle:       "Kubernetes cost trim",
		ProjectDescription: "Reduce EKS spend without cutting reliability.",
		Skills:             "Kubernetes, FinOps",
	})
	if err != nil {
		t.Fatal(err)
	}
	low := strings.ToLower(draft)
	if !strings.Contains(low, "kubernetes") {
		t.Fatalf("draft missing topic: %s", draft)
	}
	for _, bad := range []string{"bid submitted", "i have placed", "credits spent"} {
		if strings.Contains(low, bad) {
			t.Fatalf("draft claims action %q: %s", bad, draft)
		}
	}
}
