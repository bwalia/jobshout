// Package talentmatch ranks members for a brief with explainable reasons.
// The human invites or hires — AI never contacts candidates.
package talentmatch

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/agentmodule"
	"github.com/jobshout/server/internal/agentschema"
	"github.com/jobshout/server/internal/model"
)

// Runner produces a match explanation. Nil → not configured.
type Runner interface {
	Match(ctx context.Context, req MatchRequest) (string, error)
}

// MatchRequest is the launch surface.
type MatchRequest struct {
	Brief          string
	RequiredSkills string
	CandidateNotes string
}

// Module is the Talent Matchmaker specialist.
func Module(run Runner) agentmodule.Module {
	return agentmodule.Module{
		Builtin:  model.BuiltinTalentMatchmaker,
		Label:    model.AgentNameTalentMatchmaker,
		Icon:     "users",
		Hint:     "Rank talent for a brief with reasons. You invite or hire — AI never messages candidates.",
		ChatHint: "To match talent, call agent_execute on the Talent Matchmaker with brief and optional required_skills / candidate_notes. Never message or hire — the human decides.",
		Schema:   schema(),
		Seed:     Seed,
		Launch:   launch(run),
		AbsorbPrompt: func(prompt string, vals map[string]string) {
			if vals["brief"] == "" {
				vals["brief"] = prompt
			}
		},
	}
}

func schema() agentschema.Schema {
	return agentschema.Schema{
		Builtin: model.BuiltinTalentMatchmaker,
		Hint:    "Brief in → ranked matches with reasons. Human invites.",
		Fields: []agentschema.Field{
			{Key: "brief", Label: "Hiring / project brief", Type: "textarea", Required: true, MinLength: 20,
				Placeholder: "Need a Rust backend engineer for a 6-week engagement…", Question: "What are you hiring for?"},
			{Key: "required_skills", Label: "Required skills (optional)", Type: "text",
				Placeholder: "Rust, Postgres, Kubernetes"},
			{Key: "candidate_notes", Label: "Candidate notes / search results (optional)", Type: "textarea",
				Placeholder: "Paste member headlines or search hits to rank…",
				Help:        "When empty, the agent returns a matching checklist instead of named people."},
		},
		TitleRules: []agentschema.TitleRule{
			{Prefix: "Talent match: ", FromKey: "brief", Truncate: 50, Fallback: "brief"},
		},
		DescRules: []agentschema.DescRule{
			{Key: "required_skills", Prefix: "Skills: "},
			{Key: "brief", Truncate: 400},
		},
	}
}

// Seed is the built-in Talent Matchmaker.
func Seed(orgID uuid.UUID) *model.Agent {
	desc := "Ranks talent for a brief with explainable reasons. The human invites or hires — this agent never messages candidates or reveals private contact details."
	prompt := "You match talent to briefs with clear reasons. Never invent people. Never reveal email or phone. Never send invitations."
	return &model.Agent{
		ID:           uuid.New(),
		OrgID:        orgID,
		Name:         model.AgentNameTalentMatchmaker,
		Role:         "Talent Matchmaker",
		Description:  &desc,
		SystemPrompt: &prompt,
		Status:       "active",
		EngineType:   model.EngineGoNative,
		EngineConfig: map[string]any{},
		Metadata:     map[string]any{model.MetadataKeyBuiltin: model.BuiltinTalentMatchmaker},
	}
}

func launch(run Runner) agentmodule.LaunchFunc {
	return func(ctx context.Context, in agentmodule.LaunchInput) (*agentmodule.LaunchOutput, error) {
		if run == nil {
			return nil, fmt.Errorf("Talent Matchmaker is not configured")
		}
		brief := strings.TrimSpace(in.Values["brief"])
		if brief == "" {
			return nil, fmt.Errorf("brief is required")
		}
		draft, err := run.Match(ctx, MatchRequest{
			Brief:          brief,
			RequiredSkills: strings.TrimSpace(in.Values["required_skills"]),
			CandidateNotes: strings.TrimSpace(in.Values["candidate_notes"]),
		})
		if err != nil {
			return nil, err
		}
		return &agentmodule.LaunchOutput{
			Description: draft,
			Status:      "done",
			Message:     "Talent match ready — invite only after you review",
			ExtraMeta: map[string]any{
				"draft":      draft,
				"draft_kind": "talent_match",
				"ai_confirm": true,
				"human_only": "invite_or_hire",
			},
		}, nil
	}
}
