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

	"github.com/jobshout/server/internal/gatewayauth"
)

// mfluxDefaultTimeout bounds a single generation.
//
// Generous by HTTP standards because the work is genuinely slow: a cold model
// load is tens of seconds before the first denoising step, and a queued request
// waits behind whatever is already on the GPU. Bounded anyway, because a
// request that will never come back should fail rather than hold a blog run
// open indefinitely.
const mfluxDefaultTimeout = 10 * time.Minute

// MFluxClient talks to the workstation image service (image-service/ in this
// repository), which runs mflux on Apple MLX.
//
// It is the local-model path: the weights sit on the workstation's disk and no
// prompt leaves the network. Because that machine is outside the cluster, the
// service is reached over the same JWT-gated public endpoint arrangement as
// Ollama.
type MFluxClient struct {
	baseURL      string
	defaultModel string
	auth         *gatewayauth.Signer
	httpClient   *http.Client
}

// NewMFluxClient builds a client for the image service at baseURL.
//
// An empty secret means no gateway — a service running on the same machine as
// this process needs no token, and passing one it will not check is harmless
// but pointless.
func NewMFluxClient(baseURL, defaultModel, jwtSecret string, timeout time.Duration) *MFluxClient {
	if timeout <= 0 {
		timeout = mfluxDefaultTimeout
	}
	return &MFluxClient{
		baseURL:      strings.TrimRight(baseURL, "/"),
		defaultModel: defaultModel,
		auth:         gatewayauth.New(jwtSecret),
		httpClient:   &http.Client{Timeout: timeout},
	}
}

// Provider implements Client.
func (c *MFluxClient) Provider() string { return ProviderMFlux }

// UsesGateway reports whether requests are signed. Useful in startup logs, where
// "reaching the image service unsigned" is worth noticing before a ring does it.
func (c *MFluxClient) UsesGateway() bool { return c.auth.Enabled() }

type mfluxGenerateRequest struct {
	Prompt         string   `json:"prompt"`
	Model          string   `json:"model,omitempty"`
	Width          int      `json:"width"`
	Height         int      `json:"height"`
	Steps          int      `json:"steps,omitempty"`
	Seed           int64    `json:"seed"`
	Guidance       *float64 `json:"guidance,omitempty"`
	NegativePrompt string   `json:"negative_prompt,omitempty"`
}

type mfluxGenerateResponse struct {
	ImageBase64 string `json:"image_base64"`
	Model       string `json:"model"`
	Seed        int64  `json:"seed"`
	Width       int    `json:"width"`
	Height      int    `json:"height"`
	Steps       int    `json:"steps"`
	DurationMS  int    `json:"duration_ms"`
}

type mfluxModelsResponse struct {
	Models []struct {
		Name       string `json:"name"`
		Repo       string `json:"repo"`
		Downloaded bool   `json:"downloaded"`
		Turbo      bool   `json:"turbo"`
	} `json:"models"`
	Default string `json:"default"`
}

// do performs a signed request and returns the body, mapping the failures that
// have distinct causes onto errors that say which one happened.
func (c *MFluxClient) do(ctx context.Context, method, path string, body any) ([]byte, error) {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("imagegen: encode request: %w", err)
		}
		reader = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return nil, fmt.Errorf("imagegen: build request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if err := c.auth.Apply(req); err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("imagegen: call image service at %s: %w", c.baseURL, err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("imagegen: read response: %w", err)
	}

	if gatewayauth.IsAuthStatus(resp.StatusCode) {
		// Terminal, and worth saying why: retrying with the same secret
		// produces the same verdict, so a caller should fix configuration
		// rather than back off.
		return nil, fmt.Errorf(
			"imagegen: the image gateway rejected the request (status %d) — the JWT signature did not verify. "+
				"Check IMAGE_JWT_SECRET matches the gateway's secret and that this host's clock is accurate. "+
				"Response: %s",
			resp.StatusCode, snippet(data),
		)
	}
	if resp.StatusCode == http.StatusServiceUnavailable {
		// The service says it is busy, not broken. Named separately so a caller
		// can tell "come back later" apart from "this will never work".
		return nil, fmt.Errorf("imagegen: the image service is busy: %s", snippet(data))
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("imagegen: image service returned %d: %s", resp.StatusCode, snippet(data))
	}
	return data, nil
}

// Generate implements Client.
func (c *MFluxClient) Generate(ctx context.Context, req GenerateRequest) (*GenerateResponse, error) {
	if err := req.Normalize(); err != nil {
		return nil, err
	}

	model := req.Model
	if model == "" {
		model = c.defaultModel
	}

	data, err := c.do(ctx, http.MethodPost, "/api/generate", mfluxGenerateRequest{
		Prompt:         req.Prompt,
		Model:          model,
		Width:          req.Width,
		Height:         req.Height,
		Steps:          req.Steps,
		Seed:           req.Seed,
		NegativePrompt: req.NegativePrompt,
	})
	if err != nil {
		return nil, err
	}

	var parsed mfluxGenerateResponse
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, fmt.Errorf("imagegen: decode image service response: %w", err)
	}

	png, err := base64.StdEncoding.DecodeString(parsed.ImageBase64)
	if err != nil {
		return nil, fmt.Errorf("imagegen: decode image data: %w", err)
	}
	if len(png) == 0 {
		return nil, fmt.Errorf("imagegen: the image service returned an empty image")
	}

	return &GenerateResponse{
		PNG:        png,
		Provider:   ProviderMFlux,
		Model:      parsed.Model,
		Seed:       parsed.Seed,
		Width:      parsed.Width,
		Height:     parsed.Height,
		Steps:      parsed.Steps,
		DurationMS: parsed.DurationMS,
	}, nil
}

// ListModels implements Client.
func (c *MFluxClient) ListModels(ctx context.Context) ([]ModelInfo, error) {
	data, err := c.do(ctx, http.MethodGet, "/api/models", nil)
	if err != nil {
		return nil, err
	}

	var parsed mfluxModelsResponse
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, fmt.Errorf("imagegen: decode model list: %w", err)
	}

	out := make([]ModelInfo, 0, len(parsed.Models))
	for _, m := range parsed.Models {
		out = append(out, ModelInfo{
			Name:     m.Name,
			Provider: ProviderMFlux,
			Repo:     m.Repo,
			// The service's "downloaded" is exactly this package's
			// "available": weights on disk are what separates a model that
			// generates from one that downloads 31 GB first.
			Available: m.Downloaded,
			Fast:      m.Turbo,
		})
	}
	return out, nil
}

// snippetLimit bounds how much of an upstream body reaches an error message.
// Long enough to identify the failure, short enough that the message stays
// readable on a card in the UI, which is where these are read.
const snippetLimit = 200

// snippet reduces a response body to something worth showing a human. A gateway
// answers with an HTML error page, and embedding one verbatim buries the single
// useful sentence inside a full document.
func snippet(body []byte) string {
	s := strings.TrimSpace(string(body))
	if s == "" {
		return "(empty response)"
	}
	if strings.Contains(s, "<") && strings.Contains(s, ">") {
		s = stripTags(s)
	}
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > snippetLimit {
		s = strings.TrimSpace(s[:snippetLimit]) + "…"
	}
	return s
}

// stripTags removes markup without pulling in a parser. It is used only to make
// an error message readable, so it does not need to be correct about anything
// beyond "this was an HTML page".
func stripTags(s string) string {
	var b strings.Builder
	depth := 0
	for _, r := range s {
		switch {
		case r == '<':
			depth++
		case r == '>':
			if depth > 0 {
				depth--
			}
			b.WriteRune(' ')
		case depth == 0:
			b.WriteRune(r)
		}
	}
	return b.String()
}
