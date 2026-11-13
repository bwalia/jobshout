// Package hiringassist drafts JD and outreach for recruiters (human sends).
package hiringassist

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
	Draft(ctx context.Context, role, notes string) (string, error)
}

type Heuristic struct{}

func (Heuristic) Draft(_ context.Context, role, notes string) (string, error) {
	var b strings.Builder
	b.WriteString("## Hiring assistant draft\n\n### Role\n")
	b.WriteString(strings.TrimSpace(role))
	b.WriteString("\n\n### Suggested JD outline\n- Mission\n- Responsibilities\n- Must-have skills\n- Nice-to-haves\n- Process & timeline\n\n")
	if notes != "" {
		b.WriteString("### Notes\n")
		b.WriteString(strings.TrimSpace(notes))
		b.WriteString("\n\n")
	}
	b.WriteString("Review before publishing a requisition or inviting talent.\n")
	return b.String(), nil
}

func Module(run Runner) agentmodule.Module {
	return agentmodule.Module{
		Builtin:  model.BuiltinHiringAssistant,
		Label:    model.AgentNameHiringAssistant,
		Icon:     "user-plus",
		Hint:     "Draft JDs and hiring outreach. You publish and invite.",
		ChatHint: "To draft hiring copy, call agent_execute on Hiring Assistant with role and optional notes. Never invite candidates.",
		Schema: agentschema.Schema{
			Builtin: model.BuiltinHiringAssistant,
			Fields: []agentschema.Field{
				{Key: "role", Label: "Role", Type: "textarea", Required: true, MinLength: 10,
					Question: "Which role are you hiring for?"},
				{Key: "notes", Label: "Notes (optional)", Type: "textarea"},
			},
			TitleRules: []agentschema.TitleRule{{Prefix: "Hiring: ", FromKey: "role", Truncate: 50, Fallback: "role"}},
		},
		Seed:   Seed,
		Launch: launch(run),
		AbsorbPrompt: func(prompt string, vals map[string]string) {
			if vals["role"] == "" {
				vals["role"] = prompt
			}
		},
	}
}

func Seed(orgID uuid.UUID) *model.Agent {
	desc := "Drafts job descriptions and hiring outreach. The human publishes and invites."
	prompt := "You help recruiters draft clear JDs. Never invent salary. Never invite candidates."
	return &model.Agent{
		ID: uuid.New(), OrgID: orgID, Name: model.AgentNameHiringAssistant, Role: "Hiring Assistant",
		Description: &desc, SystemPrompt: &prompt, Status: "active", EngineType: model.EngineGoNative,
		EngineConfig: map[string]any{}, Metadata: map[string]any{model.MetadataKeyBuiltin: model.BuiltinHiringAssistant},
	}
}

func launch(run Runner) agentmodule.LaunchFunc {
	return func(ctx context.Context, in agentmodule.LaunchInput) (*agentmodule.LaunchOutput, error) {
		if run == nil {
			return nil, fmt.Errorf("Hiring Assistant is not configured")
		}
		role := strings.TrimSpace(in.Values["role"])
		if role == "" {
			return nil, fmt.Errorf("role is required")
		}
		draft, err := run.Draft(ctx, role, in.Values["notes"])
		if err != nil {
			return nil, err
		}
		return &agentmodule.LaunchOutput{
			Description: draft, Status: "done", Message: "Hiring draft ready",
			ExtraMeta: map[string]any{"draft": draft, "draft_kind": "hiring_assist", "ai_confirm": true},
		}, nil
	}
}
