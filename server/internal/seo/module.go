package seo

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/agentmodule"
	"github.com/jobshout/server/internal/agentschema"
	"github.com/jobshout/server/internal/model"
)

// Runner is the launch surface satisfied by the SEO service.
type Runner interface {
	CreateRun(ctx context.Context, req model.CreateSEORunRequest, orgID uuid.UUID, requestedBy *uuid.UUID) (*model.SEORun, error)
}

// Module is the SEO Analyst / Manager specialist.
func Module(run Runner) agentmodule.Module {
	return agentmodule.Module{
		Builtin:   model.BuiltinSEOAnalyst,
		Label:     "SEO Analyst",
		Icon:      "search",
		TabSlug:   "seo",
		Hint:      "Analyse a site’s SEO, propose meta/slug/sitemap fixes, and publish via WordPress, OpsAPI, or a git PR.",
		ChatHint:  "To analyse or improve SEO, call agent_execute on the SEO Analyst. Pass a public URL; add git_repo to open a PR when CMS APIs are unavailable.",
		Schema:    schema(),
		Seed:      Seed,
		Launch:    launch(run),
		StayOnTab: true,
		AbsorbPrompt: func(prompt string, vals map[string]string) {
			if vals["url"] != "" {
				return
			}
			if u := extractURL(prompt); u != "" {
				vals["url"] = u
			}
		},
	}
}

func schema() agentschema.Schema {
	return agentschema.Schema{
		Builtin: model.BuiltinSEOAnalyst,
		Hint:    "Analyse SEO health and optionally improve + publish (CMS API or git PR).",
		Fields: []agentschema.Field{
			{Key: "url", Label: "Website URL", Type: "text", Required: true, MinLength: 3,
				Placeholder: "https://example.com",
				Help:        "Public page to crawl", Question: "Which website URL should I analyse?"},
			{Key: "mode", Label: "Mode", Type: "select", Required: true, Default: "analyze",
				Options: []model.ClarifyOption{
					{Label: "Analyse only", Value: "analyze"},
					{Label: "Analyse + improve plan", Value: "improve"},
					{Label: "Improve + publish", Value: "publish"},
				},
				Question: "Analyse only, also propose fixes, or publish?"},
			{Key: "platform", Label: "CMS / site type", Type: "select", Required: false, Default: "auto",
				Options: []model.ClarifyOption{
					{Label: "Auto-detect", Value: "auto"},
					{Label: "WordPress", Value: "wordpress"},
					{Label: "OpsAPI contents API", Value: "opsapi"},
					{Label: "Static / HTML (git)", Value: "static"},
				}},
			{Key: "keywords", Label: "Focus keywords", Type: "text", Required: false,
				Placeholder: "devops, kubernetes, hiring",
				Help:        "Comma-separated; checked against title/meta/body"},
			{Key: "git_repo", Label: "GitHub repo (for PR fallback)", Type: "text", Required: false,
				Placeholder: "owner/repo",
				Help:        "When CMS APIs fail or platform is static, open a PR with sitemap + SEO_PATCH.md"},
			{Key: "git_branch", Label: "Base branch", Type: "text", Required: false, Default: "main",
				Placeholder: "main"},
			{Key: "sitemap_path", Label: "Sitemap path in repo", Type: "text", Required: false,
				Default: "sitemap.xml", Placeholder: "public/sitemap.xml"},
			{Key: "instruction", Label: "Note (optional)", Type: "textarea",
				Placeholder: "e.g. prefer UK English; keep brand voice"},
		},
		TitleRules: []agentschema.TitleRule{
			{Prefix: "SEO: ", FromKey: "url", Fallback: "site"},
		},
		DescRules: []agentschema.DescRule{
			{Key: "mode", Prefix: "Mode: "},
			{Key: "platform", Prefix: "Platform: "},
			{Key: "keywords", Prefix: "Keywords: "},
			{Key: "instruction"},
		},
	}
}

// Seed is the built-in SEO Analyst agent.
func Seed(orgID uuid.UUID) *model.Agent {
	desc := "Analyses SEO for a URL (title, meta, headings, sitemap, robots), proposes slug/meta/sitemap fixes, and publishes via WordPress, OpsAPI, or a git PR when a repo is provided."
	prompt := "You are the SEO Analyst agent. Crawl the given URL, score SEO health, propose concrete fixes (meta tags, URL slug, sitemap.xml), and when asked to publish use WordPress REST, OpsAPI CMS, or open a GitHub PR with sitemap + SEO_PATCH.md. Prefer API publish; fall back to git PR when APIs fail or the site is static. Use site-structure skills (README/STRUCTURE.md, public/, content/) to place files correctly."
	return &model.Agent{
		ID:           uuid.New(),
		OrgID:        orgID,
		Name:         model.AgentNameSEO,
		Role:         "SEO Analyst & Manager",
		Description:  &desc,
		SystemPrompt: &prompt,
		Status:       "active",
		EngineType:   model.EngineGoNative,
		EngineConfig: map[string]any{},
		Metadata:     map[string]any{model.MetadataKeyBuiltin: model.BuiltinSEOAnalyst},
	}
}

func launch(run Runner) agentmodule.LaunchFunc {
	return func(ctx context.Context, in agentmodule.LaunchInput) (*agentmodule.LaunchOutput, error) {
		if run == nil {
			return nil, fmt.Errorf("SEO Analyst is not configured")
		}
		rawURL := strings.TrimSpace(in.Values["url"])
		if rawURL == "" {
			return nil, fmt.Errorf("url is required")
		}
		if _, err := url.ParseRequestURI(ensureScheme(rawURL)); err != nil {
			// allow host-only
			if !strings.Contains(rawURL, ".") {
				return nil, fmt.Errorf("invalid url")
			}
		}
		mode := strings.TrimSpace(in.Values["mode"])
		if mode == "" {
			mode = "analyze"
		}
		platform := strings.TrimSpace(in.Values["platform"])
		if platform == "" {
			platform = "auto"
		}
		payload := model.CreateSEORunRequest{
			AgentID:     in.Agent.ID,
			TaskID:      &in.Task.ID,
			URL:         rawURL,
			Mode:        mode,
			Platform:    platform,
			GitRepo:     strings.TrimSpace(in.Values["git_repo"]),
			GitBranch:   strings.TrimSpace(in.Values["git_branch"]),
			SiteMapPath: strings.TrimSpace(in.Values["sitemap_path"]),
			Keywords:    strings.TrimSpace(in.Values["keywords"]),
			Instruction: strings.TrimSpace(in.Values["instruction"]),
		}
		created, err := run.CreateRun(ctx, payload, in.OrgID, &in.UserID)
		if err != nil {
			return nil, err
		}
		id := created.ID
		return &agentmodule.LaunchOutput{
			RunID:     &id,
			Message:   "SEO Analyst run queued",
			ExtraMeta: map[string]any{model.TaskMetaRunID: id.String()},
		}, nil
	}
}

func ensureScheme(raw string) string {
	if strings.Contains(raw, "://") {
		return raw
	}
	return "https://" + raw
}

func extractURL(prompt string) string {
	for _, f := range strings.Fields(prompt) {
		f = strings.Trim(f, ".,;:\"'()")
		if strings.HasPrefix(f, "http://") || strings.HasPrefix(f, "https://") {
			return f
		}
		if strings.Contains(f, ".") && !strings.ContainsAny(f, " @") && len(f) > 4 {
			return f
		}
	}
	return ""
}
