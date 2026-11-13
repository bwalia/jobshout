// Package skillgap drafts a skill-gap plan (pairs with Career Ops / Course Generator).
package skillgap

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
	Draft(ctx context.Context, targetRole, currentSkills string) (string, error)
}

type Heuristic struct{}

func (Heuristic) Draft(_ context.Context, targetRole, currentSkills string) (string, error) {
	var b strings.Builder
	b.WriteString("## Skill gap analysis\n\n### Target\n")
	b.WriteString(strings.TrimSpace(targetRole))
	b.WriteString("\n\n### Current skills\n")
	if currentSkills == "" {
		b.WriteString("(not provided — list them for a sharper plan)\n")
	} else {
		b.WriteString(strings.TrimSpace(currentSkills))
		b.WriteString("\n")
	}
	b.WriteString("\n### Suggested gaps to close\n- Core domain skill for the role\n- Evidence project (portfolio)\n- One focused course or practice loop\n\n")
	b.WriteString("Use Career Agent for JD fit and Course Generator for a learning path. You decide what to study.\n")
	return b.String(), nil
}

func Module(run Runner) agentmodule.Module {
	return agentmodule.Module{
		Builtin:  model.BuiltinSkillGap,
		Label:    model.AgentNameSkillGap,
		Icon:     "graduation-cap",
		Hint:     "Map skills to a target role. Pair with Career Agent and Course Generator.",
		ChatHint: "To analyse skill gaps, call agent_execute on Skill Gap Analysis with target_role and optional current_skills. For JD scoring use Career Agent; for a course use Course Generator.",
		Schema: agentschema.Schema{
			Builtin: model.BuiltinSkillGap,
			Fields: []agentschema.Field{
				{Key: "target_role", Label: "Target role", Type: "text", Required: true, MinLength: 3,
					Question: "Which role are you aiming for?"},
				{Key: "current_skills", Label: "Current skills (optional)", Type: "textarea"},
			},
			TitleRules: []agentschema.TitleRule{{Prefix: "Skill gap: ", FromKey: "target_role", Fallback: "role"}},
		},
		Seed:   Seed,
		Launch: launch(run),
		AbsorbPrompt: func(prompt string, vals map[string]string) {
			if vals["target_role"] == "" {
				vals["target_role"] = prompt
			}
		},
	}
}

func Seed(orgID uuid.UUID) *model.Agent {
	desc := "Maps current skills to a target role and suggests gaps. Pair with Career Agent and Course Generator."
	prompt := "You analyse skill gaps. Never invent certificates. Never enrol the user."
	return &model.Agent{
		ID: uuid.New(), OrgID: orgID, Name: model.AgentNameSkillGap, Role: "Skill Gap Analysis",
		Description: &desc, SystemPrompt: &prompt, Status: "active", EngineType: model.EngineGoNative,
		EngineConfig: map[string]any{}, Metadata: map[string]any{model.MetadataKeyBuiltin: model.BuiltinSkillGap},
	}
}

func launch(run Runner) agentmodule.LaunchFunc {
	return func(ctx context.Context, in agentmodule.LaunchInput) (*agentmodule.LaunchOutput, error) {
		if run == nil {
			return nil, fmt.Errorf("Skill Gap Analysis is not configured")
		}
		role := strings.TrimSpace(in.Values["target_role"])
		if role == "" {
			return nil, fmt.Errorf("target_role is required")
		}
		draft, err := run.Draft(ctx, role, in.Values["current_skills"])
		if err != nil {
			return nil, err
		}
		return &agentmodule.LaunchOutput{
			Description: draft, Status: "done", Message: "Skill gap draft ready",
			ExtraMeta: map[string]any{"draft": draft, "draft_kind": "skill_gap", "ai_confirm": true},
		}, nil
	}
}
