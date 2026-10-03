// Package linkedin is the LinkedIn Poster: it turns published articles into
// LinkedIn posts — a technical one for engineers and a plain-language one for
// business readers, each led by the problem the article solves.
//
// Drafts are written when an article is published (for orgs that connected
// LinkedIn) or when someone launches the agent. Nothing reaches LinkedIn until
// a person approves a draft on the agent's tab.
package linkedin

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/agentmodule"
	"github.com/jobshout/server/internal/agentschema"
	"github.com/jobshout/server/internal/model"
)

// LaunchRequest is a launch's values, resolved.
type LaunchRequest struct {
	OrgID   uuid.UUID
	UserID  uuid.UUID
	AgentID uuid.UUID
	TaskID  *uuid.UUID
	// Article is an article ID or words from its title. Empty means the most
	// recently published article that has no LinkedIn drafts yet.
	Article  string
	Variants []string
	Notes    string
}

// LaunchResult is what a launch started.
type LaunchResult struct {
	ArticleID    uuid.UUID
	ArticleTitle string
	Variants     []string
}

// Runner is the launch surface satisfied by the LinkedIn service.
type Runner interface {
	Launch(ctx context.Context, req LaunchRequest) (*LaunchResult, error)
}

// Module is the LinkedIn Poster specialist.
func Module(run Runner) agentmodule.Module {
	return agentmodule.Module{
		Builtin:   model.BuiltinLinkedIn,
		Label:     model.AgentNameLinkedIn,
		Icon:      "newspaper",
		TabSlug:   "linkedin",
		Hint:      "Draft LinkedIn posts from a published article: a technical one and a business one. Review and post them from this agent's tab.",
		ChatHint:  "To turn a published article into LinkedIn posts, call agent_execute on the LinkedIn Poster; an article ID or title words are optional — without them it drafts for the latest published article. It writes drafts only; posting happens on its tab after review.",
		Schema:    schema(),
		Seed:      Seed,
		Launch:    launch(run),
		StayOnTab: true,
		AbsorbPrompt: func(prompt string, vals map[string]string) {
			if strings.TrimSpace(vals["article"]) == "" {
				if id := uuidPattern.FindString(prompt); id != "" {
					vals["article"] = id
				}
			}
		},
	}
}

var uuidPattern = regexp.MustCompile(`[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`)

func schema() agentschema.Schema {
	return agentschema.Schema{
		Builtin: model.BuiltinLinkedIn,
		Hint:    "LinkedIn drafts from a published article. Empty article = the latest published one without drafts.",
		Fields: []agentschema.Field{
			{Key: "article", Label: "Article (optional)", Type: "text",
				Placeholder: "Article ID or words from its title",
				Question:    "Which published article should I write LinkedIn posts for? Leave it empty for the latest one."},
			{Key: "variants", Label: "Post styles", Type: "select", Default: "both",
				Options: []model.ClarifyOption{
					{Label: "Technical and business", Value: "both"},
					{Label: "Technical only", Value: model.LinkedInVariantTechnical},
					{Label: "Business only", Value: model.LinkedInVariantBusiness},
				}},
			{Key: "notes", Label: "Angle (optional)", Type: "textarea",
				Placeholder: "e.g. lead with the on-call cost; mention it applies to small teams",
				Help:        "The problem or point to emphasise"},
		},
		TitleRules: []agentschema.TitleRule{
			{Prefix: "LinkedIn: ", FromKey: "article", Fallback: "latest article"},
		},
		DescRules: []agentschema.DescRule{
			{Key: "article", Prefix: "Article: "},
			{Key: "variants", Prefix: "Styles: "},
			{Key: "notes"},
		},
	}
}

// Seed is the built-in LinkedIn Poster.
func Seed(orgID uuid.UUID) *model.Agent {
	desc := "Turns published articles into LinkedIn posts: a technical one for engineers and a plain-language one for business readers, each about the problem the article solves. Posts go out only when a person approves them."
	prompt := "You are the LinkedIn Poster. You turn published articles into LinkedIn posts that lead with a real problem the reader has, say what the article found, and invite discussion. You write plainly, never invent facts or numbers the article does not contain, and never use hype."
	return &model.Agent{
		ID:           uuid.New(),
		OrgID:        orgID,
		Name:         model.AgentNameLinkedIn,
		Role:         "Social Writer",
		Description:  &desc,
		SystemPrompt: &prompt,
		Status:       "active",
		EngineType:   model.EngineGoNative,
		EngineConfig: map[string]any{},
		Metadata:     map[string]any{model.MetadataKeyBuiltin: model.BuiltinLinkedIn},
	}
}

// VariantsFromValue maps the "variants" field to the variants to draft.
func VariantsFromValue(v string) []string {
	switch strings.TrimSpace(strings.ToLower(v)) {
	case model.LinkedInVariantTechnical:
		return []string{model.LinkedInVariantTechnical}
	case model.LinkedInVariantBusiness:
		return []string{model.LinkedInVariantBusiness}
	default:
		return append([]string(nil), model.LinkedInVariants...)
	}
}

func launch(run Runner) agentmodule.LaunchFunc {
	return func(ctx context.Context, in agentmodule.LaunchInput) (*agentmodule.LaunchOutput, error) {
		if run == nil {
			return nil, fmt.Errorf("the LinkedIn Poster is not configured")
		}
		if in.Agent == nil {
			return nil, fmt.Errorf("LinkedIn launch needs an agent")
		}
		req := LaunchRequest{
			OrgID:    in.OrgID,
			UserID:   in.UserID,
			AgentID:  in.Agent.ID,
			Article:  strings.TrimSpace(in.Values["article"]),
			Variants: VariantsFromValue(in.Values["variants"]),
			Notes:    strings.TrimSpace(in.Values["notes"]),
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
			Message:     fmt.Sprintf("Drafting LinkedIn posts for %q", res.ArticleTitle),
			Description: fmt.Sprintf("Drafting %s LinkedIn post(s) for %q. Review, edit and post them from the LinkedIn Poster tab.", strings.Join(res.Variants, " and "), res.ArticleTitle),
			ExtraMeta:   map[string]any{"linkedin_article_id": res.ArticleID.String()},
		}, nil
	}
}
