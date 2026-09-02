package blog

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/agentmodule"
	"github.com/jobshout/server/internal/agentschema"
	"github.com/jobshout/server/internal/model"
)

// Writer is the launch surface. BlogService satisfies it.
type Writer interface {
	Generate(ctx context.Context, orgID uuid.UUID, triggeredBy *uuid.UUID, source string, req model.GenerateBlogRequest) (*model.BlogRun, error)
}

// Module is the Article Writer specialist.
//
// All specialists are wired this way: own package, then one Register call.
func Module(w Writer) agentmodule.Module {
	return agentmodule.Module{
		Builtin:  model.BuiltinArticleWriter,
		Label:    "Article Writer",
		Icon:     "newspaper",
		TabSlug:  "articles",
		Hint:     "Give a topic to research. The writer picks its own title from sources.",
		ChatHint: "To write an article, call agent_execute on the Article Writer (or article_generate). Do not invent a topic.",
		Schema:   schema(),
		Seed:     Seed,
		Launch:   launch(w),
		AbsorbPrompt: func(prompt string, vals map[string]string) {
			if vals["topic"] == "" {
				vals["topic"] = prompt
			}
		},
	}
}

func schema() agentschema.Schema {
	return agentschema.Schema{
		Builtin:        model.BuiltinArticleWriter,
		SpecialistTool: "article_generate",
		Hint:           "Give a topic to research. The writer picks its own title from sources.",
		Fields: []agentschema.Field{
			{Key: "topic", Label: "Topic", Type: "text", Required: true, MinLength: 3, Placeholder: "e.g. Edge AI inference in 2026", Question: "What should I write about?"},
			{Key: "context", Label: "Context (optional)", Type: "textarea", Placeholder: "Audience, angle, points to cover or avoid"},
			{Key: "model", Label: "Model override (optional)", Type: "text", Placeholder: "agent default"},
		},
		TitleRules: []agentschema.TitleRule{
			{Prefix: "Write: ", FromKey: "topic", Fallback: "article"},
		},
		DescRules: []agentschema.DescRule{
			{Key: "topic", Prefix: "Topic: "},
			{Key: "context"},
		},
	}
}

// Seed is the built-in Article Writer.
func Seed(orgID uuid.UUID) *model.Agent {
	desc := "Writes SEO-optimised technical articles in markdown, converts them to HTML, and files them in the CMS as drafts for review."
	prompt := "You are a technical blog writer for a developer audience. You produce high-quality, SEO-optimised articles in pure markdown: a single H1 title, H2/H3 structure, 800-1200 words, at least one code block where it helps the reader, and a short Further Reading list."
	return &model.Agent{
		ID:           uuid.New(),
		OrgID:        orgID,
		Name:         "Article Writer",
		Role:         "Content Writer",
		Description:  &desc,
		SystemPrompt: &prompt,
		Status:       "active",
		EngineType:   model.EngineGoNative,
		EngineConfig: map[string]any{},
		Metadata:     map[string]any{model.MetadataKeyBuiltin: model.BuiltinArticleWriter},
	}
}

func launch(w Writer) agentmodule.LaunchFunc {
	return func(ctx context.Context, in agentmodule.LaunchInput) (*agentmodule.LaunchOutput, error) {
		if w == nil {
			return nil, fmt.Errorf("article writer is not configured")
		}
		uid := in.UserID
		tid := in.Task.ID
		run, err := w.Generate(ctx, in.OrgID, &uid, in.Source, model.GenerateBlogRequest{
			Briefs: []model.BlogBrief{{
				Topic:   strings.TrimSpace(in.Values["topic"]),
				Context: strings.TrimSpace(in.Values["context"]),
			}},
			Model:  strings.TrimSpace(in.Values["model"]),
			TaskID: &tid,
		})
		if err != nil {
			return nil, err
		}
		id := run.ID
		link := "Article run started. Open /articles/" + id.String() + " when it finishes."
		if in.Task != nil && in.Task.Description != nil {
			prior := strings.TrimSpace(*in.Task.Description)
			if prior != "" {
				link = prior + "\n\n" + link
			}
		}
		return &agentmodule.LaunchOutput{
			RunID:       &id,
			Description: link,
			Message:     "Article run started",
			ExtraMeta:   map[string]any{model.TaskMetaRunID: id.String()},
		}, nil
	}
}
