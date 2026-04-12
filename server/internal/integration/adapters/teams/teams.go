package teams

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

var statusColor = map[string]string{
	"task.created":   "0078D7",
	"task.started":   "FFC107",
	"task.completed": "4CAF50",
	"task.failed":    "F44336",
	"task.updated":   "9C27B0",
}

type adapter struct {
	webhookURL  string
	titlePrefix string
	client      *http.Client
}

// NewAdapter creates a Microsoft Teams NotificationAdapter from a notification config.
func NewAdapter(cfg model.NotificationConfig) integration.NotificationAdapter {
	cfgMap := cfg.Config
	titlePrefix, _ := cfgMap["title_prefix"].(string)
	if titlePrefix == "" {
		titlePrefix = "Jobshout"
	}

	return &adapter{
		webhookURL:  cfg.WebhookURL,
		titlePrefix: titlePrefix,
		client:      &http.Client{},
	}
}

func (a *adapter) Name() string { return "teams" }

func (a *adapter) Send(ctx context.Context, msg integration.NotificationMessage) error {
	color := statusColor[msg.EventType]
	if color == "" {
		color = "0078D7"
	}

	summary := fmt.Sprintf("%s: %s — %s", a.titlePrefix, msg.TaskTitle, formatEvent(msg.EventType))

	facts := []map[string]string{
		{"name": "Task", "value": msg.TaskTitle},
		{"name": "Status", "value": msg.Status},
	}
	if msg.AgentName != "" {
		facts = append(facts, map[string]string{"name": "Agent", "value": msg.AgentName})
	}
	if msg.Duration != "" {
		facts = append(facts, map[string]string{"name": "Duration", "value": msg.Duration})
	}

	card := map[string]any{
		"@type":      "MessageCard",
		"@context":   "http://schema.org/extensions",
		"themeColor": color,
		"summary":    summary,
		"sections": []map[string]any{
			{
				"activityTitle": summary,
				"facts":         facts,
				"markdown":      true,
			},
		},
	}

	if msg.URL != "" {
		card["potentialAction"] = []map[string]any{
			{
				"@type": "OpenUri",
				"name":  "View in Jobshout",
				"targets": []map[string]string{
					{"os": "default", "uri": msg.URL},
				},
			},
		}
	}

	return a.post(ctx, card)
}

func (a *adapter) Test(ctx context.Context) error {
	card := map[string]any{
		"@type":      "MessageCard",
		"@context":   "http://schema.org/extensions",
		"themeColor": "4CAF50",
		"summary":    a.titlePrefix + ": Test Notification",
		"sections": []map[string]any{
			{
				"activityTitle": a.titlePrefix + ": Connection Test",
				"facts": []map[string]string{
					{"name": "Status", "value": "Connection successful!"},
				},
				"markdown": true,
			},
		},
	}
	return a.post(ctx, card)
}

func (a *adapter) post(ctx context.Context, payload any) error {
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", a.webhookURL, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("teams webhook: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("teams webhook: status %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

func formatEvent(eventType string) string {
	switch eventType {
	case "task.created":
		return "Task created"
	case "task.started":
		return "Task started"
	case "task.completed":
		return "Task completed"
	case "task.failed":
		return "Task failed"
	case "task.updated":
		return "Task updated"
	default:
		return eventType
	}
}
