package repository

import (
	"testing"

	"github.com/jobshout/server/internal/model"
)

func ptr(s string) *string { return &s }

// boardActivity decides which agent-board column an agent's card lands in.
// Every value it can return must have a matching column in the frontend's
// COLUMNS array, or the card is silently dropped from the board.
func TestBoardActivity(t *testing.T) {
	tests := []struct {
		name    string
		kind    *string
		status  *string
		stepKey *string
		want    string
	}{
		// No activity of either kind.
		{"agent has never worked", nil, nil, nil, model.ActivityIdle},
		{"kind without status", ptr("job"), nil, nil, model.ActivityIdle},

		// Multi-agent collaboration — unchanged behaviour.
		{"job planning", ptr("job"), ptr(model.MultiAgentStatusPlanning), nil, model.ActivityPlanning},
		{"job executing", ptr("job"), ptr(model.MultiAgentStatusExecuting), nil, model.ActivityExecuting},
		{"job reviewing", ptr("job"), ptr(model.MultiAgentStatusReviewing), nil, model.ActivityReviewing},
		{"job failed", ptr("job"), ptr(model.MultiAgentStatusFailed), nil, model.ActivityFailed},
		{"job completed frees the agent", ptr("job"), ptr(model.MultiAgentStatusCompleted), nil, model.ActivityIdle},
		{"job pending", ptr("job"), ptr(model.MultiAgentStatusPending), nil, model.ActivityIdle},

		// Article generation.
		{"blog writing", ptr("blog"), ptr(model.BlogRunStatusRunning), ptr(model.BlogStepGenerating), model.ActivityExecuting},
		{"blog running with no step yet", ptr("blog"), ptr(model.BlogRunStatusRunning), nil, model.ActivityExecuting},
		{"blog converting", ptr("blog"), ptr(model.BlogRunStatusRunning), ptr(model.BlogStepConverting), model.ActivityExecuting},
		{"blog posting to the CMS", ptr("blog"), ptr(model.BlogRunStatusRunning), ptr(model.BlogStepPublishing), model.ActivityPublishing},
		{"blog failed", ptr("blog"), ptr(model.BlogRunStatusFailed), nil, model.ActivityFailed},
		{"blog completed frees the agent", ptr("blog"), ptr(model.BlogRunStatusCompleted), nil, model.ActivityIdle},
		{"blog pending", ptr("blog"), ptr(model.BlogRunStatusPending), nil, model.ActivityIdle},

		// A source we do not know about must not invent a column.
		{"unknown kind", ptr("mystery"), ptr("running"), nil, model.ActivityIdle},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := boardActivity(tt.kind, tt.status, tt.stepKey); got != tt.want {
				t.Errorf("boardActivity() = %q, want %q", got, tt.want)
			}
		})
	}
}

// Guard against a column being added on the server with no matching column in
// the UI. If this list changes, web/nextjs/app/(app)/agent-board/page.tsx must
// change with it.
func TestBoardActivityValuesAreKnownColumns(t *testing.T) {
	columns := map[string]bool{
		model.ActivityIdle:       true,
		model.ActivityPlanning:   true,
		model.ActivityExecuting:  true,
		model.ActivityReviewing:  true,
		model.ActivityPublishing: true,
		model.ActivityFailed:     true,
	}

	kinds := []string{"job", "blog"}
	statuses := []string{
		model.MultiAgentStatusPending, model.MultiAgentStatusPlanning,
		model.MultiAgentStatusExecuting, model.MultiAgentStatusReviewing,
		model.MultiAgentStatusCompleted, model.MultiAgentStatusFailed,
		model.BlogRunStatusPending, model.BlogRunStatusRunning,
		model.BlogRunStatusCompleted, model.BlogRunStatusFailed,
	}
	steps := []*string{
		nil,
		ptr(model.BlogStepQueued), ptr(model.BlogStepGenerating), ptr(model.BlogStepConverting),
		ptr(model.BlogStepGenerated), ptr(model.BlogStepPublishing), ptr(model.BlogStepPublished),
	}

	for _, k := range kinds {
		for _, s := range statuses {
			for _, step := range steps {
				got := boardActivity(&k, &s, step)
				if !columns[got] {
					t.Errorf("boardActivity(%q,%q,%v) = %q, which has no board column", k, s, step, got)
				}
			}
		}
	}
}
