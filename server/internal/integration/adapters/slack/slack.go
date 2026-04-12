package slack

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

var statusEmoji = map[string]string{
	"task.created":   ":new:",
	"task.started":   ":arrow_forward:",
	"task.completed": ":white_check_mark:",
	"task.failed":    ":x:",
	"task.updated":   ":pencil2:",
}

type adapter struct {
	webhookURL string
	channel    string
	username   string
	iconEmoji  string
	client     *http.Client
}

// NewAdapter creates a Slack NotificationAdapter from a notification config.
func NewAdapter(cfg model.NotificationConfig) integration.NotificationAdapter {
	cfgMap := cfg.Config
	channel, _ := cfgMap["channel"].(string)
	username, _ := cfgMap["username"].(string)
	iconEmoji, _ := cfgMap["icon_emoji"].(string)

	if username == "" {
		username = "Jobshout"
	}
	if iconEmoji == "" {
		iconEmoji = ":robot_face:"
	}

	return &adapter{
		webhookURL: cfg.WebhookURL,
		channel:    channel,
		username:   username,
		iconEmoji:  iconEmoji,
		client:     &http.Client{},
	}
}

func (a *adapter) Name() string { return "slack" }

func (a *adapter) Send(ctx context.Context, msg integration.NotificationMessage) error {
	emoji := statusEmoji[msg.EventType]
	if emoji == "" {
		emoji = ":bell:"
	}

	text := fmt.Sprintf("%s *%s* — %s", emoji, msg.TaskTitle, formatEvent(msg.EventType))
	if msg.AgentName != "" {
		text += fmt.Sprintf(" (agent: %s)", msg.AgentName)
	}
	if msg.Duration != "" {
		text += fmt.Sprintf(" in %s", msg.Duration)
	}

	blocks := []map[string]any{
		{
			"type": "section",
			"text": map[string]any{
				"type": "mrkdwn",
				"text": text,
			},
		},
	}

	if msg.URL != "" {
		blocks = append(blocks, map[string]any{
			"type": "actions",
			"elements": []map[string]any{
				{
					"type": "button",
					"text": map[string]any{
						"type": "plain_text",
						"text": "View in Jobshout",
					},
					"url": msg.URL,
				},
			},
		})
	}

	payload := map[string]any{
		"username":   a.username,
		"icon_emoji": a.iconEmoji,
		"text":       text,
		"blocks":     blocks,
	}
	if a.channel != "" {
		payload["channel"] = a.channel
	}

	return a.post(ctx, payload)
}

func (a *adapter) Test(ctx context.Context) error {
	payload := map[string]any{
		"username":   a.username,
		"icon_emoji": a.iconEmoji,
		"text":       ":wave: Jobshout notification test — connection successful!",
	}
	if a.channel != "" {
		payload["channel"] = a.channel
	}
	return a.post(ctx, payload)
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
		return fmt.Errorf("slack webhook: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("slack webhook: status %d: %s", resp.StatusCode, string(body))
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
