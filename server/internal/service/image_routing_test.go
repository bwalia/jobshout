package service

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/imagegen"
	"github.com/jobshout/server/internal/model"
)

type routedClient struct {
	name string
	got  *imagegen.GenerateRequest
}

func (c *routedClient) Generate(_ context.Context, req imagegen.GenerateRequest) (*imagegen.GenerateResponse, error) {
	c.got = &req
	return &imagegen.GenerateResponse{PNG: []byte{1}, Provider: c.name, Model: req.Model}, nil
}
func (c *routedClient) ListModels(context.Context) ([]imagegen.ModelInfo, error) { return nil, nil }
func (c *routedClient) Provider() string                                         { return c.name }

type routesRepo struct{ routes map[string]model.ImageRoute }

func (r *routesRepo) Usage(context.Context, uuid.UUID, time.Time) ([]model.ImageUsage, error) {
	return nil, nil
}
func (r *routesRepo) Create(context.Context, *model.GeneratedImage) error { return nil }
func (r *routesRepo) ListByOrg(context.Context, uuid.UUID, int) ([]model.GeneratedImage, error) {
	return nil, nil
}
func (r *routesRepo) GetByID(context.Context, uuid.UUID) (*model.GeneratedImage, error) {
	return nil, nil
}
func (r *routesRepo) Routes(context.Context, uuid.UUID) (map[string]model.ImageRoute, error) {
	return r.routes, nil
}
func (r *routesRepo) PutRoutes(context.Context, uuid.UUID, map[string]model.ImageRoute, *uuid.UUID) error {
	return nil
}

func TestGenerate_UsesTheOrgsModelForTheTask(t *testing.T) {
	mflux := &routedClient{name: imagegen.ProviderMFlux}
	openai := &routedClient{name: imagegen.ProviderOpenAI}
	router := imagegen.NewTestRouter(imagegen.ProviderMFlux, map[string]imagegen.Client{
		imagegen.ProviderMFlux: mflux, imagegen.ProviderOpenAI: openai,
	})
	repo := &routesRepo{routes: map[string]model.ImageRoute{
		model.ImageSourceBlogCover: {Provider: imagegen.ProviderOpenAI, Model: "gpt-image-1"},
		"course":                   {Provider: imagegen.ProviderGemini, Model: "gemini-3-pro-image"}, // not configured
	}}
	svc := NewImageService(router, nil, repo, zap.NewNop())
	gen := func(req GenerateImageRequest) *GenerateImageResult {
		t.Helper()
		req.Prompt = "a lighthouse"
		res, err := svc.Generate(context.Background(), req)
		if err != nil {
			t.Fatal(err)
		}
		return res
	}

	if res := gen(GenerateImageRequest{Source: model.ImageSourceBlogCover}); res.Provider != "openai" || res.Model != "gpt-image-1" {
		t.Errorf("routed task: got %s/%s, want openai/gpt-image-1", res.Provider, res.Model)
	}
	if res := gen(GenerateImageRequest{Source: "course"}); res.Provider != "mflux" {
		t.Errorf("route to an unconfigured provider: got %s, want the default (mflux)", res.Provider)
	}
	if res := gen(GenerateImageRequest{Source: "marketing_email"}); res.Provider != "mflux" {
		t.Errorf("task without a route: got %s, want the default (mflux)", res.Provider)
	}
	if res := gen(GenerateImageRequest{Source: model.ImageSourceBlogCover, Provider: "mflux", Model: "z-image-turbo"}); res.Provider != "mflux" || res.Model != "z-image-turbo" {
		t.Errorf("explicit choice: got %s/%s, want mflux/z-image-turbo", res.Provider, res.Model)
	}
}
