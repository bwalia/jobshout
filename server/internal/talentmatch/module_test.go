package talentmatch_test

import (
	"context"
	"strings"
	"testing"

	"github.com/jobshout/server/internal/agentmodule"
	"github.com/jobshout/server/internal/talentmatch"
)

func TestLaunch(t *testing.T) {
	out, err := talentmatch.Module(talentmatch.Heuristic{}).Launch(context.Background(), agentmodule.LaunchInput{
		Values: map[string]string{
			"brief":           "Need a Postgres-savvy backend engineer for 8 weeks.",
			"required_skills": "Postgres, Go",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	draft, _ := out.ExtraMeta["draft"].(string)
	if !strings.Contains(strings.ToLower(draft), "talent") {
		t.Fatalf("draft = %q", draft)
	}
}
