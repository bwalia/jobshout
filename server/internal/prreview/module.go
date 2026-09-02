package prreview

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/agentmodule"
	"github.com/jobshout/server/internal/agentschema"
	"github.com/jobshout/server/internal/model"
)

// Runner is the launch surface. ReviewService satisfies it.
type Runner interface {
	CreateRun(ctx context.Context, req model.CreateReviewRunRequest, orgID uuid.UUID, requestedBy *uuid.UUID) (*model.ReviewRun, error)
}

// Module is the PR Reviewer specialist.
//
// All specialists are wired this way: own package, then one Register call.
func Module(run Runner) agentmodule.Module {
	return agentmodule.Module{
		Builtin:  model.BuiltinPRReviewer,
		Label:    "PR Reviewer",
		Icon:     "git-pull-request",
		TabSlug:  "review",
		Hint:     "Queue an AI review of a GitHub pull request.",
		ChatHint: "To review a GitHub pull request, call review_pull_request (or agent_execute on the PR Reviewer). Default dry_run=true so nothing is posted. Poll with review_run_get until status is completed or failed.",
		Schema:   schema(),
		Seed:     Seed,
		Launch:   launch(run),
	}
}

func schema() agentschema.Schema {
	return agentschema.Schema{
		Builtin:        model.BuiltinPRReviewer,
		SpecialistTool: "review_pull_request",
		Hint:           "Queue an AI review of a GitHub pull request.",
		Fields: []agentschema.Field{
			{Key: "repo", Label: "Repository", Type: "repo", Required: true, MinLength: 3, Placeholder: "owner/name", Question: "Which GitHub repo should I review? Use owner/name."},
			{Key: "pr_number", Label: "Pull request number", Type: "number", Required: true, Min: 1, Placeholder: "e.g. 128", Question: "Which pull request number?"},
			{Key: "dry_run", Label: "Preview only — do not post comments on GitHub", Type: "checkbox", Default: "true"},
		},
		TitleRules: []agentschema.TitleRule{
			{Prefix: "Review: ", Format: "{repo}#{pr_number}"},
		},
		DescRules: []agentschema.DescRule{
			{Format: "Review {repo}#{pr_number}", SuffixIf: "dry_run", Suffix: " (preview only)"},
		},
	}
}

// Seed is the built-in PR Reviewer.
func Seed(orgID uuid.UUID) *model.Agent {
	desc := "Reviews GitHub pull requests with a local coder model via OpenCode: explores the repo around the diff, then posts MERGE or FIX."
	prompt := "You are a senior engineer reviewing pull requests. Use the review_pull_request tool with a repo slug (owner/name) and PR number. It posts the review to the PR by default; pass dry_run=true only if the user explicitly asks for a preview that posts nothing. Summarise the verdict and blocking findings first."
	return &model.Agent{
		ID:           uuid.New(),
		OrgID:        orgID,
		Name:         "PR Reviewer",
		Role:         "Pull Request Review Agent",
		Description:  &desc,
		SystemPrompt: &prompt,
		Status:       "active",
		EngineType:   model.EngineGoNative,
		EngineConfig: map[string]any{},
		Metadata:     map[string]any{model.MetadataKeyBuiltin: model.BuiltinPRReviewer},
	}
}

func launch(run Runner) agentmodule.LaunchFunc {
	return func(ctx context.Context, in agentmodule.LaunchInput) (*agentmodule.LaunchOutput, error) {
		if run == nil {
			return nil, fmt.Errorf("PR review is not configured")
		}
		pr, _ := strconv.Atoi(strings.TrimSpace(in.Values["pr_number"]))
		dry := in.Values["dry_run"] != "false"
		created, err := run.CreateRun(ctx, model.CreateReviewRunRequest{
			Repo:     strings.TrimSpace(in.Values["repo"]),
			PRNumber: pr,
			DryRun:   &dry,
			AgentID:  &in.Agent.ID,
			TaskID:   &in.Task.ID,
		}, in.OrgID, &in.UserID)
		if err != nil {
			return nil, err
		}
		id := created.ID
		return &agentmodule.LaunchOutput{
			RunID:     &id,
			Message:   "PR review queued",
			ExtraMeta: map[string]any{model.TaskMetaRunID: id.String()},
		}, nil
	}
}
