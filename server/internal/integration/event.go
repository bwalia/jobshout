package integration

import (
	"github.com/google/uuid"
)

// EventType identifies the kind of task lifecycle event.
type EventType string

const (
	EventTaskCreated   EventType = "task.created"
	EventTaskStarted   EventType = "task.started"
	EventTaskCompleted EventType = "task.completed"
	EventTaskFailed    EventType = "task.failed"
	EventTaskUpdated   EventType = "task.updated"
	EventBudgetAlert   EventType = "budget.alert"

	// Autonomous agent events.
	EventAgentGoalStarted   EventType = "agent.goal.started"
	EventAgentGoalCompleted EventType = "agent.goal.completed"
	EventAgentGoalFailed    EventType = "agent.goal.failed"

	// Chat events.
	EventChatMessage EventType = "chat.message"

	// Telegram events.
	EventTelegramLinked EventType = "telegram.linked"
)

// AllEventTypes returns every event type for subscription.
func AllEventTypes() []EventType {
	return []EventType{
		EventTaskCreated,
		EventTaskStarted,
		EventTaskCompleted,
		EventTaskFailed,
		EventTaskUpdated,
	}
}

// TaskEvent is the envelope published on the event bus.
type TaskEvent struct {
	Type      EventType `json:"type"`
	TaskID    uuid.UUID `json:"task_id"`
	OrgID     uuid.UUID `json:"org_id"`
	ProjectID uuid.UUID `json:"project_id"`
	Title     string    `json:"title"`
	Status    string    `json:"status"`
	Priority  string    `json:"priority"`
}
