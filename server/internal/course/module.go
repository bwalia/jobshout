package course

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/agentmodule"
	"github.com/jobshout/server/internal/agentschema"
	"github.com/jobshout/server/internal/model"
)

// Runner is the launch surface satisfied by the course service.
type Runner interface {
	CreateRun(ctx context.Context, req model.CreateCourseRunRequest, orgID uuid.UUID, requestedBy *uuid.UUID) (*model.CourseRun, error)
}

// Module is the Course Generator specialist.
func Module(run Runner) agentmodule.Module {
	return agentmodule.Module{
		Builtin:   model.BuiltinCourseGenerator,
		Label:     "Course Generator",
		Icon:      "graduation-cap",
		TabSlug:   "courses",
		Hint:      "Research a subject and write a course: chapters, theory, visuals and a quiz per chapter, saved for review.",
		ChatHint:  "To create a training course, call agent_execute on the Course Generator with a topic; add audience, level, chapters and language when the user gives them.",
		Schema:    schema(),
		Seed:      Seed,
		Launch:    launch(run),
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
		Builtin: model.BuiltinCourseGenerator,
		Hint:    "Research a subject and generate a structured course for review.",
		Fields: []agentschema.Field{
			{Key: "topic", Label: "Course topic", Type: "text", Required: true, MinLength: 3,
				Placeholder: "Kubernetes networking for platform engineers",
				Question:    "What should the course teach?"},
			{Key: "audience", Label: "Audience", Type: "text",
				Placeholder: "Backend developers new to Kubernetes"},
			{Key: "level", Label: "Level", Type: "select", Default: "beginner",
				Options: []model.ClarifyOption{
					{Label: "Beginner", Value: "beginner"},
					{Label: "Intermediate", Value: "intermediate"},
					{Label: "Advanced", Value: "advanced"},
				}},
			{Key: "chapters", Label: "Chapters", Type: "select", Default: "5",
				Options: chapterOptions(),
				Help:    "Each chapter becomes one Academy lesson"},
			{Key: "locale", Label: "Language", Type: "text", Default: "en",
				Placeholder: "en",
				Help:        "Language tag, e.g. en, hi, pa"},
			{Key: "seed_urls", Label: "Source URLs (optional)", Type: "textarea",
				Placeholder: "https://kubernetes.io/docs/concepts/services-networking/",
				Help:        "One per line; read first during research"},
			{Key: "focus", Label: "Focus areas (optional)", Type: "text",
				Placeholder: "Services, Ingress, NetworkPolicy",
				Help:        "Comma-separated; keeps research on-subject"},
			{Key: "context", Label: "Notes (optional)", Type: "textarea",
				Placeholder: "e.g. UK English; hands-on examples; avoid cloud-vendor specifics"},
		},
		TitleRules: []agentschema.TitleRule{
			{Prefix: "Course: ", FromKey: "topic", Fallback: "new course"},
		},
		DescRules: []agentschema.DescRule{
			{Key: "audience", Prefix: "Audience: "},
			{Key: "level", Prefix: "Level: "},
			{Key: "chapters", Prefix: "Chapters: "},
			{Key: "locale", Prefix: "Language: "},
			{Key: "context"},
		},
	}
}

func chapterOptions() []model.ClarifyOption {
	opts := make([]model.ClarifyOption, 0, model.CourseMaxChapters)
	for n := model.CourseMinChapters; n <= model.CourseMaxChapters; n++ {
		v := fmt.Sprint(n)
		opts = append(opts, model.ClarifyOption{Label: v, Value: v})
	}
	return opts
}

// Seed is the built-in Course Generator agent.
func Seed(orgID uuid.UUID) *model.Agent {
	desc := "Researches a subject and writes a structured course: outline, chapter theory, visuals and a quiz per chapter, saved for review before it goes to Workstation Academy."
	prompt := "You are the Course Generator agent. Research the subject, plan a course of clear chapters with learning objectives, write accurate chapter theory for the stated audience and level, illustrate it, and write a multiple-choice quiz for each chapter. Cite only what research supports."
	return &model.Agent{
		ID:           uuid.New(),
		OrgID:        orgID,
		Name:         model.AgentNameCourse,
		Role:         "Course Author",
		Description:  &desc,
		SystemPrompt: &prompt,
		Status:       "active",
		EngineType:   model.EngineGoNative,
		EngineConfig: map[string]any{},
		Metadata:     map[string]any{model.MetadataKeyBuiltin: model.BuiltinCourseGenerator},
	}
}

func launch(run Runner) agentmodule.LaunchFunc {
	return func(ctx context.Context, in agentmodule.LaunchInput) (*agentmodule.LaunchOutput, error) {
		if run == nil {
			return nil, fmt.Errorf("Course Generator is not configured")
		}
		if in.Agent == nil || in.Task == nil {
			return nil, fmt.Errorf("course launch needs an agent and a task")
		}
		req := model.CreateCourseRunRequest{
			AgentID: in.Agent.ID,
			TaskID:  &in.Task.ID,
			Brief:   BriefFromValues(in.Values),
		}
		created, err := run.CreateRun(ctx, req, in.OrgID, &in.UserID)
		if err != nil {
			return nil, err
		}
		id := created.ID
		return &agentmodule.LaunchOutput{
			RunID:     &id,
			Message:   "Course Generator run queued",
			ExtraMeta: map[string]any{model.TaskMetaRunID: id.String()},
		}, nil
	}
}
