package main

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/service"
)

// courseIllustrator adapts *service.ImageService to course.Illustrator, for
// the same reason blogIllustrator exists: the course package is handed what
// it needs and does not import the service layer.
type courseIllustrator struct {
	images *service.ImageService
}

func (c *courseIllustrator) Illustrate(ctx context.Context, orgID uuid.UUID, userID *uuid.UUID, prompt string, width, height int) (string, error) {
	result, err := c.images.Generate(ctx, service.GenerateImageRequest{
		OrgID:  orgID,
		UserID: userID,
		Prompt: prompt,
		Width:  width,
		Height: height,
		// Unseeded so two courses on one subject get different pictures.
		Seed:   -1,
		Source: "course",
	})
	if err != nil {
		return "", err
	}
	if result.URL == "" {
		return "", errors.New("image generated but there is no image store to serve it from")
	}
	return result.URL, nil
}
