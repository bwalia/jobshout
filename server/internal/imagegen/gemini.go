package imagegen

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	_ "image/jpeg"
	"image/png"
	"io"
	"net/http"
	"strings"
	"time"
)

// geminiDefaultModel is used when nothing else is configured.
const geminiDefaultModel = "gemini-3.1-flash-lite-image"

// geminiDefaultBaseURL is Google's public Gemini API root.
const geminiDefaultBaseURL = "https://generativelanguage.googleapis.com"

// geminiTimeout bounds a hosted generation. Shorter than the mflux timeout
// because there is no cold model load and no queue on this side of the call.
const geminiTimeout = 3 * time.Minute

// GeminiClient calls Gemini's generateContent API for image output.
//
// It sits in front of the workstation image service: hosted, billed per image,
// and the prompt leaves the network. The Router falls back to mflux when this
// path fails, so a quota error or an outage does not take drawing away.
type GeminiClient struct {
	baseURL      string
	apiKey       string
	defaultModel string
	httpClient   *http.Client
}

// NewGeminiClient builds a client. baseURL is the API root, e.g.
// "https://generativelanguage.googleapis.com".
func NewGeminiClient(baseURL, apiKey, defaultModel string) *GeminiClient {
	if baseURL == "" {
		baseURL = geminiDefaultBaseURL
	}
	if defaultModel == "" {
		defaultModel = geminiDefaultModel
	}
	return &GeminiClient{
		baseURL:      strings.TrimRight(baseURL, "/"),
		apiKey:       strings.TrimSpace(apiKey),
		defaultModel: strings.TrimSpace(defaultModel),
		httpClient:   &http.Client{Timeout: geminiTimeout},
	}
}

// Provider implements Client.
func (c *GeminiClient) Provider() string { return ProviderGemini }

type geminiGenerateRequest struct {
	Contents         []geminiContent        `json:"contents"`
	GenerationConfig geminiGenerationConfig `json:"generationConfig"`
}

type geminiContent struct {
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text       string      `json:"text,omitempty"`
	InlineData *geminiBlob `json:"inlineData,omitempty"`
	InlineAlt  *geminiBlob `json:"inline_data,omitempty"`
}

type geminiBlob struct {
	MimeType string `json:"mimeType,omitempty"`
	MimeAlt  string `json:"mime_type,omitempty"`
	Data     string `json:"data"`
}

type geminiGenerationConfig struct {
	// TEXT is required alongside IMAGE on several Gemini image models: IMAGE
	// alone can come back as HTTP 200 with no parts.
	ResponseModalities []string          `json:"responseModalities"`
	ImageConfig        geminiImageConfig `json:"imageConfig"`
}

type geminiImageConfig struct {
	AspectRatio string `json:"aspectRatio"`
	ImageSize   string `json:"imageSize"`
}

type geminiGenerateResponse struct {
	Candidates []struct {
		Content struct {
			Parts []geminiPart `json:"parts"`
		} `json:"content"`
		FinishReason string `json:"finishReason"`
	} `json:"candidates"`
	Error *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Status  string `json:"status"`
	} `json:"error"`
}

// geminiAspects are the ratios the image models accept. Arbitrary sizes are
// mapped onto the closest of these rather than rejected.
var geminiAspects = []struct {
	name          string
	width, height int
}{
	{"1:1", 1, 1},
	{"2:3", 2, 3},
	{"3:2", 3, 2},
	{"3:4", 3, 4},
	{"4:3", 4, 3},
	{"4:5", 4, 5},
	{"5:4", 5, 4},
	{"9:16", 9, 16},
	{"16:9", 16, 9},
	{"21:9", 21, 9},
}

// nearestGeminiAspect picks the supported ratio closest to the request.
func nearestGeminiAspect(width, height int) string {
	if height <= 0 {
		return "1:1"
	}
	want := float64(width) / float64(height)
	best := geminiAspects[0]
	bestDelta := -1.0
	for _, s := range geminiAspects {
		delta := float64(s.width)/float64(s.height) - want
		if delta < 0 {
			delta = -delta
		}
		if bestDelta < 0 || delta < bestDelta {
			best, bestDelta = s, delta
		}
	}
	return best.name
}

// geminiImageSize is the resolution bucket the lite model accepts. It tops
// out at 1K; asking for 2K is rejected rather than quietly downsampled.
func geminiImageSize(width, height int) string {
	if width <= 512 && height <= 512 {
		return "512"
	}
	return "1K"
}

// isGeminiImageModel reports whether name is a Gemini image model, so a
// request that named z-image-turbo is not sent here.
func isGeminiImageModel(name string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(name)), "gemini-")
}

func (p geminiPart) blob() *geminiBlob {
	if p.InlineData != nil && p.InlineData.Data != "" {
		return p.InlineData
	}
	if p.InlineAlt != nil && p.InlineAlt.Data != "" {
		return p.InlineAlt
	}
	return nil
}

func (b *geminiBlob) mime() string {
	if b == nil {
		return ""
	}
	if b.MimeType != "" {
		return b.MimeType
	}
	return b.MimeAlt
}

// Generate implements Client.
func (c *GeminiClient) Generate(ctx context.Context, req GenerateRequest) (*GenerateResponse, error) {
	if err := req.Normalize(); err != nil {
		return nil, err
	}
	if c.apiKey == "" {
		return nil, fmt.Errorf("imagegen: Gemini image generation needs GEMINI_API_KEY")
	}

	model := req.Model
	if model == "" {
		model = c.defaultModel
	}

	body, err := json.Marshal(geminiGenerateRequest{
		Contents: []geminiContent{{
			Parts: []geminiPart{{Text: req.Prompt}},
		}},
		GenerationConfig: geminiGenerationConfig{
			ResponseModalities: []string{"TEXT", "IMAGE"},
			ImageConfig: geminiImageConfig{
				AspectRatio: nearestGeminiAspect(req.Width, req.Height),
				ImageSize:   geminiImageSize(req.Width, req.Height),
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("imagegen: encode request: %w", err)
	}

	url := fmt.Sprintf("%s/v1beta/models/%s:generateContent", c.baseURL, model)
	started := time.Now()
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("imagegen: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-goog-api-key", c.apiKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("imagegen: call Gemini images: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("imagegen: read response: %w", err)
	}

	var parsed geminiGenerateResponse
	_ = json.Unmarshal(data, &parsed)
	if parsed.Error != nil && parsed.Error.Message != "" {
		return nil, fmt.Errorf("imagegen: Gemini rejected the request: %s", parsed.Error.Message)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("imagegen: Gemini returned %d: %s", resp.StatusCode, snippet(data))
	}

	raw, mime, err := firstGeminiImage(parsed)
	if err != nil {
		return nil, err
	}
	pngBytes, width, height, err := encodeAsPNG(raw, mime)
	if err != nil {
		return nil, err
	}
	if width == 0 {
		width = req.Width
	}
	if height == 0 {
		height = req.Height
	}

	return &GenerateResponse{
		PNG:        pngBytes,
		Provider:   ProviderGemini,
		Model:      model,
		Seed:       0,
		Width:      width,
		Height:     height,
		Steps:      0,
		DurationMS: int(time.Since(started).Milliseconds()),
	}, nil
}

func firstGeminiImage(parsed geminiGenerateResponse) ([]byte, string, error) {
	for _, cand := range parsed.Candidates {
		for _, part := range cand.Content.Parts {
			blob := part.blob()
			if blob == nil {
				continue
			}
			raw, err := decodeGeminiBase64(blob.Data)
			if err != nil {
				return nil, "", fmt.Errorf("imagegen: decode Gemini image data: %w", err)
			}
			if len(raw) == 0 {
				continue
			}
			return raw, blob.mime(), nil
		}
	}
	return nil, "", fmt.Errorf("imagegen: Gemini returned no image")
}

// pngMagic is the eight-byte PNG signature. Used to skip a re-encode when
// Gemini already returned PNG, which is what the rest of the platform stores.
var pngMagic = []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}

func decodeGeminiBase64(s string) ([]byte, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, fmt.Errorf("empty image data")
	}
	if raw, err := base64.StdEncoding.DecodeString(s); err == nil {
		return raw, nil
	}
	if raw, err := base64.RawStdEncoding.DecodeString(s); err == nil {
		return raw, nil
	}
	if raw, err := base64.URLEncoding.DecodeString(s); err == nil {
		return raw, nil
	}
	return base64.RawURLEncoding.DecodeString(s)
}

func encodeAsPNG(raw []byte, mime string) ([]byte, int, int, error) {
	if bytes.HasPrefix(raw, pngMagic) {
		cfg, err := png.DecodeConfig(bytes.NewReader(raw))
		if err != nil {
			return nil, 0, 0, fmt.Errorf("imagegen: decode Gemini PNG: %w", err)
		}
		return raw, cfg.Width, cfg.Height, nil
	}
	img, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, 0, 0, fmt.Errorf("imagegen: decode Gemini image (%s): %w", mime, err)
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, 0, 0, fmt.Errorf("imagegen: encode PNG: %w", err)
	}
	b := img.Bounds()
	return buf.Bytes(), b.Dx(), b.Dy(), nil
}

// ListModels implements Client.
func (c *GeminiClient) ListModels(ctx context.Context) ([]ModelInfo, error) {
	available := c.apiKey != ""
	return []ModelInfo{
		{
			Name:      geminiDefaultModel,
			Provider:  ProviderGemini,
			Available: available,
			Fast:      true,
		},
	}, nil
}
