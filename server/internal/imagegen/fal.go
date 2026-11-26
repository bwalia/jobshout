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

// falDefaultBaseURL is fal.ai's queue endpoint.
const falDefaultBaseURL = "https://queue.fal.run"

type falSize struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

type falInput struct {
	Prompt       string  `json:"prompt"`
	ImageSize    falSize `json:"image_size"`
	Seed         *int64  `json:"seed,omitempty"`
	OutputFormat string  `json:"output_format,omitempty"`
}

type falSubmitResponse struct {
	RequestID   string `json:"request_id"`
	StatusURL   string `json:"status_url"`
	ResponseURL string `json:"response_url"`
}

type falStatus struct {
	Status    string `json:"status"`
	Error     string `json:"error"`
	ErrorType string `json:"error_type"`
}

type falOutput struct {
	Images []struct {
		URL string `json:"url"`
	} `json:"images"`
}

// generateFal runs a FLUX model on fal.ai's queue API: submit, poll the status
// URL the submit call returned, then fetch the response URL. The model is the
// fal endpoint id (FLUX_MODEL), e.g. "fal-ai/flux-pro/v1.1".
func (c *FluxClient) generateFal(ctx context.Context, model string, req GenerateRequest) ([]byte, error) {
	in := falInput{
		Prompt:       req.Prompt,
		ImageSize:    falSize{Width: req.Width, Height: req.Height},
		OutputFormat: "png",
	}
	if req.Seed >= 0 {
		s := req.Seed
		in.Seed = &s
	}
	body, err := json.Marshal(in)
	if err != nil {
		return nil, fmt.Errorf("imagegen: encode request: %w", err)
	}
	auth := func(r *http.Request) {
		r.Header.Set("Authorization", "Key "+c.apiKey)
		r.Header.Set("Content-Type", "application/json")
	}

	resp, err := doRetry(ctx, c.httpClient, "flux", defaultRetry, func() (*http.Request, error) {
		r, err := http.NewRequest(http.MethodPost, c.baseURL+"/"+strings.Trim(model, "/"), bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		auth(r)
		return r, nil
	})
	if err != nil {
		return nil, err
	}
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	resp.Body.Close()
	if err := falHTTPError(resp.StatusCode, data); err != nil {
		return nil, err
	}
	var sub falSubmitResponse
	if err := json.Unmarshal(data, &sub); err != nil || sub.StatusURL == "" || sub.ResponseURL == "" {
		return nil, fmt.Errorf("imagegen: fal submit returned no status_url: %s", snippet(data))
	}

	get := func(url string) (int, []byte, error) {
		r, err := doRetry(ctx, c.httpClient, "flux", defaultRetry, func() (*http.Request, error) {
			rq, err := http.NewRequest(http.MethodGet, url, nil)
			if err != nil {
				return nil, err
			}
			auth(rq)
			return rq, nil
		})
		if err != nil {
			return 0, nil, err
		}
		defer r.Body.Close()
		b, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		return r.StatusCode, b, nil
	}

	deadline := time.Now().Add(pollTimeout)
	for {
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("imagegen: fal request %s did not finish within %s", sub.RequestID, pollTimeout)
		}
		if err := sleepCtx(ctx, pollEvery); err != nil {
			return nil, err
		}
		code, sd, err := get(sub.StatusURL)
		if err != nil {
			return nil, err
		}
		if err := falHTTPError(code, sd); err != nil {
			return nil, err
		}
		var st falStatus
		if err := json.Unmarshal(sd, &st); err != nil {
			return nil, fmt.Errorf("imagegen: fal status unreadable: %s", snippet(sd))
		}
		if st.Status != "COMPLETED" {
			continue // IN_QUEUE or IN_PROGRESS
		}
		if st.Error != "" {
			return nil, falFailure(st.ErrorType, st.Error)
		}
		code, rd, err := get(sub.ResponseURL)
		if err != nil {
			return nil, err
		}
		if err := falHTTPError(code, rd); err != nil {
			return nil, err
		}
		var out falOutput
		if err := json.Unmarshal(rd, &out); err != nil || len(out.Images) == 0 || out.Images[0].URL == "" {
			return nil, fmt.Errorf("imagegen: fal returned no image: %s", snippet(rd))
		}
		return download(ctx, c.httpClient, "flux", out.Images[0].URL)
	}
}

func falFailure(kind, msg string) error {
	l := strings.ToLower(kind + " " + msg)
	if strings.Contains(l, "content_policy") || strings.Contains(l, "nsfw") || strings.Contains(l, "safety") {
		return &ProviderError{Provider: ProviderFlux, Kind: FailureModerated,
			Msg: "the prompt or picture was rejected by the safety filter"}
	}
	return fmt.Errorf("imagegen: fal failed: %s", snippet([]byte(msg)))
}

func falHTTPError(status int, body []byte) error {
	switch {
	case status >= 200 && status < 300:
		return nil
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return &ProviderError{Provider: ProviderFlux, Msg: "the fal.ai key was rejected or the account is locked"}
	case status == http.StatusUnprocessableEntity:
		return falFailure("", string(body))
	}
	return fmt.Errorf("imagegen: fal returned %d: %s", status, snippet(body))
}
