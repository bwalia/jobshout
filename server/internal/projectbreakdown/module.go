// Package projectbreakdown turns a rough project idea into a scoped breakdown
// the human reviews before posting.
package projectbreakdown

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/agentmodule"
	"github.com/jobshout/server/internal/agentschema"
	"github.com/jobshout/server/internal/model"
)

// Runner drafts a breakdown. Nil → not configured.
type Runner interface {
	Draft(ctx context.Context, req DraftRequest) (string, error)
}

// DraftRequest is the launch surface.
type DraftRequest struct {
	Idea           string
	BudgetHint     string
	PreferredStack string
}

// Module is the Project Breakdown specialist.
func Module(run Runner) agentmodule.Module {
	return agentmodule.Module{
		Builtin:  model.BuiltinProjectBreakdown,
		Label:    model.AgentNameProjectBreakdown,
		Icon:     "list",
		Hint:     "Turn a rough idea into milestones, skills and a budget band. You review before posting.",
		ChatHint: "To break down a project idea, call agent_execute on Project Breakdown with idea and optional budget_hint / preferred_stack. Never publish the project — the human posts.",
		Schema:   schema(),
		Seed:     Seed,
		Launch:   launch(run),
		AbsorbPrompt: func(prompt string, vals map[string]string) {
			if vals["idea"] == "" {
				vals["idea"] = prompt
			}
		},
	}
}

func schema() agentschema.Schema {
	return agentschema.Schema{
		Builtin: model.BuiltinProjectBreakdown,
		Hint:    "Rough idea in → scoped milestones and skills out. Human posts the project.",
		Fields: []agentschema.Field{
			{Key: "idea", Label: "Project idea", Type: "textarea", Required: true, MinLength: 20,
				Placeholder: "We need a member dashboard with messaging…", Question: "What do you want built?"},
			{Key: "budget_hint", Label: "Budget hint (optional)", Type: "text",
				Placeholder: "£2–5k fixed"},
			{Key: "preferred_stack", Label: "Preferred stack (optional)", Type: "text",
				Placeholder: "Rust, Next.js, Postgres"},
		},
		TitleRules: []agentschema.TitleRule{
			{Prefix: "Breakdown: ", FromKey: "idea", Truncate: 60, Fallback: "project"},
		},
		DescRules: []agentschema.DescRule{
			{Key: "budget_hint", Prefix: "Budget: "},
			{Key: "preferred_stack", Prefix: "Stack: "},
			{Key: "idea", Truncate: 400},
		},
	}
}

// Seed is the built-in Project Breakdown agent.
func Seed(orgID uuid.UUID) *model.Agent {
	desc := "Turns a rough project idea into milestones, skills, deliverables and a budget band. The human reviews and posts — this agent never publishes a project."
	prompt := "You break projects into clear milestones and skills. Use only the idea given. Never invent a client brand. Never publish."
	return &model.Agent{
		ID:           uuid.New(),
		OrgID:        orgID,
		Name:         model.AgentNameProjectBreakdown,
		Role:         "Project Breakdown",
		Description:  &desc,
		SystemPrompt: &prompt,
		Status:       "active",
		EngineType:   model.EngineGoNative,
		EngineConfig: map[string]any{},
		Metadata:     map[string]any{model.MetadataKeyBuiltin: model.BuiltinProjectBreakdown},
	}
}

func launch(run Runner) agentmodule.LaunchFunc {
	return func(ctx context.Context, in agentmodule.LaunchInput) (*agentmodule.LaunchOutput, error) {
		if run == nil {
			return nil, fmt.Errorf("Project Breakdown is not configured")
		}
		idea := strings.TrimSpace(in.Values["idea"])
		if idea == "" {
			return nil, fmt.Errorf("idea is required")
		}
		draft, err := run.Draft(ctx, DraftRequest{
			Idea:           idea,
			BudgetHint:     strings.TrimSpace(in.Values["budget_hint"]),
			PreferredStack: strings.TrimSpace(in.Values["preferred_stack"]),
		})
		if err != nil {
			return nil, err
		}
		return &agentmodule.LaunchOutput{
			Description: draft,
			Status:      "done",
			Message:     "Project breakdown ready — review before posting",
			ExtraMeta: map[string]any{
				"draft":      draft,
				"draft_kind": "project_breakdown",
				"ai_confirm": true,
				"human_only": "post_project",
			},
		}, nil
	}
}
