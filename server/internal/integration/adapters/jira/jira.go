package jira

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/jobshout/server/internal/integration"
	"github.com/jobshout/server/internal/model"
)

// StatusMapping maps JobShout statuses to Jira status names.
var StatusMapping = map[string]string{
	"backlog":     "To Do",
	"todo":        "To Do",
	"in_progress": "In Progress",
	"review":      "In Review",
	"done":        "Done",
}

// ReverseStatusMapping maps Jira statuses back to JobShout.
var ReverseStatusMapping = map[string]string{
	"To Do":       "todo",
	"In Progress": "in_progress",
	"In Review":   "review",
	"Done":        "done",
	"Blocked":     "backlog",
}

// PriorityMapping maps JobShout priorities to Jira priority names.
var PriorityMapping = map[string]string{
	"critical": "Highest",
	"high":     "High",
	"medium":   "Medium",
	"low":      "Low",
}

type adapter struct {
	baseURL    string
	email      string
	apiToken   string
	projectKey string
	client     *http.Client
}

// NewAdapter creates a Jira TaskAdapter from an integration config.
func NewAdapter(cfg model.Integration) integration.TaskAdapter {
	creds := cfg.Credentials
	email, _ := creds["email"].(string)
	token, _ := creds["api_token"].(string)
	projectKey, _ := creds["project_key"].(string)

	return &adapter{
		baseURL:    cfg.BaseURL,
		email:      email,
		apiToken:   token,
		projectKey: projectKey,
		client:     &http.Client{},
	}
}

func (a *adapter) Name() string { return "jira" }

func (a *adapter) CreateIssue(ctx context.Context, issue integration.ExternalIssue) (string, string, error) {
	priority := PriorityMapping[issue.Priority]
	if priority == "" {
		priority = "Medium"
	}

	body := map[string]any{
		"fields": map[string]any{
			"project":     map[string]string{"key": a.projectKey},
			"summary":     issue.Title,
			"description": map[string]any{"type": "doc", "version": 1, "content": []any{map[string]any{"type": "paragraph", "content": []any{map[string]any{"type": "text", "text": issue.Description}}}}},
			"issuetype":   map[string]string{"name": "Task"},
			"priority":    map[string]string{"name": priority},
		},
	}

	resp, err := a.doRequest(ctx, "POST", "/rest/api/3/issue", body)
	if err != nil {
		return "", "", fmt.Errorf("jira create issue: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return "", "", fmt.Errorf("jira create issue: status %d: %s", resp.StatusCode, string(b))
	}

	var result struct {
		Key  string `json:"key"`
		Self string `json:"self"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", "", fmt.Errorf("jira decode response: %w", err)
	}

	issueURL := fmt.Sprintf("%s/browse/%s", a.baseURL, result.Key)
	return result.Key, issueURL, nil
}

func (a *adapter) UpdateIssue(ctx context.Context, externalID string, issue integration.ExternalIssue) error {
	body := map[string]any{
		"fields": map[string]any{
			"summary": issue.Title,
		},
	}

	resp, err := a.doRequest(ctx, "PUT", "/rest/api/3/issue/"+externalID, body)
	if err != nil {
		return fmt.Errorf("jira update issue: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("jira update issue: status %d: %s", resp.StatusCode, string(b))
	}

	// Handle status transition if needed
	if issue.Status != "" {
		if jiraStatus, ok := StatusMapping[issue.Status]; ok {
			return a.transitionIssue(ctx, externalID, jiraStatus)
		}
	}

	return nil
}

func (a *adapter) GetIssue(ctx context.Context, externalID string) (*integration.ExternalIssue, error) {
	resp, err := a.doRequest(ctx, "GET", "/rest/api/3/issue/"+externalID, nil)
	if err != nil {
		return nil, fmt.Errorf("jira get issue: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("jira get issue: status %d: %s", resp.StatusCode, string(b))
	}

	var result struct {
		Key    string `json:"key"`
		Fields struct {
			Summary  string `json:"summary"`
			Status   struct{ Name string } `json:"status"`
			Priority struct{ Name string } `json:"priority"`
		} `json:"fields"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("jira decode: %w", err)
	}

	status := result.Fields.Status.Name
	if mapped, ok := ReverseStatusMapping[status]; ok {
		status = mapped
	}

	return &integration.ExternalIssue{
		ExternalID:  result.Key,
		ExternalURL: fmt.Sprintf("%s/browse/%s", a.baseURL, result.Key),
		Title:       result.Fields.Summary,
		Status:      status,
		Priority:    result.Fields.Priority.Name,
	}, nil
}

func (a *adapter) DeleteIssue(ctx context.Context, externalID string) error {
	resp, err := a.doRequest(ctx, "DELETE", "/rest/api/3/issue/"+externalID, nil)
	if err != nil {
		return fmt.Errorf("jira delete issue: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("jira delete issue: status %d: %s", resp.StatusCode, string(b))
	}
	return nil
}

func (a *adapter) transitionIssue(ctx context.Context, key, targetStatus string) error {
	// First, get available transitions
	resp, err := a.doRequest(ctx, "GET", "/rest/api/3/issue/"+key+"/transitions", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var transitions struct {
		Transitions []struct {
			ID string `json:"id"`
			To struct {
				Name string `json:"name"`
			} `json:"to"`
		} `json:"transitions"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&transitions); err != nil {
		return err
	}

	for _, t := range transitions.Transitions {
		if t.To.Name == targetStatus {
			body := map[string]any{
				"transition": map[string]string{"id": t.ID},
			}
			tResp, err := a.doRequest(ctx, "POST", "/rest/api/3/issue/"+key+"/transitions", body)
			if err != nil {
				return err
			}
			tResp.Body.Close()
			return nil
		}
	}
	return nil // target status not available as a transition — skip silently
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

	req, err := http.NewRequestWithContext(ctx, method, a.baseURL+path, reader)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(a.email, a.apiToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	return a.client.Do(req)
}
