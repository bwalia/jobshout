package seo

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
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
		Builtin:  model.BuiltinSEOAnalyst,
		Label:    "SEO Analyst",
		Icon:     "search",
		TabSlug:  "seo",
		Hint:     "Audit URLs or GitHub site files against a keyword set; propose meta/frontmatter fixes and open a PR via a named connection.",
		ChatHint: "To analyse SEO, call agent_execute on the SEO Analyst. Pass source=urls with urls + keywords, or source=github with github_connection (named credential) + keywords and optional path_glob / max_pages. Never paste a GitHub token.",
		Schema:   schema(),
		Seed:     Seed,
		Launch:   launch(run),
		StayOnTab: true,
		AbsorbPrompt: func(prompt string, vals map[string]string) {
			if vals["urls"] != "" || vals["url"] != "" {
				return
			}
			if u := extractURL(prompt); u != "" {
				vals["urls"] = u
				if vals["source"] == "" {
					vals["source"] = "urls"
				}
			}
		},
	}
}

func schema() agentschema.Schema {
	return agentschema.Schema{
		Builtin: model.BuiltinSEOAnalyst,
		Hint:    "Keyword-driven SEO: audit public URLs one-by-one, or analyse GitHub site files via a named connection (token never in the form).",
		Fields: []agentschema.Field{
			{Key: "source", Label: "Source", Type: "select", Required: true, Default: "urls",
				Options: []model.ClarifyOption{
					{Label: "URL audit (no GitHub)", Value: "urls"},
					{Label: "GitHub site optimise", Value: "github"},
				},
				Question: "Audit public URLs, or analyse files in a connected GitHub repo?",
				Help:     "URL mode fetches pages. GitHub mode reads files via a named server-side connection."},
			{Key: "keywords", Label: "Focus keywords", Type: "textarea", Required: true,
				Placeholder: "devops jobs\nsite reliability\nhire engineers",
				Help:        "One per line or comma-separated. Every recommendation is driven by this set.",
				Question:    "Which focus keywords should guide the analysis?"},
			{Key: "urls", Label: "URLs (one per line)", Type: "textarea", Required: false,
				Placeholder: "https://example.com/\nhttps://example.com/about",
				Help:        "Required for URL audit. Analysed one-by-one up to max pages.",
				Question:    "Which URLs should I audit?"},
			{Key: "github_connection", Label: "GitHub connection name", Type: "text", Required: false,
				Placeholder: "e.g. marketing-site",
				Help:        "Named credential saved in the SEO Workspace tab. Never paste a token here.",
				Question:    "Which named GitHub connection should I use?"},
			{Key: "git_repo", Label: "Repo override (optional)", Type: "text", Required: false,
				Placeholder: "owner/repo",
				Help:        "Defaults to the repo on the named connection."},
			{Key: "git_branch", Label: "Branch", Type: "text", Required: false, Default: "main",
				Placeholder: "main"},
			{Key: "path_glob", Label: "Path glob", Type: "text", Required: false,
				Placeholder: "content/**/*.md",
				Help:        "GitHub only. Empty = common .html/.md/.mdx under the tree (capped)."},
			{Key: "max_pages", Label: "Max pages", Type: "text", Required: false, Default: "10",
				Placeholder: "10",
				Help:        "Cap how many pages are analysed in one run (max 50)."},
			{Key: "mode", Label: "Action", Type: "select", Required: true, Default: "analyze",
				Options: []model.ClarifyOption{
					{Label: "Analyse + report", Value: "analyze"},
					{Label: "Analyse + propose fixes", Value: "improve"},
					{Label: "Propose + publish / open PR", Value: "publish"},
				},
				Question: "Analyse only, also propose fixes, or publish / open a PR?"},
			{Key: "platform", Label: "CMS / site type", Type: "select", Required: false, Default: "auto",
				Options: []model.ClarifyOption{
					{Label: "Auto-detect", Value: "auto"},
					{Label: "WordPress", Value: "wordpress"},
					{Label: "OpsAPI contents API", Value: "opsapi"},
					{Label: "Static / HTML (git)", Value: "static"},
				}},
			{Key: "sitemap_path", Label: "Sitemap path in repo", Type: "text", Required: false,
				Default: "sitemap.xml", Placeholder: "public/sitemap.xml"},
			{Key: "instruction", Label: "Note (optional)", Type: "textarea",
				Placeholder: "e.g. prefer UK English; keep brand voice"},
		},
		RequireAny: []agentschema.RequireGroup{
			{Keys: []string{"urls", "github_connection"}, Slot: "target",
				Question: "Provide URLs (URL audit) or a GitHub connection name (repo mode)."},
		},
		TitleRules: []agentschema.TitleRule{
			{Prefix: "SEO: ", FromKey: "keywords", Fallback: "keywords"},
		},
		DescRules: []agentschema.DescRule{
			{Key: "source", Prefix: "Source: "},
			{Key: "mode", Prefix: "Action: "},
			{Key: "github_connection", Prefix: "GitHub: "},
			{Key: "path_glob", Prefix: "Glob: "},
			{Key: "instruction"},
		},
	}
}

// Seed is the built-in SEO Analyst agent.
func Seed(orgID uuid.UUID) *model.Agent {
	desc := "Analyses pages against a configurable keyword set — public URLs one-by-one, or files in a GitHub repo via a named connection. Proposes title/meta/H1/frontmatter fixes and can open a PR when the connection has write scope."
	prompt := "You are the SEO Analyst agent. Drive every recommendation from the operator's keyword set. For source=urls, fetch each URL and score title/meta/H1/content gaps. For source=github, use the named server-side GitHub connection (never ask for a token) to list matching files and analyse them. Propose concrete meta/frontmatter patches; when publishing, prefer opening a PR with seo-patches/ + SEO_PATCH.md. Prefer API publish for WordPress/OpsAPI when configured."
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
		source := strings.ToLower(strings.TrimSpace(in.Values["source"]))
		if source == "" {
			source = "urls"
		}
		urls := strings.TrimSpace(in.Values["urls"])
		if urls == "" {
			urls = strings.TrimSpace(in.Values["url"])
		}
		keywords := strings.TrimSpace(in.Values["keywords"])
		if keywords == "" {
			return nil, fmt.Errorf("keywords are required")
		}
		mode := strings.TrimSpace(in.Values["mode"])
		if mode == "" {
			mode = "analyze"
		}
		platform := strings.TrimSpace(in.Values["platform"])
		if platform == "" {
			platform = "auto"
		}
		maxPages := 10
		if n, err := strconv.Atoi(strings.TrimSpace(in.Values["max_pages"])); err == nil && n > 0 {
			maxPages = n
		}
		if source == "urls" {
			for _, u := range ParseURLs(urls) {
				if _, err := url.ParseRequestURI(ensureScheme(u)); err != nil && !strings.Contains(u, ".") {
					return nil, fmt.Errorf("invalid url: %s", u)
				}
			}
			if len(ParseURLs(urls)) == 0 {
				return nil, fmt.Errorf("urls is required for URL audit")
			}
		}
		if source == "github" && strings.TrimSpace(in.Values["github_connection"]) == "" {
			return nil, fmt.Errorf("github_connection is required for GitHub mode")
		}
		firstURL := ""
		if parsed := ParseURLs(urls); len(parsed) > 0 {
			firstURL = parsed[0]
		}
		payload := model.CreateSEORunRequest{
			AgentID:          in.Agent.ID,
			TaskID:           &in.Task.ID,
			Source:           source,
			URL:              firstURL,
			URLs:             urls,
			Mode:             mode,
			Platform:         platform,
			GitRepo:          strings.TrimSpace(in.Values["git_repo"]),
			GitBranch:        strings.TrimSpace(in.Values["git_branch"]),
			GitHubConnection: strings.TrimSpace(in.Values["github_connection"]),
			PathGlob:         strings.TrimSpace(in.Values["path_glob"]),
			MaxPages:         maxPages,
			SiteMapPath:      strings.TrimSpace(in.Values["sitemap_path"]),
			Keywords:         keywords,
			Instruction:      strings.TrimSpace(in.Values["instruction"]),
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
