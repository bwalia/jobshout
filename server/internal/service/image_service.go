package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/imagegen"
	"github.com/jobshout/server/internal/imagestore"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/repository"
)

// ImageService turns a prompt into a stored, recorded image.
//
// It is the single place the three steps happen together — generate, store,
// record — so that a cover image drawn by a blog run, an image an agent asked
// for, and one a person clicked for all end up in the same bucket and the same
// table, described the same way.
type ImageService struct {
	router *imagegen.Router
	// store is nil when object storage is not configured. Generation still
	// works in that case and the bytes go back to the caller; what is lost is
	// somewhere to serve them from later, which is a smaller loss than refusing
	// to draw at all on a developer's laptop.
	store  imagestore.Store
	repo   repository.ImageRepository
	logger *zap.Logger
}

// NewImageService builds the service. Both store and repo may be nil.
func NewImageService(
	router *imagegen.Router,
	store imagestore.Store,
	repo repository.ImageRepository,
	logger *zap.Logger,
) *ImageService {
	return &ImageService{router: router, store: store, repo: repo, logger: logger}
}

// Enabled reports whether any image provider is configured. Callers use it to
// leave image work out of a run rather than failing a step over it.
func (s *ImageService) Enabled() bool {
	return s != nil && s.router.Enabled()
}

// GenerateRequest is what a caller asks for.
type GenerateImageRequest struct {
	OrgID  uuid.UUID
	UserID *uuid.UUID
	Prompt string
	// Provider and Model may be empty, meaning "whatever is configured".
	Provider string
	Model    string
	Width    int
	Height   int
	Steps    int
	Seed     int64
	// Source records what asked for the image. Defaults to manual.
	Source string
}

// GenerateImageResult is the produced image, plus where it went.
type GenerateImageResult struct {
	// PNG is always populated. URL is empty when there was nowhere to store it.
	PNG []byte
	URL string

	Provider   string
	Model      string
	Seed       int64
	Width      int
	Height     int
	DurationMS int
	// RecordID is the generated_images row, nil when there was no repository to
	// write to.
	RecordID *uuid.UUID
}

// Generate produces one image, stores it if it can, and records that it
// happened.
//
// Storage and recording failures are logged and swallowed rather than returned:
// the expensive part already succeeded, and throwing away a picture that took
// 25 seconds of a shared GPU because a bookkeeping row would not insert serves
// nobody. The caller can tell what happened from the empty URL.
func (s *ImageService) Generate(ctx context.Context, req GenerateImageRequest) (*GenerateImageResult, error) {
	if !s.Enabled() {
		return nil, fmt.Errorf("image generation is not configured on this server")
	}
	if strings.TrimSpace(req.Prompt) == "" {
		return nil, fmt.Errorf("a prompt is required")
	}

	seed := req.Seed
	if seed == 0 {
		// Zero is the zero value of an omitted field, not a request for seed 0.
		// -1 is this package's "choose one", so an unset seed means unseeded
		// rather than always producing the same picture.
		seed = -1
	}

	resp, err := s.router.Generate(ctx, req.Provider, imagegen.GenerateRequest{
		Prompt: req.Prompt,
		Model:  req.Model,
		Width:  req.Width,
		Height: req.Height,
		Steps:  req.Steps,
		Seed:   seed,
	})
	if err != nil {
		return nil, err
	}

	result := &GenerateImageResult{
		PNG:        resp.PNG,
		Provider:   resp.Provider,
		Model:      resp.Model,
		Seed:       resp.Seed,
		Width:      resp.Width,
		Height:     resp.Height,
		DurationMS: resp.DurationMS,
	}

	if s.store != nil {
		url, err := s.store.Put(ctx, req.OrgID, resp.PNG)
		if err != nil {
			s.logger.Warn("image generated but not stored",
				zap.Error(err), zap.String("provider", resp.Provider))
		} else {
			result.URL = url
		}
	}

	if s.repo != nil {
		source := req.Source
		if source == "" {
			source = model.ImageSourceManual
		}
		record := &model.GeneratedImage{
			OrgID:      req.OrgID,
			CreatedBy:  req.UserID,
			Prompt:     req.Prompt,
			Provider:   resp.Provider,
			Model:      resp.Model,
			Seed:       resp.Seed,
			Width:      resp.Width,
			Height:     resp.Height,
			URL:        result.URL,
			Source:     source,
			DurationMS: resp.DurationMS,
		}
		if err := s.repo.Create(ctx, record); err != nil {
			s.logger.Warn("image generated but not recorded", zap.Error(err))
		} else {
			id := record.ID
			result.RecordID = &id
		}
	}

	return result, nil
}

// ListModels reports every image model available across configured providers.
func (s *ImageService) ListModels(ctx context.Context) []imagegen.ModelInfo {
	if !s.Enabled() {
		return nil
	}
	return s.router.ListModels(ctx)
}

// DefaultProvider names the provider used when a request does not choose one.
func (s *ImageService) DefaultProvider() string {
	if s == nil {
		return ""
	}
	return s.router.DefaultProvider()
}

// Recent lists the images this org has generated, newest first.
func (s *ImageService) Recent(ctx context.Context, orgID uuid.UUID, limit int) ([]model.GeneratedImage, error) {
	if s.repo == nil {
		return nil, fmt.Errorf("image history is not available on this server")
	}
	return s.repo.ListByOrg(ctx, orgID, limit)
}

// Fetch returns a stored image's bytes by object key.
func (s *ImageService) Fetch(ctx context.Context, key string) ([]byte, error) {
	if s.store == nil {
		return nil, fmt.Errorf("image storage is not configured on this server")
	}
	return s.store.Get(ctx, key)
}

// imagesAgentSeed is the built-in Image Generator. Migration 000035 backfills
// existing orgs; auth_service.Register covers new ones.
func imagesAgentSeed(orgID uuid.UUID) *model.Agent {
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
