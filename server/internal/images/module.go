package images

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/agentmodule"
	"github.com/jobshout/server/internal/agentschema"
	"github.com/jobshout/server/internal/model"
)

// Generator is the launch surface.
type Generator interface {
	Enabled() bool
	Generate(ctx context.Context, orgID, userID uuid.UUID, prompt, source string) (url string, recordID *uuid.UUID, err error)
}

// Module is the Image Generator specialist.
//
// All specialists are wired this way: own package, then one Register call.
func Module(gen Generator) agentmodule.Module {
	return agentmodule.Module{
		Builtin:  model.BuiltinImages,
		Label:    "Image Generator",
		Icon:     "image",
		TabSlug:  "images",
		Hint:     "Generate one image from a prompt. The board task stores the result.",
		ChatHint: "To generate an image as an agent run, call agent_execute on the Image Generator. To draw a picture that is not an agent run, call image_generate with the prompt.",
		Schema:   schema(),
		Seed:     Seed,
		Launch:   launch(gen),
		AbsorbPrompt: func(prompt string, vals map[string]string) {
			if vals["prompt"] == "" {
				vals["prompt"] = prompt
			}
		},
	}
}

func schema() agentschema.Schema {
	return agentschema.Schema{
		Builtin:        model.BuiltinImages,
		SpecialistTool: "image_generate",
		Hint:           "Generate one image from a prompt. The board task stores the result.",
		Fields: []agentschema.Field{
			{Key: "prompt", Label: "Image prompt", Type: "textarea", Required: true, MinLength: 3, Placeholder: "A dark editorial cover of a harbour at night…", Question: "What should I generate?"},
		},
		TitleRules: []agentschema.TitleRule{
			{Prefix: "Image: ", FromKey: "prompt", Truncate: 80, Fallback: "image"},
		},
		DescRules: []agentschema.DescRule{
			{Key: "prompt"},
		},
	}
}

// Seed is the built-in Image Generator.
func Seed(orgID uuid.UUID) *model.Agent {
	desc := "Generates one image from a prompt and stores it on the Task Manager board."
	prompt := "You generate images from a written prompt. Ask for the picture the user wants. Do not invent a subject."
	return &model.Agent{
		ID:           uuid.New(),
		OrgID:        orgID,
		Name:         "Image Generator",
		Role:         "Image",
		Description:  &desc,
		SystemPrompt: &prompt,
		Status:       "active",
		EngineType:   model.EngineGoNative,
		EngineConfig: map[string]any{},
		Metadata:     map[string]any{model.MetadataKeyBuiltin: model.BuiltinImages},
	}
}

func launch(gen Generator) agentmodule.LaunchFunc {
	return func(ctx context.Context, in agentmodule.LaunchInput) (*agentmodule.LaunchOutput, error) {
		if gen == nil || !gen.Enabled() {
			return nil, fmt.Errorf("image generation is not configured")
		}
		// Always task_manager — the Image Generator tab and chat agent_execute
		// share this launch. Do not pass in.Source ("chat"); history used to
		// record every specialist image as task_manager.
		url, rec, err := gen.Generate(ctx, in.OrgID, in.UserID, strings.TrimSpace(in.Values["prompt"]), "task_manager")
		if err != nil {
			return nil, err
		}
		text := "Generated image"
		if url != "" {
			text = "Generated image: " + url
		}
		return &agentmodule.LaunchOutput{
			RunID:       rec,
			ImageURL:    url,
			Description: text,
			Status:      "done",
			Message:     "Image generated",
		}, nil
	}
}
