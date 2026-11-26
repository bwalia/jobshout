package main

import (
	"context"

	"github.com/jobshout/server/internal/blog"
	"github.com/jobshout/server/internal/imagegen"
	"github.com/jobshout/server/internal/llm"
	"github.com/jobshout/server/internal/marketingemail"
	"github.com/jobshout/server/internal/service"
	"github.com/jobshout/server/internal/tools"
)

// The image service is consumed by two packages that cannot import it.
//
// internal/tools cannot, because internal/service imports internal/executor
// which imports internal/tools — depending on service from there would close
// the loop. internal/blog declines to for its own reason: it is handed what it
// needs and knows nothing about the platform's service layer, which is what
// lets its tests run without one.
//
// Both therefore declare the narrow interface they want in their own terms, and
// these adapters translate. Wiring is the right place for that: it is the only
// layer that already knows about every other one.

// blogIllustrator adapts *service.ImageService to blog.Illustrator.
type blogIllustrator struct {
	images *service.ImageService
}

func (b *blogIllustrator) Enabled() bool { return b.images.Enabled() }

func (b *blogIllustrator) Letters(need imagegen.TextNeed) bool { return b.images.CanLetter(need) }

func (b *blogIllustrator) Generate(ctx context.Context, req blog.IllustrationRequest) (*blog.Illustration, error) {
	// A provider/model chosen for this one post (WithImageChoice) applies to
	// its cover only, never to figures or other callers.
	provider, model := "", req.Model
	// It is skipped for a picture whose labels must be spelt right when the
	// chosen provider cannot letter them: the pipeline then draws that one with
	// the default, and uses the choice for the wordless art under the labels.
	if c, ok := blog.ImageChoiceFrom(ctx); ok && req.Source == "blog_cover" && model == "" &&
		(req.NoText || c.Provider == "" || imagegen.Letters(c.Provider, c.Model, req.Text)) {
		provider, model = c.Provider, c.Model
	}
	result, err := b.images.Generate(ctx, service.GenerateImageRequest{
		OrgID:    req.OrgID,
		Prompt:   req.Prompt,
		Provider: provider,
		Model:    model,
		Width:    req.Width,
		Height:   req.Height,
		Steps:    req.Steps,
		// Unseeded: two articles about the same subject should not get the same
		// picture, which is what a fixed seed would produce.
		Seed:       -1,
		Source:     req.Source,
		BlogRunID:  req.RunID,
		Text:       req.Text,
		ExpectText: req.ExpectText,
		NoText:     req.NoText,
	})
	if err != nil {
		return nil, err
	}
	return &blog.Illustration{
		URL:      result.URL,
		Provider: result.Provider,
		Model:    result.Model,
		Seed:     result.Seed,
		Width:    result.Width,
		Height:   result.Height,
		PNG:      result.PNG,
	}, nil
}

// emailIllustrator adapts *service.ImageService to marketingemail.Illustrator.
type emailIllustrator struct {
	images *service.ImageService
}

func (e *emailIllustrator) Enabled() bool { return e.images != nil && e.images.Enabled() }

func (e *emailIllustrator) Generate(ctx context.Context, req marketingemail.IllustrationRequest) (*marketingemail.Illustration, error) {
	result, err := e.images.Generate(ctx, service.GenerateImageRequest{
		OrgID:  req.OrgID,
		Prompt: req.Prompt,
		Width:  req.Width,
		Height: req.Height,
		Seed:   -1,
		Source: req.Source,
	})
	if err != nil {
		return nil, err
	}
	return &marketingemail.Illustration{URL: result.URL}, nil
}

// toolImageGenerator adapts *service.ImageService to tools.ImageGenerator.
type toolImageGenerator struct {
	images *service.ImageService
}

func (t *toolImageGenerator) Enabled() bool { return t.images.Enabled() }

func (t *toolImageGenerator) GenerateImage(ctx context.Context, req tools.ImageRequest) (*tools.ImageResult, error) {
	result, err := t.images.Generate(ctx, service.GenerateImageRequest{
		OrgID:  req.OrgID,
		Prompt: req.Prompt,
		Width:  req.Width,
		Height: req.Height,
		Seed:   req.Seed,
		Source: req.Source,
	})
	if err != nil {
		return nil, err
	}
	return &tools.ImageResult{
		URL:      result.URL,
		Provider: result.Provider,
		Model:    result.Model,
		Seed:     result.Seed,
		Width:    result.Width,
		Height:   result.Height,
	}, nil
}

// coverDescriber adapts the Ollama vision call to blog.CoverDescriber.
type coverDescriber struct {
	client *llm.OllamaClient
	model  string
}

func (d *coverDescriber) DescribeImage(ctx context.Context, prompt string, png []byte) (string, error) {
	return d.client.DescribeImage(ctx, d.model, prompt, png)
}
