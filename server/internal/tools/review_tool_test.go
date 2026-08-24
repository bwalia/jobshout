package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/model"
)

type fakeReviewStarter struct {
	got   model.CreateReviewRunRequest
	orgID uuid.UUID
	run   *model.ReviewRun
	err   error
}

func (f *fakeReviewStarter) CreateRun(_ context.Context, req model.CreateReviewRunRequest, orgID uuid.UUID, _ *uuid.UUID) (*model.ReviewRun, error) {
	f.got = req
	f.orgID = orgID
	return f.run, f.err
}

func TestReviewPullRequestToolQueuesDryRun(t *testing.T) {
	orgID := uuid.New()
	starter := &fakeReviewStarter{
		run: &model.ReviewRun{
			ID: uuid.New(), Status: "queued", Repo: "bwalia/jobshout", PRNumber: 8, DryRun: true,
		},
	}
	tool := NewReviewPullRequestTool(starter)
	if _, ok := tool.(SchemaProvider); !ok {
		t.Fatal("review_pull_request must advertise a schema")
	}

	out, err := tool.Execute(WithOrg(context.Background(), orgID), map[string]any{
		"repo": "bwalia/jobshout", "pr_number": float64(8),
	})
	if err != nil {
		t.Fatal(err)
	}
	if starter.orgID != orgID {
		t.Fatalf("org = %s", starter.orgID)
	}
	if starter.got.DryRun == nil || !*starter.got.DryRun {
		t.Fatal("dry_run should default true")
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed["status"] != "queued" {
		t.Fatalf("out = %s", out)
	}
}

func TestReviewPullRequestToolRequiresOrg(t *testing.T) {
	tool := NewReviewPullRequestTool(&fakeReviewStarter{})
	_, err := tool.Execute(context.Background(), map[string]any{"repo": "bwalia/jobshout", "pr_number": 1})
	if err == nil {
		t.Fatal("expected error")
	}
}
