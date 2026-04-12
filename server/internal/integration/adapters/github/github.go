package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/jobshout/server/internal/integration"
	"github.com/jobshout/server/internal/model"
)

// StatusToState maps JobShout task status to GitHub issue state.
func StatusToState(status string) string {
	if status == "done" {
		return "closed"
	}
	return "open"
}

// StateToStatus maps GitHub issue state to JobShout task status.
func StateToStatus(state string) string {
	if state == "closed" {
		return "done"
	}
	return "in_progress"
}

type adapter struct {
	token  string
	owner  string
	repo   string
	client *http.Client
}

// NewAdapter creates a GitHub Issues TaskAdapter from an integration config.
func NewAdapter(cfg model.Integration) integration.TaskAdapter {
	creds := cfg.Credentials
	token, _ := creds["token"].(string)
	owner, _ := creds["owner"].(string)
	repo, _ := creds["repo"].(string)

	return &adapter{
		token:  token,
		owner:  owner,
		repo:   repo,
		client: &http.Client{},
	}
}

func (a *adapter) Name() string { return "github" }

func (a *adapter) CreateIssue(ctx context.Context, issue integration.ExternalIssue) (string, string, error) {
	body := map[string]any{
		"title":  issue.Title,
		"body":   issue.Description,
		"labels": append(issue.Labels, "jobshout", "jobshout:priority:"+issue.Priority),
	}

	resp, err := a.doRequest(ctx, "POST", fmt.Sprintf("/repos/%s/%s/issues", a.owner, a.repo), body)
	if err != nil {
		return "", "", fmt.Errorf("github create issue: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return "", "", fmt.Errorf("github create issue: status %d: %s", resp.StatusCode, string(b))
	}

	var result struct {
		Number  int    `json:"number"`
		HTMLURL string `json:"html_url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", "", fmt.Errorf("github decode response: %w", err)
	}

	return strconv.Itoa(result.Number), result.HTMLURL, nil
}

func (a *adapter) UpdateIssue(ctx context.Context, externalID string, issue integration.ExternalIssue) error {
	body := map[string]any{
		"title": issue.Title,
		"state": StatusToState(issue.Status),
	}
	if issue.Description != "" {
		body["body"] = issue.Description
	}

	resp, err := a.doRequest(ctx, "PATCH", fmt.Sprintf("/repos/%s/%s/issues/%s", a.owner, a.repo, externalID), body)
	if err != nil {
		return fmt.Errorf("github update issue: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("github update issue: status %d: %s", resp.StatusCode, string(b))
	}
	return nil
}

func (a *adapter) GetIssue(ctx context.Context, externalID string) (*integration.ExternalIssue, error) {
	resp, err := a.doRequest(ctx, "GET", fmt.Sprintf("/repos/%s/%s/issues/%s", a.owner, a.repo, externalID), nil)
	if err != nil {
		return nil, fmt.Errorf("github get issue: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("github get issue: status %d: %s", resp.StatusCode, string(b))
	}

	var result struct {
		Number  int    `json:"number"`
		Title   string `json:"title"`
		Body    string `json:"body"`
		State   string `json:"state"`
		HTMLURL string `json:"html_url"`
		Labels  []struct {
			Name string `json:"name"`
		} `json:"labels"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("github decode: %w", err)
	}

	labels := make([]string, len(result.Labels))
	for i, l := range result.Labels {
		labels[i] = l.Name
	}

	return &integration.ExternalIssue{
		ExternalID:  strconv.Itoa(result.Number),
		ExternalURL: result.HTMLURL,
		Title:       result.Title,
		Description: result.Body,
		Status:      StateToStatus(result.State),
		Labels:      labels,
	}, nil
}

func (a *adapter) DeleteIssue(ctx context.Context, externalID string) error {
	// GitHub doesn't support deleting issues — close it instead
	body := map[string]any{"state": "closed"}
	resp, err := a.doRequest(ctx, "PATCH", fmt.Sprintf("/repos/%s/%s/issues/%s", a.owner, a.repo, externalID), body)
	if err != nil {
		return fmt.Errorf("github close issue: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("github close issue: status %d: %s", resp.StatusCode, string(b))
	}
	return nil
}

func (a *adapter) doRequest(ctx context.Context, method, path string, body any) (*http.Response, error) {
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, "https://api.github.com"+path, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "token "+a.token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	return a.client.Do(req)
}
