// Package opsapi is a client for the opsapi CMS, the system that owns the
// public website's blog. JobShout writes articles; opsapi publishes them.
//
// Only the piece JobShout needs is implemented: creating a post. Posts are
// created as drafts, so nothing this client sends appears on a public site
// until someone approves it in the opsapi dashboard.
package opsapi

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

// DefaultTimeout bounds a single CMS call. A post body is a few tens of KB, so
// anything slower than this is a problem with the link, not the payload.
const DefaultTimeout = 30 * time.Second

// Post statuses accepted by opsapi's POST /api/v2/cms/posts.
const (
	StatusDraft     = "draft"
	StatusPublished = "published"
)

// Config is what the client needs to reach an opsapi deployment.
type Config struct {
	// BaseURL is the API root, e.g. https://opsapi.example.com — no trailing
	// slash and no /api/v2 suffix; both are added where needed.
	BaseURL string
	// APIKey is a namespace-scoped opsapi API key ("opsk_..."), minted via
	// opsapi's POST /api/v2/api-keys with scope {"cms":["create"]}. It is a
	// long-lived machine credential — no refresh dance, no expiry to babysit.
	APIKey string
	// Namespace is the opsapi namespace slug that owns the posts. Sent as
	// X-Namespace-Slug; it must be the namespace the API key belongs to, or
	// opsapi refuses the request with a 403.
	Namespace string
	Timeout   time.Duration
}

// Complete reports whether enough is configured to talk to opsapi at all.
func (c Config) Complete() bool {
	return c.BaseURL != "" && c.APIKey != "" && c.Namespace != ""
}

// Client talks to opsapi's CMS endpoints.
type Client struct {
	cfg  Config
	http *http.Client
}

// NewClient builds a Client. Returns nil when the config is incomplete, so
// callers can hold a possibly-nil *Client and treat nil as "publishing is not
// configured" rather than carrying a separate flag.
func NewClient(cfg Config) *Client {
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")
	// A key pasted into a secret store often arrives with a trailing newline,
	// which Go rejects as an invalid header value on every request. Trimmed
	// before Complete() so a whitespace-only key reads as "not configured".
	cfg.APIKey = strings.TrimSpace(cfg.APIKey)
	if !cfg.Complete() {
		return nil
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = DefaultTimeout
	}
	return &Client{
		cfg:  cfg,
		http: &http.Client{Timeout: cfg.Timeout},
	}
}

// Namespace returns the namespace slug posts are written to, for logging and
// for telling a user where an article went.
func (c *Client) Namespace() string { return c.cfg.Namespace }

// BaseURL returns the configured API root.
func (c *Client) BaseURL() string { return c.cfg.BaseURL }

// CreatePostRequest is the body of POST /api/v2/cms/posts.
//
// Fields opsapi accepts but JobShout has no opinion on — category, featured
// image, scheduling, visibility — are left out so opsapi's defaults apply.
type CreatePostRequest struct {
	Title          string   `json:"title"`
	Slug           string   `json:"slug,omitempty"`
	Excerpt        string   `json:"excerpt,omitempty"`
	ContentHTML    string   `json:"content_html"`
	Status         string   `json:"status"`
	AuthorName     string   `json:"author_name,omitempty"`
	Tags           []string `json:"tags,omitempty"`
	SEOTitle       string   `json:"seo_title,omitempty"`
	SEODescription string   `json:"seo_description,omitempty"`
}

// Post is the subset of opsapi's created-post row that JobShout stores. The
// UUID is what lets a user find the draft in the opsapi dashboard.
type Post struct {
	UUID   string `json:"uuid"`
	Title  string `json:"title"`
	Slug   string `json:"slug"`
	Status string `json:"status"`
}

// postEnvelope is opsapi's house response shape (see helper/cms-http.lua).
type postEnvelope struct {
	Success bool   `json:"success"`
	Data    Post   `json:"data"`
	Error   string `json:"error"`
}

// CreatePost creates one blog post and returns what opsapi stored.
//
// No status is retried: the API key either works or it doesn't. A 401 means
// the key was revoked or expired, and a 400/403 will say the same thing on a
// second attempt.
func (c *Client) CreatePost(ctx context.Context, req CreatePostRequest) (*Post, error) {
	if req.Status == "" {
		req.Status = StatusDraft
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("opsapi: encode post: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.cfg.BaseURL+"/api/v2/cms/posts", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("opsapi: build post request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("X-Namespace-Slug", c.cfg.Namespace)

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("opsapi: create post %q: %w", req.Title, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return nil, c.postError(resp, req.Title)
	}

	var env postEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return nil, fmt.Errorf("opsapi: decode post response: %w", err)
	}
	if !env.Success {
		return nil, fmt.Errorf("opsapi: create post %q: %s", req.Title, env.Error)
	}
	return &env.Data, nil
}

// postError turns a rejection into something that names the likely cause.
// opsapi returns 403 for a key missing the cms scope, a key used against a
// namespace it does not belong to, and a namespace without the cms feature —
// different fixes, so the message points at all of them.
func (c *Client) postError(resp *http.Response, title string) error {
	body := readBodySnippet(resp)
	switch resp.StatusCode {
	case http.StatusUnauthorized:
		return fmt.Errorf(
			"opsapi: create post %q rejected (401) — OPSAPI_API_KEY is invalid, revoked or expired. "+
				"Mint a new key via opsapi's POST /api/v2/api-keys. Response: %s", title, body)
	case http.StatusForbidden:
		return fmt.Errorf(
			"opsapi: create post %q forbidden (403) — the API key needs the cms:create scope, must belong to "+
				"namespace %q, and that namespace must have the cms feature enabled. Response: %s",
			title, c.cfg.Namespace, body)
	case http.StatusNotFound:
		return fmt.Errorf(
			"opsapi: namespace %q not found — check OPSAPI_NAMESPACE. Response: %s",
			c.cfg.Namespace, body)
	default:
		return fmt.Errorf("opsapi: create post %q failed (status %d): %s", title, resp.StatusCode, body)
	}
}

// readBodySnippet reads a bounded amount of an error body. opsapi can return an
// nginx HTML error page rather than JSON, and a whole one adds nothing a log
// reader needs.
func readBodySnippet(resp *http.Response) string {
	b, err := io.ReadAll(io.LimitReader(resp.Body, 2048))
	if err != nil || len(b) == 0 {
		return "(no body)"
	}
	return strings.TrimSpace(string(b))
}
