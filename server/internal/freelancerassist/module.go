// Package freelancerassist drafts outreach and profile tips for freelancers.
package freelancerassist

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
	Draft(ctx context.Context, goal, contextNotes string) (string, error)
}

type Heuristic struct{}

func (Heuristic) Draft(_ context.Context, goal, contextNotes string) (string, error) {
	var b strings.Builder
	b.WriteString("## Freelancer assistant draft\n\n")
	b.WriteString("### Goal\n")
	b.WriteString(strings.TrimSpace(goal))
	b.WriteString("\n\n")
	if contextNotes != "" {
		b.WriteString("### Context\n")
		b.WriteString(strings.TrimSpace(contextNotes))
		b.WriteString("\n\n")
	}
	b.WriteString("### Suggested next steps\n")
	b.WriteString("1. Tighten your portfolio with one recent evidence link\n")
	b.WriteString("2. Send at most three tailored proposals today\n")
	b.WriteString("3. Follow up on viewed proposals after 48h\n\n")
	b.WriteString("Edit before sending — AI never messages clients for you.\n")
	return b.String(), nil
}

func Module(run Runner) agentmodule.Module {
	return agentmodule.Module{
		Builtin:  model.BuiltinFreelancerAssistant,
		Label:    model.AgentNameFreelancerAssistant,
		Icon:     "briefcase",
		Hint:     "Draft outreach and weekly freelance actions. You send.",
		ChatHint: "To get freelancer coaching, call agent_execute on Freelancer Assistant with goal and optional context. Never send messages for the user.",
		Schema: agentschema.Schema{
			Builtin: model.BuiltinFreelancerAssistant,
			Fields: []agentschema.Field{
				{Key: "goal", Label: "Goal", Type: "textarea", Required: true, MinLength: 10,
					Placeholder: "Win two Rust projects this month", Question: "What should we work on?"},
				{Key: "context", Label: "Context (optional)", Type: "textarea"},
			},
			TitleRules: []agentschema.TitleRule{{Prefix: "Freelancer: ", FromKey: "goal", Truncate: 50, Fallback: "plan"}},
		},
		Seed:   Seed,
		Launch: launch(run),
		AbsorbPrompt: func(prompt string, vals map[string]string) {
			if vals["goal"] == "" {
				vals["goal"] = prompt
			}
		},
	}
}

func Seed(orgID uuid.UUID) *model.Agent {
	desc := "Drafts freelance outreach and weekly actions. The human sends every message."
	prompt := "You coach freelancers. Never invent clients. Never send messages."
	return &model.Agent{
		ID: uuid.New(), OrgID: orgID, Name: model.AgentNameFreelancerAssistant, Role: "Freelancer Assistant",
		Description: &desc, SystemPrompt: &prompt, Status: "active", EngineType: model.EngineGoNative,
		EngineConfig: map[string]any{}, Metadata: map[string]any{model.MetadataKeyBuiltin: model.BuiltinFreelancerAssistant},
	}
}

func launch(run Runner) agentmodule.LaunchFunc {
	return func(ctx context.Context, in agentmodule.LaunchInput) (*agentmodule.LaunchOutput, error) {
		if run == nil {
			return nil, fmt.Errorf("Freelancer Assistant is not configured")
		}
		goal := strings.TrimSpace(in.Values["goal"])
		if goal == "" {
			return nil, fmt.Errorf("goal is required")
		}
		draft, err := run.Draft(ctx, goal, in.Values["context"])
		if err != nil {
			return nil, err
		}
		return &agentmodule.LaunchOutput{
			Description: draft, Status: "done", Message: "Freelancer draft ready",
			ExtraMeta: map[string]any{"draft": draft, "draft_kind": "freelancer_assist", "ai_confirm": true},
		}, nil
	}
}
