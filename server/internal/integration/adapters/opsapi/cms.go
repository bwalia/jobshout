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
	// Token is a seed opsapi JWT. It is exchanged for a fresh one before it
	// expires; see tokenSource.
	Token string
	// Namespace is the opsapi namespace slug that owns the posts. Sent as
	// X-Namespace-Slug, which is how opsapi's namespace middleware scopes the
	// request.
	Namespace string
	Timeout   time.Duration
}

// Complete reports whether enough is configured to talk to opsapi at all.
func (c Config) Complete() bool {
	return c.BaseURL != "" && c.Token != "" && c.Namespace != ""
}

// Client talks to opsapi's CMS endpoints.
type Client struct {
	cfg    Config
	http   *http.Client
	tokens *tokenSource
}

// NewClient builds a Client. Returns nil when the config is incomplete, so
// callers can hold a possibly-nil *Client and treat nil as "publishing is not
// configured" rather than carrying a separate flag.
func NewClient(cfg Config) *Client {
	if !cfg.Complete() {
		return nil
	}
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")
	if cfg.Timeout <= 0 {
		cfg.Timeout = DefaultTimeout
	}
	httpClient := &http.Client{Timeout: cfg.Timeout}
	return &Client{
		cfg:    cfg,
		http:   httpClient,
		tokens: newTokenSource(cfg.BaseURL, cfg.Token, httpClient),
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
// A 401 is retried once against a freshly minted token: our cached expiry is a
// guess made without the signing secret, so opsapi rejecting a token we thought
// was good is expected rather than exceptional. Every other status is returned
// as-is — a 400 from validation or a 403 from RBAC will say the same thing on
// the second attempt.
func (c *Client) CreatePost(ctx context.Context, req CreatePostRequest) (*Post, error) {
	if req.Status == "" {
		req.Status = StatusDraft
	}

	token, err := c.tokens.get(ctx)
	if err != nil {
		return nil, err
	}

	post, status, err := c.createPost(ctx, token, req)
	if err == nil {
		return post, nil
	}
	if status != http.StatusUnauthorized {
		return nil, err
	}

	token, refreshErr := c.tokens.forceRefresh(ctx)
	if refreshErr != nil {
		return nil, refreshErr
	}
	post, _, err = c.createPost(ctx, token, req)
	return post, err
}

// createPost performs one attempt and reports the HTTP status alongside the
// error so CreatePost can decide whether a retry is worth anything.
func (c *Client) createPost(ctx context.Context, token string, req CreatePostRequest) (*Post, int, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, 0, fmt.Errorf("opsapi: encode post: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.cfg.BaseURL+"/api/v2/cms/posts", bytes.NewReader(body))
	if err != nil {
		return nil, 0, fmt.Errorf("opsapi: build post request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+token)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("X-Namespace-Slug", c.cfg.Namespace)

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, 0, fmt.Errorf("opsapi: create post %q: %w", req.Title, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return nil, resp.StatusCode, c.postError(resp, req.Title)
	}

	var env postEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return nil, resp.StatusCode, fmt.Errorf("opsapi: decode post response: %w", err)
	}
	if !env.Success {
		return nil, resp.StatusCode, fmt.Errorf("opsapi: create post %q: %s", req.Title, env.Error)
	}
	return &env.Data, resp.StatusCode, nil
}

// postError turns a rejection into something that names the likely cause.
// opsapi returns 403 for both "namespace not enabled for cms" and "user lacks
// the cms module", which are different fixes, so the message points at both.
func (c *Client) postError(resp *http.Response, title string) error {
	body := readBodySnippet(resp)
	switch resp.StatusCode {
	case http.StatusUnauthorized:
		return fmt.Errorf("opsapi: create post %q rejected (401): %s", title, body)
	case http.StatusForbidden:
		return fmt.Errorf(
			"opsapi: create post %q forbidden (403) — the token's user needs the \"cms\" module in namespace %q, "+
				"and that namespace must have the cms feature enabled. Response: %s",
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
