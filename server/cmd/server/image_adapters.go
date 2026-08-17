package main

import (
	"context"

	"github.com/jobshout/server/internal/blog"
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

func (b *blogIllustrator) Generate(ctx context.Context, req blog.IllustrationRequest) (*blog.Illustration, error) {
	result, err := b.images.Generate(ctx, service.GenerateImageRequest{
		OrgID:  req.OrgID,
		Prompt: req.Prompt,
		Width:  req.Width,
		Height: req.Height,
		// Unseeded: two articles about the same subject should not get the same
		// picture, which is what a fixed seed would produce.
		Seed:   -1,
		Source: req.Source,
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
	}, nil
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
