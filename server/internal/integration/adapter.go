package integration

import (
	"context"

	"github.com/google/uuid"
)

// ExternalIssue is the adapter-neutral envelope exchanged with external task systems.
type ExternalIssue struct {
	ExternalID  string            `json:"external_id"`
	ExternalURL string            `json:"external_url"`
	Title       string            `json:"title"`
	Description string            `json:"description"`
	Status      string            `json:"status"`
	Priority    string            `json:"priority"`
	Labels      []string          `json:"labels,omitempty"`
	Assignee    string            `json:"assignee,omitempty"`
	Extra       map[string]string `json:"extra,omitempty"`
}

// TaskAdapter is the interface for external task system adapters (Jira, GitHub, etc.).
type TaskAdapter interface {
	Name() string
	CreateIssue(ctx context.Context, issue ExternalIssue) (externalID string, externalURL string, err error)
	UpdateIssue(ctx context.Context, externalID string, issue ExternalIssue) error
	GetIssue(ctx context.Context, externalID string) (*ExternalIssue, error)
	DeleteIssue(ctx context.Context, externalID string) error
}

// NotificationMessage is the adapter-neutral notification envelope.
type NotificationMessage struct {
	OrgID     uuid.UUID `json:"org_id"`
	EventType string    `json:"event_type"`
	TaskTitle string    `json:"task_title"`
	TaskID    string    `json:"task_id"`
	Status    string    `json:"status"`
	AgentName string    `json:"agent_name,omitempty"`
	Duration  string    `json:"duration,omitempty"`
	URL       string    `json:"url,omitempty"`
	Extra     map[string]string `json:"extra,omitempty"`
}

// NotificationAdapter is the interface for notification adapters (Slack, Teams, etc.).
type NotificationAdapter interface {
	Name() string
	Send(ctx context.Context, msg NotificationMessage) error
	Test(ctx context.Context) error
}
