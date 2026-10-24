package marketingemail

import (
	"context"
	"net/http"
	"sync"
	"time"
)

// LogoChecker says whether a logo URL answers 200 right now, so an email never
// ships a broken header image. Results are cached: a working logo for
// logoOKTTL, a missing one for logoMissingTTL (so a PNG uploaded later is
// picked up within minutes).
type LogoChecker struct {
	HTTP *http.Client

	mu    sync.Mutex
	cache map[string]logoResult
	now   func() time.Time
}

type logoResult struct {
	ok      bool
	expires time.Time
}

const (
	logoOKTTL      = time.Hour
	logoMissingTTL = 10 * time.Minute
	logoTimeout    = 8 * time.Second
)

// NewLogoChecker returns a checker with its own short-timeout HTTP client.
func NewLogoChecker() *LogoChecker {
	return &LogoChecker{HTTP: &http.Client{Timeout: logoTimeout}}
}

// Available reports whether logoURL is an https URL that answers 200.
func (c *LogoChecker) Available(ctx context.Context, logoURL string) bool {
	if c == nil || !IsHTTPS(logoURL) || !safeHref(logoURL) {
		return false
	}
	now := time.Now
	if c.now != nil {
		now = c.now
	}
	c.mu.Lock()
	if r, ok := c.cache[logoURL]; ok && now().Before(r.expires) {
		c.mu.Unlock()
		return r.ok
	}
	c.mu.Unlock()

	ok := c.probe(ctx, logoURL)
	ttl := logoMissingTTL
	if ok {
		ttl = logoOKTTL
	}
	c.mu.Lock()
	if c.cache == nil {
		c.cache = map[string]logoResult{}
	}
	c.cache[logoURL] = logoResult{ok: ok, expires: now().Add(ttl)}
	c.mu.Unlock()
	return ok
}

// probe tries HEAD, then GET for servers that refuse HEAD.
func (c *LogoChecker) probe(ctx context.Context, logoURL string) bool {
	client := c.HTTP
	if client == nil {
		client = &http.Client{Timeout: logoTimeout}
	}
	for _, method := range []string{http.MethodHead, http.MethodGet} {
		req, err := http.NewRequestWithContext(ctx, method, logoURL, nil)
		if err != nil {
			return false
		}
		resp, err := client.Do(req)
		if err != nil {
			return false
		}
		resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			return true
		}
		if resp.StatusCode != http.StatusMethodNotAllowed && resp.StatusCode != http.StatusNotImplemented {
			return false
		}
	}
	return false
}
