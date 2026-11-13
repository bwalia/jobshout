package proposalgen_test

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/agentmodule"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/proposalgen"
)

func TestLaunchDraft(t *testing.T) {
	mod := proposalgen.Module(proposalgen.Heuristic{})
	out, err := mod.Launch(context.Background(), agentmodule.LaunchInput{
		OrgID:  uuid.New(),
		UserID: uuid.New(),
		Agent:  &model.Agent{ID: uuid.New()},
		Values: map[string]string{
			"project_title":       "API audit",
			"project_description": "Need a Rust Axum performance review for a payments API.",
			"skills":              "Rust, Axum",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Status != "done" {
		t.Fatalf("status = %q", out.Status)
	}
	draft, _ := out.ExtraMeta["draft"].(string)
	if !strings.Contains(draft, "API audit") {
		t.Fatalf("draft = %q", draft)
	}
}

func TestLaunchRequiresFields(t *testing.T) {
	_, err := proposalgen.Module(proposalgen.Heuristic{}).Launch(context.Background(), agentmodule.LaunchInput{
		Values: map[string]string{},
	})
	if err == nil || !strings.Contains(err.Error(), "required") {
		t.Fatalf("err = %v", err)
	}
}
