package blog

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/agentmodule"
	"github.com/jobshout/server/internal/agentschema"
	"github.com/jobshout/server/internal/audience"
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
		Hint:     "Give a topic and say who is reading. The writer picks its own title from sources.",
		ChatHint: "To write an article, call agent_execute on the Article Writer (or article_generate). Do not invent a topic. Ask who it is for — developers, a technote, or a business audience — rather than assuming developers.",
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
		Hint:           "Give a topic and say who is reading. The writer picks its own title from sources.",
		Fields: []agentschema.Field{
			{Key: "topic", Label: "Topic", Type: "text", Required: true, MinLength: 3, Placeholder: "e.g. Edge AI inference in 2026", Question: "What should I write about?"},
			{
				Key: "audience", Label: "Written for", Type: "select",
				Question: "Who is reading this — developers, or a business audience?",
				Default:  audience.DefaultKey,
				Help:     "Changes how it is researched, planned, written and reviewed, not just the wording.",
				Options:  audienceOptions(),
			},
			{Key: "industry", Label: "Industry (optional)", Type: "text", Placeholder: "e.g. NHS trusts, 3PL logistics, commercial property", Help: "Frames the examples and consequences for one sector."},
			{Key: "context", Label: "Context (optional)", Type: "textarea", Placeholder: "Angle, points to cover or avoid"},
			{Key: "model", Label: "Model override (optional)", Type: "text", Placeholder: "agent default"},
		},
		TitleRules: []agentschema.TitleRule{
			{Prefix: "Write: ", FromKey: "topic", Fallback: "article"},
		},
		DescRules: []agentschema.DescRule{
			{Key: "topic", Prefix: "Topic: "},
			{Key: "audience", Prefix: "For: "},
			{Key: "industry", Prefix: "Industry: "},
			{Key: "context"},
		},
	}
}

// audienceOptions is the audience registry as schema picker entries.
//
// The mapping lives here rather than in the audience package because that
// package deliberately imports nothing from the server — it is consumed by
// model, research and blog, and a dependency on model would be a cycle.
//
// Adding a reader is adding a row to audience.profiles: the form, the chat
// interview and the schedule picker all read this.
func audienceOptions() []model.ClarifyOption {
	opts := audience.Options()
	out := make([]model.ClarifyOption, 0, len(opts))
	for _, o := range opts {
		out = append(out, model.ClarifyOption{Label: o.Label, Value: o.Value})
	}
	return out
}

// Seed is the built-in Article Writer.
func Seed(orgID uuid.UUID) *model.Agent {
	desc := "Writes SEO-optimised articles in markdown for whichever audience a run names — developer deep dives, developer technotes, or plain-English business briefings — converts them to HTML, and files them for review as CMS drafts and in the JobShout.com Insights review queue."
	prompt := "You are a content writer. Every run tells you who is reading — a developer audience, an engineer who wants the task solved, or a business manager who does not write code — and that decides how you research, structure and write the piece, not just its wording. You produce high-quality, SEO-optimised articles in pure markdown: a single H1 title, H2/H3 structure, and code only where the reader is someone who would run it. You write from verified sources and never invent a citation."
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
			Audience: strings.TrimSpace(in.Values["audience"]),
			Industry: strings.TrimSpace(in.Values["industry"]),
			Model:    strings.TrimSpace(in.Values["model"]),
			TaskID:   &tid,
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
