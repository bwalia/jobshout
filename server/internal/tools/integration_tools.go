package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/integration"
	"github.com/jobshout/server/internal/model"
)

// IntegrationConfigStore lists an org's configured task integrations. Satisfied
// by repository.IntegrationRepository — declared here (not imported) so the
// tools package stays decoupled from the repository layer, mirroring the
// executor.SkillProvider pattern.
type IntegrationConfigStore interface {
	ListByOrg(ctx context.Context, orgID uuid.UUID) ([]model.Integration, error)
}

// NotificationConfigStore lists an org's configured notification channels.
type NotificationConfigStore interface {
	ListByOrg(ctx context.Context, orgID uuid.UUID) ([]model.NotificationConfig, error)
}

// AdapterBuilder turns a stored config into a live adapter. Satisfied by
// *integration.Registry.
type AdapterBuilder interface {
	GetTask(cfg model.Integration) (integration.TaskAdapter, error)
	GetNotification(cfg model.NotificationConfig) (integration.NotificationAdapter, error)
}

// NewIntegrationTools returns the agent-callable tools that wrap the org's
// configured integrations: Jira/GitHub task systems and Slack/Teams/Email
// notification channels. Each tool resolves the calling org (from context) to
// that org's own credentials at execution time, so a single registered tool
// serves every tenant. Register the returned tools into the shared Registry.
func NewIntegrationTools(integ IntegrationConfigStore, notif NotificationConfigStore, adapters AdapterBuilder) []Tool {
	return []Tool{
		&createIssueTool{provider: model.ProviderJira, store: integ, adapters: adapters},
		&getIssueTool{provider: model.ProviderJira, store: integ, adapters: adapters},
		&createIssueTool{provider: model.ProviderGitHub, store: integ, adapters: adapters},
		&getIssueTool{provider: model.ProviderGitHub, store: integ, adapters: adapters},
		&sendMessageTool{name: "slack_send_message", channel: model.ProviderSlack, store: notif, adapters: adapters},
		&sendMessageTool{name: "teams_send_message", channel: model.ProviderTeams, store: notif, adapters: adapters},
		&sendMessageTool{name: "email_send", channel: "email", store: notif, adapters: adapters},
	}
}

var errNoOrg = fmt.Errorf("no organization in context: integration tools require an org-scoped execution")

// resolveTaskAdapter finds and builds the org's configured adapter for provider.
func resolveTaskAdapter(ctx context.Context, store IntegrationConfigStore, adapters AdapterBuilder, provider string) (integration.TaskAdapter, error) {
	orgID, ok := OrgFromContext(ctx)
	if !ok {
		return nil, errNoOrg
	}
	cfgs, err := store.ListByOrg(ctx, orgID)
	if err != nil {
		return nil, fmt.Errorf("list integrations: %w", err)
	}
	for _, cfg := range cfgs {
		if cfg.Provider == provider {
			return adapters.GetTask(cfg)
		}
	}
	return nil, fmt.Errorf("no %s integration is configured for this organization", provider)
}

// resolveNotificationAdapter finds and builds the org's enabled adapter for channel.
func resolveNotificationAdapter(ctx context.Context, store NotificationConfigStore, adapters AdapterBuilder, channel string) (integration.NotificationAdapter, uuid.UUID, error) {
	orgID, ok := OrgFromContext(ctx)
	if !ok {
		return nil, uuid.Nil, errNoOrg
	}
	cfgs, err := store.ListByOrg(ctx, orgID)
	if err != nil {
		return nil, orgID, fmt.Errorf("list notification channels: %w", err)
	}
	for _, cfg := range cfgs {
		if cfg.ChannelType == channel && cfg.Enabled {
			ad, err := adapters.GetNotification(cfg)
			return ad, orgID, err
		}
	}
	return nil, orgID, fmt.Errorf("no enabled %s notification channel is configured for this organization", channel)
}

// --- createIssueTool: jira_create_issue / github_create_issue ---

type createIssueTool struct {
	provider string
	store    IntegrationConfigStore
	adapters AdapterBuilder
}

func (t *createIssueTool) Name() string { return t.provider + "_create_issue" }

func (t *createIssueTool) Description() string {
	return fmt.Sprintf(`Create an issue/ticket in %s using this organization's configured integration.
Input parameters:
  title       (string, required) - Issue title/summary
  description (string, optional) - Issue body
  priority    (string, optional) - one of: critical, high, medium, low
Returns the created issue's external id and URL.`, t.provider)
}

func (t *createIssueTool) Execute(ctx context.Context, input map[string]any) (string, error) {
	adapter, err := resolveTaskAdapter(ctx, t.store, t.adapters, t.provider)
	if err != nil {
		return "", err
	}
	title, err := stringParam(input, "title", true)
	if err != nil {
		return "", err
	}
	desc, _ := stringParam(input, "description", false)
	priority, _ := stringParam(input, "priority", false)

	id, url, err := adapter.CreateIssue(ctx, integration.ExternalIssue{
		Title:       title,
		Description: desc,
		Priority:    priority,
	})
	if err != nil {
		return "", fmt.Errorf("%s: %w", t.Name(), err)
	}
	out, _ := json.Marshal(map[string]any{"external_id": id, "url": url})
	return string(out), nil
}

// --- getIssueTool: jira_get_issue / github_get_issue ---

type getIssueTool struct {
	provider string
	store    IntegrationConfigStore
	adapters AdapterBuilder
}

func (t *getIssueTool) Name() string { return t.provider + "_get_issue" }

func (t *getIssueTool) Description() string {
	return fmt.Sprintf(`Fetch an issue/ticket from %s by its external id.
Input parameters:
  external_id (string, required) - The issue key/number, e.g. PROJ-123
Returns the issue's title, status, priority and URL as JSON.`, t.provider)
}

func (t *getIssueTool) Execute(ctx context.Context, input map[string]any) (string, error) {
	adapter, err := resolveTaskAdapter(ctx, t.store, t.adapters, t.provider)
	if err != nil {
		return "", err
	}
	extID, err := stringParam(input, "external_id", true)
	if err != nil {
		return "", err
	}
	issue, err := adapter.GetIssue(ctx, extID)
	if err != nil {
		return "", fmt.Errorf("%s: %w", t.Name(), err)
	}
	out, _ := json.Marshal(issue)
	return string(out), nil
}

// --- sendMessageTool: slack_send_message / teams_send_message / email_send ---

type sendMessageTool struct {
	name     string
	channel  string
	store    NotificationConfigStore
	adapters AdapterBuilder
}

func (t *sendMessageTool) Name() string { return t.name }

func (t *sendMessageTool) Description() string {
	return fmt.Sprintf(`Send a message to the organization's configured %s channel.
Input parameters:
  message (string, required) - The message text to send
Returns "sent" on success.`, t.channel)
}

func (t *sendMessageTool) Execute(ctx context.Context, input map[string]any) (string, error) {
	adapter, orgID, err := resolveNotificationAdapter(ctx, t.store, t.adapters, t.channel)
	if err != nil {
		return "", err
	}
	message, err := stringParam(input, "message", true)
	if err != nil {
		return "", err
	}
	// Map the free-form agent message onto the notification envelope; adapters
	// render TaskTitle as the headline. EventType "agent.message" is unknown to
	// the adapters' emoji maps, so it renders with a neutral bell.
	if err := adapter.Send(ctx, integration.NotificationMessage{
		OrgID:     orgID,
		EventType: "agent.message",
		TaskTitle: message,
	}); err != nil {
		return "", fmt.Errorf("%s: %w", t.name, err)
	}
	return "sent", nil
}
