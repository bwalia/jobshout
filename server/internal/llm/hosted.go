package llm

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Failure classes for hosted (API-key) providers. Callers match them with
// errors.Is — a *ProviderError unwraps to exactly one — so an agent can tell a
// bad key from a busy provider without parsing provider-specific text.
var (
	// ErrProviderNotConfigured: the provider has no credentials on this
	// server, so it was never registered.
	ErrProviderNotConfigured = errors.New("llm provider is not configured")
	// ErrProviderAuth: the provider rejected the credentials (401/403, or a
	// key it reports as invalid).
	ErrProviderAuth = errors.New("llm provider rejected the credentials")
	// ErrProviderRateLimited: 429 or a quota error.
	ErrProviderRateLimited = errors.New("llm provider rate limit")
	// ErrProviderUnavailable: 5xx or a network failure reaching the provider.
	ErrProviderUnavailable = errors.New("llm provider unavailable")
	// ErrProviderTimeout: the per-request HTTP timeout elapsed.
	ErrProviderTimeout = errors.New("llm provider timed out")
	// ErrProviderBadRequest: the provider refused the request itself — an
	// unknown model, a blocked prompt, an invalid parameter.
	ErrProviderBadRequest = errors.New("llm provider rejected the request")
	// ErrMalformedResponse: a 2xx whose body is not the shape we expect.
	ErrMalformedResponse = errors.New("llm provider returned a malformed response")
)

// ProviderError is a classified failure from a hosted provider. Message is the
// provider's own explanation, trimmed; it never carries request headers, so an
// API key cannot leak through it.
type ProviderError struct {
	Provider string
	// Kind is one of the Err* sentinels above.
	Kind error
	// Status is the HTTP status, or zero when no response arrived.
	Status  int
	Message string
	// RetryAfter is the wait the provider asked for, when it named one.
	RetryAfter time.Duration
	// noRetry marks a failure that looks transient by status but is not — a
	// 429 for an exhausted billing quota clears only when someone pays.
	noRetry bool
}

func (e *ProviderError) Error() string {
	var b strings.Builder
	b.WriteString(e.Provider)
	b.WriteString(": ")
	b.WriteString(e.Kind.Error())
	if e.Status != 0 {
		fmt.Fprintf(&b, " (HTTP %d)", e.Status)
	}
	if e.Message != "" {
		b.WriteString(": ")
		b.WriteString(e.Message)
	}
	return b.String()
}

func (e *ProviderError) Unwrap() error { return e.Kind }

// Retryable reports whether sending the same request again could succeed.
// Timeouts are deliberately excluded: a generation that ran out the clock once
// will usually do so again, and a retry doubles the wait.
func (e *ProviderError) Retryable() bool {
	if e.noRetry {
		return false
	}
	return e.Kind == ErrProviderRateLimited || e.Kind == ErrProviderUnavailable
}

// kindForStatus maps an HTTP status to a failure class.
func kindForStatus(status int) error {
	switch {
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return ErrProviderAuth
	case status == http.StatusTooManyRequests:
		return ErrProviderRateLimited
	case status == http.StatusRequestTimeout:
		return ErrProviderTimeout
	case status >= 500:
		return ErrProviderUnavailable
	default:
		return ErrProviderBadRequest
	}
}

// retryPolicy bounds how a hosted client retries a transient failure.
type retryPolicy struct {
	// attempts is the total number of tries, including the first.
	attempts int
	// base is the first backoff; each later one doubles.
	base time.Duration
	// max caps any single wait. A provider asking for longer than this (a
	// per-day quota) is not waited on at all: the error is returned.
	max time.Duration
}

var defaultRetryPolicy = retryPolicy{attempts: 3, base: time.Second, max: 20 * time.Second}

// errorClassifier turns a non-2xx response into a ProviderError. Each provider
// supplies its own, because the error bodies differ.
type errorClassifier func(status int, header http.Header, body []byte) *ProviderError

// doHosted sends a request built by build, retrying transient failures under
// p. It returns the body of the first 2xx response.
//
// build is called once per attempt because a request body can be read only
// once. A cancelled or expired ctx is returned as-is — never retried and never
// relabelled — so the caller's cancellation reaches the task unchanged.
func doHosted(
	ctx context.Context, hc *http.Client, provider string, p retryPolicy,
	build func(context.Context) (*http.Request, error), classify errorClassifier,
) ([]byte, error) {
	attempts := max(p.attempts, 1)
	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		body, perr := doOnce(ctx, hc, provider, build, classify)
		if perr == nil {
			return body, nil
		}
		var pe *ProviderError
		if !errors.As(perr, &pe) || !pe.Retryable() || attempt == attempts {
			return nil, perr
		}
		lastErr = perr

		wait := p.base << (attempt - 1)
		if pe.RetryAfter > 0 {
			if pe.RetryAfter > p.max {
				return nil, perr
			}
			wait = pe.RetryAfter
		}
		wait = min(wait, p.max)
		t := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			t.Stop()
			return nil, ctx.Err()
		case <-t.C:
		}
		noteRetry(ctx)
	}
	return nil, lastErr
}

func doOnce(
	ctx context.Context, hc *http.Client, provider string,
	build func(context.Context) (*http.Request, error), classify errorClassifier,
) (_ []byte, err error) {
	req, err := build(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s: build request: %w", provider, err)
	}
	// One attempt is one HTTP request actually sent. It is timed from sending
	// to having read the reply, so the API duration excludes doHosted's
	// backoff waits, and each retry is kept as its own attempt.
	attempt := beginAttempt(ctx)
	defer func() { attempt.end(err) }()
	resp, err := hc.Do(req)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		kind := ErrProviderUnavailable
		var ne net.Error
		if errors.As(err, &ne) && ne.Timeout() {
			kind = ErrProviderTimeout
		}
		return nil, &ProviderError{Provider: provider, Kind: kind, Message: err.Error()}
	}
	defer resp.Body.Close()
	attempt.response(resp)

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxHostedResponseBytes))
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		kind := ErrProviderUnavailable
		var ne net.Error
		if errors.As(err, &ne) && ne.Timeout() {
			kind = ErrProviderTimeout
		}
		return nil, &ProviderError{Provider: provider, Kind: kind, Status: resp.StatusCode,
			Message: "reading response: " + err.Error()}
	}
	if resp.StatusCode/100 != 2 {
		return nil, classify(resp.StatusCode, resp.Header, body)
	}
	return body, nil
}

// maxHostedResponseBytes caps a reply we will read. A full article is well
// under a megabyte; this only stops a misbehaving endpoint exhausting memory.
const maxHostedResponseBytes = 16 << 20

// retryAfterHeader reads Retry-After (seconds) or OpenAI's retry-after-ms.
func retryAfterHeader(h http.Header) time.Duration {
	if v := strings.TrimSpace(h.Get("retry-after-ms")); v != "" {
		if ms, err := strconv.ParseFloat(v, 64); err == nil && ms > 0 {
			return time.Duration(ms * float64(time.Millisecond))
		}
	}
	if v := strings.TrimSpace(h.Get("Retry-After")); v != "" {
		if s, err := strconv.ParseFloat(v, 64); err == nil && s > 0 {
			return time.Duration(s * float64(time.Second))
		}
	}
	return 0
}

// malformed builds the error for a 2xx body we could not use.
func malformed(provider, detail string) *ProviderError {
	return &ProviderError{Provider: provider, Kind: ErrMalformedResponse, Message: detail}
}
