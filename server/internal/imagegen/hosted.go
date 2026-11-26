package imagegen

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// FailureKind classifies why a provider call failed, so the Router can decide
// between "try the next provider", "simplify the prompt" and "give up".
type FailureKind int

const (
	// FailureOther is anything not worth special handling.
	FailureOther FailureKind = iota
	// FailureRateLimited means the provider asked us to slow down or is out of
	// quota. The next provider in the chain should be tried.
	FailureRateLimited
	// FailureModerated means the provider's safety filter rejected the prompt
	// or the picture. A simplified prompt may pass.
	FailureModerated
)

// ProviderError is a classified provider failure. The message is written to be
// shown to a person; it never contains a credential.
type ProviderError struct {
	Provider string
	Kind     FailureKind
	Msg      string
}

func (e *ProviderError) Error() string { return "imagegen: " + e.Provider + ": " + e.Msg }

// IsModerated reports whether err is a safety-filter rejection.
func IsModerated(err error) bool {
	var pe *ProviderError
	return errors.As(err, &pe) && pe.Kind == FailureModerated
}

// IsRateLimited reports whether err is a rate-limit or quota failure.
func IsRateLimited(err error) bool {
	var pe *ProviderError
	return errors.As(err, &pe) && pe.Kind == FailureRateLimited
}

// Output limits applied to every hosted result before it is saved.
const (
	// MinImageDimension is the smallest side a cover may have. Below it the
	// picture is a thumbnail, not a cover.
	MinImageDimension = 256
	// MaxImageBytes bounds a download so a misbehaving host cannot fill memory.
	MaxImageBytes = 25 << 20
)

// retryPolicy bounds the retry-with-backoff around a single HTTP call.
type retryPolicy struct {
	attempts int
	base     time.Duration
}

var defaultRetry = retryPolicy{attempts: 3, base: time.Second}

// doRetry sends the request built by build, retrying 429 and 5xx responses and
// transport errors with exponential backoff (honouring Retry-After). The final
// response is returned open for the caller to read; a 429 that survives every
// attempt comes back as a rate-limited ProviderError.
func doRetry(ctx context.Context, hc *http.Client, provider string, p retryPolicy, build func() (*http.Request, error)) (*http.Response, error) {
	var lastErr error
	for attempt := 0; attempt < p.attempts; attempt++ {
		if attempt > 0 {
			if err := sleepCtx(ctx, backoff(p.base, attempt)); err != nil {
				return nil, err
			}
		}
		req, err := build()
		if err != nil {
			return nil, fmt.Errorf("imagegen: build request: %w", err)
		}
		resp, err := hc.Do(req.WithContext(ctx))
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			lastErr = fmt.Errorf("imagegen: call %s: %w", provider, err)
			continue
		}
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			wait := retryAfter(resp)
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
			resp.Body.Close()
			if resp.StatusCode == http.StatusTooManyRequests {
				lastErr = &ProviderError{Provider: provider, Kind: FailureRateLimited,
					Msg: "rate limited, try again shortly or use another provider"}
			} else {
				lastErr = fmt.Errorf("imagegen: %s returned %d: %s", provider, resp.StatusCode, snippet(body))
			}
			if wait > 0 && attempt+1 < p.attempts {
				if err := sleepCtx(ctx, wait); err != nil {
					return nil, err
				}
			}
			continue
		}
		return resp, nil
	}
	return nil, lastErr
}

func backoff(base time.Duration, attempt int) time.Duration {
	return base << (attempt - 1)
}

func retryAfter(resp *http.Response) time.Duration {
	if s, err := strconv.Atoi(strings.TrimSpace(resp.Header.Get("Retry-After"))); err == nil && s > 0 && s <= 30 {
		return time.Duration(s) * time.Second
	}
	return 0
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// download fetches a result URL, bounded by MaxImageBytes. No provider key is
// sent: result URLs are pre-signed, and a key must never travel to a CDN.
func download(ctx context.Context, hc *http.Client, provider, url string) ([]byte, error) {
	resp, err := doRetry(ctx, hc, provider, defaultRetry, func() (*http.Request, error) {
		return http.NewRequest(http.MethodGet, url, nil)
	})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("imagegen: %s image download returned %d", provider, resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, MaxImageBytes+1))
	if err != nil {
		return nil, fmt.Errorf("imagegen: read %s image: %w", provider, err)
	}
	if len(data) > MaxImageBytes {
		return nil, fmt.Errorf("imagegen: %s image is larger than %d MB", provider, MaxImageBytes>>20)
	}
	return data, nil
}

// finishImage validates raw (mime type, size, resolution) and normalises it to
// PNG, the format the rest of the platform stores.
func finishImage(provider, model string, raw []byte, wantW, wantH int, started time.Time) (*GenerateResponse, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("imagegen: %s returned an empty image", provider)
	}
	if len(raw) > MaxImageBytes {
		return nil, fmt.Errorf("imagegen: %s image is larger than %d MB", provider, MaxImageBytes>>20)
	}
	if !bytes.HasPrefix(raw, pngMagic) && !(len(raw) > 3 && raw[0] == 0xFF && raw[1] == 0xD8) {
		return nil, fmt.Errorf("imagegen: %s returned data that is not a PNG or JPEG image", provider)
	}
	pngBytes, w, h, err := encodeAsPNG(raw, "")
	if err != nil {
		return nil, err
	}
	if w < MinImageDimension || h < MinImageDimension {
		return nil, fmt.Errorf("imagegen: %s image is %dx%d, below the %dpx minimum", provider, w, h, MinImageDimension)
	}
	return &GenerateResponse{
		PNG: pngBytes, Provider: provider, Model: model,
		Width: w, Height: h,
		DurationMS: int(time.Since(started).Milliseconds()),
	}, nil
}

// pollEvery is how often an async job is checked; pollTimeout bounds the wait.
// Variables so tests can shorten them.
var (
	pollEvery   = 2 * time.Second
	pollTimeout = 3 * time.Minute
)
