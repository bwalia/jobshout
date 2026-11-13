package projectbreakdown_test

import (
	"context"
	"strings"
	"testing"

	"github.com/jobshout/server/internal/agentmodule"
	"github.com/jobshout/server/internal/projectbreakdown"
)

func TestLaunch(t *testing.T) {
	out, err := projectbreakdown.Module(projectbreakdown.Heuristic{}).Launch(context.Background(), agentmodule.LaunchInput{
		Values: map[string]string{"idea": "Build a dashboard for freelancers with messaging and credits."},
	})
	if err != nil {
		t.Fatal(err)
	}
	draft, _ := out.ExtraMeta["draft"].(string)
	if !strings.Contains(strings.ToLower(draft), "milestone") {
		t.Fatalf("draft = %q", draft)
	}
}
