// Package proposalgen is the Proposal Generator specialist for JobShout.com.
// It drafts a project proposal the human must edit and submit — AI never bids.
package proposalgen

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/agentmodule"
	"github.com/jobshout/server/internal/agentschema"
	"github.com/jobshout/server/internal/model"
)

// Runner drafts a proposal. Nil → not configured.
type Runner interface {
	Draft(ctx context.Context, req DraftRequest) (string, error)
}

// DraftRequest is the launch surface (no credentials).
type DraftRequest struct {
	ProjectTitle       string
	ProjectDescription string
	Skills             string
	FreelancerProfile  string
	Tone               string
}

// Module is the Proposal Generator specialist.
func Module(run Runner) agentmodule.Module {
	return agentmodule.Module{
		Builtin:  model.BuiltinProposalGenerator,
		Label:    model.AgentNameProposalGenerator,
		Icon:     "file-text",
		Hint:     "Draft a project proposal from the brief and your profile. You edit and submit.",
		ChatHint: "To draft a freelance proposal, call agent_execute on the Proposal Generator with project_title, project_description, and optional skills / freelancer_profile / tone. Never submit a bid — the human confirms.",
		Schema:   schema(),
		Seed:     Seed,
		Launch:   launch(run),
		AbsorbPrompt: func(prompt string, vals map[string]string) {
			if vals["project_description"] == "" {
				vals["project_description"] = prompt
			}
		},
	}
}

func schema() agentschema.Schema {
	return agentschema.Schema{
		Builtin: model.BuiltinProposalGenerator,
		Hint:    "Draft a proposal. The human edits and submits — AI never spends credits or bids.",
		Fields: []agentschema.Field{
			{Key: "project_title", Label: "Project title", Type: "text", Required: true, MinLength: 3,
				Placeholder: "Rust API performance audit", Question: "What is the project title?"},
			{Key: "project_description", Label: "Project description", Type: "textarea", Required: true, MinLength: 20,
				Placeholder: "Brief from the client…", Question: "Paste the project brief."},
			{Key: "skills", Label: "Required skills (optional)", Type: "text",
				Placeholder: "Rust, Axum, Postgres"},
			{Key: "freelancer_profile", Label: "Your profile notes (optional)", Type: "textarea",
				Placeholder: "Years of experience, relevant work…"},
			{Key: "tone", Label: "Tone (optional)", Type: "text", Default: "clear and confident",
				Placeholder: "clear and confident"},
		},
		TitleRules: []agentschema.TitleRule{
			{Prefix: "Proposal: ", FromKey: "project_title", Fallback: "draft"},
		},
		DescRules: []agentschema.DescRule{
			{Key: "skills", Prefix: "Skills: "},
			{Key: "project_description", Truncate: 400},
		},
	}
}

// Seed is the built-in Proposal Generator.
func Seed(orgID uuid.UUID) *model.Agent {
	desc := "Drafts a freelance project proposal from the client brief and the member's profile. The human always edits and submits — this agent never places a bid or spends credits."
	prompt := "You draft project proposals. Use only facts in the brief and profile. Never invent clients, budgets, or past work. End with a clear next step the human can edit. Never place a bid."
	return &model.Agent{
		ID:           uuid.New(),
		OrgID:        orgID,
		Name:         model.AgentNameProposalGenerator,
		Role:         "Proposal Generator",
		Description:  &desc,
		SystemPrompt: &prompt,
		Status:       "active",
		EngineType:   model.EngineGoNative,
		EngineConfig: map[string]any{},
		Metadata:     map[string]any{model.MetadataKeyBuiltin: model.BuiltinProposalGenerator},
	}
}

func launch(run Runner) agentmodule.LaunchFunc {
	return func(ctx context.Context, in agentmodule.LaunchInput) (*agentmodule.LaunchOutput, error) {
		if run == nil {
			return nil, fmt.Errorf("Proposal Generator is not configured")
		}
		title := strings.TrimSpace(in.Values["project_title"])
		desc := strings.TrimSpace(in.Values["project_description"])
		if title == "" || desc == "" {
			return nil, fmt.Errorf("project_title and project_description are required")
		}
		draft, err := run.Draft(ctx, DraftRequest{
			ProjectTitle:       title,
			ProjectDescription: desc,
			Skills:             strings.TrimSpace(in.Values["skills"]),
			FreelancerProfile:  strings.TrimSpace(in.Values["freelancer_profile"]),
			Tone:               strings.TrimSpace(in.Values["tone"]),
		})
		if err != nil {
			return nil, err
		}
		prior := ""
		if in.Task != nil && in.Task.Description != nil {
			prior = *in.Task.Description
		}
		outDesc := draft
		if prior != "" {
			outDesc = strings.TrimSpace(prior) + "\n\n" + draft
		}
		return &agentmodule.LaunchOutput{
			Description: outDesc,
			Status:      "done",
			Message:     "Proposal draft ready — edit before submitting",
			ExtraMeta: map[string]any{
				"draft":      draft,
				"draft_kind": "proposal",
				"ai_confirm": true,
				"human_only": "submit_bid",
			},
		}, nil
	}
}
