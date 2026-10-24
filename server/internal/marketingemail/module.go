// Package marketingemail is the Marketing Email Writer: a short brief in, a
// production-quality HTML marketing email out, published live as an OpsAPI CMS
// page (template "email") whose public URL is the email's browser version.
//
// The model writes structured JSON, never HTML; Go validates it and renders it
// with html/template. Emails go to CMS pages, not posts, so they never appear
// in the blog listing. Re-running an email updates the same page (upsert by
// slug); Unpublish archives it.
//
// It is its own package, registered with agentmodule — see
// .claude/rules/agent-modules.md. No platform switch names it.
package marketingemail

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/agentmodule"
	"github.com/jobshout/server/internal/agentschema"
	"github.com/jobshout/server/internal/model"
)

// TabSlug is the agent's Task Manager tab.
const TabSlug = "emails"

// Runner is the launch surface satisfied by *Service.
type Runner interface {
	Start(ctx context.Context, req StartRequest) (uuid.UUID, error)
}

// Module is the Marketing Email Writer specialist.
//
// All specialists are wired this way: own package, then one Register call.
func Module(run Runner) agentmodule.Module {
	return agentmodule.Module{
		Builtin:   model.BuiltinMarketingEmailWriter,
		Label:     model.AgentNameMarketingEmail,
		Icon:      "send",
		TabSlug:   TabSlug,
		Hint:      "Give it a brief: it writes a marketing email, renders production HTML and publishes the browser version live on the website.",
		ChatHint:  "To write a marketing email, call agent_execute on the Marketing Email Writer with the brief; add audience, goal (announce, promote, nurture, newsletter), a call-to-action label and https URL, products, and tone when the user gives them. It publishes a live browser version and reports its URL; publish=draft files it unpublished.",
		Schema:    schema(),
		Seed:      Seed,
		Launch:    launch(run),
		StayOnTab: true,
		AbsorbPrompt: func(prompt string, vals map[string]string) {
			if strings.TrimSpace(vals["brief"]) == "" {
				vals["brief"] = strings.TrimSpace(prompt)
			}
		},
	}
}

func schema() agentschema.Schema {
	return agentschema.Schema{
		Builtin: model.BuiltinMarketingEmailWriter,
		Hint:    "A brief in, a published HTML email out. The browser version goes live on the website; paste the HTML into your email service.",
		Fields: []agentschema.Field{
			{Key: "brief", Label: "What is the email about?", Type: "textarea", Required: true, MinLength: 10,
				Placeholder: "Workstation AI shop is live — NVIDIA DGX Spark, Mac Studio M5 and Trade membership with an AI engineer on call",
				Question:    "What should the email be about? Include the facts it must state."},
			{Key: "audience", Label: "Audience (optional)", Type: "text",
				Placeholder: "IT managers at UK engineering firms"},
			{Key: "goal", Label: "Goal", Type: "select", Default: model.MarketingEmailGoalAnnounce,
				Options: []model.ClarifyOption{
					{Label: "Announce something new", Value: model.MarketingEmailGoalAnnounce},
					{Label: "Promote a product or offer", Value: model.MarketingEmailGoalPromote},
					{Label: "Nurture a relationship", Value: model.MarketingEmailGoalNurture},
					{Label: "Newsletter", Value: model.MarketingEmailGoalNewsletter},
				}},
			{Key: "primary_cta_label", Label: "Button label (optional)", Type: "text", Placeholder: "Shop AI workstations"},
			{Key: "primary_cta_url", Label: "Button URL (optional)", Type: "text", Placeholder: "https://www.workstation.co.uk/shop",
				Help: "https only."},
			{Key: "products", Label: "Products to feature (optional)", Type: "textarea",
				Placeholder: "Shop URLs or slugs, one per line",
				Help:        "Only products named here are featured."},
			{Key: "tone", Label: "Tone (optional)", Type: "text", Placeholder: "confident, plain, no hype"},
			{Key: "destination", Label: "Website", Type: "website", FormDefault: "workstation",
				Help: "The OpsAPI website the email's page is filed in. A saved website reuses its key; blank uses this ring's CMS."},
			{Key: "publish", Label: "Publish", Type: "select", Default: model.MarketingEmailPublishLive,
				Options: []model.ClarifyOption{
					{Label: "Live — publish the browser version now", Value: model.MarketingEmailPublishLive},
					{Label: "Draft — file it unpublished", Value: model.MarketingEmailPublishDraft},
				}},
			{Key: "model", Label: "Model override (optional)", Type: "text", Placeholder: "agent default"},
		},
		TitleRules: []agentschema.TitleRule{
			{Prefix: "Email: ", FromKey: "brief", Truncate: 80, Fallback: "new marketing email"},
		},
		DescRules: []agentschema.DescRule{
			{Key: "goal", Prefix: "Goal: "},
			{Key: "audience", Prefix: "Audience: "},
			{Key: "publish", Prefix: "Publish: "},
			{Key: "brief", Truncate: 400},
		},
	}
}

// BriefFromValues resolves launch values into a brief. Nothing here is a
// credential: destination is a website name resolved server-side.
func BriefFromValues(v map[string]string) model.MarketingEmailBrief {
	return model.MarketingEmailBrief{
		Brief:           v["brief"],
		Audience:        v["audience"],
		Goal:            v["goal"],
		PrimaryCTALabel: v["primary_cta_label"],
		PrimaryCTAURL:   v["primary_cta_url"],
		Products:        v["products"],
		Tone:            v["tone"],
		Destination:     v["destination"],
		Publish:         v["publish"],
		Model:           v["model"],
	}
}

// Seed is the built-in Marketing Email Writer.
func Seed(orgID uuid.UUID) *model.Agent {
	desc := "Writes a marketing email from a short brief, renders it as production HTML and publishes its browser version live as a CMS page on the website. Re-running updates the same page; unpublishing archives it."
	prompt := "You are the Marketing Email Writer. You write one clear, specific marketing email from a brief: a subject that says what is inside, a preheader that continues it, a few short sections and one call to action. You use only facts in the brief — never an invented price, specification, discount, date or customer quote — and you never write HTML."
	return &model.Agent{
		ID:           uuid.New(),
		OrgID:        orgID,
		Name:         model.AgentNameMarketingEmail,
		Role:         "Marketing Email Writer",
		Description:  &desc,
		SystemPrompt: &prompt,
		Status:       "active",
		EngineType:   model.EngineGoNative,
		EngineConfig: map[string]any{},
		Metadata:     map[string]any{model.MetadataKeyBuiltin: model.BuiltinMarketingEmailWriter},
	}
}

func launch(run Runner) agentmodule.LaunchFunc {
	return func(ctx context.Context, in agentmodule.LaunchInput) (*agentmodule.LaunchOutput, error) {
		if run == nil {
			return nil, fmt.Errorf("the Marketing Email Writer is not configured")
		}
		if in.Agent == nil {
			return nil, fmt.Errorf("marketing email launch needs an agent")
		}
		req := StartRequest{OrgID: in.OrgID, UserID: in.UserID, AgentID: in.Agent.ID, Brief: BriefFromValues(in.Values)}
		if in.Task != nil {
			id := in.Task.ID
			req.TaskID = &id
		}
		runID, err := run.Start(ctx, req)
		if err != nil {
			return nil, err
		}
		return &agentmodule.LaunchOutput{
			RunID:     &runID,
			Message:   "Marketing Email Writer started: writing the email",
			ExtraMeta: map[string]any{model.TaskMetaRunID: runID.String()},
		}, nil
	}
}
