package model

import (
	"time"

	"github.com/google/uuid"
)

// Provider constants for integrations.
const (
	ProviderJira   = "jira"
	ProviderGitHub = "github"
	ProviderSlack  = "slack"
	ProviderTeams  = "teams"
)

// Integration represents a configured connection to an external system.
type Integration struct {
	ID           uuid.UUID      `json:"id"`
	OrgID        uuid.UUID      `json:"org_id"`
	Name         string         `json:"name"`
	Provider     string         `json:"provider"`
	BaseURL      string         `json:"base_url"`
	Credentials  map[string]any `json:"credentials,omitempty"`
	Config       map[string]any `json:"config"`
	Status       string         `json:"status"`
	LastSyncedAt *time.Time     `json:"last_synced_at"`
	CreatedBy    *uuid.UUID     `json:"created_by"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
}

// CreateIntegrationRequest is the payload for creating an integration.
type CreateIntegrationRequest struct {
	Name        string         `json:"name" validate:"required"`
	Provider    string         `json:"provider" validate:"required,oneof=jira github slack teams"`
	BaseURL     string         `json:"base_url"`
	Credentials map[string]any `json:"credentials" validate:"required"`
	Config      map[string]any `json:"config"`
}

// UpdateIntegrationRequest is the payload for updating an integration.
type UpdateIntegrationRequest struct {
	Name        *string        `json:"name,omitempty"`
	BaseURL     *string        `json:"base_url,omitempty"`
	Credentials map[string]any `json:"credentials,omitempty"`
	Config      map[string]any `json:"config,omitempty"`
	Status      *string        `json:"status,omitempty"`
}

// IntegrationTaskLink maps a local task to an external issue.
type IntegrationTaskLink struct {
	ID            uuid.UUID  `json:"id"`
	IntegrationID uuid.UUID  `json:"integration_id"`
	TaskID        uuid.UUID  `json:"task_id"`
	ExternalID    string     `json:"external_id"`
	ExternalURL   *string    `json:"external_url"`
	SyncDirection string     `json:"sync_direction"`
	LastSyncedAt  *time.Time `json:"last_synced_at"`
	SyncStatus    string     `json:"sync_status"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

// LinkTaskRequest is the payload for linking a task to an external system.
type LinkTaskRequest struct {
	Direction string `json:"direction" validate:"omitempty,oneof=push pull bidirectional"`
}

// IntegrationSyncLog is an audit record for every sync operation.
type IntegrationSyncLog struct {
	ID            uuid.UUID      `json:"id"`
	IntegrationID uuid.UUID      `json:"integration_id"`
	TaskLinkID    *uuid.UUID     `json:"task_link_id"`
	Direction     string         `json:"direction"`
	Status        string         `json:"status"`
	ErrorMessage  *string        `json:"error_message"`
	RequestBody   map[string]any `json:"request_body,omitempty"`
	ResponseBody  map[string]any `json:"response_body,omitempty"`
	DurationMs    *int           `json:"duration_ms"`
	CreatedAt     time.Time      `json:"created_at"`
}

// NotificationConfig stores per-org notification channel configuration.
type NotificationConfig struct {
	ID          uuid.UUID      `json:"id"`
	OrgID       uuid.UUID      `json:"org_id"`
	Name        string         `json:"name"`
	ChannelType string         `json:"channel_type"`
	WebhookURL  string         `json:"webhook_url"`
	Config      map[string]any `json:"config"`
	Enabled     bool           `json:"enabled"`
	Events      []string       `json:"events"`
	CreatedBy   *uuid.UUID     `json:"created_by"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
}

// CreateNotificationConfigRequest is the payload for creating a notification config.
type CreateNotificationConfigRequest struct {
	Name        string         `json:"name" validate:"required"`
	ChannelType string         `json:"channel_type" validate:"required,oneof=slack teams"`
	WebhookURL  string         `json:"webhook_url" validate:"required,url"`
	Config      map[string]any `json:"config"`
	Events      []string       `json:"events" validate:"required,min=1"`
}

// UpdateNotificationConfigRequest is the payload for updating a notification config.
type UpdateNotificationConfigRequest struct {
	Name       *string        `json:"name,omitempty"`
	WebhookURL *string        `json:"webhook_url,omitempty"`
	Config     map[string]any `json:"config,omitempty"`
	Enabled    *bool          `json:"enabled,omitempty"`
	Events     []string       `json:"events,omitempty"`
}
