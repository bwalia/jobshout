package model

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// Outbound webhook event types.
const (
	WebhookEventRunCompleted      = "run.completed"
	WebhookEventRunFailed         = "run.failed"
	WebhookEventApprovalRequested = "approval.requested"
	WebhookEventApprovalDecided   = "approval.decided"
	// WebhookEventPing is sent only by the endpoint "test" action.
	WebhookEventPing = "ping"
	// WebhookEventAll subscribes an endpoint to every event type.
	WebhookEventAll = "*"
)

// AllWebhookEvents is every subscribable event type.
var AllWebhookEvents = []string{
	WebhookEventRunCompleted, WebhookEventRunFailed,
	WebhookEventApprovalRequested, WebhookEventApprovalDecided,
}

// Delivery statuses.
const (
	WebhookDeliveryPending   = "pending"
	WebhookDeliverySucceeded = "succeeded"
	WebhookDeliveryFailed    = "failed"
)

// WebhookEndpoint is an org-configured receiver (table org_webhook_endpoints).
// Secret is the HMAC signing secret; it is returned only when created.
type WebhookEndpoint struct {
	ID          uuid.UUID  `json:"id"`
	OrgID       uuid.UUID  `json:"org_id"`
	URL         string     `json:"url"`
	Secret      string     `json:"secret,omitempty"`
	Events      []string   `json:"events"`
	Description string     `json:"description"`
	Enabled     bool       `json:"enabled"`
	CreatedBy   *uuid.UUID `json:"created_by"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// Subscribed reports whether the endpoint wants eventType.
func (e *WebhookEndpoint) Subscribed(eventType string) bool {
	for _, ev := range e.Events {
		if ev == eventType || ev == WebhookEventAll {
			return true
		}
	}
	return false
}

// WebhookDelivery is one attempt-tracked delivery of an event to an endpoint.
type WebhookDelivery struct {
	ID             uuid.UUID       `json:"id"`
	EndpointID     uuid.UUID       `json:"endpoint_id"`
	OrgID          uuid.UUID       `json:"org_id"`
	EventID        uuid.UUID       `json:"event_id"`
	EventType      string          `json:"event_type"`
	Payload        json.RawMessage `json:"payload"`
	Status         string          `json:"status"`
	Attempts       int             `json:"attempts"`
	LastStatusCode *int            `json:"last_status_code"`
	LastError      *string         `json:"last_error"`
	NextAttemptAt  *time.Time      `json:"next_attempt_at"`
	DeliveredAt    *time.Time      `json:"delivered_at"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

// CreateWebhookEndpointRequest is the body of POST /api/v1/webhook-endpoints.
// Secret is optional: when empty the server generates one (returned once).
type CreateWebhookEndpointRequest struct {
	URL         string   `json:"url" validate:"required,url"`
	Secret      string   `json:"secret" validate:"omitempty,min=16"`
	Events      []string `json:"events" validate:"required,min=1"`
	Description string   `json:"description"`
}

// UpdateWebhookEndpointRequest is the body of PUT /api/v1/webhook-endpoints/{id}.
type UpdateWebhookEndpointRequest struct {
	URL         *string  `json:"url" validate:"omitempty,url"`
	Events      []string `json:"events"`
	Description *string  `json:"description"`
	Enabled     *bool    `json:"enabled"`
	// RotateSecret generates a new signing secret, returned in the response.
	RotateSecret bool `json:"rotate_secret"`
}
