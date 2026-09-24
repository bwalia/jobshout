// Package jobshoutcom is a client for the JobShout.com marketplace API — the
// public site whose Insights hub (/insights) carries news, articles and
// podcasts about AI and work.
//
// Only what the platform needs is implemented: an agent submitting an item.
// Submissions from agents always land in the Insights review queue, so nothing
// this client sends is public until an editor approves it on jobshout.com.
package jobshoutcom

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

// DefaultTimeout bounds a single call. The API is in the same cluster
// namespace, so anything slower than this is a problem with the link.
const DefaultTimeout = 30 * time.Second

// Config is what the client needs to reach a JobShout.com API.
type Config struct {
	// BaseURL is the API root, e.g. http://jobshout-com-api:8088 — the
	// in-cluster Service in the same ring namespace. No /api/v1 suffix.
	BaseURL string
	// Token is JobShout.com's shared JOBSHOUT_INTERNAL_TOKEN. The API trusts
	// identity headers only when they arrive with it.
	Token string
	// Agent is who submissions are attributed to, e.g. "Article Writer".
	Agent   string
	Timeout time.Duration
}

// Complete reports whether enough is configured to submit anything.
func (c Config) Complete() bool {
	return c.BaseURL != "" && c.Token != "" && c.Agent != ""
}

// Client submits to the Insights hub.
type Client struct {
	cfg  Config
	http *http.Client
}

// NewClient returns nil when the config is incomplete, so callers can hold a
// possibly-nil *Client and read nil as "Insights is not configured".
func NewClient(cfg Config) *Client {
	cfg.BaseURL = strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	// Secrets pasted into a store often carry a trailing newline, which is an
	// invalid header value on every request.
	cfg.Token = strings.TrimSpace(cfg.Token)
	cfg.Agent = strings.TrimSpace(cfg.Agent)
	if !cfg.Complete() {
		return nil
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = DefaultTimeout
	}
	return &Client{cfg: cfg, http: &http.Client{Timeout: cfg.Timeout}}
}

// Kinds accepted by the Insights API.
const (
	KindArticle = "article"
	KindBlog    = "blog"
	KindPost    = "post"
)

// SubmitInsightRequest is the body of POST /api/v1/insights.
type SubmitInsightRequest struct {
	Kind          string   `json:"kind"`
	Title         string   `json:"title"`
	Summary       string   `json:"summary,omitempty"`
	BodyMarkdown  string   `json:"body_md"`
	CoverImageURL string   `json:"cover_image_url,omitempty"`
	CoverImageAlt string   `json:"cover_image_alt,omitempty"`
	LinkURL       string   `json:"link_url,omitempty"`
	Topics        []string `json:"topics"`
	// Submit asks for review. An agent's item goes to review either way; this
	// is sent so the API validates it as a complete item, not a draft.
	Submit bool `json:"submit"`
}

// Insight is the part of the API's response the platform records.
type Insight struct {
	ID     string `json:"id"`
	Slug   string `json:"slug"`
	Status string `json:"status"`
}

type apiError struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// SubmitInsight files one item for review.
func (c *Client) SubmitInsight(ctx context.Context, req SubmitInsightRequest) (*Insight, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("jobshoutcom: encode insight: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.cfg.BaseURL+"/api/v1/insights", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("jobshoutcom: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("X-Jobshout-Agent", c.cfg.Agent)
	httpReq.Header.Set("X-Jobshout-Internal-Token", c.cfg.Token)

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("jobshoutcom: submit %q: %w", req.Title, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		var e apiError
		msg := strings.TrimSpace(string(raw))
		if json.Unmarshal(raw, &e) == nil && e.Error.Message != "" {
			msg = e.Error.Message
		}
		if len(msg) > 300 {
			msg = msg[:300]
		}
		return nil, fmt.Errorf("jobshoutcom: submit %q: %d %s", req.Title, resp.StatusCode, msg)
	}
	var out Insight
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("jobshoutcom: decode response: %w", err)
	}
	if out.ID == "" {
		return nil, fmt.Errorf("jobshoutcom: response had no insight id")
	}
	return &out, nil
}
