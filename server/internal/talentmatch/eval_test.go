package talentmatch_test

import (
	"context"
	"strings"
	"testing"

	"github.com/jobshout/server/internal/talentmatch"
)

func TestEvalNeverInventContact(t *testing.T) {
	draft, err := talentmatch.Heuristic{}.Match(context.Background(), talentmatch.MatchRequest{
		Brief:          "Hire a designer",
		CandidateNotes: "Alex — product design\nSam — brand",
	})
	if err != nil {
		t.Fatal(err)
	}
	low := strings.ToLower(draft)
	if strings.Contains(low, "@") || strings.Contains(low, "mailto") {
		t.Fatalf("draft leaked contact: %s", draft)
	}
}
