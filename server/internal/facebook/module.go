// Package facebook is the Facebook Page Manager: it plans a content calendar
// for a connected Facebook Page, writes the posts and their creatives,
// schedules and publishes them once a person approves, and reports what
// performed.
//
// Drafts are written when someone launches the agent. Nothing reaches
// Facebook until a person approves a post on the agent's tab; approved posts
// publish at their scheduled time. Facebook's Graph API cannot create Pages,
// so the agent never claims to — it gives the manual setup steps instead.
package facebook

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/agentmodule"
	"github.com/jobshout/server/internal/agentschema"
	"github.com/jobshout/server/internal/model"
)

// Runner is the launch surface satisfied by Service.
type Runner interface {
	Launch(ctx context.Context, req LaunchRequest) (*LaunchResult, error)
	Status(ctx context.Context, orgID uuid.UUID) (*Status, error)
}

// Module is the Facebook Page Manager specialist.
func Module(run Runner) agentmodule.Module {
	return agentmodule.Module{
		Builtin:   Builtin,
		Label:     AgentName,
		Icon:      "megaphone",
		TabSlug:   "facebook",
		Hint:      "Plan and write a Facebook content calendar for a connected Page. Review, schedule and publish the posts from this agent's tab.",
		ChatHint:  "To plan Facebook posts, call agent_execute on the Facebook Page Manager; the Page, objective and facts to use (offers, events) are optional, but never invent offers, prices or testimonials — ask the user what is real. It writes drafts only; publishing happens on its tab after approval. It cannot create a Facebook Page (Meta does not allow it): tell the user to create it in Facebook and connect it.",
		Schema:    schema(),
		Seed:      Seed,
		Launch:    launch(run),
		StayOnTab: true,
		AbsorbPrompt: func(prompt string, vals map[string]string) {
			if p := strings.TrimSpace(prompt); strings.TrimSpace(vals["notes"]) == "" && len([]rune(p)) > 24 {
				vals["notes"] = p
			}
		},
		Requirements: []agentmodule.Requirement{{
			Key: "facebook_oauth", Kind: "integration", Blocking: false,
			Message: "Connect Facebook on the agent's tab (and set FACEBOOK_APP_ID, FACEBOOK_APP_SECRET, FACEBOOK_TOKEN_KEY) to publish. Drafting works without it.",
		}},
		Ready: func(ctx context.Context, orgID uuid.UUID) []agentmodule.Issue {
			if run == nil {
				return nil
			}
			st, err := run.Status(ctx, orgID)
			if err != nil {
				return nil
			}
			switch {
			case !st.Configured:
				return []agentmodule.Issue{{Severity: "warning", Code: "facebook_not_configured", Message: "This deployment has no Meta app configured, so the agent can draft but not publish."}}
			case !st.Connected:
				return []agentmodule.Issue{{Severity: "warning", Code: "facebook_not_connected", Message: "No Facebook Page is connected for this org."}}
			}
			return nil
		},
	}
}

func schema() agentschema.Schema {
	return agentschema.Schema{
		Builtin: Builtin,
		Hint:    "A Facebook content calendar for a connected Page. Drafts only; you approve before anything is published.",
		Fields: []agentschema.Field{
			{Key: "page", Label: "Page (optional)", Type: "text", Placeholder: "Page name or ID",
				Question: "Which Facebook Page is this for? Leave it empty if only one is connected."},
			{Key: "objective", Label: "Objective", Type: "select", Default: "awareness",
				Question: "What should this campaign achieve?",
				Options: []model.ClarifyOption{
					{Label: "Brand awareness", Value: "awareness"},
					{Label: "Engagement", Value: "engagement"},
					{Label: "Promote products or services", Value: "promotion"},
					{Label: "Educate the audience", Value: "education"},
					{Label: "Announce an event or campaign", Value: "event"},
				}},
			{Key: "days", Label: "Calendar length", Type: "select", Default: "7",
				Options: []model.ClarifyOption{{Label: "Next 7 days", Value: "7"}, {Label: "Next 14 days", Value: "14"}, {Label: "Next 30 days", Value: "30"}}},
			{Key: "per_week", Label: "Posts per week", Type: "select", Default: "3",
				Options: []model.ClarifyOption{{Label: "3", Value: "3"}, {Label: "5", Value: "5"}, {Label: "7", Value: "7"}}},
			{Key: "images", Label: "Creatives", Type: "select", Default: "yes",
				Options: []model.ClarifyOption{{Label: "Generate an image for each post", Value: "yes"}, {Label: "Text only", Value: "no"}}},
			{Key: "notes", Label: "Offers, events and facts (optional)", Type: "textarea",
				Placeholder: "e.g. 10% off winter servicing until 31 Jan; open day on 14 Feb",
				Help:        "The only offers, prices, dates and claims the posts may state. Leave empty and none are used."},
		},
		TitleRules: []agentschema.TitleRule{
			{Prefix: "Facebook: ", FromKey: "page", Fallback: "content calendar"},
		},
		DescRules: []agentschema.DescRule{
			{Key: "page", Prefix: "Page: "},
			{Key: "objective", Prefix: "Objective: "},
			{Key: "days", Prefix: "Days: "},
			{Key: "notes"},
		},
	}
}

// Seed is the built-in Facebook Page Manager.
func Seed(orgID uuid.UUID) *model.Agent {
	desc := "Plans a Facebook content calendar for a connected Page, writes the posts and their creatives, schedules and publishes them once a person approves, and reports what performed."
	prompt := "You are the Facebook Page Manager, an experienced social media marketing manager. You write original, human-sounding Facebook posts in the Page's brand voice and never invent testimonials, statistics, certifications, discounts or business claims."
	return &model.Agent{
		ID:           uuid.New(),
		OrgID:        orgID,
		Name:         AgentName,
		Role:         "Social Media Manager",
		Description:  &desc,
		SystemPrompt: &prompt,
		Status:       "active",
		EngineType:   model.EngineGoNative,
		EngineConfig: map[string]any{},
		Metadata:     map[string]any{model.MetadataKeyBuiltin: Builtin},
	}
}

func launch(run Runner) agentmodule.LaunchFunc {
	return func(ctx context.Context, in agentmodule.LaunchInput) (*agentmodule.LaunchOutput, error) {
		if run == nil {
			return nil, fmt.Errorf("the Facebook Page Manager is not configured")
		}
		if in.Agent == nil {
			return nil, fmt.Errorf("Facebook launch needs an agent")
		}
		req := LaunchRequest{
			OrgID:     in.OrgID,
			UserID:    in.UserID,
			AgentID:   in.Agent.ID,
			Page:      strings.TrimSpace(in.Values["page"]),
			Objective: objectiveText(in.Values["objective"]),
			Days:      atoi(in.Values["days"]),
			PerWeek:   atoi(in.Values["per_week"]),
			Notes:     strings.TrimSpace(in.Values["notes"]),
			Images:    !strings.EqualFold(strings.TrimSpace(in.Values["images"]), "no"),
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
			Message:     fmt.Sprintf("Drafting %d Facebook post(s) for %q", res.Posts, res.PageName),
			Description: fmt.Sprintf("Drafting %d Facebook post(s) over %d days for %q. Review, edit, schedule and publish them from the Facebook Page Manager tab.", res.Posts, res.Days, res.PageName),
		}, nil
	}
}

func objectiveText(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "engagement":
		return "grow engagement: comments, shares and conversation"
	case "promotion":
		return "promote the business's products and services"
	case "education":
		return "educate the audience with useful information"
	case "event":
		return "announce and build attendance for an event or campaign"
	default:
		return "build brand awareness"
	}
}

func atoi(s string) int {
	n := 0
	for _, r := range strings.TrimSpace(s) {
		if r < '0' || r > '9' {
			return 0
		}
		n = n*10 + int(r-'0')
	}
	return n
}
