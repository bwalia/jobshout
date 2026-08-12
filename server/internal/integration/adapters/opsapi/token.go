package opsapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// refreshMargin is how long before expiry we re-mint. opsapi issues one-hour
// tokens, so a few minutes of headroom covers a slow request and modest clock
// skew between the two hosts without refreshing on every call.
const refreshMargin = 5 * time.Minute

// graceWindow mirrors opsapi's REFRESH_GRACE_PERIOD (jwt-helper.lua): an
// expired token can still be exchanged for a fresh one for this long. We only
// use it to produce a better error message — once it has passed, no amount of
// retrying helps and an operator has to seed a new token.
const graceWindow = 7 * 24 * time.Hour

// tokenSource keeps a usable opsapi JWT available.
//
// opsapi has no unattended login: POST /auth/login always requires an emailed
// OTP, so a server cannot obtain a token from credentials. What it does have is
// POST /auth/refresh, which re-mints a token from the one you present and
// re-reads the user, roles and namespace permissions from its database while
// doing so. That makes a single seeded token self-sustaining: we exchange it
// before it expires and keep the replacement in memory.
//
// The seed only dies if this process is down for longer than opsapi's seven-day
// grace window, at which point a human has to paste in a new one.
type tokenSource struct {
	baseURL string
	http    *http.Client

	mu      sync.Mutex
	token   string
	expires time.Time
}

func newTokenSource(baseURL, seed string, httpClient *http.Client) *tokenSource {
	return &tokenSource{
		baseURL: baseURL,
		http:    httpClient,
		token:   seed,
		expires: tokenExpiry(seed),
	}
}

// get returns a token that is valid now, refreshing first if the current one is
// close to expiry.
func (t *tokenSource) get(ctx context.Context) (string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if time.Until(t.expires) > refreshMargin {
		return t.token, nil
	}
	return t.refreshLocked(ctx)
}

// forceRefresh re-mints even when the cached token still looks valid. Called
// after a 401, where opsapi disagrees with our reading of the expiry — a clock
// difference, or a token invalidated on their side.
func (t *tokenSource) forceRefresh(ctx context.Context) (string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.refreshLocked(ctx)
}

// refreshLocked exchanges the current token for a new one. Callers hold t.mu.
//
// The request deliberately carries no body: /auth/refresh prefers an opaque
// refresh token from the body or a cookie, and only falls through to the
// Authorization-header path when it finds neither.
func (t *tokenSource) refreshLocked(ctx context.Context) (string, error) {
	if t.token == "" {
		return "", fmt.Errorf("opsapi: no token configured (set OPSAPI_TOKEN)")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.baseURL+"/auth/refresh", bytes.NewReader(nil))
	if err != nil {
		return "", fmt.Errorf("opsapi: build refresh request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+t.token)
	req.Header.Set("Accept", "application/json")

	resp, err := t.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("opsapi: refresh token: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", t.refreshError(resp)
	}

	var out struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("opsapi: decode refresh response: %w", err)
	}
	if out.Token == "" {
		return "", fmt.Errorf("opsapi: refresh returned an empty token")
	}

	t.token = out.Token
	t.expires = tokenExpiry(out.Token)
	return t.token, nil
}

// refreshError explains a rejected refresh in terms of what an operator can do,
// since the usual cause — the seed sat unused past the grace window — is not
// something a retry fixes.
func (t *tokenSource) refreshError(resp *http.Response) error {
	body := readBodySnippet(resp)
	if resp.StatusCode == http.StatusUnauthorized {
		stale := time.Since(t.expires)
		if !t.expires.IsZero() && stale > graceWindow {
			return fmt.Errorf(
				"opsapi: refresh rejected — the configured token expired %s ago, past opsapi's %s grace window. "+
					"Seed OPSAPI_TOKEN with a freshly issued token. Response: %s",
				stale.Round(time.Hour), graceWindow, body)
		}
		return fmt.Errorf(
			"opsapi: refresh rejected (401) — OPSAPI_TOKEN is not a valid opsapi JWT, or its user is deactivated. "+
				"Response: %s", body)
	}
	return fmt.Errorf("opsapi: refresh failed (status %d): %s", resp.StatusCode, body)
}

// tokenExpiry reads the exp claim without verifying the signature — we do not
// hold opsapi's signing secret, and this value only schedules the refresh; the
// server remains the authority on whether a token is good.
//
// A token we cannot parse gets a zero expiry, which reads as "already expired"
// and sends us to refresh on first use. That is the right instinct: better to
// find out immediately than at publish time.
func tokenExpiry(token string) time.Time {
	if token == "" {
		return time.Time{}
	}
	parsed, _, err := jwt.NewParser().ParseUnverified(token, jwt.MapClaims{})
	if err != nil {
		return time.Time{}
	}
	exp, err := parsed.Claims.GetExpirationTime()
	if err != nil || exp == nil {
		return time.Time{}
	}
	return exp.Time
}
