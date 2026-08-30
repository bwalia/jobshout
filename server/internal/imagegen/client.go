// Package imagegen provides a provider-agnostic interface for turning a text
// prompt into a picture. Concrete implementations live alongside this file
// (mflux.go, openai.go), and Router selects between them at runtime — the same
// shape the llm package uses for language models, for the same reasons.
//
// The pictures themselves are not this package's problem beyond producing them:
// storing the bytes and handing out a URL belongs to the caller, so that a
// generated image and an uploaded one end up in the same place by the same
// route.
package imagegen

import (
	"context"
	"fmt"
	"strings"
)

// Provider names. These are the values that appear in configuration and in the
// API, so they are constants rather than loose strings.
const (
	// ProviderMFlux is the workstation image service (mflux on Apple MLX).
	ProviderMFlux = "mflux"
	// ProviderGemini is Google's hosted Gemini image API.
	ProviderGemini = "gemini"
	// ProviderOpenAI is OpenAI's hosted image API.
	ProviderOpenAI = "openai"
)

// DefaultWidth and DefaultHeight produce a 16:9 image, which is the shape a
// cover image is displayed in. Generating square and cropping would throw away
// pixels the model was asked to spend effort on.
const (
	DefaultWidth  = 1024
	DefaultHeight = 576
)

// GenerateRequest is the input to a Generate call.
type GenerateRequest struct {
	// Prompt describes the image. Required.
	Prompt string
	// Model overrides the client's configured default.
	Model string
	// Width and Height are in pixels; zero means the package default.
	Width, Height int
	// Steps is the number of denoising steps. Zero means the provider default.
	// Ignored by providers that do not expose it.
	Steps int
	// Seed makes generation reproducible. Negative means "choose one", and the
	// chosen value comes back on the response — without that, an image that
	// came out well could never be reproduced.
	Seed int64
	// NegativePrompt describes what to avoid. Ignored by providers that have no
	// such concept rather than being faked with prompt text, which would change
	// the meaning of the prompt the caller wrote.
	NegativePrompt string
}

// GenerateResponse holds the produced image and what produced it.
type GenerateResponse struct {
	// PNG is the raw image data.
	PNG []byte
	// Provider and Model record what actually served the request, which is not
	// always what was asked for — an empty Model means "the provider's default"
	// and the response says which that was.
	Provider string
	Model    string
	// Seed is the seed actually used, including one the provider chose.
	Seed int64
	// Width, Height and Steps are what was actually generated.
	Width, Height, Steps int
	// DurationMS is how long the provider took. Recorded because image
	// generation is slow enough that a run's timing is worth explaining.
	DurationMS int
}

// ModelInfo describes one model a provider can run.
type ModelInfo struct {
	Name     string `json:"name"`
	Provider string `json:"provider"`
	// Repo identifies the upstream weights, where the provider has such a
	// notion. Empty for hosted APIs.
	Repo string `json:"repo,omitempty"`
	// Available reports whether this model can generate now. A local model
	// whose weights are not downloaded is known but not available, and calling
	// it triggers a multi-gigabyte download instead of an image — a difference
	// worth showing rather than hiding.
	Available bool `json:"available"`
	// Fast marks models that reach a usable image in a handful of steps.
	Fast bool `json:"fast"`
}

// Client is one image generation provider.
type Client interface {
	// Generate produces a single image.
	Generate(ctx context.Context, req GenerateRequest) (*GenerateResponse, error)
	// ListModels reports what this provider can run. A provider that cannot be
	// asked returns its static list rather than an error, so a picker still has
	// something to show when discovery is unavailable.
	ListModels(ctx context.Context) ([]ModelInfo, error)
	// Provider is the name this client is registered under.
	Provider() string
}

// Normalize fills in defaults and rejects requests that cannot be served.
//
// It runs before the provider is chosen so that every provider sees the same
// validated request, rather than each re-deriving what a zero width means.
func (r *GenerateRequest) Normalize() error {
	r.Prompt = strings.TrimSpace(r.Prompt)
	if r.Prompt == "" {
		return fmt.Errorf("imagegen: a prompt is required")
	}
	if r.Width <= 0 {
		r.Width = DefaultWidth
	}
	if r.Height <= 0 {
		r.Height = DefaultHeight
	}
	// Dimensions are rounded up to a multiple of 16 rather than rejected: the
	// VAE and transformer both downsample, so a size that is not a multiple is
	// resolved somewhere inside the model anyway. Deciding it here means the
	// response reports the size that was actually produced.
	r.Width = roundUpTo(r.Width, 16)
	r.Height = roundUpTo(r.Height, 16)
	if r.Seed < 0 {
		r.Seed = -1
	}
	r.Model = strings.TrimSpace(r.Model)
	r.NegativePrompt = strings.TrimSpace(r.NegativePrompt)
	return nil
}

func roundUpTo(v, multiple int) int {
	if v%multiple == 0 {
		return v
	}
	return v + (multiple - v%multiple)
}
