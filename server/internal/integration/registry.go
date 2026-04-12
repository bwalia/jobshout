package integration

import (
	"fmt"

	"github.com/jobshout/server/internal/model"
)

// TaskAdapterFactory creates a TaskAdapter from an integration config.
type TaskAdapterFactory func(cfg model.Integration) TaskAdapter

// NotificationAdapterFactory creates a NotificationAdapter from a notification config.
type NotificationAdapterFactory func(cfg model.NotificationConfig) NotificationAdapter

// Registry holds adapter factories keyed by provider/channel type.
type Registry struct {
	taskFactories         map[string]TaskAdapterFactory
	notificationFactories map[string]NotificationAdapterFactory
}

// NewRegistry creates a new adapter registry.
func NewRegistry() *Registry {
	return &Registry{
		taskFactories:         make(map[string]TaskAdapterFactory),
		notificationFactories: make(map[string]NotificationAdapterFactory),
	}
}

// RegisterTask registers a TaskAdapter factory for a provider (e.g. "jira", "github").
func (r *Registry) RegisterTask(provider string, factory TaskAdapterFactory) {
	r.taskFactories[provider] = factory
}

// GetTask creates a TaskAdapter for the given integration config.
func (r *Registry) GetTask(cfg model.Integration) (TaskAdapter, error) {
	factory, ok := r.taskFactories[cfg.Provider]
	if !ok {
		return nil, fmt.Errorf("no task adapter registered for provider %q", cfg.Provider)
	}
	return factory(cfg), nil
}

// RegisterNotification registers a NotificationAdapter factory for a channel type (e.g. "slack", "teams").
func (r *Registry) RegisterNotification(channelType string, factory NotificationAdapterFactory) {
	r.notificationFactories[channelType] = factory
}

// GetNotification creates a NotificationAdapter for the given notification config.
func (r *Registry) GetNotification(cfg model.NotificationConfig) (NotificationAdapter, error) {
	factory, ok := r.notificationFactories[cfg.ChannelType]
	if !ok {
		return nil, fmt.Errorf("no notification adapter registered for channel %q", cfg.ChannelType)
	}
	return factory(cfg), nil
}
