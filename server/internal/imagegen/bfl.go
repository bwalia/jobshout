package imagegen

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// FLUX hosts. FLUX_HOST picks one; the provider name stays "flux" either way,
// so a per-task choice survives a change of host.
const (
	FluxHostBFL = "bfl"
	FluxHostFal = "fal"
)

// bflDefaultBaseURL is Black Forest Labs' global API endpoint. The regional
// endpoints (api.eu.bfl.ai, api.us.bfl.ai) can be set with FLUX_BASE_URL.
const bflDefaultBaseURL = "https://api.bfl.ai"

// FluxClient is the FLUX provider. It submits an async job and polls for the
// result, on either Black Forest Labs' own API or fal.ai.
//
// The model id is configuration (FLUX_MODEL) with no default in code: BFL's
// model paths (flux-2-pro, flux-2-max, ...) change faster than releases.
type FluxClient struct {
	host         string
	baseURL      string
	apiKey       string
	defaultModel string
	httpClient   *http.Client
}

// NewFluxClient builds a client. host is "bfl" (default) or "fal"; baseURL may
// be empty for the host's public endpoint.
func NewFluxClient(host, baseURL, apiKey, defaultModel string) *FluxClient {
	host = strings.ToLower(strings.TrimSpace(host))
	if host != FluxHostFal {
		host = FluxHostBFL
	}
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		if host == FluxHostFal {
			baseURL = falDefaultBaseURL
		} else {
			baseURL = bflDefaultBaseURL
		}
	}
	return &FluxClient{
		host: host, baseURL: baseURL,
		apiKey:       strings.TrimSpace(apiKey),
		defaultModel: strings.TrimSpace(defaultModel),
		httpClient:   &http.Client{Timeout: 60 * time.Second},
	}
}

// Provider implements Client.
func (c *FluxClient) Provider() string { return ProviderFlux }

// DefaultModel is the configured FLUX model.
func (c *FluxClient) DefaultModel() string { return c.defaultModel }

// Host reports "bfl" or "fal".
func (c *FluxClient) Host() string { return c.host }

// ListModels implements Client: the one configured model.
func (c *FluxClient) ListModels(ctx context.Context) ([]ModelInfo, error) {
	if c.defaultModel == "" {
		return nil, nil
	}
	return []ModelInfo{{Name: c.defaultModel, Provider: ProviderFlux, Available: c.apiKey != ""}}, nil
}

// Generate implements Client.
func (c *FluxClient) Generate(ctx context.Context, req GenerateRequest) (*GenerateResponse, error) {
	if err := req.Normalize(); err != nil {
		return nil, err
	}
	if c.apiKey == "" {
		return nil, &ProviderError{Provider: ProviderFlux, Msg: "no API key configured (set BFL_API_KEY, or FAL_KEY with FLUX_HOST=fal)"}
	}
	model := req.Model
	if model == "" {
		model = c.defaultModel
	}
	if model == "" {
		return nil, &ProviderError{Provider: ProviderFlux, Msg: "no model configured (set FLUX_MODEL)"}
	}
	started := time.Now()

	var raw []byte
	var err error
	if c.host == FluxHostFal {
		raw, err = c.generateFal(ctx, model, req)
	} else {
		raw, err = c.generateBFL(ctx, model, req)
	}
	if err != nil {
		return nil, err
	}
	resp, err := finishImage(ProviderFlux, model, raw, req.Width, req.Height, started)
	if err != nil {
		return nil, err
	}
	resp.Seed = req.Seed
	return resp, nil
}

type bflSubmitRequest struct {
	Prompt       string `json:"prompt"`
	Width        int    `json:"width"`
	Height       int    `json:"height"`
	Seed         *int64 `json:"seed,omitempty"`
	OutputFormat string `json:"output_format,omitempty"`
}

type bflSubmitResponse struct {
	ID         string `json:"id"`
	PollingURL string `json:"polling_url"`
	Detail     any    `json:"detail"`
}

type bflResult struct {
	Status string `json:"status"`
	Result struct {
		Sample string `json:"sample"`
	} `json:"result"`
	Details any `json:"details"`
}

func (c *FluxClient) generateBFL(ctx context.Context, model string, req GenerateRequest) ([]byte, error) {
	sub := bflSubmitRequest{Prompt: req.Prompt, Width: req.Width, Height: req.Height, OutputFormat: "png"}
	if req.Seed >= 0 {
		s := req.Seed
		sub.Seed = &s
	}
	body, err := json.Marshal(sub)
	if err != nil {
		return nil, fmt.Errorf("imagegen: encode request: %w", err)
	}
	url := fmt.Sprintf("%s/v1/%s", c.baseURL, strings.TrimLeft(model, "/"))

	resp, err := doRetry(ctx, c.httpClient, "flux", defaultRetry, func() (*http.Request, error) {
		r, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		r.Header.Set("x-key", c.apiKey)
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("accept", "application/json")
		return r, nil
	})
	if err != nil {
		return nil, err
	}
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	resp.Body.Close()
	if err := bflHTTPError(resp.StatusCode, data); err != nil {
		return nil, err
	}
	var sr bflSubmitResponse
	if err := json.Unmarshal(data, &sr); err != nil || sr.PollingURL == "" {
		return nil, fmt.Errorf("imagegen: flux submit returned no polling_url: %s", snippet(data))
	}

	// The docs say to poll the URL the API returned, never a constructed one.
	deadline := time.Now().Add(pollTimeout)
	for {
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("imagegen: flux job %s did not finish within %s", sr.ID, pollTimeout)
		}
		if err := sleepCtx(ctx, pollEvery); err != nil {
			return nil, err
		}
		pr, err := doRetry(ctx, c.httpClient, "flux", defaultRetry, func() (*http.Request, error) {
			r, err := http.NewRequest(http.MethodGet, sr.PollingURL, nil)
			if err != nil {
				return nil, err
			}
			r.Header.Set("x-key", c.apiKey)
			r.Header.Set("accept", "application/json")
			return r, nil
		})
		if err != nil {
			return nil, err
		}
		pd, _ := io.ReadAll(io.LimitReader(pr.Body, 1<<20))
		pr.Body.Close()
		if err := bflHTTPError(pr.StatusCode, pd); err != nil {
			return nil, err
		}
		var res bflResult
		if err := json.Unmarshal(pd, &res); err != nil {
			return nil, fmt.Errorf("imagegen: flux poll returned unreadable data: %s", snippet(pd))
		}
		switch {
		case res.Status == "Ready":
			if res.Result.Sample == "" {
				return nil, fmt.Errorf("imagegen: flux job finished without an image")
			}
			return download(ctx, c.httpClient, "flux", res.Result.Sample)
		case strings.Contains(res.Status, "Moderated"):
			return nil, &ProviderError{Provider: ProviderFlux, Kind: FailureModerated,
				Msg: "the prompt or picture was rejected by the safety filter"}
		case res.Status == "Error" || res.Status == "Failed" || res.Status == "Task not found":
			return nil, fmt.Errorf("imagegen: flux job %s: %s", res.Status, snippet(pd))
		}
		// Pending, Queued, Processing, ...: keep polling.
	}
}

// bflHTTPError maps BFL's documented HTTP failures: 402 out of credits, 429
// over the active-task limit.
func bflHTTPError(status int, body []byte) error {
	switch {
	case status >= 200 && status < 300:
		return nil
	case status == http.StatusPaymentRequired:
		return &ProviderError{Provider: ProviderFlux, Kind: FailureRateLimited, Msg: "out of credits at Black Forest Labs"}
	case status == http.StatusTooManyRequests:
		return &ProviderError{Provider: ProviderFlux, Kind: FailureRateLimited, Msg: "too many active jobs"}
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return &ProviderError{Provider: ProviderFlux, Msg: "the API key was rejected"}
	case status == http.StatusUnprocessableEntity && strings.Contains(strings.ToLower(string(body)), "moderat"):
		return &ProviderError{Provider: ProviderFlux, Kind: FailureModerated, Msg: "the prompt was rejected by the safety filter"}
	}
	return fmt.Errorf("imagegen: flux returned %d: %s", status, snippet(body))
}
