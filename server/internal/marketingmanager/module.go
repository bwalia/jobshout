// Package marketingmanager is the Marketing Manager: it runs an org's OpsAPI
// Marketing Cloud — templates, contacts and lists, segments, campaigns,
// sends, delivery and engagement results — through OpsAPI's own APIs.
//
// It reuses OpsAPI for everything that matters: consent and suppression
// (can_send), approval (four-eyes), test mode (dry_run), sanitising, merge
// fields, batching, tracking and analytics. JobShout adds the agent: chat
// tools over a fixed operation table (ops.go), a confirmation gate on
// anything that reaches people (gate.go), and a results review (review.go).
//
// Tenant isolation is OpsAPI's: each org reaches only the namespace its own
// key belongs to, and OpsAPI refuses a key used with any other namespace.
//
// It is its own package, registered with agentmodule — see
// .claude/rules/agent-modules.md. No platform switch names it.
package marketingmanager

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

// Module is the Marketing Manager specialist. svc may be nil (not configured).
//
// All specialists are wired this way: own package, then one Register call.
func Module(svc *Service) agentmodule.Module {
	return agentmodule.Module{
		Builtin: model.BuiltinMarketingManager,
		Label:   model.AgentNameMarketingMgr,
		Icon:    "megaphone",
		Hint:    "Runs your OpsAPI Marketing Cloud: templates, contacts and lists, campaigns and sends. Run it for a review of recent campaign results.",
		ChatHint: "For email marketing in OpsAPI — templates, contacts, lists and segments, campaigns, sending, delivery/open/click results — use the marketing_read, marketing_write and marketing_send tools (find them with catalog_search \"marketing\"). " +
			"Read status first: it says whether the workspace is in test mode. Sends, imports and consent changes always need the user's confirmation; never claim a send went out unless the tool result says so. " +
			"For a written review of campaign results, call agent_execute on the Marketing Manager.",
		Schema: schema(),
		Seed:   Seed,
		Launch: launch(svc),
		AbsorbPrompt: func(prompt string, vals map[string]string) {
			p := strings.ToLower(prompt)
			if vals["action"] == "" && (strings.Contains(p, "status") || strings.Contains(p, "set up") || strings.Contains(p, "setup")) {
				vals["action"] = model.MarketingManagerStatus
			}
		},
		Requirements: []agentmodule.Requirement{{
			Key: "opsapi_marketing", Kind: "integration", Blocking: true,
			Message: "An OpsAPI website integration with credentials.marketing_api_key (scopes opsapi_marketing_* and crm), or OPSAPI_MARKETING_API_KEY on the server.",
		}},
		Ready: func(ctx context.Context, orgID uuid.UUID) []agentmodule.Issue {
			if svc == nil {
				return []agentmodule.Issue{{Severity: "warning", Code: "marketing_unconfigured", Message: "The Marketing Manager is not set up on this server."}}
			}
			c, err := svc.Connect(ctx, orgID, "")
			if err != nil {
				return []agentmodule.Issue{{Severity: "warning", Code: "marketing_no_workspace", Message: err.Error()}}
			}
			st, err := c.Sending(ctx)
			if err != nil {
				return []agentmodule.Issue{{Severity: "warning", Code: "marketing_unreachable", Message: err.Error()}}
			}
			if st.DryRun {
				return []agentmodule.Issue{{Severity: "info", Code: "marketing_dry_run", Message: "Workspace " + c.Namespace() + " is in test mode: sends are recorded, nothing leaves."}}
			}
			if !svc.AllowLiveSend {
				return []agentmodule.Issue{{Severity: "info", Code: "marketing_live_blocked", Message: "Workspace " + c.Namespace() + " sends live email; this server blocks live sends (MARKETING_LIVE_SEND)."}}
			}
			return nil
		},
	}
}

func schema() agentschema.Schema {
	return agentschema.Schema{
		Builtin: model.BuiltinMarketingManager,
		Hint:    "Reviews your OpsAPI campaigns and says what to change. Day-to-day marketing work happens in chat.",
		Fields: []agentschema.Field{
			{Key: "action", Label: "What to do", Type: "select", Default: model.MarketingManagerReview,
				Question: "Review recent campaign results, or check the marketing set-up?",
				Options: []model.ClarifyOption{
					{Label: "Review campaign results and suggest improvements", Value: model.MarketingManagerReview},
					{Label: "Check the set-up (sending, test mode, lists, templates)", Value: model.MarketingManagerStatus},
				}},
			{Key: "campaign", Label: "Campaign id (optional)", Type: "text", Placeholder: "blank reviews the 10 most recent",
				Help: "An OpsAPI campaign uuid."},
			{Key: "website", Label: "Website (optional)", Type: "text", Placeholder: "saved website name or namespace",
				Help: "Which OpsAPI workspace. Blank uses the one with a marketing key."},
		},
		TitleRules: []agentschema.TitleRule{
			{IfKey: "campaign", Format: "Marketing review: {campaign}"},
			{IfKey: "action", Format: "Marketing Manager: {action}", Fallback: "Marketing review"},
		},
		DescRules: []agentschema.DescRule{
			{Key: "action", Prefix: "Action: "},
			{Key: "campaign", Prefix: "Campaign: "},
			{Key: "website", Prefix: "Website: "},
		},
	}
}

// Seed is the built-in Marketing Manager.
func Seed(orgID uuid.UUID) *model.Agent {
	desc := "Manages the OpsAPI Marketing Cloud: email templates, contacts and lists, segments, campaigns, scheduling and sending, and delivery, open and click results — with a confirmation before anything reaches people."
	prompt := "You are the Marketing Manager. You run the organisation's OpsAPI Marketing Cloud through its tools. " +
		"Check status first: say plainly whether the workspace is in test mode (dry run) or live. " +
		"Before a send: validate the campaign, preview the audience, and offer a test send. Never send, import or change consent without the user's confirmation, and never call a send live unless the user explicitly approved a live send. " +
		"Consent, unsubscribes and approval belong to OpsAPI and people — you cannot override them, and you don't approve campaigns. " +
		"Report numbers exactly as OpsAPI gives them; opens are estimates. Suggest improvements from the numbers, not guesses."
	return &model.Agent{
		ID:           uuid.New(),
		OrgID:        orgID,
		Name:         model.AgentNameMarketingMgr,
		Role:         "Marketing Manager",
		Description:  &desc,
		SystemPrompt: &prompt,
		Status:       "active",
		EngineType:   model.EngineGoNative,
		EngineConfig: map[string]any{},
		Metadata:     map[string]any{model.MetadataKeyBuiltin: model.BuiltinMarketingManager},
	}
}

func launch(svc *Service) agentmodule.LaunchFunc {
	return func(ctx context.Context, in agentmodule.LaunchInput) (*agentmodule.LaunchOutput, error) {
		if svc == nil {
			return nil, fmt.Errorf("the Marketing Manager is not configured on this server")
		}
		website := in.Values["website"]
		switch strings.TrimSpace(in.Values["action"]) {
		case model.MarketingManagerStatus:
			md, meta, err := svc.statusReport(ctx, in.OrgID, website)
			if err != nil {
				return nil, err
			}
			return &agentmodule.LaunchOutput{Message: "Marketing set-up checked", Description: md, Status: "done", ExtraMeta: meta}, nil
		default:
			limit, _ := strconv.Atoi(in.Values["limit"])
			rep, err := svc.Review(ctx, in.OrgID, website, in.Values["campaign"], limit)
			if err != nil {
				return nil, err
			}
			return &agentmodule.LaunchOutput{
				Message:     fmt.Sprintf("Reviewed %d campaign(s) in %s", len(rep.Campaigns), rep.Namespace),
				Description: rep.Markdown(),
				Brief:       rep,
				Status:      "done",
				ExtraMeta:   map[string]any{"marketing_namespace": rep.Namespace, "campaigns_reviewed": len(rep.Campaigns)},
			}, nil
		}
	}
}
