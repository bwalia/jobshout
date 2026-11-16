package interviewprep_test

import (
	"context"
	"strings"
	"testing"

	"github.com/jobshout/server/internal/agentmodule"
	"github.com/jobshout/server/internal/interviewprep"
)

func TestLaunchRequiresRole(t *testing.T) {
	_, err := interviewprep.Module(interviewprep.Heuristic{}).Launch(context.Background(), agentmodule.LaunchInput{
		Values: map[string]string{},
	})
	if err == nil || !strings.Contains(err.Error(), "role") {
		t.Fatalf("expected role error, got %v", err)
	}
}

func TestLaunchDraft(t *testing.T) {
	out, err := interviewprep.Module(interviewprep.Heuristic{}).Launch(context.Background(), agentmodule.LaunchInput{
		Values: map[string]string{"role": "Staff Engineer", "company": "Acme"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Status != "done" || !strings.Contains(out.Description, "Staff Engineer") {
		t.Fatalf("unexpected output: %+v", out)
	}
}
