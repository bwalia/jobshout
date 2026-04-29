package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// PullRequestClient is a minimal REST client for the GitHub Pulls API,
// intentionally separate from the issue-based TaskAdapter so the blog
// generator can use it without a persisted Integration row.
type PullRequestClient struct {
	Token   string
	BaseURL string // defaults to https://api.github.com
	HTTP    *http.Client
}

// NewPullRequestClient builds a client with sensible defaults.
func NewPullRequestClient(token string) *PullRequestClient {
	return &PullRequestClient{
		Token:   token,
		BaseURL: "https://api.github.com",
		HTTP:    &http.Client{Timeout: 30 * time.Second},
	}
}

// CreatedPR is the subset of the GitHub API response fields the caller needs.
type CreatedPR struct {
	Number  int    `json:"number"`
	HTMLURL string `json:"html_url"`
	State   string `json:"state"`
}

// CreatePullRequest opens a PR. `head` may be "owner:branch" for cross-fork PRs
// but for same-repo branches just "branch-name" is fine.
func (c *PullRequestClient) CreatePullRequest(
	ctx context.Context,
	owner, repo, title, head, base, body string,
) (*CreatedPR, error) {
	if c.Token == "" {
		return nil, fmt.Errorf("github: missing token")
	}
	payload := map[string]any{
		"title": title,
		"head":  head,
		"base":  base,
		"body":  body,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("github: marshal payload: %w", err)
	}

	url := fmt.Sprintf("%s/repos/%s/%s/pulls", c.BaseURL, owner, repo)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("github: build request: %w", err)
	}
	req.Header.Set("Authorization", "token "+c.Token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("github: do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("github: create pr: status %d: %s", resp.StatusCode, string(b))
	}

	var out CreatedPR
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("github: decode response: %w", err)
	}
	return &out, nil
}
