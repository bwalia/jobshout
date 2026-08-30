package imagegen

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/jobshout/server/internal/config"
)

const testSecret = "test-image-secret"

// onePixelPNG is a real, decodable PNG. Tests use it rather than arbitrary bytes
// so that "the client returned image data" means the same thing here as in
// production.
func onePixelPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode test png: %v", err)
	}
	return buf.Bytes()
}

func TestNormalize_DefaultsAndRounding(t *testing.T) {
	cases := []struct {
		name         string
		in           GenerateRequest
		wantW, wantH int
		wantErr      bool
	}{
		{
			name:  "zero dimensions become the 16:9 default",
			in:    GenerateRequest{Prompt: "a hill"},
			wantW: DefaultWidth, wantH: DefaultHeight,
		},
		{
			name:  "dimensions round up to a multiple of 16",
			in:    GenerateRequest{Prompt: "a hill", Width: 1000, Height: 500},
			wantW: 1008, wantH: 512,
		},
		{
			name:  "already-aligned dimensions are left alone",
			in:    GenerateRequest{Prompt: "a hill", Width: 1024, Height: 576},
			wantW: 1024, wantH: 576,
		},
		{
			name:    "an empty prompt is refused",
			in:      GenerateRequest{Prompt: "   "},
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := tc.in
			err := req.Normalize()
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected an error")
				}
				return
			}
			if err != nil {
				t.Fatalf("Normalize: %v", err)
			}
			if req.Width != tc.wantW || req.Height != tc.wantH {
				t.Errorf("size = %dx%d, want %dx%d", req.Width, req.Height, tc.wantW, tc.wantH)
			}
		})
	}
}

// The service is reached over the public internet, so every request must carry
// a token the gateway can verify.
func TestMFluxClient_SignsRequests(t *testing.T) {
	var seen string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.Header.Get("x-api-key")
		_ = json.NewEncoder(w).Encode(mfluxGenerateResponse{
			ImageBase64: base64.StdEncoding.EncodeToString(onePixelPNG(t)),
			Model:       "z-image-turbo",
			Seed:        42,
			Width:       1024,
			Height:      576,
			Steps:       8,
			DurationMS:  1234,
		})
	}))
	defer srv.Close()

	c := NewMFluxClient(srv.URL, "z-image-turbo", testSecret, 0)
	if !c.UsesGateway() {
		t.Error("client should report gateway mode when a secret is set")
	}

	resp, err := c.Generate(context.Background(), GenerateRequest{Prompt: "a lighthouse"})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if len(resp.PNG) == 0 {
		t.Fatal("no image data returned")
	}
	if _, err := png.Decode(bytes.NewReader(resp.PNG)); err != nil {
		t.Errorf("returned data is not a decodable PNG: %v", err)
	}
	if resp.Seed != 42 || resp.Model != "z-image-turbo" {
		t.Errorf("metadata = seed %d model %q", resp.Seed, resp.Model)
	}
	if seen == "" {
		t.Fatal("the service received no x-api-key header")
	}

	claims := jwt.MapClaims{}
	if _, err := jwt.ParseWithClaims(seen, claims, func(*jwt.Token) (any, error) {
		return []byte(testSecret), nil
	}); err != nil {
		t.Fatalf("token did not verify: %v", err)
	}
	if claims["app"] != "jobshout" {
		t.Errorf("app claim = %v", claims["app"])
	}
}

// Without a secret the client must talk to the service unsigned — that is what
// makes a local run work.
func TestMFluxClient_NoSecretSendsNoHeader(t *testing.T) {
	var seen string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.Header.Get("x-api-key")
		_ = json.NewEncoder(w).Encode(mfluxGenerateResponse{
			ImageBase64: base64.StdEncoding.EncodeToString(onePixelPNG(t)),
			Model:       "z-image-turbo",
		})
	}))
	defer srv.Close()

	c := NewMFluxClient(srv.URL, "z-image-turbo", "", 0)
	if c.UsesGateway() {
		t.Error("client should not report gateway mode without a secret")
	}
	if _, err := c.Generate(context.Background(), GenerateRequest{Prompt: "a hill"}); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if seen != "" {
		t.Errorf("unsigned request carried x-api-key = %q", seen)
	}
}

// A rejected signature must say what to fix, and must not be confused with the
// service being busy.
func TestMFluxClient_DistinguishesAuthFromBusy(t *testing.T) {
	cases := []struct {
		status    int
		wantWords []string
	}{
		{http.StatusUnauthorized, []string{"signature", "IMAGE_JWT_SECRET"}},
		{http.StatusForbidden, []string{"signature", "IMAGE_JWT_SECRET"}},
		{http.StatusServiceUnavailable, []string{"busy"}},
	}

	for _, tc := range cases {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(tc.status)
			_, _ = w.Write([]byte(`{"detail":"nope"}`))
		}))

		c := NewMFluxClient(srv.URL, "z-image-turbo", testSecret, 0)
		_, err := c.Generate(context.Background(), GenerateRequest{Prompt: "a hill"})
		srv.Close()

		if err == nil {
			t.Fatalf("status %d: expected an error", tc.status)
		}
		for _, want := range tc.wantWords {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("status %d: error %q should mention %q", tc.status, err, want)
			}
		}
	}
}

// An HTML error page from a gateway must not be pasted whole into an error a
// human reads on a card.
func TestSnippet_StripsMarkupAndTruncates(t *testing.T) {
	body := []byte(`<!DOCTYPE html><html><head><title>Access denied</title></head>` +
		`<body><h1>Access denied</h1><p>Your token was not accepted.</p></body></html>`)

	got := snippet(body)
	if strings.Contains(got, "<") || strings.Contains(got, ">") {
		t.Errorf("snippet still contains markup: %q", got)
	}
	if !strings.Contains(got, "Access denied") {
		t.Errorf("snippet lost the useful text: %q", got)
	}

	long := snippet([]byte(strings.Repeat("verbose ", 200)))
	if len(long) > snippetLimit+len("…") {
		t.Errorf("snippet not truncated: %d chars", len(long))
	}
}

// The picker's whole purpose is telling a downloaded model apart from one that
// would trigger a 31 GB download, so that flag must survive the round trip.
func TestMFluxClient_ListModelsCarriesAvailability(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"models":[
			{"name":"z-image-turbo","repo":"Tongyi-MAI/Z-Image-Turbo","downloaded":true,"turbo":true},
			{"name":"schnell","repo":"black-forest-labs/FLUX.1-schnell","downloaded":false,"turbo":true}
		],"default":"z-image-turbo"}`))
	}))
	defer srv.Close()

	models, err := NewMFluxClient(srv.URL, "z-image-turbo", "", 0).ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if len(models) != 2 {
		t.Fatalf("got %d models, want 2", len(models))
	}
	if !models[0].Available || models[0].Provider != ProviderMFlux {
		t.Errorf("first model = %+v", models[0])
	}
	if models[1].Available {
		t.Error("a model whose weights are absent must not be reported as available")
	}
}

// A 16:9 request must come back landscape. Matching on pixel count instead of
// aspect ratio would happily return a square.
func TestNearestSize_MatchesAspectRatio(t *testing.T) {
	cases := []struct {
		w, h int
		want string
	}{
		{1024, 576, "1536x1024"},  // 16:9 → the landscape option
		{1024, 1024, "1024x1024"}, // square → square
		{600, 900, "1024x1536"},   // portrait → portrait
	}
	for _, tc := range cases {
		if got, _, _ := nearestSize(tc.w, tc.h); got != tc.want {
			t.Errorf("nearestSize(%d,%d) = %s, want %s", tc.w, tc.h, got, tc.want)
		}
	}
}

// A stub client for Router tests.
type stubClient struct {
	provider      string
	models        []ModelInfo
	calls         int
	generateCalls int
	lastModel     string
	err           error
}

func (s *stubClient) Provider() string { return s.provider }
func (s *stubClient) Generate(ctx context.Context, req GenerateRequest) (*GenerateResponse, error) {
	s.generateCalls++
	s.lastModel = req.Model
	if s.err != nil {
		return nil, s.err
	}
	return &GenerateResponse{PNG: []byte{1}, Provider: s.provider, Model: req.Model}, nil
}
func (s *stubClient) ListModels(ctx context.Context) ([]ModelInfo, error) {
	s.calls++
	return s.models, s.err
}

// An agent may hold a provider that has since been removed. That must not
// permanently break the agent.
func TestRouter_UnknownProviderFallsBackToDefault(t *testing.T) {
	r := NewTestRouter(ProviderMFlux, map[string]Client{
		ProviderMFlux: &stubClient{provider: ProviderMFlux},
	})

	c, err := r.For("a-provider-that-was-deleted")
	if err != nil {
		t.Fatalf("For: %v", err)
	}
	if c.Provider() != ProviderMFlux {
		t.Errorf("fell back to %q, want %q", c.Provider(), ProviderMFlux)
	}
}

func TestRouter_NoProvidersIsAClearError(t *testing.T) {
	r := NewTestRouter("", map[string]Client{})
	if r.Enabled() {
		t.Error("a router with no clients must not report itself enabled")
	}
	_, err := r.For(ProviderMFlux)
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "IMAGE_BASE_URL") {
		t.Errorf("error should name the setting to fix: %q", err)
	}
}

// With the workstation asleep, the picker should still show the hosted models
// rather than an error.
func TestRouter_ListModelsSurvivesOneProviderFailing(t *testing.T) {
	working := &stubClient{provider: ProviderOpenAI, models: []ModelInfo{{Name: "gpt-image-1"}}}
	broken := &stubClient{provider: ProviderMFlux, err: context.DeadlineExceeded}

	r := NewTestRouter(ProviderMFlux, map[string]Client{
		ProviderMFlux:  broken,
		ProviderOpenAI: working,
	})

	models := r.ListModels(context.Background())
	if len(models) != 1 || models[0].Name != "gpt-image-1" {
		t.Fatalf("models = %+v", models)
	}
}

// Discovery is cached so opening a picker is instant; a failure is not cached,
// so a workstation that wakes up is noticed on the next open.
func TestRouter_CachesSuccessButNotFailure(t *testing.T) {
	ok := &stubClient{provider: ProviderOpenAI, models: []ModelInfo{{Name: "gpt-image-1"}}}
	bad := &stubClient{provider: ProviderMFlux, err: context.DeadlineExceeded}
	r := NewTestRouter(ProviderOpenAI, map[string]Client{ProviderOpenAI: ok, ProviderMFlux: bad})

	r.ListModels(context.Background())
	r.ListModels(context.Background())

	if ok.calls != 1 {
		t.Errorf("successful provider queried %d times, want 1 (result should be cached)", ok.calls)
	}
	if bad.calls != 2 {
		t.Errorf("failing provider queried %d times, want 2 (failure must not be cached)", bad.calls)
	}
}

// A generation that outlives its context must be abandoned, not left holding a
// blog run open.
func TestGeminiAspect_MatchesRequestedShape(t *testing.T) {
	cases := []struct {
		w, h int
		want string
	}{
		{1024, 576, "16:9"},
		{1024, 1024, "1:1"},
		{1024, 688, "3:2"},
		{600, 900, "2:3"},
	}
	for _, tc := range cases {
		if got := nearestGeminiAspect(tc.w, tc.h); got != tc.want {
			t.Errorf("nearestGeminiAspect(%d,%d) = %s, want %s", tc.w, tc.h, got, tc.want)
		}
	}
}

func TestGeminiClient_GenerateDecodesInlineImage(t *testing.T) {
	pngBytes := onePixelPNG(t)
	var seenKey, seenPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenKey = r.Header.Get("x-goog-api-key")
		seenPath = r.URL.Path
		_ = json.NewEncoder(w).Encode(geminiGenerateResponse{
			Candidates: []struct {
				Content struct {
					Parts []geminiPart `json:"parts"`
				} `json:"content"`
				FinishReason string `json:"finishReason"`
			}{{
				Content: struct {
					Parts []geminiPart `json:"parts"`
				}{Parts: []geminiPart{{
					InlineData: &geminiBlob{
						MimeType: "image/png",
						Data:     base64.StdEncoding.EncodeToString(pngBytes),
					},
				}}},
			}},
		})
	}))
	defer srv.Close()

	c := NewGeminiClient(srv.URL, "test-gemini-key", geminiDefaultModel)
	resp, err := c.Generate(context.Background(), GenerateRequest{Prompt: "a lighthouse", Width: 1024, Height: 576})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if resp.Provider != ProviderGemini || resp.Model != geminiDefaultModel {
		t.Errorf("metadata = %+v", resp)
	}
	if !bytes.Equal(resp.PNG, pngBytes) {
		t.Error("returned PNG does not match the fixture")
	}
	if seenKey != "test-gemini-key" {
		t.Errorf("api key header = %q", seenKey)
	}
	if !strings.Contains(seenPath, geminiDefaultModel) {
		t.Errorf("path %q should name the model", seenPath)
	}
}

func TestEncodeAsPNG_ConvertsJPEG(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	var jpegBuf bytes.Buffer
	if err := jpeg.Encode(&jpegBuf, img, nil); err != nil {
		t.Fatalf("encode jpeg: %v", err)
	}

	pngBytes, w, h, err := encodeAsPNG(jpegBuf.Bytes(), "image/jpeg")
	if err != nil {
		t.Fatalf("encodeAsPNG: %v", err)
	}
	if w != 2 || h != 2 {
		t.Errorf("size = %dx%d, want 2x2", w, h)
	}
	if !bytes.HasPrefix(pngBytes, pngMagic) {
		t.Error("JPEG was not converted to PNG")
	}
}

func TestDecodeGeminiBase64_AcceptsURLEncoding(t *testing.T) {
	raw := []byte{0xff, 0xd8, 0xff, 0x00}
	url := base64.URLEncoding.EncodeToString(raw)
	got, err := decodeGeminiBase64(url)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !bytes.Equal(got, raw) {
		t.Errorf("got %x, want %x", got, raw)
	}
}

func TestRouter_NilIsAClearError(t *testing.T) {
	var r *Router
	_, err := r.Generate(context.Background(), "", GenerateRequest{Prompt: "a hill", Model: "z-image-turbo"})
	if err == nil {
		t.Fatal("expected an error")
	}
}

func TestGeminiClient_APIErrorIsSurface(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{"message": "quota exceeded"},
		})
	}))
	defer srv.Close()

	_, err := NewGeminiClient(srv.URL, "key", "").Generate(context.Background(), GenerateRequest{Prompt: "a hill"})
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "quota exceeded") {
		t.Errorf("error should carry the API message: %q", err)
	}
}

func TestRouter_GeminiFallsBackToMFlux(t *testing.T) {
	gemini := &stubClient{provider: ProviderGemini, err: fmt.Errorf("quota")}
	mflux := &stubClient{provider: ProviderMFlux}
	r := NewTestRouter(ProviderGemini, map[string]Client{
		ProviderGemini: gemini,
		ProviderMFlux:  mflux,
	})

	resp, err := r.Generate(context.Background(), "", GenerateRequest{Prompt: "a hill"})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if resp.Provider != ProviderMFlux {
		t.Errorf("fell back to %q, want mflux", resp.Provider)
	}
	if gemini.generateCalls != 1 || mflux.generateCalls != 1 {
		t.Errorf("gemini calls=%d mflux calls=%d", gemini.generateCalls, mflux.generateCalls)
	}
	if mflux.lastModel != "" {
		t.Errorf("fallback should clear a Gemini model name, got %q", mflux.lastModel)
	}
}

func TestRouter_GeminiSuccessSkipsMFlux(t *testing.T) {
	gemini := &stubClient{provider: ProviderGemini}
	mflux := &stubClient{provider: ProviderMFlux}
	r := NewTestRouter(ProviderGemini, map[string]Client{
		ProviderGemini: gemini,
		ProviderMFlux:  mflux,
	})

	resp, err := r.Generate(context.Background(), "", GenerateRequest{Prompt: "a hill"})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if resp.Provider != ProviderGemini {
		t.Errorf("provider = %q, want gemini", resp.Provider)
	}
	if mflux.generateCalls != 0 {
		t.Error("mflux should not run when Gemini succeeds")
	}
}

func TestRouter_ExplicitMFluxSkipsGemini(t *testing.T) {
	gemini := &stubClient{provider: ProviderGemini}
	mflux := &stubClient{provider: ProviderMFlux}
	r := NewTestRouter(ProviderGemini, map[string]Client{
		ProviderGemini: gemini,
		ProviderMFlux:  mflux,
	})

	resp, err := r.Generate(context.Background(), ProviderMFlux, GenerateRequest{Prompt: "a hill"})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if resp.Provider != ProviderMFlux {
		t.Errorf("provider = %q, want mflux", resp.Provider)
	}
	if gemini.generateCalls != 0 {
		t.Error("explicit mflux must not hop through Gemini")
	}
}

func TestRouter_NamedWorkstationModelSkipsGemini(t *testing.T) {
	gemini := &stubClient{provider: ProviderGemini}
	mflux := &stubClient{provider: ProviderMFlux}
	r := NewTestRouter(ProviderGemini, map[string]Client{
		ProviderGemini: gemini,
		ProviderMFlux:  mflux,
	})

	resp, err := r.Generate(context.Background(), "", GenerateRequest{Prompt: "a hill", Model: "z-image-turbo"})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if resp.Provider != ProviderMFlux {
		t.Errorf("provider = %q, want mflux", resp.Provider)
	}
	if gemini.generateCalls != 0 {
		t.Error("a named workstation model must not hop through Gemini")
	}
	if mflux.lastModel != "z-image-turbo" {
		t.Errorf("mflux model = %q", mflux.lastModel)
	}
}

func TestRouter_CancelledContextDoesNotFallBack(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	gemini := &stubClient{provider: ProviderGemini, err: context.Canceled}
	mflux := &stubClient{provider: ProviderMFlux}
	r := NewTestRouter(ProviderGemini, map[string]Client{
		ProviderGemini: gemini,
		ProviderMFlux:  mflux,
	})

	_, err := r.Generate(ctx, ProviderGemini, GenerateRequest{Prompt: "a hill"})
	if err == nil {
		t.Fatal("expected the cancelled request to fail")
	}
	if mflux.generateCalls != 0 {
		t.Error("a cancelled context must not fall back to the workstation")
	}
}

func TestRouter_NoFallbackSkipsWorkstation(t *testing.T) {
	gemini := &stubClient{provider: ProviderGemini, err: fmt.Errorf("down")}
	mflux := &stubClient{provider: ProviderMFlux}
	r := NewTestRouter(ProviderGemini, map[string]Client{
		ProviderGemini: gemini,
		ProviderMFlux:  mflux,
	})

	_, err := r.Generate(context.Background(), ProviderGemini, GenerateRequest{
		Prompt:     "a labeled comparison table",
		NoFallback: true,
	})
	if err == nil {
		t.Fatal("expected Gemini's error when fallback is forbidden")
	}
	if mflux.generateCalls != 0 {
		t.Error("NoFallback must not draw a labeled figure on the workstation")
	}
}

func TestRouter_GeminiClearsModelOnFallback(t *testing.T) {
	gemini := &stubClient{provider: ProviderGemini, err: fmt.Errorf("down")}
	mflux := &stubClient{provider: ProviderMFlux}
	r := NewTestRouter(ProviderGemini, map[string]Client{
		ProviderGemini: gemini,
		ProviderMFlux:  mflux,
	})

	_, err := r.Generate(context.Background(), ProviderGemini, GenerateRequest{
		Prompt: "a hill",
		Model:  geminiDefaultModel,
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if mflux.lastModel != "" {
		t.Errorf("fallback model = %q, want empty so mflux uses its default", mflux.lastModel)
	}
}

func TestNewRouter_GeminiBecomesDefaultWhenKeySet(t *testing.T) {
	r := NewRouter(&config.Config{
		ImageProvider:     ProviderMFlux,
		ImageBaseURL:      "http://example.invalid",
		ImageDefaultModel: "z-image-turbo",
		GeminiAPIKey:      "test-key",
		ImageGeminiModel:  geminiDefaultModel,
	})
	if r.DefaultProvider() != ProviderGemini {
		t.Errorf("default = %q, want gemini", r.DefaultProvider())
	}
	if _, err := r.For(ProviderMFlux); err != nil {
		t.Errorf("mflux should still be registered: %v", err)
	}
}

func TestMFluxClient_HonoursContextCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Drain the body before blocking: the server only watches for the
		// client going away once the request has been consumed, so a handler
		// that blocks on an unread body never sees the cancellation.
		_, _ = io.Copy(io.Discard, r.Body)
		// Bounded rather than an open-ended wait on Done(), because
		// srv.Close() blocks on outstanding handlers — a handler that never
		// returns hangs the test suite instead of failing it.
		select {
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
		}
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	c := NewMFluxClient(srv.URL, "z-image-turbo", "", 0)
	if _, err := c.Generate(ctx, GenerateRequest{Prompt: "a hill"}); err == nil {
		t.Fatal("expected the cancelled request to fail")
	}
}
