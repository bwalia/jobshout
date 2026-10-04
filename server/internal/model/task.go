package model

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

type Task struct {
	ID              uuid.UUID  `json:"id"`
	ProjectID       uuid.UUID  `json:"project_id"`
	ParentID        *uuid.UUID `json:"parent_id"`
	Title           string     `json:"title"`
	Description     *string    `json:"description"`
	Status          string     `json:"status"`
	Priority        string     `json:"priority"`
	AssignedAgentID *uuid.UUID `json:"assigned_agent_id"`
	AssignedUserID  *uuid.UUID `json:"assigned_user_id"`
	StoryPoints     *int       `json:"story_points"`
	DueDate         *time.Time `json:"due_date"`
	Position        int        `json:"position"`
	CreatedBy       *uuid.UUID `json:"created_by"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	CompletedAt     *time.Time `json:"completed_at"`
	// Last-run fields are filled from the newest task_runs row on list/get.
	// They are not stored on tasks.
	LastRunID     *uuid.UUID     `json:"last_run_id,omitempty"`
	LastRunStatus *string        `json:"last_run_status,omitempty"`
	LastRunAt     *time.Time     `json:"last_run_at,omitempty"`
	Metadata      map[string]any `json:"metadata,omitempty"`
}

type CreateTaskRequest struct {
	ProjectID   string  `json:"project_id" validate:"required,uuid"`
	Title       string  `json:"title" validate:"required,min=2"`
	Description *string `json:"description"`
	// Status lets the board create a task directly in the column the user
	// clicked "Add task" in. Empty means backlog.
	Status          string         `json:"status" validate:"omitempty,oneof=backlog todo in_progress review done"`
	Priority        string         `json:"priority" validate:"omitempty,oneof=low medium high critical"`
	AssignedAgentID *string        `json:"assigned_agent_id"`
	AssignedUserID  *string        `json:"assigned_user_id"`
	StoryPoints     *int           `json:"story_points"`
	DueDate         *string        `json:"due_date"`
	ParentID        *string        `json:"parent_id"`
	Metadata        map[string]any `json:"metadata,omitempty"`
}

type UpdateTaskRequest struct {
	Title           *string        `json:"title"`
	Description     *string        `json:"description"`
	Status          *string        `json:"status" validate:"omitempty,oneof=backlog todo in_progress review done"`
	Priority        *string        `json:"priority" validate:"omitempty,oneof=low medium high critical"`
	AssignedAgentID OptionalString `json:"assigned_agent_id"`
	AssignedUserID  OptionalString `json:"assigned_user_id"`
	StoryPoints     *int           `json:"story_points"`
	DueDate         OptionalString `json:"due_date"`
	Metadata        map[string]any `json:"metadata,omitempty"`
	// ModelOverride sets which model this task's runs use instead of the
	// agent's. Absent leaves it unchanged; an empty provider and model clear
	// it. Stored under metadata["model_override"], merged so other metadata
	// keys (launch values, run_id) are kept.
	ModelOverride *TaskModelOverride `json:"model_override,omitempty"`
}

// TaskModelOverride is a task's own model choice. Empty means "use the agent's".
type TaskModelOverride struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
}

// IsZero reports whether the override names nothing.
func (o TaskModelOverride) IsZero() bool {
	return strings.TrimSpace(o.Provider) == "" && strings.TrimSpace(o.Model) == ""
}

// TaskMetaModelOverride is the metadata key a task's model override lives under.
const TaskMetaModelOverride = "model_override"

// ModelOverrideFrom reads a task's model override from its metadata. Nil when
// unset or malformed: a bad value must not break running the task.
func ModelOverrideFrom(meta map[string]any) *TaskModelOverride {
	raw, ok := meta[TaskMetaModelOverride].(map[string]any)
	if !ok {
		return nil
	}
	o := TaskModelOverride{}
	o.Provider, _ = raw["provider"].(string)
	o.Model, _ = raw["model"].(string)
	o.Provider, o.Model = strings.TrimSpace(o.Provider), strings.TrimSpace(o.Model)
	if o.IsZero() {
		return nil
	}
	return &o
}

// WithModelOverride returns a copy of meta with the override set, or removed
// when o is zero. Other keys are untouched.
func WithModelOverride(meta map[string]any, o TaskModelOverride) map[string]any {
	out := make(map[string]any, len(meta)+1)
	for k, v := range meta {
		out[k] = v
	}
	if o.IsZero() {
		delete(out, TaskMetaModelOverride)
		return out
	}
	out[TaskMetaModelOverride] = map[string]any{
		"provider": strings.TrimSpace(o.Provider),
		"model":    strings.TrimSpace(o.Model),
	}
	return out
}

type TransitionTaskRequest struct {
	Status string `json:"status" validate:"required,oneof=backlog todo in_progress review done"`
}

type ReorderTaskRequest struct {
	Status   string `json:"status" validate:"required,oneof=backlog todo in_progress review done"`
	Position int    `json:"position" validate:"min=0"`
}

type TaskComment struct {
	ID        uuid.UUID  `json:"id"`
	TaskID    uuid.UUID  `json:"task_id"`
	AuthorID  *uuid.UUID `json:"author_id"`
	AgentID   *uuid.UUID `json:"agent_id"`
	Body      string     `json:"body"`
	CreatedAt time.Time  `json:"created_at"`
}

type AddCommentRequest struct {
	Body string `json:"body" validate:"required,min=1"`
}

// TaskHistory is GET /tasks/{id}/history — status changes, generic runs, and
// a specialist launch pointer when the task has no matching task_runs row.
type TaskHistory struct {
	CompletedAt *time.Time         `json:"completed_at"`
	Events      []TaskHistoryEvent `json:"events"`
}

type TaskHistoryEvent struct {
	ID          string     `json:"id"`
	Kind        string     `json:"kind"` // status | run | specialist
	OldStatus   *string    `json:"old_status,omitempty"`
	NewStatus   *string    `json:"new_status,omitempty"`
	RunID       *uuid.UUID `json:"run_id,omitempty"`
	Status      *string    `json:"status,omitempty"`
	AgentID     *uuid.UUID `json:"agent_id,omitempty"`
	LaunchKind  *string    `json:"launch_kind,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	ChangedAt   *time.Time `json:"changed_at,omitempty"`
}

const (
	TaskMetaLaunchValues = "launch_values"
	TaskMetaLaunchKind   = "launch_kind"
	TaskMetaRunID        = "run_id"
)

// LaunchValues reads the last specialist form values stored on the task.
func (t *Task) LaunchValues() map[string]string {
	if t == nil || t.Metadata == nil {
		return nil
	}
	raw, ok := t.Metadata[TaskMetaLaunchValues].(map[string]any)
	if !ok {
		return nil
	}
	out := make(map[string]string, len(raw))
	for k, v := range raw {
		if v == nil {
			continue
		}
		out[k] = strings.TrimSpace(fmt.Sprint(v))
	}
	return out
}
