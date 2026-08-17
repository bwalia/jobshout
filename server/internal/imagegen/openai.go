package imagegen

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// openAIDefaultModel is used when nothing else is configured.
const openAIDefaultModel = "gpt-image-1"

// openAITimeout bounds a hosted generation. Shorter than the mflux timeout
// because there is no cold model load and no queue on this side of the call —
// if OpenAI has not answered in three minutes, something is wrong rather than
// slow.
const openAITimeout = 3 * time.Minute

// OpenAIClient calls OpenAI's image generation API.
//
// It exists so that a ring can generate images when the workstation is not
// reachable — a Mac that is asleep, rebooting, or off the network takes the
// local path down with it, and an article pipeline that fails for want of a
// picture is worse than one that pays a few cents for it. Prompts do leave the
// network on this path, which is why it is opt-in per environment rather than
// a silent fallback.
type OpenAIClient struct {
	baseURL      string
	apiKey       string
	defaultModel string
	httpClient   *http.Client
}

// NewOpenAIClient builds a client. baseURL is the root, e.g.
// "https://api.openai.com".
func NewOpenAIClient(baseURL, apiKey, defaultModel string) *OpenAIClient {
	if baseURL == "" {
		baseURL = "https://api.openai.com"
	}
	if defaultModel == "" {
		defaultModel = openAIDefaultModel
	}
	return &OpenAIClient{
		baseURL:      strings.TrimRight(baseURL, "/"),
		apiKey:       apiKey,
		defaultModel: defaultModel,
		httpClient:   &http.Client{Timeout: openAITimeout},
	}
}

// Provider implements Client.
func (c *OpenAIClient) Provider() string { return ProviderOpenAI }

type openAIImageRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Size   string `json:"size"`
	N      int    `json:"n"`
}

type openAIImageResponse struct {
	Data []struct {
		B64JSON string `json:"b64_json"`
	} `json:"data"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error"`
}

// openAISizes are the only dimensions the API accepts. Arbitrary sizes are not
// supported, so a request for one is served by the closest supported shape
// rather than rejected — the caller wants a picture of roughly that proportion,
// and refusing over 24 pixels would help nobody.
var openAISizes = []struct {
	name          string
	width, height int
}{
	{"1024x1024", 1024, 1024},
	{"1536x1024", 1536, 1024},
	{"1024x1536", 1024, 1536},
}

// nearestSize picks the supported size whose aspect ratio is closest to the
// requested one. Ratio rather than area: a cover asked for as 16:9 should come
// back landscape, and matching on total pixels would happily return a square.
func nearestSize(width, height int) (string, int, int) {
	want := float64(width) / float64(height)
	best := openAISizes[0]
	bestDelta := -1.0
	for _, s := range openAISizes {
		delta := float64(s.width)/float64(s.height) - want
		if delta < 0 {
			delta = -delta
		}
		if bestDelta < 0 || delta < bestDelta {
			best, bestDelta = s, delta
		}
	}
	return best.name, best.width, best.height
}

// Generate implements Client.
func (c *OpenAIClient) Generate(ctx context.Context, req GenerateRequest) (*GenerateResponse, error) {
	if err := req.Normalize(); err != nil {
		return nil, err
	}
	if c.apiKey == "" {
		return nil, fmt.Errorf("imagegen: OpenAI image generation needs OPENAI_API_KEY")
	}

	model := req.Model
	if model == "" {
		model = c.defaultModel
	}
	size, width, height := nearestSize(req.Width, req.Height)

	// The negative prompt has no field in this API. Appending it to the prompt
	// would change what the caller asked for in a way they could not see, so it
	// is dropped — documented on GenerateRequest.NegativePrompt.
	body, err := json.Marshal(openAIImageRequest{
		Model:  model,
		Prompt: req.Prompt,
		Size:   size,
		N:      1,
	})
	if err != nil {
		return nil, fmt.Errorf("imagegen: encode request: %w", err)
	}

	started := time.Now()
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/images/generations", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("imagegen: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("imagegen: call OpenAI images: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("imagegen: read response: %w", err)
	}

	var parsed openAIImageResponse
	// Decoded before the status check so an API error message is used in
	// preference to the bare status code — "content policy" says far more than
	// "400".
	_ = json.Unmarshal(data, &parsed)
	if parsed.Error != nil && parsed.Error.Message != "" {
		return nil, fmt.Errorf("imagegen: OpenAI rejected the request: %s", parsed.Error.Message)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("imagegen: OpenAI returned %d: %s", resp.StatusCode, snippet(data))
	}
	if len(parsed.Data) == 0 || parsed.Data[0].B64JSON == "" {
		return nil, fmt.Errorf("imagegen: OpenAI returned no image")
	}

	png, err := base64.StdEncoding.DecodeString(parsed.Data[0].B64JSON)
	if err != nil {
		return nil, fmt.Errorf("imagegen: decode image data: %w", err)
	}

	return &GenerateResponse{
		PNG:      png,
		Provider: ProviderOpenAI,
		Model:    model,
		// The API neither takes nor reports a seed, so there is none to record.
		// Zero here means "not reproducible", which is the truth.
		Seed:       0,
		Width:      width,
		Height:     height,
		Steps:      0,
		DurationMS: int(time.Since(started).Milliseconds()),
	}, nil
}

// ListModels implements Client.
//
// The API has no endpoint that lists image models, so this is static. It is
// still correct to report: the set changes rarely, and a picker with two real
// options beats one that is empty because discovery was impossible.
func (c *OpenAIClient) ListModels(ctx context.Context) ([]ModelInfo, error) {
	available := c.apiKey != ""
	return []ModelInfo{
		{Name: "gpt-image-1", Provider: ProviderOpenAI, Available: available},
		{Name: "dall-e-3", Provider: ProviderOpenAI, Available: available},
	}, nil
}
