package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/imagegen"
	"github.com/jobshout/server/internal/middleware"
	"github.com/jobshout/server/internal/service"
)

// stubImageClient stands in for a provider without a GPU behind it.
type stubImageClient struct {
	provider string
	models   []imagegen.ModelInfo
	err      error
}

func (s *stubImageClient) Provider() string { return s.provider }

func (s *stubImageClient) Generate(ctx context.Context, req imagegen.GenerateRequest) (*imagegen.GenerateResponse, error) {
	if s.err != nil {
		return nil, s.err
	}
	return &imagegen.GenerateResponse{
		PNG:        []byte{0x89, 'P', 'N', 'G'},
		Provider:   s.provider,
		Model:      "z-image-turbo",
		Seed:       99,
		Width:      req.Width,
		Height:     req.Height,
		DurationMS: 1234,
	}, nil
}

func (s *stubImageClient) ListModels(ctx context.Context) ([]imagegen.ModelInfo, error) {
	return s.models, s.err
}

// withOrg attaches the auth context the handler reads.
func withOrg(r *http.Request, orgID string) *http.Request {
	ctx := context.WithValue(r.Context(), middleware.ContextKeyOrgID, orgID)
	return r.WithContext(ctx)
}

func newTestImageHandler(clients map[string]imagegen.Client, defaultProvider string) *ImageHandler {
	router := imagegen.NewTestRouter(defaultProvider, clients)
	// No store and no repository: this is the developer-laptop configuration,
	// and it is the one where the "generated but not stored" path has to work.
	return NewImageHandler(service.NewImageService(router, nil, nil, zap.NewNop()))
}

// A server with no provider must render as a disabled control, not an error.
func TestListModels_ReportsDisabledRatherThanFailing(t *testing.T) {
	h := newTestImageHandler(map[string]imagegen.Client{}, "")

	rec := httptest.NewRecorder()
	h.ListModels(rec, httptest.NewRequest(http.MethodGet, "/images/models", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var body imageModelsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Enabled {
		t.Error("enabled should be false with no providers configured")
	}
	if body.Models == nil {
		t.Error("models must be an empty array, not null — the UI maps over it")
	}
}

// The picker's availability flag is the whole point of the payload, so it must
// survive to the wire.
func TestListModels_CarriesAvailability(t *testing.T) {
	h := newTestImageHandler(map[string]imagegen.Client{
		imagegen.ProviderMFlux: &stubImageClient{
			provider: imagegen.ProviderMFlux,
			models: []imagegen.ModelInfo{
				{Name: "z-image-turbo", Provider: imagegen.ProviderMFlux, Available: true, Fast: true},
				{Name: "schnell", Provider: imagegen.ProviderMFlux, Available: false, Fast: true},
			},
		},
	}, imagegen.ProviderMFlux)

	rec := httptest.NewRecorder()
	h.ListModels(rec, httptest.NewRequest(http.MethodGet, "/images/models", nil))

	var body imageModelsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !body.Enabled || body.DefaultProvider != imagegen.ProviderMFlux {
		t.Fatalf("payload = %+v", body)
	}
	if len(body.Models) != 2 {
		t.Fatalf("got %d models, want 2", len(body.Models))
	}
	if !body.Models[0].Available || body.Models[1].Available {
		t.Errorf("availability did not survive: %+v", body.Models)
	}
}

// Without object storage the bytes must come back inline, so the caller can
// still display what it asked for.
func TestGenerate_ReturnsInlineImageWhenNothingStoresIt(t *testing.T) {
	h := newTestImageHandler(map[string]imagegen.Client{
		imagegen.ProviderMFlux: &stubImageClient{provider: imagegen.ProviderMFlux},
	}, imagegen.ProviderMFlux)

	body, _ := json.Marshal(generateImageRequest{Prompt: "a lighthouse"})
	req := withOrg(httptest.NewRequest(http.MethodPost, "/images/generate", bytes.NewReader(body)), uuid.New().String())

	rec := httptest.NewRecorder()
	h.Generate(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	var resp generateImageResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.URL != "" {
		t.Errorf("url = %q, want empty with no store configured", resp.URL)
	}
	if resp.ImageBase64 == "" {
		t.Error("image_base64 must be populated when there is no URL")
	}
	if resp.Seed != 99 {
		t.Errorf("seed = %d, want the provider's chosen seed", resp.Seed)
	}
}

// An empty prompt is the caller's mistake, not the server's.
func TestGenerate_RejectsEmptyPrompt(t *testing.T) {
	h := newTestImageHandler(map[string]imagegen.Client{
		imagegen.ProviderMFlux: &stubImageClient{provider: imagegen.ProviderMFlux},
	}, imagegen.ProviderMFlux)

	body, _ := json.Marshal(generateImageRequest{Prompt: "   "})
	req := withOrg(httptest.NewRequest(http.MethodPost, "/images/generate", bytes.NewReader(body)), uuid.New().String())

	rec := httptest.NewRecorder()
	h.Generate(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

// A request with no organization must not reach the generator at all.
func TestGenerate_RequiresOrgContext(t *testing.T) {
	h := newTestImageHandler(map[string]imagegen.Client{
		imagegen.ProviderMFlux: &stubImageClient{provider: imagegen.ProviderMFlux},
	}, imagegen.ProviderMFlux)

	body, _ := json.Marshal(generateImageRequest{Prompt: "a lighthouse"})
	rec := httptest.NewRecorder()
	h.Generate(rec, httptest.NewRequest(http.MethodPost, "/images/generate", bytes.NewReader(body)))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

// A server that cannot draw must say so with 503, not 500: nothing is broken,
// the capability is simply absent.
func TestGenerate_UnconfiguredIsServiceUnavailable(t *testing.T) {
	h := newTestImageHandler(map[string]imagegen.Client{}, "")

	body, _ := json.Marshal(generateImageRequest{Prompt: "a lighthouse"})
	req := withOrg(httptest.NewRequest(http.MethodPost, "/images/generate", bytes.NewReader(body)), uuid.New().String())

	rec := httptest.NewRecorder()
	h.Generate(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503: %s", rec.Code, rec.Body.String())
	}
}
