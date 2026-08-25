package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/model"
)

// ReviewStarter queues a PR review. Declared here so tests can fake it without
// importing the service package (which imports the executor, which imports tools).
type ReviewStarter interface {
	CreateRun(ctx context.Context, req model.CreateReviewRunRequest, orgID uuid.UUID, requestedBy *uuid.UUID) (*model.ReviewRun, error)
}

type reviewPullRequestTool struct {
	starter ReviewStarter
}

func NewReviewPullRequestTool(starter ReviewStarter) Tool {
	return &reviewPullRequestTool{starter: starter}
}

func (t *reviewPullRequestTool) Name() string { return "review_pull_request" }

func (t *reviewPullRequestTool) Description() string {
	return `Queue an AI review of a GitHub pull request. Returns immediately with a run id; poll by reading the run in JobShout. Posts the review to the PR by default; pass dry_run=true for a preview that posts nothing.

Input parameters:
  repo (string, required) — owner/name, e.g. bwalia/jobshout
  pr_number (integer, required) — pull request number
  dry_run (boolean, optional) — false (default) posts to GitHub; true prints findings only
  force (boolean, optional) — re-review even if this commit was already started`
}

func (t *reviewPullRequestTool) Parameters() ParameterSchema {
	return ObjectSchema(map[string]any{
		"repo":      map[string]any{"type": "string", "description": "GitHub repo as owner/name"},
		"pr_number": map[string]any{"type": "integer", "description": "Pull request number"},
		"dry_run":   map[string]any{"type": "boolean", "description": "If true, do not post to GitHub"},
		"force":     map[string]any{"type": "boolean", "description": "Re-review even if already started"},
	}, "repo", "pr_number")
}

func (t *reviewPullRequestTool) Execute(ctx context.Context, input map[string]any) (string, error) {
	if t.starter == nil {
		return "", fmt.Errorf("PR review is not configured on this ring")
	}
	orgID, ok := OrgFromContext(ctx)
	if !ok {
		return "", fmt.Errorf("no organization in context")
	}
	repo, err := stringParam(input, "repo", true)
	if err != nil {
		return "", err
	}
	pr := intParam(input, "pr_number", 0)
	if pr < 1 {
		return "", fmt.Errorf("pr_number must be a positive integer")
	}
	dry := false
	if v, ok := input["dry_run"]; ok && v != nil {
		if b, ok := v.(bool); ok {
			dry = b
		}
	}
	force := false
	if v, ok := input["force"]; ok {
		if b, ok := v.(bool); ok {
			force = b
		}
	}
	req := model.CreateReviewRunRequest{
		Repo:     repo,
		PRNumber: pr,
		DryRun:   &dry,
		Force:    force,
	}
	if agentID, ok := AgentFromContext(ctx); ok {
		req.AgentID = &agentID
	}
	run, err := t.starter.CreateRun(ctx, req, orgID, nil)
	if err != nil {
		return "", err
	}
	out, _ := json.Marshal(map[string]any{
		"id":      run.ID.String(),
		"status":  run.Status,
		"repo":    run.Repo,
		"pr":      run.PRNumber,
		"dry_run": run.DryRun,
	})
	return string(out), nil
}
