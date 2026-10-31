// Package metalead is the Meta Lead Analyst: pull Instagram/Facebook Lead Ads
// Instant Form submissions via Graph API (launch backfill + leadgen webhook)
// and run LLM per-lead briefs plus a rollup summary.
//
// Credentials are named server-side Page connections — never schema fields.
// Registered with agentmodule; see .claude/rules/agent-modules.md.
package metalead

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

// TabSlug is the agent's Workspace tab.
const TabSlug = "meta-leads"

// LaunchRequest is a launch's values, resolved.
type LaunchRequest struct {
	OrgID      uuid.UUID
	UserID     uuid.UUID
	AgentID    uuid.UUID
	TaskID     *uuid.UUID
	Connection string // connection name
	Form       string // form id or name; empty = all forms (capped)
	Since      string // RFC3339 or relative like 7d
	Limit      int
	Focus      string
}

// LaunchResult is what a launch started.
type LaunchResult struct {
	RunID   uuid.UUID
	Message string
}

// Runner is the launch surface satisfied by *Service.
type Runner interface {
	Launch(ctx context.Context, req LaunchRequest) (*LaunchResult, error)
}

// Module is the Meta Lead Analyst specialist.
func Module(run Runner) agentmodule.Module {
	return agentmodule.Module{
		Builtin:  model.BuiltinMetaLead,
		Label:    model.AgentNameMetaLead,
		Icon:     "users",
		TabSlug:  TabSlug,
		Hint:     "Pull Meta Lead Ads form submissions from Instagram/Facebook and analyse them with an LLM: per-lead briefs and a rollup.",
		ChatHint: "To analyse Meta Instant Form leads, call agent_execute on the Meta Lead Analyst with connection (the named Page connection); optional form, since, limit, and focus. Webhooks also analyse new leadgen events for connected Pages.",
		Schema:   schema(),
		Seed:     Seed,
		Launch:   launch(run),
		StayOnTab: true,
		AbsorbPrompt: func(prompt string, vals map[string]string) {
			if strings.TrimSpace(vals["focus"]) == "" && strings.TrimSpace(vals["connection"]) != "" {
				vals["focus"] = strings.TrimSpace(prompt)
			}
		},
	}
}

func schema() agentschema.Schema {
	return agentschema.Schema{
		Builtin: model.BuiltinMetaLead,
		Hint:    "Pull Lead Ads Instant Form leads for a named Page connection and analyse them. Tokens stay server-side.",
		Fields: []agentschema.Field{
			{Key: "connection", Label: "Page connection", Type: "text", Required: true,
				Placeholder: "e.g. workstation-ig",
				Question:    "Which named Meta Page connection should I use?",
				Help:        "Name of a server-side Page connection (token stored encrypted). Never paste an access token here."},
			{Key: "form", Label: "Lead form (optional)", Type: "text",
				Placeholder: "Form ID or name",
				Help:        "Empty = recent leads across forms on that Page (capped)."},
			{Key: "since", Label: "Since (optional)", Type: "text",
				Placeholder: "7d or 2026-10-01T00:00:00Z",
				Help:        "Only leads created after this time."},
			{Key: "limit", Label: "Max leads (optional)", Type: "text", Default: "25",
				Placeholder: "25",
				Help:        "Hard cap per launch (max 200)."},
			{Key: "focus", Label: "Analysis focus (optional)", Type: "textarea",
				Placeholder: "Qualify for UK B2B SaaS demo; flag students and freelancers as low fit",
				Help:        "Angle for fit scoring and next actions."},
		},
		TitleRules: []agentschema.TitleRule{
			{Prefix: "Meta leads: ", FromKey: "connection", Fallback: "Page"},
		},
		DescRules: []agentschema.DescRule{
			{Key: "form", Prefix: "Form: "},
			{Key: "since", Prefix: "Since: "},
			{Key: "focus", Truncate: 200},
		},
	}
}

// Seed is the built-in Meta Lead Analyst.
func Seed(orgID uuid.UUID) *model.Agent {
	desc := "Pulls Instagram and Facebook Lead Ads Instant Form submissions via the Meta Graph API and runs LLM analysis: a brief per lead (intent, fit, next action, spam risk) plus a rollup for the run. New leads can also arrive through the leadgen webhook."
	prompt := "You are the Meta Lead Analyst. You read Instant Form field data only — never invent contact details — and write clear, actionable qualification notes for sales and ops."
	return &model.Agent{
		ID:           uuid.New(),
		OrgID:        orgID,
		Name:         model.AgentNameMetaLead,
		Role:         "Lead Analyst",
		Description:  &desc,
		SystemPrompt: &prompt,
		Status:       "active",
		EngineType:   model.EngineGoNative,
		EngineConfig: map[string]any{},
		Metadata:     map[string]any{model.MetadataKeyBuiltin: model.BuiltinMetaLead},
	}
}

func launch(run Runner) agentmodule.LaunchFunc {
	return func(ctx context.Context, in agentmodule.LaunchInput) (*agentmodule.LaunchOutput, error) {
		if run == nil {
			return nil, fmt.Errorf("the Meta Lead Analyst is not configured")
		}
		if in.Agent == nil {
			return nil, fmt.Errorf("Meta Lead launch needs an agent")
		}
		conn := strings.TrimSpace(in.Values["connection"])
		if conn == "" {
			return nil, fmt.Errorf("connection is required (named Page connection)")
		}
		req := LaunchRequest{
			OrgID:      in.OrgID,
			UserID:     in.UserID,
			AgentID:    in.Agent.ID,
			Connection: conn,
			Form:       strings.TrimSpace(in.Values["form"]),
			Since:      strings.TrimSpace(in.Values["since"]),
			Limit:      parseLimit(in.Values["limit"]),
			Focus:      strings.TrimSpace(in.Values["focus"]),
		}
		if in.Task != nil {
			tid := in.Task.ID
			req.TaskID = &tid
		}
		res, err := run.Launch(ctx, req)
		if err != nil {
			return nil, err
		}
		return &agentmodule.LaunchOutput{
			Message:     res.Message,
			Description: res.Message,
			ExtraMeta:   map[string]any{"meta_lead_run_id": res.RunID.String()},
		}, nil
	}
}

func parseLimit(v string) int {
	v = strings.TrimSpace(v)
	if v == "" {
		return 25
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return 25
	}
	return n
}
