// Package teambuilder suggests human+agent team compositions for a brief.
package teambuilder

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/agentmodule"
	"github.com/jobshout/server/internal/agentschema"
	"github.com/jobshout/server/internal/model"
)

type Runner interface {
	Draft(ctx context.Context, brief string) (string, error)
}

type Heuristic struct{}

func (Heuristic) Draft(_ context.Context, brief string) (string, error) {
	return "## Team builder\n\n### Brief\n" + strings.TrimSpace(brief) +
		"\n\n### Suggested shape\n- 1 lead (human)\n- 1 specialist (human or hireable showcase agent)\n- 1 reviewer\n\n" +
		"Invite people yourself — AI never forms a team unilaterally.\n", nil
}

func Module(run Runner) agentmodule.Module {
	return agentmodule.Module{
		Builtin:  model.BuiltinTeamBuilder,
		Label:    model.AgentNameTeamBuilder,
		Icon:     "users",
		Hint:     "Suggest a human+agent team for a brief. You invite.",
		ChatHint: "To propose a team, call agent_execute on Team Builder with brief. Never invite members.",
		Schema: agentschema.Schema{
			Builtin: model.BuiltinTeamBuilder,
			Fields: []agentschema.Field{
				{Key: "brief", Label: "Brief", Type: "textarea", Required: true, MinLength: 15,
					Question: "What should the team deliver?"},
			},
			TitleRules: []agentschema.TitleRule{{Prefix: "Team: ", FromKey: "brief", Truncate: 50, Fallback: "brief"}},
		},
		Seed:   Seed,
		Launch: launch(run),
		AbsorbPrompt: func(prompt string, vals map[string]string) {
			if vals["brief"] == "" {
				vals["brief"] = prompt
			}
		},
	}
}

func Seed(orgID uuid.UUID) *model.Agent {
	desc := "Suggests human and agent team shapes for a delivery brief. The human invites."
	prompt := "You propose team shapes. Never invent named people. Never invite."
	return &model.Agent{
		ID: uuid.New(), OrgID: orgID, Name: model.AgentNameTeamBuilder, Role: "Team Builder",
		Description: &desc, SystemPrompt: &prompt, Status: "active", EngineType: model.EngineGoNative,
		EngineConfig: map[string]any{}, Metadata: map[string]any{model.MetadataKeyBuiltin: model.BuiltinTeamBuilder},
	}
}

func launch(run Runner) agentmodule.LaunchFunc {
	return func(ctx context.Context, in agentmodule.LaunchInput) (*agentmodule.LaunchOutput, error) {
		if run == nil {
			return nil, fmt.Errorf("Team Builder is not configured")
		}
		brief := strings.TrimSpace(in.Values["brief"])
		if brief == "" {
			return nil, fmt.Errorf("brief is required")
		}
		draft, err := run.Draft(ctx, brief)
		if err != nil {
			return nil, err
		}
		return &agentmodule.LaunchOutput{
			Description: draft, Status: "done", Message: "Team shape ready",
			ExtraMeta: map[string]any{"draft": draft, "draft_kind": "team_builder", "ai_confirm": true},
		}, nil
	}
}
