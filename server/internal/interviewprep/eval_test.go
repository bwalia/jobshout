package interviewprep_test

import (
	"context"
	"strings"
	"testing"

	"github.com/jobshout/server/internal/interviewprep"
)

func TestHeuristicDraft(t *testing.T) {
	draft, err := interviewprep.Heuristic{}.Draft(context.Background(), "Platform Engineer", "JobShout", "Rust + k8s")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Platform Engineer", "JobShout", "Rust", "draft"} {
		if !strings.Contains(draft, want) {
			t.Fatalf("draft missing %q:\n%s", want, draft)
		}
	}
}
