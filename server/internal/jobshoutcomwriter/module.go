// Package jobshoutcomwriter is the JobShout.com Content Writer: long-form
// articles on the latest developments in AI, written for the Insights hub on
// jobshout.com.
//
// It writes with the Article Writer's research-and-writing pipeline, as its own
// agent: its runs are attributed to it, listed on its own tab, and written for
// the hidden long-form Insights reader. Writing ends with a CMS draft. A person
// then publishes it live from this agent's tab, which makes the CMS post public
// and publishes the article on jobshout.com in one step.
package jobshoutcomwriter

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

// DefaultFocus steers a topic-less run toward the subjects Insights covers.
var DefaultFocus = []string{
	"AI models and reasoning models",
	"AI agents and agent harnesses",
	"model routing and inference economics",
	"MCP and tool use",
	"AI evaluation and benchmarks",
	"AI security and guardrails",
	"AI infrastructure",
}

// Module is the JobShout.com Content Writer specialist.
func Module(w Writer) agentmodule.Module {
	return agentmodule.Module{
		Builtin:   model.BuiltinJobShoutComWriter,
		Label:     model.AgentNameJobShoutComWriter,
		Icon:      "newspaper",
		TabSlug:   "insights-writer",
		Hint:      "Long-form AI-trends articles for jobshout.com. Leave the topic empty to write about what is trending.",
		ChatHint:  "To write a long-form article for jobshout.com Insights, call agent_execute on the JobShout.com Content Writer; a topic is optional — without one it picks a trending AI subject. It files a CMS draft; publishing live happens on its tab.",
		Schema:    schema(),
		Seed:      Seed,
		Launch:    launch(w),
		StayOnTab: true,
		AbsorbPrompt: func(prompt string, vals map[string]string) {
			if strings.TrimSpace(vals["topic"]) == "" {
				vals["topic"] = strings.TrimSpace(prompt)
			}
		},
	}
}

func schema() agentschema.Schema {
	return agentschema.Schema{
		Builtin: model.BuiltinJobShoutComWriter,
		Hint:    "Long-form AI-trends article for jobshout.com Insights. Empty topic = pick what is trending.",
		Fields: []agentschema.Field{
			{Key: "topic", Label: "Topic (optional)", Type: "text",
				Placeholder: "e.g. Decision models in agent harnesses",
				Question:    "What should the article cover? Leave it empty and I'll pick a trending AI subject."},
			{Key: "context", Label: "Brief (optional)", Type: "textarea",
				Placeholder: "Sources to start from, angle, points to cover or avoid"},
			{Key: "focus", Label: "Focus areas (optional)", Type: "text",
				Placeholder: "agents, model routing, AI security",
				Help:        "Comma-separated; steers topic discovery when no topic is given"},
			{Key: "model", Label: "Model override (optional)", Type: "text", Placeholder: "agent default"},
		},
		TitleRules: []agentschema.TitleRule{
			{Prefix: "Insights: ", FromKey: "topic", Fallback: "trending AI article"},
		},
		DescRules: []agentschema.DescRule{
			{Key: "topic", Prefix: "Topic: "},
			{Key: "focus", Prefix: "Focus: "},
			{Key: "context"},
		},
	}
}

// Seed is the built-in JobShout.com Content Writer.
func Seed(orgID uuid.UUID) *model.Agent {
	desc := "Writes independent long-form articles on the latest developments in AI for jobshout.com Insights, files them in the CMS as drafts, and publishes them on jobshout.com when a person publishes them live."
	prompt := "You are the JobShout.com Content Writer. You write independent, long-form analysis of the latest developments in AI for engineers and technical leaders. You write from verified sources, attribute every vendor and partner claim to whoever makes it, date anything that can go stale, never claim JobShout measured something it did not, and say where a technology does not fit as well as where it does."
	return &model.Agent{
		ID:           uuid.New(),
		OrgID:        orgID,
		Name:         model.AgentNameJobShoutComWriter,
		Role:         "Insights Writer",
		Description:  &desc,
		SystemPrompt: &prompt,
		Status:       "active",
		EngineType:   model.EngineGoNative,
		EngineConfig: map[string]any{},
		Metadata:     map[string]any{model.MetadataKeyBuiltin: model.BuiltinJobShoutComWriter},
	}
}

// Request builds the run request for a launch's values. Exported for the
// schedule and tests: a scheduled run is the same request with no topic.
func Request(values map[string]string) model.GenerateBlogRequest {
	req := model.GenerateBlogRequest{
		Writer:      model.BuiltinJobShoutComWriter,
		Audience:    audience.InsightsKey,
		Model:       strings.TrimSpace(values["model"]),
		AutoPublish: true, // files the CMS draft; going live is a person's call
	}
	topic := strings.TrimSpace(values["topic"])
	if topic == "" {
		req.Trending = true
		req.TrendingCount = 1
		req.Focus = splitFocus(values["focus"])
		if len(req.Focus) == 0 {
			req.Focus = append([]string(nil), DefaultFocus...)
		}
		return req
	}
	req.Briefs = []model.BlogBrief{{Topic: topic, Context: strings.TrimSpace(values["context"])}}
	return req
}

func splitFocus(raw string) []string {
	var out []string
	for _, f := range strings.Split(raw, ",") {
		if f = strings.TrimSpace(f); f != "" {
			out = append(out, f)
		}
	}
	return out
}

func launch(w Writer) agentmodule.LaunchFunc {
	return func(ctx context.Context, in agentmodule.LaunchInput) (*agentmodule.LaunchOutput, error) {
		if w == nil {
			return nil, fmt.Errorf("the JobShout.com Content Writer is not configured")
		}
		uid := in.UserID
		req := Request(in.Values)
		if in.Task != nil {
			tid := in.Task.ID
			req.TaskID = &tid
		}
		run, err := w.Generate(ctx, in.OrgID, &uid, in.Source, req)
		if err != nil {
			return nil, err
		}
		id := run.ID
		return &agentmodule.LaunchOutput{
			RunID:       &id,
			Description: "Insights article run started. It files a CMS draft when done; publish it live from the JobShout.com Content Writer tab.",
			Message:     "Insights article run started",
			ExtraMeta:   map[string]any{model.TaskMetaRunID: id.String()},
		}, nil
	}
}
