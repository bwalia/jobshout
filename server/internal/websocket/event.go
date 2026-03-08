package websocket

import (
	"encoding/json"
	"time"
)

// Event type constants for messages broadcast over the WebSocket hub.
// Consumers register handlers against these string values.
const (
	// EventAgentStatusChanged is broadcast when an agent transitions between
	// idle / active / paused / offline states.
	EventAgentStatusChanged = "agent.status_changed"

	// EventTaskTransitioned is broadcast when a task moves from one workflow
	// status to another (e.g. todo -> in_progress).
	EventTaskTransitioned = "task.transitioned"

	// EventTaskAssigned is broadcast when a task is assigned to (or unassigned
	// from) an agent or user.
	EventTaskAssigned = "task.assigned"

	// EventMetricsUpdated is broadcast when aggregated metrics snapshots are
	// recalculated and available for connected dashboards.
	EventMetricsUpdated = "metrics.updated"
)

// Event is the canonical envelope for all WebSocket messages.
// The Payload field carries event-specific data as raw JSON so that
// handlers can unmarshal into the concrete type they expect without
// requiring a central registry of payload structs.
type Event struct {
	// Type identifies the event kind; matches one of the Event* constants.
	Type string `json:"type"`

	// Payload contains event-specific data encoded as raw JSON.
	Payload json.RawMessage `json:"payload"`

	// Timestamp records when the event was created on the server.
	Timestamp time.Time `json:"timestamp"`

	// OrgID scopes the event to a specific organisation so the hub can route
	// it only to connections belonging to that org.
	OrgID string `json:"org_id"`
}

// NewEvent constructs an Event with the current UTC timestamp.
// payload must be JSON-serialisable; it will be marshalled here so callers
// do not need to manage the raw JSON themselves.
func NewEvent(eventType string, orgID string, payload any) (Event, error) {
	rawPayload, err := json.Marshal(payload)
	if err != nil {
		return Event{}, err
	}

	return Event{
		Type:      eventType,
		Payload:   rawPayload,
		Timestamp: time.Now().UTC(),
		OrgID:     orgID,
	}, nil
}

// AgentStatusChangedPayload is the payload for EventAgentStatusChanged.
type AgentStatusChangedPayload struct {
	AgentID   string `json:"agent_id"`
	AgentName string `json:"agent_name"`
	OldStatus string `json:"old_status"`
	NewStatus string `json:"new_status"`
}

// TaskTransitionedPayload is the payload for EventTaskTransitioned.
type TaskTransitionedPayload struct {
	TaskID    string `json:"task_id"`
	TaskTitle string `json:"task_title"`
	OldStatus string `json:"old_status"`
	NewStatus string `json:"new_status"`
	ProjectID string `json:"project_id"`
}

// TaskAssignedPayload is the payload for EventTaskAssigned.
type TaskAssignedPayload struct {
	TaskID          string  `json:"task_id"`
	TaskTitle       string  `json:"task_title"`
	ProjectID       string  `json:"project_id"`
	AssignedAgentID *string `json:"assigned_agent_id"`
	AssignedUserID  *string `json:"assigned_user_id"`
}

// MetricsUpdatedPayload is the payload for EventMetricsUpdated.
type MetricsUpdatedPayload struct {
	TasksCompleted  int     `json:"tasks_completed"`
	ActiveAgents    int     `json:"active_agents"`
	AvgUtilization  float64 `json:"avg_utilization"`
	PeriodStartedAt string  `json:"period_started_at"`
}
