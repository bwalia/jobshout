// Package interviewprep drafts interview prep notes (pairs with Career Ops).
package interviewprep

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
	Draft(ctx context.Context, role, company, jdNotes string) (string, error)
}

type Heuristic struct{}

func (Heuristic) Draft(_ context.Context, role, company, jdNotes string) (string, error) {
	var b strings.Builder
	b.WriteString("## Interview prep (draft)\n\n")
	b.WriteString("### Role\n")
	b.WriteString(strings.TrimSpace(role))
	b.WriteString("\n\n### Company\n")
	if company == "" {
		b.WriteString("(not provided)\n")
	} else {
		b.WriteString(strings.TrimSpace(company))
		b.WriteString("\n")
	}
	b.WriteString("\n### Focus from the JD\n")
	if jdNotes == "" {
		b.WriteString("Paste JD notes for sharper questions.\n")
	} else {
		b.WriteString(strings.TrimSpace(jdNotes))
		b.WriteString("\n")
	}
	b.WriteString("\n### Suggested prep\n")
	b.WriteString("- 3 stories with metrics (impact, ownership, collaboration)\n")
	b.WriteString("- 5 clarifying questions about the team and success criteria\n")
	b.WriteString("- One honest gap + how you would close it in 30 days\n\n")
	b.WriteString("This is a draft only. You decide what to say in the interview.\n")
	return b.String(), nil
}

func Module(run Runner) agentmodule.Module {
	return agentmodule.Module{
		Builtin:  model.BuiltinInterviewPrep,
		Label:    model.AgentNameInterviewPrep,
		Icon:     "message-square",
		Hint:     "Draft interview prep notes. Nothing is sent for you.",
		ChatHint: "For interview prep, call agent_execute on Interview Prep with role and optional company / jd_notes. Pair with Career Agent for JD fit.",
		Schema: agentschema.Schema{
			Builtin: model.BuiltinInterviewPrep,
			Fields: []agentschema.Field{
				{Key: "role", Label: "Role", Type: "text", Required: true, MinLength: 2,
					Question: "Which role is the interview for?"},
				{Key: "company", Label: "Company (optional)", Type: "text"},
				{Key: "jd_notes", Label: "JD notes (optional)", Type: "textarea"},
			},
			TitleRules: []agentschema.TitleRule{{Prefix: "Interview prep: ", FromKey: "role", Fallback: "role"}},
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
	desc := "Drafts interview prep notes from a role and JD. A person always takes the interview."
	prompt := "You prepare interview notes. Never invent employment history. Never contact the employer."
	return &model.Agent{
		ID: uuid.New(), OrgID: orgID, Name: model.AgentNameInterviewPrep, Role: "Interview Prep",
		Description: &desc, SystemPrompt: &prompt, Status: "active", EngineType: model.EngineGoNative,
		EngineConfig: map[string]any{}, Metadata: map[string]any{model.MetadataKeyBuiltin: model.BuiltinInterviewPrep},
	}
}

func launch(run Runner) agentmodule.LaunchFunc {
	return func(ctx context.Context, in agentmodule.LaunchInput) (*agentmodule.LaunchOutput, error) {
		if run == nil {
			return nil, fmt.Errorf("Interview Prep is not configured")
		}
		role := strings.TrimSpace(in.Values["role"])
		if role == "" {
			return nil, fmt.Errorf("role is required")
		}
		draft, err := run.Draft(ctx, role, in.Values["company"], in.Values["jd_notes"])
		if err != nil {
			return nil, err
		}
		return &agentmodule.LaunchOutput{
			Description: draft, Status: "done", Message: "Interview prep draft ready",
			ExtraMeta: map[string]any{"draft": draft, "draft_kind": "interview_prep", "ai_confirm": true},
		}, nil
	}
}
