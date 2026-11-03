// Package metalead is the Meta Lead Analyst: find sales leads on a connected
// Facebook Page and its Instagram account — Lead Ads Instant Form submissions,
// DMs, comments, @mentions and tags, by webhook as they arrive and by launch
// backfill — qualify each with an LLM brief, and sync real leads into OpsAPI
// CRM. People label verdicts in the tab, and an eval scores models on them.
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
	Include    string // IncludeAll (default), IncludeForms or IncludeInbox
	Form       string // form id or name; empty = all forms (capped)
	Since      string // RFC3339 or relative like 7d
	Limit      int
	Focus      string
	// PushCRM syncs analysed leads into OpsAPI CRM contacts (default true).
	PushCRM bool
	// Website is the optional OpsAPI website / namespace for CRM push.
	Website string
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
		Builtin:   model.BuiltinMetaLead,
		Label:     model.AgentNameMetaLead,
		Icon:      "users",
		TabSlug:   TabSlug,
		Hint:      "Find leads in Instagram and Facebook DMs, comments, mentions, tags and Lead Ads forms, qualify them, and sync contacts into OpsAPI CRM.",
		ChatHint:  "The Meta Lead Analyst qualifies Instagram and Facebook DMs, comments and @mentions as they arrive by webhook (nothing to launch) and lists them in its tab. To backfill past DMs, comments, tags and Lead Ads form leads and push them into OpsAPI CRM, call agent_execute on it with connection (the named Page connection); optional include (all, forms or inbox), form, since, limit, focus, website, and push_to_crm.",
		Schema:    schema(),
		Seed:      Seed,
		Launch:    launch(run),
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
		Hint:    "Backfill recent DMs, comments, tags and Lead Ads form leads, qualify them, and sync real leads into OpsAPI CRM contacts. Tokens stay server-side.",
		Fields: []agentschema.Field{
			{Key: "connection", Label: "Page connection", Type: "text", Required: true,
				Placeholder: "e.g. workstation-ig",
				Question:    "Which named Meta Page connection should I use?",
				Help:        "Name of a server-side Page connection (token stored encrypted). Never paste an access token here."},
			{Key: "include", Label: "What to pull", Type: "select", Default: IncludeAll,
				Options: []model.ClarifyOption{
					{Label: "Everything: forms, DMs, comments and tags", Value: IncludeAll},
					{Label: "Lead Ads forms only", Value: IncludeForms},
					{Label: "DMs, comments and tags only", Value: IncludeInbox},
				},
				Help: "DMs, comments and tags are the recent ones Graph lists; items already captured by webhook are not analysed again."},
			{Key: "form", Label: "Lead form (optional)", Type: "text",
				Placeholder: "Form ID or name",
				Help:        "Empty = recent leads across forms on that Page (capped)."},
			{Key: "since", Label: "Since (optional)", Type: "text",
				Placeholder: "7d or 2026-10-01T00:00:00Z",
				Help:        "Only leads created after this time."},
			{Key: "limit", Label: "Max items (optional)", Type: "text", Default: "25",
				Placeholder: "25",
				Help:        "The newest this many across forms, DMs, comments and tags (max 200)."},
			{Key: "focus", Label: "Analysis focus (optional)", Type: "textarea",
				Placeholder: "Qualify for UK B2B SaaS demo; flag students and freelancers as low fit",
				Help:        "Angle for fit scoring and next actions."},
			{Key: "push_to_crm", Label: "Push to OpsAPI CRM", Type: "select", Default: "yes",
				Options: []model.ClarifyOption{
					{Label: "Yes — create/update CRM contacts", Value: "yes"},
					{Label: "No — analyse only", Value: "no"},
				},
				Help: "Uses the org's OpsAPI marketing/CRM workspace (same as Marketing Manager)."},
			{Key: "website", Label: "OpsAPI website (optional)", Type: "text",
				Placeholder: "saved website name or namespace",
				Help:        "Which OpsAPI workspace receives CRM contacts. Empty = the org's only marketing workspace (or ring workspace)."},
		},
		TitleRules: []agentschema.TitleRule{
			{Prefix: "Meta leads: ", FromKey: "connection", Fallback: "Page"},
		},
		DescRules: []agentschema.DescRule{
			{Key: "form", Prefix: "Form: "},
			{Key: "since", Prefix: "Since: "},
			{Key: "website", Prefix: "CRM: "},
			{Key: "focus", Truncate: 200},
		},
	}
}

// Seed is the built-in Meta Lead Analyst.
func Seed(orgID uuid.UUID) *model.Agent {
	desc := "Finds sales leads in Instagram and Facebook: Lead Ads Instant Form submissions plus DMs, comments and @mentions arriving by webhook. Each one gets an LLM brief (is it a lead, intent, fit, evidence quote, next action) and qualified contacts sync into OpsAPI CRM."
	prompt := "You are the Meta Lead Analyst. You qualify Instant Form submissions, DMs, comments and mentions from the data provided only — never invent contact details — write clear qualification notes, and sync real leads into the organisation's OpsAPI CRM."
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
			Include:    parseInclude(in.Values["include"]),
			Form:       strings.TrimSpace(in.Values["form"]),
			Since:      strings.TrimSpace(in.Values["since"]),
			Limit:      parseLimit(in.Values["limit"]),
			Focus:      strings.TrimSpace(in.Values["focus"]),
			PushCRM:    parsePushCRM(in.Values["push_to_crm"]),
			Website:    strings.TrimSpace(in.Values["website"]),
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

func parseInclude(v string) string {
	switch v = strings.ToLower(strings.TrimSpace(v)); v {
	case IncludeForms, IncludeInbox:
		return v
	}
	return IncludeAll
}

func parsePushCRM(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "no", "false", "0", "off":
		return false
	default:
		return true
	}
}
