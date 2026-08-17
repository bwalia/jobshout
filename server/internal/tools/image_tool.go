package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
)

// ImageRequest and ImageResult are this package's own view of an image
// generation call.
//
// They restate what internal/service already models rather than importing it,
// because internal/service imports internal/executor, which imports this
// package — depending on service from here would close that loop. The wiring in
// cmd/server adapts between the two, which is where a translation between two
// layers belongs anyway.
type ImageRequest struct {
	OrgID  uuid.UUID
	Prompt string
	Width  int
	Height int
	Seed   int64
	Source string
}

// ImageResult is what came back.
type ImageResult struct {
	// URL is empty when the image was generated but object storage was not
	// configured to keep it.
	URL      string
	Provider string
	Model    string
	Seed     int64
	Width    int
	Height   int
}

// ImageGenerator is what generate_image needs from the platform. Declared here,
// where it is consumed, so a test can substitute a fake without a model, a
// bucket or a database — the same pattern ResearchClient follows.
type ImageGenerator interface {
	GenerateImage(ctx context.Context, req ImageRequest) (*ImageResult, error)
	Enabled() bool
}

// ImageSourceAgentTool marks images this tool produced in the platform's record
// of what has been drawn. It matches model.ImageSourceAgentTool, spelled here
// to keep this package free of that dependency for the reason above.
const ImageSourceAgentTool = "agent_tool"

// GenerateImageTool lets an agent draw a picture.
type GenerateImageTool struct{ images ImageGenerator }

// NewGenerateImageTool builds the tool over an image service.
func NewGenerateImageTool(images ImageGenerator) *GenerateImageTool {
	return &GenerateImageTool{images: images}
}

// Name is the identifier the LLM uses to select this tool.
func (t *GenerateImageTool) Name() string { return "generate_image" }

// Description explains the tool to the model; included verbatim in the prompt.
//
// It says what a good prompt looks like because the quality of the picture is
// almost entirely decided by the sentence the model writes here, and a model
// left to guess writes "an image about kubernetes".
func (t *GenerateImageTool) Description() string {
	return `Generate an image from a text description and return its URL.

Input parameters:
  prompt (string, required) - What to draw. Describe the subject, the style and
    the mood in one sentence, e.g. "a flat vector illustration of a satellite
    dish on a hill at dawn, warm amber tones, minimal editorial style".
    Vague prompts produce vague pictures; name a concrete subject.
  width (integer, optional)  - Pixels, default 1024. Rounded to a multiple of 16.
  height (integer, optional) - Pixels, default 576 (16:9).
  seed (integer, optional)   - Reuse a seed from a previous result to reproduce
    that exact image. Omit to get a new one.

Returns JSON with url, provider, model, seed and dimensions. The url can be
embedded in HTML or markdown. Generation takes roughly 15-30 seconds, so ask for
one image at a time and do not call this repeatedly to "try again" unless the
first result was genuinely unusable.

This tool draws pictures. It cannot read, edit or describe an existing image.`
}

// Parameters advertises the JSON-Schema for native tool-calling providers.
func (t *GenerateImageTool) Parameters() ParameterSchema {
	return ObjectSchema(map[string]any{
		"prompt": map[string]any{
			"type":        "string",
			"description": `What to draw, e.g. "a flat vector illustration of a lighthouse at dawn, warm amber tones"`,
		},
		"width": map[string]any{
			"type":        "integer",
			"description": "Width in pixels (default 1024)",
		},
		"height": map[string]any{
			"type":        "integer",
			"description": "Height in pixels (default 576)",
		},
		"seed": map[string]any{
			"type":        "integer",
			"description": "Reuse a previous result's seed to reproduce that image exactly",
		},
	}, "prompt")
}

// Execute generates the image and returns its URL as JSON.
func (t *GenerateImageTool) Execute(ctx context.Context, input map[string]any) (string, error) {
	if !t.images.Enabled() {
		// Returned as an error rather than an empty result so the model is told
		// the capability is absent, instead of concluding its prompt was bad
		// and rewriting it four times.
		return "", fmt.Errorf("image generation is not configured on this server")
	}

	prompt, err := stringParam(input, "prompt", true)
	if err != nil {
		return "", err
	}

	orgID, ok := OrgFromContext(ctx)
	if !ok {
		return "", fmt.Errorf("generate_image: no organization in context")
	}

	// -1 rather than 0 for an omitted seed: 0 is a valid seed, so defaulting to
	// it would make every unseeded image identical.
	seed := int64(intParam(input, "seed", -1))

	result, err := t.images.GenerateImage(ctx, ImageRequest{
		OrgID:  orgID,
		Prompt: prompt,
		Width:  intParam(input, "width", 0),
		Height: intParam(input, "height", 0),
		Seed:   seed,
		Source: ImageSourceAgentTool,
	})
	if err != nil {
		return "", fmt.Errorf("generate_image: %w", err)
	}

	out := map[string]any{
		"provider": result.Provider,
		"model":    result.Model,
		"seed":     result.Seed,
		"width":    result.Width,
		"height":   result.Height,
	}
	if result.URL != "" {
		out["url"] = result.URL
	} else {
		// The image exists but has nowhere to live. Say so plainly rather than
		// returning a blank url the model would embed into a broken <img>.
		out["url"] = ""
		out["note"] = "the image was generated but object storage is not configured, so it has no URL"
	}

	encoded, err := json.Marshal(out)
	if err != nil {
		return "", fmt.Errorf("generate_image: encode result: %w", err)
	}
	return string(encoded), nil
}

// compile-time checks that the tool satisfies both interfaces.
var (
	_ Tool           = (*GenerateImageTool)(nil)
	_ SchemaProvider = (*GenerateImageTool)(nil)
)
