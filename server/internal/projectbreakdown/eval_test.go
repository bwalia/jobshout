package projectbreakdown_test

import (
	"context"
	"strings"
	"testing"

	"github.com/jobshout/server/internal/projectbreakdown"
)

func TestEvalDoesNotPublish(t *testing.T) {
	draft, err := projectbreakdown.Heuristic{}.Draft(context.Background(), projectbreakdown.DraftRequest{
		Idea: "Mobile app for field engineers",
	})
	if err != nil {
		t.Fatal(err)
	}
	low := strings.ToLower(draft)
	if strings.Contains(low, "published") || strings.Contains(low, "project is live") {
		t.Fatalf("draft claims publish: %s", draft)
	}
}
