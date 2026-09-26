package research

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/agentmodule"
	"github.com/jobshout/server/internal/agentschema"
	"github.com/jobshout/server/internal/model"
)

// Runner is the launch surface. ResearchService satisfies it.
type Runner interface {
	Available() bool
	Research(ctx context.Context, orgID uuid.UUID, req Request, progress ProgressFunc) (*Brief, error)
}

// Module is the Research Agent specialist.
//
// All specialists are wired this way: own package, then one Register call.
func Module(run Runner) agentmodule.Module {
	return agentmodule.Module{
		Builtin: model.BuiltinResearcher,
		Label:   "Research Agent",
		Icon:    "search",
		// No TabSlug: Research lives in the agents list, not the Task Manager rail.
		Hint:     "Research a subject and return cited findings.",
		ChatHint: "To research a subject, call agent_execute on the Research Agent (or research_run). Do not invent a topic.",
		Schema:   schema(),
		Seed:     Seed,
		Launch:   launch(run),
		AbsorbPrompt: func(prompt string, vals map[string]string) {
			if vals["topic"] == "" {
				vals["topic"] = prompt
			}
		},
	}
}

func schema() agentschema.Schema {
	return agentschema.Schema{
		Builtin:        model.BuiltinResearcher,
		SpecialistTool: "research_run",
		Hint:           "Research a subject and return cited findings.",
		Fields: []agentschema.Field{
			{Key: "topic", Label: "Topic", Type: "text", Required: true, MinLength: 3, Placeholder: "e.g. Kubernetes cost optimisation patterns", Question: "What should I research?"},
			{Key: "context", Label: "Context (optional)", Type: "textarea", Placeholder: "Angle, constraints, what to emphasise"},
		},
		TitleRules: []agentschema.TitleRule{
			{Prefix: "Research: ", FromKey: "topic", Fallback: "topic"},
		},
		DescRules: []agentschema.DescRule{
			{Key: "topic", Prefix: "Topic: "},
			{Key: "context"},
		},
	}
}

// Seed is the built-in Research Agent.
func Seed(orgID uuid.UUID) *model.Agent {
	desc := "Searches the internet, reads the sources it finds, and returns verified findings with citations that have been checked against the pages they came from."
	prompt := "You are a research agent. You plan searches, read sources in full, and extract only claims the source text actually states — each with the verbatim passage supporting it. You never cite a page you have not read, and you reject a citation when you are unsure it supports the claim."
	return &model.Agent{
		ID:           uuid.New(),
		OrgID:        orgID,
		Name:         "Research Agent",
		Role:         "Researcher",
		Description:  &desc,
		SystemPrompt: &prompt,
		Status:       "active",
		EngineType:   model.EngineGoNative,
		EngineConfig: map[string]any{},
		Metadata:     map[string]any{model.MetadataKeyBuiltin: model.BuiltinResearcher},
	}
}

func launch(run Runner) agentmodule.LaunchFunc {
	return func(ctx context.Context, in agentmodule.LaunchInput) (*agentmodule.LaunchOutput, error) {
		if run == nil || !run.Available() {
			return nil, fmt.Errorf("research is not configured on this server")
		}
		brief, err := run.Research(ctx, in.OrgID, Request{
			Topic:   strings.TrimSpace(in.Values["topic"]),
			Context: strings.TrimSpace(in.Values["context"]),
		}, nil)
		if err != nil {
			return nil, err
		}
		prior := ""
		if in.Task != nil && in.Task.Description != nil {
			prior = *in.Task.Description
		}
		return &agentmodule.LaunchOutput{
			Brief:       brief,
			Description: formatBrief(brief, prior),
			Status:      "done",
			Message:     "Research complete",
		}, nil
	}
}

func formatBrief(brief *Brief, prior string) string {
	var parts []string
	if strings.TrimSpace(prior) != "" {
		parts = append(parts, strings.TrimSpace(prior))
	}
	if brief == nil {
		return strings.Join(parts, "\n\n")
	}
	if strings.TrimSpace(brief.Summary) != "" {
		parts = append(parts, "## Summary\n\n"+strings.TrimSpace(brief.Summary))
	}
	if len(brief.Findings) > 0 {
		var lines []string
		for _, f := range brief.Findings {
			claim := strings.TrimSpace(f.Claim)
			if claim == "" {
				claim = "(finding)"
			}
			if f.SourceURL != "" {
				lines = append(lines, "- "+claim+" ([source]("+f.SourceURL+"))")
			} else {
				lines = append(lines, "- "+claim)
			}
		}
		parts = append(parts, "## Findings\n\n"+strings.Join(lines, "\n"))
	}
	return strings.Join(parts, "\n\n")
}
