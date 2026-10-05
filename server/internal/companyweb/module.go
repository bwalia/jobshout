package companyweb

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/agentmodule"
	"github.com/jobshout/server/internal/agentschema"
	"github.com/jobshout/server/internal/model"
)

// Runner is the launch surface.
type Runner interface {
	Available() bool
	Lookup(ctx context.Context, companyName string) ([]Result, error)
}

// Module is the Company Website Finder specialist.
func Module(run Runner) agentmodule.Module {
	return agentmodule.Module{
		Builtin:  model.BuiltinCompanyWeb,
		Label:    model.AgentNameCompanyWeb,
		Icon:     "globe",
		Hint:     "Search the web for a company’s official website.",
		ChatHint: "To find a company website, call agent_execute on the Company Website Finder with company_name. Do not invent a URL.",
		Schema:   schema(),
		Seed:     Seed,
		Launch:   launch(run),
		AbsorbPrompt: func(prompt string, vals map[string]string) {
			if vals["company_name"] == "" {
				vals["company_name"] = prompt
			}
		},
	}
}

func schema() agentschema.Schema {
	return agentschema.Schema{
		Builtin: model.BuiltinCompanyWeb,
		Hint:    "Search the web for a company’s official website.",
		Fields: []agentschema.Field{
			{
				Key:         "company_name",
				Label:       "Company name",
				Type:        "text",
				Required:    true,
				MinLength:   2,
				Placeholder: "e.g. Acme Ltd",
				Question:    "Which company’s website should I find?",
			},
		},
		TitleRules: []agentschema.TitleRule{
			{Prefix: "Company website: ", FromKey: "company_name", Fallback: "company"},
		},
		DescRules: []agentschema.DescRule{
			{Key: "company_name", Prefix: "Company: "},
		},
	}
}

// Seed is the built-in Company Website Finder.
func Seed(orgID uuid.UUID) *model.Agent {
	desc := "Searches the web for a company’s official website from its name and returns the best match with sources."
	prompt := "You find official company websites. You search the web using the company name, prefer the corporate homepage over social or directory pages, never invent a URL, and always report the search evidence you used."
	return &model.Agent{
		ID:           uuid.New(),
		OrgID:        orgID,
		Name:         model.AgentNameCompanyWeb,
		Role:         "Researcher",
		Description:  &desc,
		SystemPrompt: &prompt,
		Status:       "active",
		EngineType:   model.EngineGoNative,
		EngineConfig: map[string]any{},
		Metadata:     map[string]any{model.MetadataKeyBuiltin: model.BuiltinCompanyWeb},
	}
}

func launch(run Runner) agentmodule.LaunchFunc {
	return func(ctx context.Context, in agentmodule.LaunchInput) (*agentmodule.LaunchOutput, error) {
		if run == nil || !run.Available() {
			return nil, fmt.Errorf("company website search is not configured on this server (needs Brave Search)")
		}
		name := strings.TrimSpace(in.Values["company_name"])
		if name == "" {
			return nil, fmt.Errorf("company_name is required")
		}
		results, err := run.Lookup(ctx, name)
		if err != nil {
			return nil, err
		}
		prior := ""
		if in.Task != nil && in.Task.Description != nil {
			prior = *in.Task.Description
		}
		desc, msg := formatResults(name, results, prior)
		return &agentmodule.LaunchOutput{
			Description: desc,
			Status:      "done",
			Message:     msg,
			ExtraMeta: map[string]any{
				"company_name": name,
				"website":      bestURL(results),
			},
		}, nil
	}
}

func bestURL(results []Result) string {
	if len(results) == 0 {
		return ""
	}
	return results[0].URL
}

func formatResults(name string, results []Result, prior string) (description, message string) {
	var b strings.Builder
	if prior != "" {
		b.WriteString(strings.TrimSpace(prior))
		b.WriteString("\n\n")
	}
	b.WriteString("Company: ")
	b.WriteString(name)
	b.WriteString("\n\n")
	if len(results) == 0 {
		b.WriteString("No confident official website found in web search results. Try a more specific legal or trading name.")
		return b.String(), "No website found"
	}
	top := results[0]
	b.WriteString("Website: ")
	b.WriteString(top.URL)
	b.WriteString("\n")
	if top.Title != "" {
		b.WriteString("Title: ")
		b.WriteString(top.Title)
		b.WriteString("\n")
	}
	b.WriteString("\nOther candidates:\n")
	limit := len(results)
	if limit > 4 {
		limit = 4
	}
	for i := 1; i < limit; i++ {
		r := results[i]
		b.WriteString(fmt.Sprintf("- %s — %s\n", r.URL, r.Title))
	}
	return b.String(), fmt.Sprintf("Found %s", top.URL)
}
