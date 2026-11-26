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

// MidjourneyWarning is shown beside the provider in the UI. Midjourney has no
// official public API; every route to it is a third-party service.
const MidjourneyWarning = "Midjourney has no official public API. This adapter calls a service you configure; " +
	"such services may breach Midjourney's terms of service and can break or disappear without notice."

// MidjourneyClient talks to a bridge service the operator names explicitly
// (MIDJOURNEY_API_URL). It is off unless MIDJOURNEY_ENABLED=true.
//
// There is no vendor contract to follow, so this defines a small one that a
// bridge must satisfy. Request: POST {url} with
//
//	{"prompt","aspect_ratio","width","height","seed"?,"model"?}
//
// and "Authorization: Bearer <key>". Response: either an image body
// (Content-Type image/*), or JSON with one of "image_base64" or "image_url".
// A bridge that needs job polling should do that itself and answer when done.
type MidjourneyClient struct {
	url          string
	apiKey       string
	defaultModel string
	httpClient   *http.Client
}

// NewMidjourneyClient builds a client for the bridge at url.
func NewMidjourneyClient(url, apiKey, defaultModel string, timeout time.Duration) *MidjourneyClient {
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	return &MidjourneyClient{
		url: strings.TrimSpace(url), apiKey: strings.TrimSpace(apiKey),
		defaultModel: strings.TrimSpace(defaultModel),
		httpClient:   &http.Client{Timeout: timeout},
	}
}

// Provider implements Client.
func (c *MidjourneyClient) Provider() string { return ProviderMidjourney }

// DefaultModel is the configured version label, if any.
func (c *MidjourneyClient) DefaultModel() string { return c.defaultModel }

// ListModels implements Client.
func (c *MidjourneyClient) ListModels(ctx context.Context) ([]ModelInfo, error) {
	name := c.defaultModel
	if name == "" {
		name = "midjourney"
	}
	return []ModelInfo{{Name: name, Provider: ProviderMidjourney, Available: c.url != ""}}, nil
}

type mjResponse struct {
	ImageBase64 string `json:"image_base64"`
	ImageURL    string `json:"image_url"`
	Error       string `json:"error"`
}

// Generate implements Client.
func (c *MidjourneyClient) Generate(ctx context.Context, req GenerateRequest) (*GenerateResponse, error) {
	if err := req.Normalize(); err != nil {
		return nil, err
	}
	if c.url == "" {
		return nil, &ProviderError{Provider: ProviderMidjourney, Msg: "no bridge URL configured (set MIDJOURNEY_API_URL)"}
	}
	model := req.Model
	if model == "" {
		model = c.defaultModel
	}
	payload := map[string]any{
		"prompt":       req.Prompt,
		"aspect_ratio": nearestGeminiAspect(req.Width, req.Height),
		"width":        req.Width,
		"height":       req.Height,
	}
	if req.Seed >= 0 {
		payload["seed"] = req.Seed
	}
	if model != "" {
		payload["model"] = model
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("imagegen: encode request: %w", err)
	}
	started := time.Now()
	resp, err := doRetry(ctx, c.httpClient, "midjourney", retryPolicy{attempts: 2, base: 2 * time.Second}, func() (*http.Request, error) {
		r, err := http.NewRequest(http.MethodPost, c.url, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		r.Header.Set("Content-Type", "application/json")
		if c.apiKey != "" {
			r.Header.Set("Authorization", "Bearer "+c.apiKey)
		}
		return r, nil
	})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, MaxImageBytes+1))
	if err != nil {
		return nil, fmt.Errorf("imagegen: read midjourney response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("imagegen: midjourney bridge returned %d: %s", resp.StatusCode, snippet(data))
	}

	var raw []byte
	if strings.HasPrefix(resp.Header.Get("Content-Type"), "image/") {
		raw = data
	} else {
		var mr mjResponse
		if err := json.Unmarshal(data, &mr); err != nil {
			return nil, fmt.Errorf("imagegen: midjourney bridge returned unreadable data: %s", snippet(data))
		}
		switch {
		case mr.Error != "":
			return nil, fmt.Errorf("imagegen: midjourney bridge: %s", snippet([]byte(mr.Error)))
		case mr.ImageBase64 != "":
			if raw, err = base64.StdEncoding.DecodeString(mr.ImageBase64); err != nil {
				return nil, fmt.Errorf("imagegen: decode midjourney image: %w", err)
			}
		case mr.ImageURL != "":
			if raw, err = download(ctx, c.httpClient, "midjourney", mr.ImageURL); err != nil {
				return nil, err
			}
		default:
			return nil, fmt.Errorf("imagegen: midjourney bridge returned no image")
		}
	}
	out, err := finishImage(ProviderMidjourney, model, raw, req.Width, req.Height, started)
	if err != nil {
		return nil, err
	}
	out.Seed = req.Seed
	return out, nil
}
